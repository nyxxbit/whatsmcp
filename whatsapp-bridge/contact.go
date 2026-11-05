package main

// Name a contact the way WhatsApp Web's "Edit contact" does: an app state "contact" mutation, which
// every linked device applies, and with save_on_phone the phone also writes the name to its own
// address book. Gabriel, 05/11/2026: rename a bank manager to "Alex Carter" so it shows that way
// on his phone too.
//
//	POST /api/contact-name {"jid": "5511999990003", "name": "Alex Carter"}
//	POST /api/contact-name {"jid": "100000000000003@lid", "name": "Alex Carter", "save_on_phone": false}

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// buildContactName builds the patch. The contact list is keyed by the phone JID; the @lid goes inside.
// shortcut: version 2 is what WhatsApp Web sends (as in Baileys' addOrEditContact); if a phone ever
// ignores the name, compare with a contact mutation that came from the phone.
func buildContactName(pn, lid types.JID, fullName string, saveOnPhone bool) appstate.PatchInfo {
	first := fullName
	if i := strings.IndexByte(fullName, ' '); i > 0 {
		first = fullName[:i]
	}
	act := &waSyncAction.ContactAction{
		FullName:                 proto.String(fullName),
		FirstName:                proto.String(first),
		SaveOnPrimaryAddressbook: proto.Bool(saveOnPhone),
	}
	if !lid.IsEmpty() {
		act.LidJID = proto.String(lid.String())
	}
	return appstate.PatchInfo{
		Type: appstate.WAPatchCriticalUnblockLow,
		Mutations: []appstate.MutationInfo{{
			Index:   []string{appstate.IndexContact, pn.String()},
			Version: 2,
			Value:   &waSyncAction.SyncActionValue{ContactAction: act},
		}},
	}
}

func init() {
	http.HandleFunc("/api/contact-name", func(w http.ResponseWriter, r *http.Request) {
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
			JID         string `json:"jid"`
			Name        string `json:"name"`
			SaveOnPhone *bool  `json:"save_on_phone"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(req.Name)
		if req.JID == "" || name == "" {
			http.Error(w, "jid and name are required", http.StatusBadRequest)
			return
		}
		j := types.JID{User: req.JID, Server: types.DefaultUserServer}
		if strings.Contains(req.JID, "@") {
			var err error
			j, err = types.ParseJID(req.JID)
			if err != nil || !isPersonChat(j) {
				http.Error(w, fmt.Sprintf("invalid jid %q: a number, <pn>@s.whatsapp.net or <lid>@lid", req.JID), http.StatusBadRequest)
				return
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		var pn, lid types.JID
		for _, x := range chatTwins(ctx, c, j) {
			if x.Server == types.HiddenUserServer {
				lid = x
			} else {
				pn = x
			}
		}
		if pn.IsEmpty() {
			http.Error(w, "no phone number known for this @lid, and the contact list is keyed by number", http.StatusBadRequest)
			return
		}
		save := req.SaveOnPhone == nil || *req.SaveOnPhone
		err := c.SendAppState(ctx, buildContactName(pn, lid, name, save))
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			fmt.Printf("[bridge] naming %v %q failed: %v\n", pn, name, err)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		fmt.Printf("[bridge] contact %v (%v) named %q, save on phone %v\n", pn, lid, name, save)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "jid": pn.String(), "lid": lid.String(), "name": name, "save_on_phone": save})
	})
}
