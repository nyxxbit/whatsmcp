# whatsmeow, patched

Copy of `go.mau.fi/whatsmeow v0.0.0-20261007111105-c386243a72ba` with one change in `send.go`
(`getMessageContent`): a menu reply (`list_response`, `buttons_response`, `interactive_response`)
is sent without the `<biz>` node. With it the server answers 479 "Invalid stanza sent
(smax-invalid)"; without it the Localiza bot takes both a button and a list reply (28/10/2026).

To update whatsmeow: copy the new module version over this folder and re-apply the change
(search for `whatsmcp:` in `send.go`).
