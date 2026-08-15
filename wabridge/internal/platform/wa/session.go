package wa

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow/appstate"

	"github.com/nyxxbit/wabridge/internal/core/domain"
)

// connectBackoffStart and connectBackoffMax bound the connection retry.
const (
	connectBackoffStart = 5 * time.Second
	connectBackoffMax   = 60 * time.Second
)

// Connect is the single (re)connection entry point (startup and the tray
// button): already connected → no-op; a valid session that dropped → enters
// the reconnection loop with backoff (self-heals after a network outage);
// logged out → QR. Serialized by a mutex; guarded by reconnecting/qrActive so
// loops don't stack up and two QR channels never open at once.
func (c *Client) Connect() {
	c.connectMu.Lock()
	switch {
	case c.wm.Store.ID != nil && c.wm.IsConnected():
		c.connectMu.Unlock()
		return
	case c.wm.Store.ID != nil:
		if c.reconnecting {
			c.connectMu.Unlock()
			return // a reconnection loop is already running
		}
		c.reconnecting = true
		c.connectMu.Unlock()
		go c.connectWithRetry()
		return
	case c.qrActive:
		c.connectMu.Unlock()
		c.log.Info("QR pairing already in progress")
		return
	}
	c.qrActive = true
	c.connectMu.Unlock()

	go func() {
		c.wm.Disconnect()
		time.Sleep(500 * time.Millisecond)
		c.pairWithQR()
		c.connectMu.Lock()
		c.qrActive = false
		c.connectMu.Unlock()
	}()
}

// connectWithRetry tries to connect the existing session until it succeeds,
// with exponential backoff. Fixes the gap where an INITIAL connection failure
// (e.g. the bridge (re)starting with the network down) left the bridge stuck:
// whatsmeow's autoreconnect only reacts to drops AFTER a connection, not to a
// first attempt that fails. Once it connects once, whatsmeow's autoreconnect
// takes over subsequent drops.
func (c *Client) connectWithRetry() {
	defer func() {
		c.connectMu.Lock()
		c.reconnecting = false
		c.connectMu.Unlock()
	}()

	c.wm.Disconnect() // start from a clean state
	time.Sleep(300 * time.Millisecond)

	backoff := connectBackoffStart
	for attempt := 1; ; attempt++ {
		if c.wm.Store.ID == nil {
			return // turned into a logout midway; the QR flow handles it
		}
		if c.wm.IsConnected() {
			return // already connected (autoreconnect handled it)
		}
		err := c.wm.Connect()
		if err == nil || c.wm.IsConnected() {
			c.log.Info("connected to WhatsApp", "attempt", attempt)
			return
		}
		c.log.Warn("connection failed; retrying", "attempt", attempt, "in", backoff.String(), "err", err)
		time.Sleep(backoff)
		if backoff < connectBackoffMax {
			if backoff *= 2; backoff > connectBackoffMax {
				backoff = connectBackoffMax
			}
		}
	}
}

// Disconnect drops the connection (keeps the session; reconnects later).
func (c *Client) Disconnect() { c.wm.Disconnect() }

// Status reflects the real connection state for the tray/diagnostics.
func (c *Client) Status() domain.SessionStatus {
	switch {
	case c.wm.Store.ID != nil && c.wm.IsConnected():
		return domain.NewSessionStatus(domain.SessionConnectedState, c.wm.Store.ID.User)
	case c.wm.Store.ID != nil:
		return domain.NewSessionStatus(domain.SessionOfflineState, c.wm.Store.ID.User)
	default:
		return domain.NewSessionStatus(domain.SessionLoggedOutState, "")
	}
}

// SyncLabels triggers the app state fullSync, emitting label events.
// EmitAppStateEventsOnFullSync must be turned on, otherwise fullSync
// re-applies the state but does NOT re-emit label_edit/label_jid (the bug
// that wiped out labels).
func (c *Client) SyncLabels(ctx context.Context) error {
	if c.wm.Store.ID == nil {
		return fmt.Errorf("wa: no session to sync labels")
	}
	c.wm.EmitAppStateEventsOnFullSync = true
	if err := c.wm.FetchAppState(ctx, appstate.WAPatchRegular, true, false); err != nil {
		return fmt.Errorf("wa: sync labels: %w", err)
	}
	return nil
}
