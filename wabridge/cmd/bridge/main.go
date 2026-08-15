// Command bridge is the composition root of wabridge: the ONLY place that
// knows concrete implementations. This is where dependency injection (adapters)
// is wired up and features are registered. No business rule lives in
// this file.
//
// Flow: chdir to the exe folder → single instance (port) → logging with
// rotation → SQLite persistence → whatsmeow adapter → features in the Registry
// → REST (same contract as the legacy bridge) → tray (systray) that blocks the
// main thread.
package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nyxxbit/wabridge/internal/core/eventbus"
	"github.com/nyxxbit/wabridge/internal/core/ports"
	"github.com/nyxxbit/wabridge/internal/core/registry"
	"github.com/nyxxbit/wabridge/internal/delivery/rest"
	"github.com/nyxxbit/wabridge/internal/delivery/tray"
	"github.com/nyxxbit/wabridge/internal/features/contacts"
	"github.com/nyxxbit/wabridge/internal/features/labels"
	"github.com/nyxxbit/wabridge/internal/features/messaging"
	"github.com/nyxxbit/wabridge/internal/platform/logging"
	"github.com/nyxxbit/wabridge/internal/platform/persistence/sqlite"
	"github.com/nyxxbit/wabridge/internal/platform/wa"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	// cwd = the exe's folder (store/, logs, and qr.png use relative paths).
	if exe, err := os.Executable(); err == nil {
		_ = os.Chdir(filepath.Dir(exe))
	}

	addr := envOr("WABRIDGE_ADDR", ":8080")
	storeDir := envOr("WABRIDGE_STORE_DIR", "store")

	// Single instance: if the address already responds, a bridge is already running.
	if c, err := net.DialTimeout("tcp", localDial(addr), time.Second); err == nil {
		_ = c.Close()
		return nil
	}

	// stderr → file truncated on every boot: captures panics without ever
	// growing (the -H=windowsgui build has no console). Normal logging goes to
	// the RotatingFile.
	if f, err := os.OpenFile("wabridge.panic.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err == nil {
		os.Stderr = f
	}

	// ── Production-grade logging (size-based rotation; no more Notepad) ──
	logFile, err := logging.NewRotatingFile("wabridge.log", 1<<20)
	if err != nil {
		return fmt.Errorf("open log: %w", err)
	}
	log := logging.New(logFile)

	// ── Persistence (same schema as the legacy bridge) ──
	store, err := sqlite.Open(storeDir + "/messages.db")
	if err != nil {
		return err
	}
	msgRepo := sqlite.NewMessageRepository(store)
	chatRepo := sqlite.NewChatRepository(store)
	labelRepo := sqlite.NewLabelRepository(store)
	contactRepo, err := sqlite.OpenContactRepository(storeDir + "/whatsapp.db")
	if err != nil {
		return err
	}

	// ── whatsmeow adapter (translates library events → domain events on the bus) ──
	bus := eventbus.New(log)
	adapter, err := wa.New(wa.Config{
		WhatsappDBPath: storeDir + "/whatsapp.db",
		QRPath:         "qr.png",
		Bus:            bus,
		Log:            log,
		ChatNames:      chatRepo,
	})
	if err != nil {
		return err
	}

	// Download use case (Repository + Fetcher + on-disk cache).
	downloader := messaging.NewDownloader(msgRepo, adapter, storeDir, log)

	// Features: adding a new one is just another .Add(...) call
	reg := registry.New(log).
		Add(messaging.New()).
		Add(labels.New()).
		Add(contacts.New())
	deps := ports.FeatureDeps{
		Log:      log,
		Bus:      bus,
		Contacts: contactRepo,
		Messages: msgRepo,
		Chats:    chatRepo,
		Labels:   labelRepo,
		Sender:   adapter,
	}
	if err := reg.StartAll(deps); err != nil {
		return err
	}

	// ── REST delivery (SAME contract: /api/send, /api/download, /api/sync-labels, /logs) ──
	server, err := rest.NewServer(rest.Config{
		Sender:      adapter,
		Downloader:  downloader,
		LabelSyncer: adapter,
		LogPath:     logFile.Path(),
		Log:         log,
	})
	if err != nil {
		return err
	}
	go func() {
		if err := server.Start(addr); err != nil {
			log.Error("REST server stopped", "err", err)
		}
	}()

	// Clean shutdown triggered by "Quit" in the tray.
	shutdown := func() {
		adapter.Disconnect()
		_ = contactRepo.Close()
		_ = store.Close()
		_ = logFile.Close()
		os.Exit(0)
	}

	// Unified connection: valid session → reconnects; logged out → QR code (tray
	// opens the PNG).
	// WABRIDGE_NO_CONNECT=1 skips this step (smoke test of boot/REST without
	// touching WhatsApp).
	if envOr("WABRIDGE_NO_CONNECT", "") != "1" {
		adapter.Connect()
	} else {
		log.Warn("WABRIDGE_NO_CONNECT=1: WhatsApp connection skipped (test mode)")
	}

	log.Info("wabridge is up", "log", logFile.Path(), "rest", addr, "store", storeDir)

	// Tray blocks the main thread until "Quit".
	tray.New(adapter, bus, log, "http://127.0.0.1"+addr+"/logs", shutdown).Run()
	return nil
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// localDial turns ":8080" into "127.0.0.1:8080" for the single-instance check.
func localDial(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	return addr
}
