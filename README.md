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
wabridge/             Clean-architecture rewrite of the bridge, incomplete
```

The bridge owns the WhatsApp session and writes to SQLite. The MCP server reads from it. They are separate processes, so restarting the MCP server never touches the session.

### Status

Two bridges live in this repository and they are not interchangeable. Read this before picking one.

| Component | State | Notes |
| --- | --- | --- |
| `whatsapp-bridge/` | **Active.** Use this one. | Every message type listed above is implemented here. This is the binary that runs. |
| `wabridge/` | **Incomplete rewrite.** Do not use yet. | Better structure, tested per package, but message coverage stopped at plain text. See below. |
| `whatsapp-mcp-server/` | Inherited from upstream, unchanged. | Reads the store and passes `media_type` through, so new types surface without changes here. |
| `whisper-tool/` | Standalone utility. | Independent of the bridges; reads the media the bridge already downloaded. |

**The gap in `wabridge`.** Its `extractText` reads `conversation` and `extendedTextMessage` and stops, which is the exact behaviour this project exists to fix. Captions, container types, native events, reactions and locations are not implemented there, and `extractMedia` handles images and documents only. It also names media files from the wall clock, which collides during a history sync.

The rewrite was written before that work landed and has not been updated since. It is kept in the repository because the architecture is worth continuing, not because it is ready. Porting `unwrapMessage`, `extractTextContent` and `extractMediaInfo` from `whatsapp-bridge/main.go` into `translate.go` and `media.go` is what closes it.

## Running the bridge

```bash
cd whatsapp-bridge
go build -o whatsapp-bridge .
./whatsapp-bridge
```

The bridge runs as a **system tray application on Windows** and writes its output to `bridge.log` rather than the terminal, so an empty console is expected. On first run it writes the pairing QR to `qr.png` and tries to open it in the default image viewer; if nothing opens, open that file manually and scan it from WhatsApp under *Linked devices*.

The session persists in `store/whatsapp.db` and messages land in `store/messages.db`. The REST API listens on `:8080`.

## Connecting an MCP client

Start the server:

```bash
cd whatsapp-mcp-server
uv run main.py
```

Then point your client at it. For Claude Desktop, add this to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "whatsapp": {
      "command": "uv",
      "args": [
        "--directory", "/absolute/path/to/whatsmcp/whatsapp-mcp-server",
        "run", "main.py"
      ]
    }
  }
}
```

Cursor uses the same shape in `.cursor/mcp.json`. Restart the client after editing.

### Tools exposed

| Tool | Purpose |
| --- | --- |
| `search_contacts` | Find contacts by name or number |
| `list_chats` | List chats, with filters and sorting |
| `get_chat` | Metadata for one chat |
| `get_direct_chat_by_contact` | Find the direct chat with a contact |
| `get_contact_chats` | Every chat a contact takes part in |
| `list_messages` | Read messages, filtered by date, sender or content |
| `get_message_context` | Messages around a given one |
| `get_last_interaction` | Most recent message with a contact |
| `send_message` | Send text to a contact or group |
| `send_file` | Send an image, video, document or raw audio |
| `send_audio_message` | Send a playable voice note |
| `download_media` | Download media from a message and return the local path |

Anything that can read your messages and send on your behalf deserves the same caution as any tool with access to private data: an LLM acting on message content it did not write can be steered by that content.

## Tests

```bash
cd whatsapp-bridge
go test ./...
```

The suite covers caption extraction, container unwrapping (including nested containers, asserting that media survives), non-media types, native events, and that media filenames derive from the message rather than the wall clock.

## Requirements

- Go 1.25+
- Python 3.11+ with [uv](https://docs.astral.sh/uv/)
- CGO enabled (SQLite)
- Windows for the tray and the QR auto-open; the bridge itself is otherwise portable
- `whisper-tool` additionally needs ffmpeg and `faster-whisper` with a CUDA-capable GPU

## Credits

Built on [whatsmeow](https://github.com/tulir/whatsmeow) and originally forked from [lharries/whatsapp-mcp](https://github.com/lharries/whatsapp-mcp).

## License

MIT
