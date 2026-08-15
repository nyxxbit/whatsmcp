package wa

import (
	"context"
	"reflect"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"

	"github.com/nyxxbit/wabridge/internal/core/domain"
)

// extractText pulls the text out of a message (plain conversation or extended text).
func extractText(msg *waProto.Message) string {
	if msg == nil {
		return ""
	}
	if t := msg.GetConversation(); t != "" {
		return t
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		return ext.GetText()
	}
	return ""
}

// chatName resolves the chat's display name, in the legacy bridge's order:
// already-saved name → (group) conversation/GetGroupInfo → (contact) store →
// sender → JID.
func (c *Client) chatName(jid types.JID, chatJID string, conversation any, sender string) string {
	if dj, err := domain.NewJID(chatJID); err == nil {
		if name, err := c.chatNames.FindName(context.Background(), dj); err == nil && name != "" {
			return name
		}
	}
	if jid.Server == types.GroupServer {
		return c.groupName(jid, conversation)
	}
	if contact, err := c.wm.Store.Contacts.GetContact(context.Background(), jid); err == nil && contact.FullName != "" {
		return contact.FullName
	}
	if sender != "" {
		return sender
	}
	return jid.User
}

func (c *Client) groupName(jid types.JID, conversation any) string {
	if name := nameFromConversation(conversation); name != "" {
		return name
	}
	if info, err := c.wm.GetGroupInfo(context.Background(), jid); err == nil && info.Name != "" {
		return info.Name
	}
	return "Group " + jid.User
}

// nameFromConversation extracts DisplayName/Name from a history sync
// conversation via reflection (the concrete type varies across proto
// versions); ports the legacy behavior.
func nameFromConversation(conversation any) string {
	if conversation == nil {
		return ""
	}
	v := reflect.ValueOf(conversation)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return ""
	}
	v = v.Elem()
	for _, field := range []string{"DisplayName", "Name"} {
		f := v.FieldByName(field)
		if f.IsValid() && f.Kind() == reflect.Ptr && !f.IsNil() {
			if s := f.Elem().String(); s != "" {
				return s
			}
		}
	}
	return ""
}
