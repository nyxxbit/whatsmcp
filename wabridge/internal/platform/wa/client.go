// Package wa adapta a biblioteca whatsmeow ao núcleo: traduz os eventos da lib em
// eventos de domínio (publicados no bus) e implementa as portas de saída
// (MessageSender, MediaFetcher, SessionManager, LabelSyncer). É o ÚNICO pacote
// que conhece os tipos do whatsmeow, features e entrega só enxergam ports.
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

// Garantias em tempo de compilação de que o adapter satisfaz as portas de saída.
var (
	_ ports.MessageSender  = (*Client)(nil)
	_ ports.MediaFetcher   = (*Client)(nil)
	_ ports.SessionManager = (*Client)(nil)
	_ ports.LabelSyncer    = (*Client)(nil)
)

const eventQueueSize = 4096 // capacidade da fila de eventos (history sync grande)

// Config reúne as dependências do adapter (injetadas no composition root).
type Config struct {
	WhatsappDBPath string         // store de sessão do whatsmeow (ex: "store/whatsapp.db")
	QRPath         string         // caminho do PNG de QR (ex: "qr.png")
	Bus            ports.EventBus // para publicar eventos de domínio
	Log            ports.Logger
	ChatNames      ports.ChatRepository // lookup do nome já salvo (resolução de conversa)
}

// Client é o adapter do whatsmeow.
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

// New constrói o adapter: abre o store de sessão, cria o client whatsmeow, sobe o
// worker da fila e registra o handler de eventos. Fail-fast nas dependências.
func New(cfg Config) (*Client, error) {
	if cfg.Bus == nil || cfg.Log == nil || cfg.ChatNames == nil {
		return nil, fmt.Errorf("wa: Bus, Log e ChatNames são obrigatórios")
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
		return nil, fmt.Errorf("wa: abrir store de sessão: %w", err)
	}
	device, err := container.GetFirstDevice(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			device = container.NewDevice()
			cfg.Log.Info("wa: novo device criado (sem sessão, precisará de QR)")
		} else {
			return nil, fmt.Errorf("wa: obter device: %w", err)
		}
	}
	wm := whatsmeow.NewClient(device, newWALog(cfg.Log, "Client"))
	if wm == nil {
		return nil, fmt.Errorf("wa: falha ao criar client whatsmeow")
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
