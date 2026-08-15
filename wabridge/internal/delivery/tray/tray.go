// Package tray is the system tray delivery (native systray, no console
// window). It shows live status, connects/reconnects on demand, opens the QR
// code when published on the bus, and opens the log in the browser (no more
// Notepad).
package tray

import (
	"context"
	_ "embed"
	"os/exec"
	"time"

	"fyne.io/systray"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/core/ports"
)

//go:embed icon.ico
var iconData []byte

// Tray controls the tray icon. Depends only on ports (SessionManager, Bus).
type Tray struct {
	session  ports.SessionManager
	bus      ports.EventBus
	log      ports.Logger
	logsURL  string
	shutdown func()

	statusItem *systray.MenuItem
}

// New creates the tray and immediately subscribes to QRCodeReady on the bus
// (before Connect, so the event is never missed). Fail-fast on dependencies.
func New(session ports.SessionManager, bus ports.EventBus, log ports.Logger, logsURL string, shutdown func()) *Tray {
	if session == nil || bus == nil || log == nil {
		panic("tray: session, bus, and log are required")
	}
	if shutdown == nil {
		shutdown = func() {}
	}
	t := &Tray{session: session, bus: bus, log: log, logsURL: logsURL, shutdown: shutdown}
	bus.Subscribe(domain.EventQRCodeReady, func(_ context.Context, evt domain.Event) error {
		if qr, ok := evt.(domain.QRCodeReady); ok {
			t.log.Info("opening QR code in viewer", "file", qr.Path())
			openExternal(qr.Path())
		}
		return nil
	})
	return t
}

// Run starts the tray (BLOCKS the main thread until "Quit"). systray requires
// the main thread, which is why this is the last step of the composition root.
func (t *Tray) Run() { systray.Run(t.onReady, t.onExit) }

func (t *Tray) onReady() {
	systray.SetIcon(iconData)
	systray.SetTitle("")
	systray.SetTooltip("WhatsApp Bridge")

	title := systray.AddMenuItem("WhatsApp Bridge", "")
	title.Disable()
	t.statusItem = systray.AddMenuItem("Status: starting...", "WhatsApp connection state")
	t.statusItem.Disable()
	systray.AddSeparator()
	mConnect := systray.AddMenuItem("Connect / Reconnect", "Detects the state and connects; generates a QR code if logged out")
	mLog := systray.AddMenuItem("View log (browser)", "Opens the end of the log in the browser")
	mFolder := systray.AddMenuItem("Open folder", "Opens the bridge folder")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit (stops the bridge)", "Stops the bridge")

	go t.statusLoop()

	go func() {
		for {
			select {
			case <-mConnect.ClickedCh:
				go t.session.Connect()
			case <-mLog.ClickedCh:
				openExternal(t.logsURL)
			case <-mFolder.ClickedCh:
				_ = exec.Command("explorer", ".").Start()
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

func (t *Tray) onExit() { t.shutdown() }

// statusLoop re-evaluates the state every 5s (detects disconnects/reconnects on its own).
func (t *Tray) statusLoop() {
	for {
		t.refreshStatus()
		time.Sleep(5 * time.Second)
	}
}

func (t *Tray) refreshStatus() {
	if t.statusItem == nil {
		return
	}
	st := t.session.Status()
	switch st.State() {
	case domain.SessionConnectedState:
		t.statusItem.SetTitle("Status: CONNECTED (" + st.Account() + ")")
		systray.SetTooltip("WhatsApp Bridge - connected")
	case domain.SessionOfflineState:
		t.statusItem.SetTitle("Status: reconnecting...")
		systray.SetTooltip("WhatsApp Bridge - reconnecting")
		// Self-healing: a valid session that's down triggers a reconnect
		// (idempotent; the adapter ignores it if a loop is already running). It
		// recovers on its own once the internet comes back, with no need for
		// the button or restarting the exe.
		go t.session.Connect()
	default:
		t.statusItem.SetTitle("Status: LOGGED OUT - click Connect (QR)")
		systray.SetTooltip("WhatsApp Bridge - logged out, scan the QR code")
	}
}
