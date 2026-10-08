package main

// Star (favorite) messages through app state, without sending anything to anyone.
//
//	POST /api/star {"chat_jid": "...", "message_id": "...", "from_me": true}
//	POST /api/star {"starred": true, "messages": [{"chat_jid": "...", "message_id": "...", "from_me": true}, ...]}
//
// sender defaults to the chat itself, which is what a 1:1 chat needs: whatsmeow writes "0" as the
// participant when target and sender are the same user. In a group, pass the author's JID as sender.
// starred defaults to true; false removes the star. All messages go out in a single app state patch.
//
// Starring is the quiet way to gather old messages for a screenshot with their original date: the
// phone lists them under "Starred messages". Forwarding stamps today's date, and a quoted reply shows
// up for the other side too.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
)

type starTarget struct {
	ChatJID   string `json:"chat_jid"`
	MessageID string `json:"message_id"`
	FromMe    bool   `json:"from_me"`
	Sender    string `json:"sender"`
}

func init() {
	http.HandleFunc("/api/star", func(w http.ResponseWriter, r *http.Request) {
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
			ChatJID   string       `json:"chat_jid"`
			MessageID string       `json:"message_id"`
			FromMe    bool         `json:"from_me"`
			Sender    string       `json:"sender"`
			Starred   *bool        `json:"starred"`
			Messages  []starTarget `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		targets := req.Messages
		if req.MessageID != "" {
			targets = append(targets, starTarget{req.ChatJID, req.MessageID, req.FromMe, req.Sender})
		}
		if len(targets) == 0 {
			http.Error(w, "chat_jid and message_id (or messages) are required", http.StatusBadRequest)
			return
		}
		starred := req.Starred == nil || *req.Starred

		var patch appstate.PatchInfo
		for i, t := range targets {
			chat, err := types.ParseJID(t.ChatJID)
			if err != nil || t.MessageID == "" {
				http.Error(w, fmt.Sprintf("message %d: invalid chat_jid or empty message_id", i), http.StatusBadRequest)
				return
			}
			sender := chat
			if t.Sender != "" {
				if sender, err = types.ParseJID(t.Sender); err != nil {
					http.Error(w, fmt.Sprintf("message %d: invalid sender: %v", i, err), http.StatusBadRequest)
					return
				}
			}
			p := appstate.BuildStar(chat, sender, types.MessageID(t.MessageID), t.FromMe, starred)
			patch.Type = p.Type
			patch.Mutations = append(patch.Mutations, p.Mutations...)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		err := c.SendAppState(ctx, patch)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			fmt.Printf("Star failed for %d message(s): %v\n", len(targets), err)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		fmt.Printf("Starred=%v %d message(s)\n", starred, len(targets))
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "starred": starred, "count": len(targets)})
	})
}
