package main

// Mute chats through app state: the phone's own "Mute notifications", without sending anything to anyone.
//
//	POST /api/mute {"chat_jid": "..."}                          one week (the default)
//	POST /api/mute {"chat_jids": ["...", "..."], "hours": 8}    hours 0 = always
//	POST /api/mute {"chat_jid": "...", "mute": false}           unmute
//
// A 1:1 chat goes muted together with its twin (number and @lid) in the same patch: a contact reached
// by number answers from its @lid, and the phone may keep the conversation under either JID.
//
// sendWhatsAppMessage mutes every 1:1 chat it starts. The cold quote requests to suppliers are handled
// here, and their replies (bots, greetings, follow-ups) kept buzzing Gabriel's phone (03/11/2026).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
)

const newChatMute = 7 * 24 * time.Hour

func isPersonChat(j types.JID) bool {
	return j.User != "" && (j.Server == types.DefaultUserServer || j.Server == types.HiddenUserServer)
}

// chatTwins returns the chat plus its number/@lid counterpart, when the LID map knows it.
func chatTwins(ctx context.Context, c *whatsmeow.Client, j types.JID) []types.JID {
	out := []types.JID{j}
	var twin types.JID
	var err error
	switch j.Server {
	case types.DefaultUserServer:
		twin, err = c.Store.LIDs.GetLIDForPN(ctx, j)
	case types.HiddenUserServer:
		twin, err = c.Store.LIDs.GetPNForLID(ctx, j)
	}
	if err == nil && !twin.IsEmpty() && twin != j {
		out = append(out, twin)
	}
	return out
}

// startsChat says whether a send to these JIDs (a chat and its twin) opens a new conversation with a
// person: no group, and no message of either JID stored yet. It must run before the send is stored.
func startsChat(jids []types.JID, stored func(chatJID string) (int, error)) bool {
	if len(jids) == 0 {
		return false
	}
	for _, j := range jids {
		if !isPersonChat(j) {
			return false
		}
		if n, err := stored(j.String()); err != nil || n > 0 {
			return false
		}
	}
	return true
}

func storedMessages(chatJID string) (int, error) {
	if globalStore == nil {
		return 0, fmt.Errorf("no message store")
	}
	var n int
	err := globalStore.db.QueryRow("SELECT COUNT(*) FROM messages WHERE chat_jid = ?", chatJID).Scan(&n)
	return n, err
}

func muteChats(ctx context.Context, c *whatsmeow.Client, jids []types.JID, mute bool, d time.Duration) error {
	var patch appstate.PatchInfo
	for _, j := range jids {
		p := appstate.BuildMute(j, mute, d)
		patch.Type = p.Type
		patch.Mutations = append(patch.Mutations, p.Mutations...)
	}
	return c.SendAppState(ctx, patch)
}

// muteNewChat runs after the first send to a chat: by then the send has filled the LID cache, so the
// @lid twin the reply will come from is known.
func muteNewChat(c *whatsmeow.Client, j types.JID) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	jids := chatTwins(ctx, c, j)
	if err := muteChats(ctx, c, jids, true, newChatMute); err != nil {
		fmt.Printf("[bridge] new chat %v sent, but muting failed: %v\n", jids, err)
		return
	}
	fmt.Printf("[bridge] new chat muted for %v: %v\n", newChatMute, jids)
}

func init() {
	http.HandleFunc("/api/mute", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c := globalClient
		if c == nil || c.Store.ID == nil {
			http.Error(w, "not connected", http.StatusServiceUnavailable)
			return
		}
		var req struct {
			ChatJID  string   `json:"chat_jid"`
			ChatJIDs []string `json:"chat_jids"`
			Hours    *float64 `json:"hours"`
			Mute     *bool    `json:"mute"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		targets := req.ChatJIDs
		if req.ChatJID != "" {
			targets = append(targets, req.ChatJID)
		}
		if len(targets) == 0 {
			http.Error(w, "chat_jid or chat_jids is required", http.StatusBadRequest)
			return
		}
		mute := req.Mute == nil || *req.Mute
		d := newChatMute
		if req.Hours != nil {
			if *req.Hours < 0 {
				http.Error(w, "hours must be >= 0 (0 = always)", http.StatusBadRequest)
				return
			}
			d = time.Duration(*req.Hours * float64(time.Hour))
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		var jids []types.JID
		seen := map[types.JID]bool{}
		for _, s := range targets {
			j, err := types.ParseJID(s)
			if err != nil || j.User == "" {
				http.Error(w, fmt.Sprintf("invalid chat_jid %q", s), http.StatusBadRequest)
				return
			}
			chats := []types.JID{j}
			if isPersonChat(j) {
				chats = chatTwins(ctx, c, j)
			}
			for _, x := range chats {
				if !seen[x] {
					seen[x] = true
					jids = append(jids, x)
				}
			}
		}
		err := muteChats(ctx, c, jids, mute, d)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			fmt.Printf("Mute=%v failed for %v: %v\n", mute, jids, err)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		names := make([]string, len(jids))
		for i, j := range jids {
			names[i] = j.String()
		}
		fmt.Printf("Mute=%v (%v) %v\n", mute, d, names)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "muted": mute, "hours": d.Hours(), "chats": names})
	})
}
