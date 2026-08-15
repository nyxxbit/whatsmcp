package wa

import (
	"testing"

	"go.mau.fi/whatsmeow/types"

	"github.com/nyxxbit/wabridge/internal/core/domain"
)

// Guards against the "broad view" bug: the sender arrives with an agent/device
// suffix (e.g. 100000000000002:11@lid). Without stripping it, it won't match
// lid_map and the number won't resolve. senderJID must return the BARE JID
// (user@server).
func TestSenderJID_removeAgentEDevice(t *testing.T) {
	fallback := domain.MustJID("000@lid")
	cases := map[string]struct {
		sender types.JID
		want   string
	}{
		"lid with device":    {types.JID{User: "100000000000002", Device: 11, Server: "lid"}, "100000000000002@lid"},
		"lid with agent+dev": {types.JID{User: "100000000000002", RawAgent: 2, Device: 11, Server: "lid"}, "100000000000002@lid"},
		"pn with device":     {types.JID{User: "5511999990003", Device: 3, Server: "s.whatsapp.net"}, "5511999990003@s.whatsapp.net"},
		"bare stays bare":    {types.JID{User: "5511999990003", Server: "s.whatsapp.net"}, "5511999990003@s.whatsapp.net"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := senderJID(c.sender, fallback); got.String() != c.want {
				t.Fatalf("senderJID = %q, want %q", got.String(), c.want)
			}
		})
	}
}
