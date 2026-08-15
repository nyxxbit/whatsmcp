// Package tray é a entrega via bandeja do sistema (systray nativo, sem janela de
// console). Mostra o status ao vivo, conecta/reconecta sob demanda, abre o QR
// quando publicado no bus e abre o log no navegador (fim do Notepad).
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

// Tray controla o ícone da bandeja. Depende apenas de ports (SessionManager, Bus).
type Tray struct {
	session  ports.SessionManager
	bus      ports.EventBus
	log      ports.Logger
	logsURL  string
	shutdown func()

	statusItem *systray.MenuItem
}

// New cria a tray e já assina QRCodeReady no bus (antes do Connect, para nunca
// perder o evento). Fail-fast nas dependências.
func New(session ports.SessionManager, bus ports.EventBus, log ports.Logger, logsURL string, shutdown func()) *Tray {
	if session == nil || bus == nil || log == nil {
		panic("tray: session, bus e log são obrigatórios")
	}
	if shutdown == nil {
		shutdown = func() {}
	}
	t := &Tray{session: session, bus: bus, log: log, logsURL: logsURL, shutdown: shutdown}
	bus.Subscribe(domain.EventQRCodeReady, func(_ context.Context, evt domain.Event) error {
		if qr, ok := evt.(domain.QRCodeReady); ok {
			t.log.Info("abrindo QR no visualizador", "arquivo", qr.Path())
			openExternal(qr.Path())
		}
		return nil
	})
	return t
}

// Run sobe a bandeja (BLOQUEIA a thread principal até "Sair"). systray exige a
// main thread, por isso é o último passo do composition root.
func (t *Tray) Run() { systray.Run(t.onReady, t.onExit) }

func (t *Tray) onReady() {
	systray.SetIcon(iconData)
	systray.SetTitle("")
	systray.SetTooltip("WhatsApp Bridge")

	title := systray.AddMenuItem("WhatsApp Bridge", "")
	title.Disable()
	t.statusItem = systray.AddMenuItem("Status: iniciando...", "Estado da conexão com o WhatsApp")
	t.statusItem.Disable()
	systray.AddSeparator()
	mConnect := systray.AddMenuItem("Conectar / Reconectar", "Detecta o estado e conecta; gera QR se deslogado")
	mLog := systray.AddMenuItem("Ver log (navegador)", "Abre o final do log no navegador")
	mFolder := systray.AddMenuItem("Abrir pasta", "Abre a pasta do bridge")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Sair (para o bridge)", "Encerra o bridge")

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

// statusLoop reavalia o estado a cada 5s (detecta queda/volta sozinho).
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
		t.statusItem.SetTitle("Status: CONECTADO (" + st.Account() + ")")
		systray.SetTooltip("WhatsApp Bridge - conectado")
	case domain.SessionOfflineState:
		t.statusItem.SetTitle("Status: reconectando...")
		systray.SetTooltip("WhatsApp Bridge - reconectando")
		// Auto-cura: sessão válida porém caída → dispara reconexão (idempotente;
		// o adapter ignora se já houver um laço em andamento). Recupera sozinho
		// quando a internet volta, sem precisar do botão ou de reiniciar o exe.
		go t.session.Connect()
	default:
		t.statusItem.SetTitle("Status: DESLOGADO - clique Conectar (QR)")
		systray.SetTooltip("WhatsApp Bridge - deslogado, escaneie o QR")
	}
}
