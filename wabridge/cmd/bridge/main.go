// Command bridge é o composition root do wabridge: o ÚNICO lugar que conhece
// implementações concretas. Aqui se monta a injeção de dependências (adapters) e
// se registram as features (armas). Nenhuma regra de negócio mora neste arquivo.
//
// Fluxo: chdir p/ a pasta do exe → instância única (porta) → logging com rotação
// → persistência SQLite → adapter whatsmeow → features no Registry → REST (mesmo
// contrato do legado) → bandeja (systray) que bloqueia a main thread.
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
	// cwd = pasta do exe (store/, logs e qr.png usam caminhos relativos).
	if exe, err := os.Executable(); err == nil {
		_ = os.Chdir(filepath.Dir(exe))
	}

	addr := envOr("WABRIDGE_ADDR", ":8080")
	storeDir := envOr("WABRIDGE_STORE_DIR", "store")

	// Instância única: se o endereço já responde, já tem um bridge rodando.
	if c, err := net.DialTimeout("tcp", localDial(addr), time.Second); err == nil {
		_ = c.Close()
		return nil
	}

	// stderr → arquivo truncado a cada boot: captura panic sem nunca crescer
	// (o build -H=windowsgui não tem console). O log normal vai pro RotatingFile.
	if f, err := os.OpenFile("wabridge.panic.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err == nil {
		os.Stderr = f
	}

	// ── Logging profissional (rotação por tamanho; fim do Notepad) ──
	logFile, err := logging.NewRotatingFile("wabridge.log", 1<<20)
	if err != nil {
		return fmt.Errorf("abrir log: %w", err)
	}
	log := logging.New(logFile)

	// ── Persistência (mesmo schema do legado) ──
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

	// ── Adapter whatsmeow (traduz eventos da lib → eventos de domínio no bus) ──
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

	// Caso de uso de download (Repository + Fetcher + cache em disco).
	downloader := messaging.NewDownloader(msgRepo, adapter, storeDir, log)

	// ── Features (armas plugáveis): adicionar nova = só mais um .Add(...) ──
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

	// ── Entrega REST (MESMO contrato: /api/send, /api/download, /api/sync-labels, /logs) ──
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
			log.Error("servidor REST encerrado", "err", err)
		}
	}()

	// Encerramento limpo acionado pelo "Sair" da bandeja.
	shutdown := func() {
		adapter.Disconnect()
		_ = contactRepo.Close()
		_ = store.Close()
		_ = logFile.Close()
		os.Exit(0)
	}

	// Conexão unificada: sessão válida → reconecta; deslogado → QR (tray abre o PNG).
	// WABRIDGE_NO_CONNECT=1 pula este passo (smoke test do boot/REST sem tocar no WhatsApp).
	if envOr("WABRIDGE_NO_CONNECT", "") != "1" {
		adapter.Connect()
	} else {
		log.Warn("WABRIDGE_NO_CONNECT=1: conexão com o WhatsApp pulada (modo teste)")
	}

	log.Info("wabridge no ar", "log", logFile.Path(), "rest", addr, "store", storeDir)

	// Bandeja bloqueia a main thread até "Sair".
	tray.New(adapter, bus, log, "http://127.0.0.1"+addr+"/logs", shutdown).Run()
	return nil
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// localDial transforma ":8080" em "127.0.0.1:8080" para o teste de instância única.
func localDial(addr string) string {
	if strings.HasPrefix(addr, ":") {
		return "127.0.0.1" + addr
	}
	return addr
}
