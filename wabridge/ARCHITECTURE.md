# wabridge architecture

A rewrite of the bridge daemon following clean architecture and DDD. The goal is
to make new capabilities additive: adding one should not require editing existing
files.

## Status: incomplete, not the bridge to run

The structure here is finished and tested per package. Message coverage is not.

`extractText` in `platform/wa/translate.go` reads `conversation` and
`extendedTextMessage` and stops. Media captions, container types (ephemeral,
view-once, document-with-caption, edited), native events, reactions, locations,
contacts and polls are all missing, and `extractMedia` in `platform/wa/media.go`
handles images and documents only. Media filenames also come from the wall clock,
which collides when a history sync processes many messages in the same second.

That is the behaviour the project exists to fix, and it is implemented in
`../whatsapp-bridge/main.go`. Closing the gap means porting `unwrapMessage`,
`extractTextContent` and `extractMediaInfo` into the two functions named above.
Until then, run `whatsapp-bridge`, not this.

## Shared core, pluggable features

A shared core (event bus, ports, registry, domain types) is consumed by features
that stay decoupled from each other and from the core itself:

```
                 +---------------- CORE ----------------+
                 |  EventBus . Ports . Registry . Domain |
                 +------^---------------^---------^------+
                        | publish/subscribe        | register
        +---------------+-----------+   +----------+-------------+
   [ messaging ]   [ contacts ]  [ labels ]   [ your feature... ]
```

Adding a feature means implementing the `Feature` interface and registering it.
No existing file changes. Features depend on ports (interfaces), never on
concrete implementations.

## Layers

The dependency rule points inward: outer layers know inner ones, never the
reverse.

| Layer | Folder | Knows | Does not know |
| --- | --- | --- | --- |
| Domain | `internal/core/domain` | nothing | everything else |
| Core | `internal/core` | domain | features, platform, delivery |
| Features | `internal/features` | core, domain | platform, delivery, other features |
| Platform | `internal/platform` | core, domain | features, delivery |
| Delivery | `internal/delivery` | core, domain | platform internals |

Platform holds the adapters: the whatsmeow client, SQLite persistence, logging.
Delivery holds the entry points: the REST API and the system tray.

## Storage compatibility

The rewrite writes to the same SQLite file as the original bridge
(`../whatsapp-bridge/store/messages.db`) and keeps identical JSON keys on the
REST API. Existing consumers, including the MCP server, keep working unchanged.
This is deliberate: it makes the rewrite swappable in place, and reversible.

## Testing

Each package is tested in isolation through its ports. The SQLite tests run
against a temporary database; one integration test reads a real WhatsApp store
and is skipped unless `WABRIDGE_WADB` points at one.

```bash
go test ./...
```
