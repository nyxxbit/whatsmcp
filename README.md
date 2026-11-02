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
whatsapp-bridge/      Go daemon: whatsmeow client, SQLite store, REST API, MCP server, system tray
whisper-tool/         Local voice-note transcription (faster-whisper, CUDA)
wabridge/             Clean-architecture rewrite of the bridge, incomplete
```

The bridge owns the WhatsApp session, writes to SQLite and serves both the REST API and the MCP endpoint, so an MCP client reads and sends through the same process that holds the session.

### Status

Two bridges live in this repository and they are not interchangeable. Read this before picking one.

| Component | State | Notes |
| --- | --- | --- |
| `whatsapp-bridge/` | **Active.** Use this one. | Every message type listed above is implemented here, and it serves the MCP tools. This is the binary that runs. |
| `wabridge/` | **Incomplete rewrite.** Do not use yet. | Better structure, tested per package, but message coverage stopped at plain text. See below. |
| `whisper-tool/` | Standalone utility. | Independent of the bridges; reads the media the bridge already downloaded. |

**The gap in `wabridge`.** Its `extractText` reads `conversation` and `extendedTextMessage` and stops, which is the exact behaviour this project exists to fix. Captions, container types, native events, reactions and locations are not implemented there, and `extractMedia` handles images and documents only. It also names media files from the wall clock, which collides during a history sync.

The rewrite was written before that work landed and has not been updated since. It is kept in the repository because the architecture is worth continuing, not because it is ready. Porting `unwrapMessage`, `extractTextContent` and `extractMediaInfo` from `whatsapp-bridge/main.go` into `translate.go` and `media.go` is what closes it.

## Running the bridge

```powershell
cd whatsapp-bridge
.\build.ps1
```

Build with `build.ps1`, not a bare `go build`: it passes `-H=windowsgui`, checks that the binary came out as a GUI
app and swaps the running one, rolling back if the new binary does not come up. A bare `go build` gives a console
app, and Windows then opens a black CMD window every time the logon task starts the bridge.

The bridge runs as a **system tray application on Windows** and writes its output to `bridge.log` rather than the terminal, so an empty console is expected. On first run it writes the pairing QR to `qr.png` and tries to open it in the default image viewer; if nothing opens, open that file manually and scan it from WhatsApp under *Linked devices*.

The session persists in `store/whatsapp.db` and messages land in `store/messages.db`. The REST API and the MCP endpoint listen on `127.0.0.1:8080` and `[::1]:8080` only.

## Connecting an MCP client

The bridge serves MCP over Streamable HTTP at `/mcp`, on its own port, so there is nothing else to start. For Claude Code:

```bash
claude mcp add --transport http whatsapp http://127.0.0.1:8080/mcp
```

`/mcp` then lists `whatsapp` as connected whenever the bridge is up. Any client that speaks Streamable HTTP connects the same way. The endpoint is stateless, so a bridge restart never leaves the client holding a dead session.

The store is the owner's whole WhatsApp, so the tools never read across chats: `list_messages` needs a chat, and a sender alone means the direct chat with that person, never every group they are in. `list_chats` leaves the last message out unless asked. A conversation stored under both its LID and its phone number is read on both.

Like the REST API, the endpoint answers on loopback only and turns away cross-origin browser requests and any `Host` that is not localhost, which is how DNS rebinding reaches a local server.

### Tools exposed

| Tool | Purpose |
| --- | --- |
| `search_contacts` | Find contacts by name or number |
| `list_chats` | List chats, with filters and sorting |
| `get_chat` | Metadata for one chat |
| `get_direct_chat_by_contact` | Find the direct chat with a contact |
| `get_contact_chats` | Every chat a contact takes part in |
| `list_messages` | Read one chat, or a sender's direct chat, filtered by date, sender or content |
| `get_message_context` | Messages around a given one |
| `get_last_interaction` | Most recent message of the direct chat with a contact |
| `send_message` | Send text to a contact or group |
| `send_file` | Send an image, video, document or audio; an Opus `.ogg` goes as a voice note |
| `download_media` | Download media from a message and return the local path |
| `connection_status` | Whether the bridge is logged in and the WhatsApp server answers, and the last stored message |

Anything that can read your messages and send on your behalf deserves the same caution as any tool with access to private data: an LLM acting on message content it did not write can be steered by that content.

## Tests

```bash
cd whatsapp-bridge
go test ./...
```

The suite covers caption extraction, container unwrapping (including nested containers, asserting that media survives), non-media types, native events, that media filenames derive from the message rather than the wall clock, and the MCP endpoint's guards: the loopback `Host` and cross-origin checks, and that `list_messages` never reads outside one chat.

## Requirements

- Go 1.26+
- CGO enabled (SQLite)
- Windows for the tray and the QR auto-open; the bridge itself is otherwise portable
- `whisper-tool` additionally needs Python, ffmpeg and `faster-whisper` with a CUDA-capable GPU

## Credits

Built on [whatsmeow](https://github.com/tulir/whatsmeow) and originally forked from [lharries/whatsapp-mcp](https://github.com/lharries/whatsapp-mcp).

## License

MIT
