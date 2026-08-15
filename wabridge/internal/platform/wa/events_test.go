package wa

import (
	"testing"

	"go.mau.fi/whatsmeow/types"

	"github.com/nyxxbit/wabridge/internal/core/domain"
)

// Trava o bug "visão ampla": o remetente chega com sufixo de agent/device
// (ex.: 100000000000002:11@lid). Sem removê-lo, o lid_map não casa e o número não
// resolve. senderJID precisa devolver o JID BARE (user@server).
func TestSenderJID_removeAgentEDevice(t *testing.T) {
	fallback := domain.MustJID("000@lid")
	cases := map[string]struct {
		sender types.JID
		want   string
	}{
		"lid com device":     {types.JID{User: "100000000000002", Device: 11, Server: "lid"}, "100000000000002@lid"},
		"lid com agent+dev":  {types.JID{User: "100000000000002", RawAgent: 2, Device: 11, Server: "lid"}, "100000000000002@lid"},
		"pn com device":      {types.JID{User: "5511999990003", Device: 3, Server: "s.whatsapp.net"}, "5511999990003@s.whatsapp.net"},
		"bare permanece bare": {types.JID{User: "5511999990003", Server: "s.whatsapp.net"}, "5511999990003@s.whatsapp.net"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := senderJID(c.sender, fallback); got.String() != c.want {
				t.Fatalf("senderJID = %q, esperava %q", got.String(), c.want)
			}
		})
	}
}
