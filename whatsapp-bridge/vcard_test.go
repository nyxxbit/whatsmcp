package main

import (
	"reflect"
	"testing"
)

func TestTelefonesDoVcard(t *testing.T) {
	cases := []struct {
		name  string
		vcard string
		want  []string
	}{
		{
			name:  "plain TEL with waid",
			vcard: "BEGIN:VCARD\nVERSION:3.0\nFN:Rental\nTEL;type=CELL;waid=553799830144:+55 37 9983-0144\nEND:VCARD",
			want:  []string{"553799830144"},
		},
		{
			// the layout WhatsApp itself uses when a contact is shared: grouped properties
			name: "item1.TEL with waid and CRLF line endings",
			vcard: "BEGIN:VCARD\r\nVERSION:3.0\r\nN:;Scissor Lift Rental;;;\r\nFN:Scissor Lift Rental\r\n" +
				"item1.TEL;waid=5519999999999:+55 19 99999-9999\r\nitem1.X-ABLabel:Mobile\r\nEND:VCARD",
			want: []string{"5519999999999"},
		},
		{
			name:  "no waid falls back to the number after the colon",
			vcard: "BEGIN:VCARD\nitem2.TEL;type=WORK:+55 (19) 3652-9800\nEND:VCARD",
			want:  []string{"+551936529800"},
		},
		{
			name:  "a repeated phone is listed once",
			vcard: "TEL;waid=5511911112222:+55 11 91111-2222\nitem1.TEL;waid=5511911112222:+55 11 91111-2222\nitem2.TEL;waid=5511933334444:+55 11 93333-4444",
			want:  []string{"5511911112222", "5511933334444"},
		},
		{
			// a layout nobody anticipated: no recognizable TEL, but a waid is there
			name:  "waid outside a TEL line is still found",
			vcard: "BEGIN:VCARD\nX-WA-BIZ-NAME:Rental\nitem1.X-PHONE;WAID=5519988887777:+55 19 98888-7777\nEND:VCARD",
			want:  []string{"5519988887777"},
		},
		{
			// EMAIL and X-ABLabel in the same group must not turn into phones
			name:  "grouped non-TEL lines are ignored",
			vcard: "item1.EMAIL;type=INTERNET:contact@example.com\nitem1.X-ABLabel:TEL office",
			want:  []string{},
		},
	}
	for _, c := range cases {
		if got := telefonesDoVcard(c.vcard); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestFormataContatoWithGroupedTel(t *testing.T) {
	v := "BEGIN:VCARD\nFN:Scissor Lift Rental\nitem1.TEL;waid=5519999999999:+55 19 99999-9999\nEND:VCARD"
	if got := formataContato("Scissor Lift Rental", v); got != "[contact] Scissor Lift Rental - 5519999999999" {
		t.Errorf("got %q", got)
	}
}
