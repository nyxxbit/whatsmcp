package wa

import (
	"context"
	"fmt"
	"time"

	"go.mau.fi/whatsmeow/appstate"

	"github.com/nyxxbit/wabridge/internal/core/domain"
)

// connectBackoffStart e connectBackoffMax limitam a retentativa de conexão.
const (
	connectBackoffStart = 5 * time.Second
	connectBackoffMax   = 60 * time.Second
)

// Connect é o ponto único de (re)conexão (startup e botão da tray): já conectado
// → nada; sessão válida caída → entra no laço de reconexão com backoff (auto-cura
// após queda de internet); deslogado → QR. Serializado por mutex; protegido por
// reconnecting/qrActive para não empilhar laços nem abrir dois canais de QR.
func (c *Client) Connect() {
	c.connectMu.Lock()
	switch {
	case c.wm.Store.ID != nil && c.wm.IsConnected():
		c.connectMu.Unlock()
		return
	case c.wm.Store.ID != nil:
		if c.reconnecting {
			c.connectMu.Unlock()
			return // já há um laço de reconexão em andamento
		}
		c.reconnecting = true
		c.connectMu.Unlock()
		go c.connectWithRetry()
		return
	case c.qrActive:
		c.connectMu.Unlock()
		c.log.Info("pareamento por QR já em andamento")
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

// connectWithRetry tenta conectar a sessão existente até conseguir, com backoff
// exponencial. Corrige o buraco em que uma falha de conexão INICIAL (ex.: bridge
// (re)iniciado com a rede fora) deixava o bridge preso, o autoreconnect do
// whatsmeow só reage a quedas PÓS-conexão, não à primeira tentativa que falha.
// Depois que conecta uma vez, o autoreconnect do whatsmeow assume as quedas.
func (c *Client) connectWithRetry() {
	defer func() {
		c.connectMu.Lock()
		c.reconnecting = false
		c.connectMu.Unlock()
	}()

	c.wm.Disconnect() // parte de um estado limpo
	time.Sleep(300 * time.Millisecond)

	backoff := connectBackoffStart
	for attempt := 1; ; attempt++ {
		if c.wm.Store.ID == nil {
			return // virou logout no meio, o fluxo de QR cuida
		}
		if c.wm.IsConnected() {
			return // já está conectado (autoreconnect cuidou)
		}
		err := c.wm.Connect()
		if err == nil || c.wm.IsConnected() {
			c.log.Info("conectado ao WhatsApp", "tentativa", attempt)
			return
		}
		c.log.Warn("conexão falhou; nova tentativa", "tentativa", attempt, "em", backoff.String(), "err", err)
		time.Sleep(backoff)
		if backoff < connectBackoffMax {
			if backoff *= 2; backoff > connectBackoffMax {
				backoff = connectBackoffMax
			}
		}
	}
}

// Disconnect derruba a conexão (mantém a sessão; reconecta depois).
func (c *Client) Disconnect() { c.wm.Disconnect() }

// Status reflete o estado real da conexão para a tray/diagnóstico.
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

// SyncLabels dispara o fullSync de app state, emitindo os eventos de etiqueta.
// EmitAppStateEventsOnFullSync precisa estar ligado, senão o fullSync re-aplica
// mas NÃO re-emite label_edit/label_jid (o bug que zerava as etiquetas).
func (c *Client) SyncLabels(ctx context.Context) error {
	if c.wm.Store.ID == nil {
		return fmt.Errorf("wa: sem sessão para sincronizar etiquetas")
	}
	c.wm.EmitAppStateEventsOnFullSync = true
	if err := c.wm.FetchAppState(ctx, appstate.WAPatchRegular, true, false); err != nil {
		return fmt.Errorf("wa: sincronizar etiquetas: %w", err)
	}
	return nil
}
