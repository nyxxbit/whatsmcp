package domain

import "strings"

// Identity é a identidade resolvida de um remetente - "VISÃO AMPLA": carrega o
// NOME, o NÚMERO e o JID cru ao mesmo tempo, nunca um no lugar do outro. Nome e
// telefone podem vir vazios (desconhecidos); o JID está sempre presente.
//
// Value Object imutável; igualdade por valor.
type Identity struct {
	jid   JID
	name  string
	phone string
}

// NewIdentity cria a identidade (fail-fast: JID obrigatório; nome/telefone opcionais).
func NewIdentity(jid JID, name, phone string) (Identity, error) {
	if jid.IsZero() {
		return Identity{}, ErrInvalidJID
	}
	return Identity{jid: jid, name: strings.TrimSpace(name), phone: strings.TrimSpace(phone)}, nil
}

// JID devolve o identificador cru (sempre presente).
func (i Identity) JID() JID { return i.jid }

// Name devolve o nome legível ("" se desconhecido).
func (i Identity) Name() string { return i.name }

// Phone devolve o número ("" se não resolvido, ex.: @lid sem mapeamento).
func (i Identity) Phone() string { return i.phone }

// HasName indica se o nome foi resolvido.
func (i Identity) HasName() bool { return i.name != "" }

// HasPhone indica se o número foi resolvido.
func (i Identity) HasPhone() bool { return i.phone != "" }

// Display monta uma string única com tudo que se sabe - "Nome (numero)", ou só o
// que houver, caindo no user do JID em último caso. Para logs/relatórios de uma linha.
func (i Identity) Display() string {
	switch {
	case i.name != "" && i.phone != "":
		return i.name + " (" + i.phone + ")"
	case i.name != "":
		return i.name
	case i.phone != "":
		return i.phone
	default:
		return i.jid.User()
	}
}
