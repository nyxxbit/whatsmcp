package wa

import (
	"context"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"github.com/nyxxbit/wabridge/internal/core/domain"
)

// handleEvent is the dispatcher registered with whatsmeow. Messages/history go
// into a queue (asynchronous processing by a single worker, so a large
// history sync doesn't block the dispatcher); everything else becomes a
// domain event immediately.
func (c *Client) handleEvent(evt any) {
	switch v := evt.(type) {
	case *events.Message:
		c.enqueue(v)
	case *events.HistorySync:
		c.enqueue(v)
	case *events.LabelEdit:
		c.onLabelEdit(v)
	case *events.LabelAssociationChat:
		c.onLabelAssociation(v)
	case *events.Connected:
		c.publish(domain.NewSessionConnected(c.accountUser(), time.Now()))
		c.log.Info("connected to WhatsApp", "account", c.accountUser())
	case *events.LoggedOut:
		c.publish(domain.NewSessionLoggedOut(time.Now()))
		c.log.Warn("device logged out, needs to reconnect (QR)")
	}
}

func (c *Client) enqueue(evt any) {
	select {
	case c.queue <- evt:
	default:
		c.log.Warn("event queue full, dropping event")
	}
}

// worker processes the queue in order (1 goroutine = no write concurrency).
func (c *Client) worker() {
	for evt := range c.queue {
		switch v := evt.(type) {
		case *events.Message:
			c.onMessage(v)
		case *events.HistorySync:
			c.onHistorySync(v)
		}
	}
}

// onMessage translates a live message into MessageReceived (with the chat resolved).
func (c *Client) onMessage(v *events.Message) {
	chatJID := v.Info.Chat.String()
	dj, err := domain.NewJID(chatJID)
	if err != nil {
		c.log.Warn("invalid chat JID", "jid", chatJID, "err", err)
		return
	}
	content := extractText(v.Message)
	media := extractMedia(v.Message)
	if content == "" && media == nil {
		return // no text and no media: nothing to do (same as legacy)
	}
	name := c.chatName(v.Info.Chat, chatJID, nil, v.Info.Sender.User)
	sender := senderJID(v.Info.Sender, dj)

	msg, err := domain.NewMessage(v.Info.ID, dj, sender, content, v.Info.Timestamp, v.Info.IsFromMe, media)
	if err != nil {
		c.log.Warn("invalid message while translating", "err", err)
		return
	}
	chat, err := domain.NewChat(dj, name, v.Info.Timestamp)
	if err != nil {
		return
	}
	received, err := domain.NewMessageReceived(msg, chat)
	if err != nil {
		return
	}
	c.publish(received)
}

// onHistorySync translates a history batch into HistorySynced (batch persistence).
func (c *Client) onHistorySync(v *events.HistorySync) {
	var (
		chats []domain.Chat
		msgs  []domain.Message
	)
	for _, conv := range v.Data.GetConversations() {
		chatJID := conv.GetID()
		if chatJID == "" {
			continue
		}
		jid, err := types.ParseJID(chatJID)
		if err != nil {
			continue
		}
		dj, err := domain.NewJID(chatJID)
		if err != nil {
			continue
		}
		history := conv.GetMessages()
		if len(history) == 0 {
			continue
		}
		name := c.chatName(jid, chatJID, conv, "")

		if first := history[0].GetMessage(); first != nil {
			if ts := tsFromUnix(first.GetMessageTimestamp()); !ts.IsZero() {
				if chat, err := domain.NewChat(dj, name, ts); err == nil {
					chats = append(chats, chat)
				}
			}
		}

		for _, hm := range history {
			wmi := hm.GetMessage()
			if wmi == nil {
				continue
			}
			content := extractText(wmi.GetMessage())
			media := extractMedia(wmi.GetMessage())
			if content == "" && media == nil {
				continue
			}
			ts := tsFromUnix(wmi.GetMessageTimestamp())
			if ts.IsZero() {
				continue
			}
			key := wmi.GetKey()
			isFromMe := key.GetFromMe()
			sender := historySenderJID(isFromMe, key.GetParticipant(), c.accountUser(), jid)
			if msg, err := domain.NewMessage(key.GetID(), dj, sender, content, ts, isFromMe, media); err == nil {
				msgs = append(msgs, msg)
			}
		}
	}
	c.publish(domain.NewHistorySynced(chats, msgs, time.Now()))
	c.log.Info("history sync processed", "chats", len(v.Data.GetConversations()), "messages", len(msgs))
}

func (c *Client) onLabelEdit(v *events.LabelEdit) {
	label, err := domain.NewLabel(v.LabelID, v.Action.GetName(), int(v.Action.GetColor()), v.Action.GetDeleted())
	if err != nil {
		return
	}
	c.publish(domain.NewLabelEdited(label, time.Now()))
}

func (c *Client) onLabelAssociation(v *events.LabelAssociationChat) {
	dj, err := domain.NewJID(v.JID.String())
	if err != nil {
		return
	}
	assoc, err := domain.NewLabelAssociation(v.LabelID, dj, v.Action.GetLabeled())
	if err != nil {
		return
	}
	c.publish(domain.NewChatLabeled(assoc, time.Now()))
}

// publish delivers a domain event onto the bus.
func (c *Client) publish(evt domain.Event) {
	c.bus.Publish(context.Background(), evt)
}

// accountUser returns the authenticated account's number ("" if logged out).
func (c *Client) accountUser() string {
	if c.wm.Store.ID != nil {
		return c.wm.Store.ID.User
	}
	return ""
}

// senderJID converts the live sender (types.JID) into a domain.JID, WITHOUT
// the agent/device suffix (e.g. 100000000000002:11 → 100000000000002),
// otherwise it won't match lid_map and the number won't resolve. Falls back
// to the chat on failure.
func senderJID(sender types.JID, fallback domain.JID) domain.JID {
	if dj, err := domain.NewJID(sender.ToNonAD().String()); err == nil {
		return dj
	}
	return fallback
}

// historySenderJID resolves the sender of a history message (participant, own
// account, or the chat's JID), also without agent/device.
func historySenderJID(isFromMe bool, participant, account string, chat types.JID) domain.JID {
	var raw string
	switch {
	case !isFromMe && participant != "":
		raw = participant
	case isFromMe:
		raw = account
	default:
		raw = chat.User
	}
	if strings.Contains(raw, "@") {
		if pj, err := types.ParseJID(raw); err == nil {
			raw = pj.ToNonAD().String()
		}
	}
	if dj, err := domain.ParseRecipient(raw); err == nil {
		return dj
	}
	if dj, err := domain.NewJID(chat.ToNonAD().String()); err == nil {
		return dj
	}
	return domain.JID{}
}

func tsFromUnix(sec uint64) time.Time {
	if sec == 0 {
		return time.Time{}
	}
	return time.Unix(int64(sec), 0)
}
