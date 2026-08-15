# whatsmcp

A WhatsApp bridge and MCP server that records every message type WhatsApp can send, including the ones most bridges silently drop.

Most WhatsApp bridges read `conversation` and `extendedTextMessage` and stop there. That is enough for a demo and not enough for a system of record: a photo sent with a caption arrives with empty text, a document sent with a caption disappears entirely, and a native event never shows up at all. whatsmcp handles the full message tree, so what you read back matches what was actually sent.

## What it captures

| Category | Types |
| --- | --- |
| Text | conversation, extended text |
| Media | image, video, audio, document, sticker, round video (captions included) |
| Containers | ephemeral, view-once (v1/v2/extension), document-with-caption, edited |
| Events | native events with start/end time, location and cancellation state |
| Interaction | reactions, deletions, pinned messages, polls and votes, button and list replies |
| Other | location, live location, contacts, albums, orders, group invites |

Container types are unwrapped recursively, up to five levels. Unwrapping runs before media extraction; do it the other way around and the text survives while the attachment is lost.

## Why containers matter

WhatsApp does not send a captioned document as a `documentMessage`. It sends a `documentWithCaptionMessage` that *contains* one. The same happens with disappearing messages, view-once media and edits. A bridge that reads only the outer message finds no known type and stores nothing, so the message is lost entirely rather than truncated.

This is also the root of [lharries/whatsapp-mcp#310](https://github.com/lharries/whatsapp-mcp/issues/310), where an approved native event is created in the group but reads return nothing and clients fall back to a mismatched plain-text summary.

## Components

```
whatsapp-bridge/      Go daemon: whatsmeow client, SQLite store, REST API, system tray
whatsapp-mcp-server/  MCP server exposing the store to LLM clients
whisper-tool/         Local voice-note transcription (faster-whisper, CUDA)
wabridge/             Clean-architecture rewrite of the bridge, in progress
```

The bridge owns the WhatsApp session and writes to SQLite. The MCP server reads from it. They are separate processes, so restarting the MCP server never touches the session.

## Quick start

```bash
cd whatsapp-bridge
go build -o whatsapp-bridge .
./whatsapp-bridge          # scan the QR code on first run
```

The session persists in `store/whatsapp.db`; messages land in `store/messages.db`. The REST API listens on `:8080`.

```bash
cd whatsapp-mcp-server
uv run main.py
```

## Tests

```bash
cd whatsapp-bridge
go test ./...
```

The suite covers caption extraction, container unwrapping (including nested containers, asserting that media survives), non-media types and native events.

## Requirements

- Go 1.24+
- Python 3.11+ with [uv](https://docs.astral.sh/uv/)
- CGO enabled (SQLite)

## Credits

Built on [whatsmeow](https://github.com/tulir/whatsmeow) and originally forked from [lharries/whatsapp-mcp](https://github.com/lharries/whatsapp-mcp).

## License

MIT
