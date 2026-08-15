// Package wa adapts the whatsmeow library to the core: it translates the
// library's events into domain events (published on the bus) and implements
// the outbound ports (MessageSender, MediaFetcher, SessionManager,
// LabelSyncer). It is the ONLY package that knows whatsmeow's types; features
// and delivery only ever see ports.
package wa

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"

	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// Compile-time guarantees that the adapter satisfies the outbound ports.
var (
	_ ports.MessageSender  = (*Client)(nil)
	_ ports.MediaFetcher   = (*Client)(nil)
	_ ports.SessionManager = (*Client)(nil)
	_ ports.LabelSyncer    = (*Client)(nil)
)

const eventQueueSize = 4096 // event queue capacity (large history sync)

// Config gathers the adapter's dependencies (injected at the composition root).
type Config struct {
	WhatsappDBPath string         // whatsmeow's session store (e.g. "store/whatsapp.db")
	QRPath         string         // path to the QR PNG (e.g. "qr.png")
	Bus            ports.EventBus // for publishing domain events
	Log            ports.Logger
	ChatNames      ports.ChatRepository // lookup for an already-saved name (chat resolution)
}

// Client is the whatsmeow adapter.
type Client struct {
	wm        *whatsmeow.Client
	bus       ports.EventBus
	log       ports.Logger
	chatNames ports.ChatRepository
	qrPath    string

	queue chan any

	connectMu    sync.Mutex
	qrActive     bool
	reconnecting bool
}

// New builds the adapter: opens the session store, creates the whatsmeow
// client, starts the queue worker, and registers the event handler. Fail-fast
// on missing dependencies.
func New(cfg Config) (*Client, error) {
	if cfg.Bus == nil || cfg.Log == nil || cfg.ChatNames == nil {
		return nil, fmt.Errorf("wa: Bus, Log, and ChatNames are required")
	}
	if cfg.WhatsappDBPath == "" {
		cfg.WhatsappDBPath = "store/whatsapp.db"
	}
	if cfg.QRPath == "" {
		cfg.QRPath = "qr.png"
	}

	ctx := context.Background()
	dsn := "file:" + cfg.WhatsappDBPath + "?_foreign_keys=on"
	container, err := sqlstore.New(ctx, "sqlite3", dsn, newWALog(cfg.Log, "Database"))
	if err != nil {
		return nil, fmt.Errorf("wa: open session store: %w", err)
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			device = container.NewDevice()
			cfg.Log.Info("wa: new device created (no session, will need QR)")
		} else {
			return nil, fmt.Errorf("wa: get device: %w", err)
		}
	}
	wm := whatsmeow.NewClient(device, newWALog(cfg.Log, "Client"))
	if wm == nil {
		return nil, fmt.Errorf("wa: failed to create whatsmeow client")
	}

	c := &Client{
		wm:        wm,
		bus:       cfg.Bus,
		log:       cfg.Log,
		chatNames: cfg.ChatNames,
		qrPath:    cfg.QRPath,
		queue:     make(chan any, eventQueueSize),
	}
	go c.worker()
	wm.AddEventHandler(c.handleEvent)
	return c, nil
}
