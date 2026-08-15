// Package domain contém o modelo de domínio puro do bridge: entidades,
// value objects e eventos. Não importa infraestrutura, libs de rede nem
// banco de dados - é o centro da Clean Architecture.
package domain

import (
	"errors"
	"fmt"
	"strings"
)

// ServerType identifica o servidor de um JID do WhatsApp.
type ServerType string

const (
	ServerPN        ServerType = "s.whatsapp.net" // número de telefone
	ServerLID       ServerType = "lid"            // identificador oculto (Linked ID)
	ServerGroup     ServerType = "g.us"           // grupo
	ServerBroadcast ServerType = "broadcast"      // lista de transmissão
)

// ErrInvalidJID sinaliza um JID malformado (usado no fail-fast dos construtores).
var ErrInvalidJID = errors.New("domain: jid inválido")

// JID é um Value Object IMUTÁVEL que identifica uma entidade no WhatsApp
// (usuário, grupo, lista). Igualdade é por valor; não há setters, qualquer
// "mudança" produz uma nova instância.
type JID struct {
	user   string
	server ServerType
}

// NewJID constrói um JID a partir de "user@server", validando na hora (fail-fast).
// Programação positiva: a função só devolve um JID se ele for válido.
func NewJID(raw string) (JID, error) {
	raw = strings.TrimSpace(raw)
	at := strings.LastIndex(raw, "@")
	if at <= 0 || at >= len(raw)-1 {
		return JID{}, fmt.Errorf("%w: %q", ErrInvalidJID, raw)
	}
	return JID{user: raw[:at], server: ServerType(raw[at+1:])}, nil
}

// MustJID é um auxiliar para testes e constantes: entra em panic se inválido.
func MustJID(raw string) JID {
	jid, err := NewJID(raw)
	if err != nil {
		panic(err)
	}
	return jid
}

// NewJIDFromParts monta um JID a partir de user e server já separados (fail-fast).
func NewJIDFromParts(user string, server ServerType) (JID, error) {
	user = strings.TrimSpace(user)
	if user == "" || server == "" {
		return JID{}, fmt.Errorf("%w: user=%q server=%q", ErrInvalidJID, user, server)
	}
	return JID{user: user, server: server}, nil
}

// ParseRecipient interpreta um destinatário vindo da API: aceita um JID completo
// ("user@server") ou um número cru (vira @s.whatsapp.net, como no bridge legado).
// Programação positiva: devolve erro só quando nem isso é possível.
func ParseRecipient(raw string) (JID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return JID{}, fmt.Errorf("%w: destinatário vazio", ErrInvalidJID)
	}
	if strings.Contains(raw, "@") {
		return NewJID(raw)
	}
	return JID{user: raw, server: ServerPN}, nil
}

// User devolve a parte de usuário (antes do @).
func (j JID) User() string { return j.user }

// Server devolve o tipo de servidor (depois do @).
func (j JID) Server() ServerType { return j.server }

// IsLID indica se é um identificador oculto (@lid), os peões usam este.
func (j JID) IsLID() bool { return j.server == ServerLID }

// IsPN indica se é um número de telefone (@s.whatsapp.net).
func (j JID) IsPN() bool { return j.server == ServerPN }

// IsGroup indica se é um grupo (@g.us).
func (j JID) IsGroup() bool { return j.server == ServerGroup }

// IsZero indica o JID vazio (zero value), útil para guard clauses.
func (j JID) IsZero() bool { return j.user == "" && j.server == "" }

// String reconstrói a forma canônica "user@server".
func (j JID) String() string {
	if j.IsZero() {
		return ""
	}
	return j.user + "@" + string(j.server)
}
