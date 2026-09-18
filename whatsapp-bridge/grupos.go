package main

// Endpoints de grupo e de contato que faltavam no bridge.
//
// Ate' 18/09/2026 o bridge so' sabia enviar, baixar, apagar, parear e clicar em menu. Criar
// grupo, ver em quais grupos a conta esta' e conferir se um numero tem WhatsApp exigiam o
// celular na mao. Ficam em arquivo proprio porque o main.go ja' tem 2.300 linhas.
//
// Tres cuidados que valem para todos eles:
//
//  1. **Grupo novo notifica gente de verdade.** Quem entra recebe "fulano adicionou voce".
//     Nao e' operacao silenciosa, entao o endpoint exige o nome e nao inventa participante.
//  2. **O nome do grupo tem teto de 25 caracteres** no protocolo. Passar disso devolve 406 do
//     servidor, que e' erro opaco; a checagem e' feita aqui, com mensagem que se entende.
//  3. **IsOnWhatsApp e' a forma certa de descartar telefone fixo**, e nao tentar enviar e ver
//     o erro. Enviar para numero invalido conta contra a reputacao da conta, e em 10/09/2026
//     umas dez mensagens frias em meia hora derrubaram o aparelho.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

const maxNomeGrupo = 25

// jidDe aceita "5517999999999", "5517999999999@s.whatsapp.net" ou "123@lid" e devolve o JID.
func jidDe(bruto string) (types.JID, error) {
	bruto = strings.TrimSpace(bruto)
	if bruto == "" {
		return types.EmptyJID, fmt.Errorf("numero vazio")
	}
	if strings.ContainsRune(bruto, '@') {
		return types.ParseJID(bruto)
	}
	so := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, bruto)
	if so == "" {
		return types.EmptyJID, fmt.Errorf("numero sem digitos: %q", bruto)
	}
	return types.NewJID(so, types.DefaultUserServer), nil
}

func respJSON(w http.ResponseWriter, status int, corpo any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(corpo)
}

func erroJSON(w http.ResponseWriter, status int, msg string) {
	respJSON(w, status, map[string]any{"success": false, "error": msg})
}

// clientePronto devolve o cliente conectado, ou escreve o erro e devolve nil.
func clientePronto(w http.ResponseWriter) *whatsmeow.Client {
	c := globalClient
	if c == nil || c.Store.ID == nil {
		erroJSON(w, http.StatusServiceUnavailable, "bridge nao esta' conectado")
		return nil
	}
	return c
}

func soPost(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		erroJSON(w, http.StatusMethodNotAllowed, "use POST")
		return false
	}
	return true
}

func grupoResumo(g *types.GroupInfo) map[string]any {
	participantes := make([]map[string]any, 0, len(g.Participants))
	for _, p := range g.Participants {
		participantes = append(participantes, map[string]any{
			"jid": p.JID.String(), "admin": p.IsAdmin, "super_admin": p.IsSuperAdmin,
		})
	}
	return map[string]any{
		"jid": g.JID.String(), "name": g.Name, "topic": g.Topic,
		"created": g.GroupCreated.Format(time.RFC3339),
		"owner":   g.OwnerJID.String(), "participants": participantes,
	}
}

func registrarRotasGrupo() {
	// ---- criar grupo -------------------------------------------------------------
	// POST {"name":"DRG Gastos","participants":["5517996736334"]}
	// A propria conta entra sozinha, o servidor adiciona; nao precisa listar.
	http.HandleFunc("/api/group-create", func(w http.ResponseWriter, r *http.Request) {
		if !soPost(w, r) {
			return
		}
		var req struct {
			Name         string   `json:"name"`
			Participants []string `json:"participants"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			erroJSON(w, http.StatusBadRequest, "JSON invalido: "+err.Error())
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			erroJSON(w, http.StatusBadRequest, "name e' obrigatorio")
			return
		}
		if len([]rune(req.Name)) > maxNomeGrupo {
			erroJSON(w, http.StatusBadRequest, fmt.Sprintf(
				"o nome tem %d caracteres e o WhatsApp aceita %d; encurte antes de tentar",
				len([]rune(req.Name)), maxNomeGrupo))
			return
		}
		if len(req.Participants) == 0 {
			erroJSON(w, http.StatusBadRequest,
				"participants e' obrigatorio: um grupo precisa de pelo menos mais uma pessoa")
			return
		}
		c := clientePronto(w)
		if c == nil {
			return
		}
		jids := make([]types.JID, 0, len(req.Participants))
		for _, p := range req.Participants {
			j, err := jidDe(p)
			if err != nil {
				erroJSON(w, http.StatusBadRequest, err.Error())
				return
			}
			jids = append(jids, j)
		}
		g, err := c.CreateGroup(context.Background(), whatsmeow.ReqCreateGroup{
			Name: req.Name, Participants: jids,
		})
		if err != nil {
			fmt.Printf("[bridge] group-create falhou: %v\n", err)
			erroJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		fmt.Printf("[bridge] grupo criado: %s (%s)\n", g.Name, g.JID)
		respJSON(w, http.StatusOK, map[string]any{"success": true, "group": grupoResumo(g)})
	})

	// ---- listar os grupos em que a conta esta' ------------------------------------
	http.HandleFunc("/api/groups", func(w http.ResponseWriter, r *http.Request) {
		c := clientePronto(w)
		if c == nil {
			return
		}
		gs, err := c.GetJoinedGroups(context.Background())
		if err != nil {
			erroJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		fora := make([]map[string]any, 0, len(gs))
		for _, g := range gs {
			fora = append(fora, map[string]any{
				"jid": g.JID.String(), "name": g.Name,
				"participants": len(g.Participants),
			})
		}
		respJSON(w, http.StatusOK, map[string]any{"success": true, "count": len(fora),
			"groups": fora})
	})

	// ---- um grupo em detalhe -------------------------------------------------------
	// GET /api/group-info?jid=...@g.us
	http.HandleFunc("/api/group-info", func(w http.ResponseWriter, r *http.Request) {
		c := clientePronto(w)
		if c == nil {
			return
		}
		j, err := jidDe(r.URL.Query().Get("jid"))
		if err != nil {
			erroJSON(w, http.StatusBadRequest, err.Error())
			return
		}
		g, err := c.GetGroupInfo(context.Background(), j)
		if err != nil {
			erroJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		respJSON(w, http.StatusOK, map[string]any{"success": true, "group": grupoResumo(g)})
	})

	// ---- entrar e sair de participante ---------------------------------------------
	// POST {"jid":"...@g.us","action":"add|remove|promote|demote","participants":[...]}
	http.HandleFunc("/api/group-participants", func(w http.ResponseWriter, r *http.Request) {
		if !soPost(w, r) {
			return
		}
		var req struct {
			JID          string   `json:"jid"`
			Action       string   `json:"action"`
			Participants []string `json:"participants"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			erroJSON(w, http.StatusBadRequest, "JSON invalido: "+err.Error())
			return
		}
		acoes := map[string]whatsmeow.ParticipantChange{
			"add": whatsmeow.ParticipantChangeAdd, "remove": whatsmeow.ParticipantChangeRemove,
			"promote": whatsmeow.ParticipantChangePromote,
			"demote":  whatsmeow.ParticipantChangeDemote,
		}
		acao, ok := acoes[strings.ToLower(strings.TrimSpace(req.Action))]
		if !ok {
			erroJSON(w, http.StatusBadRequest, "action tem que ser add, remove, promote ou demote")
			return
		}
		if len(req.Participants) == 0 {
			erroJSON(w, http.StatusBadRequest, "participants e' obrigatorio")
			return
		}
		c := clientePronto(w)
		if c == nil {
			return
		}
		grupo, err := jidDe(req.JID)
		if err != nil {
			erroJSON(w, http.StatusBadRequest, err.Error())
			return
		}
		jids := make([]types.JID, 0, len(req.Participants))
		for _, p := range req.Participants {
			j, err := jidDe(p)
			if err != nil {
				erroJSON(w, http.StatusBadRequest, err.Error())
				return
			}
			jids = append(jids, j)
		}
		res, err := c.UpdateGroupParticipants(context.Background(), grupo, jids, acao)
		if err != nil {
			erroJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		fora := make([]map[string]any, 0, len(res))
		for _, p := range res {
			fora = append(fora, map[string]any{"jid": p.JID.String(), "error": p.Error})
		}
		respJSON(w, http.StatusOK, map[string]any{"success": true, "result": fora})
	})

	// ---- renomear o grupo e mudar a descricao ---------------------------------------
	// POST {"jid":"...@g.us","name":"...","topic":"..."}   os dois sao opcionais
	http.HandleFunc("/api/group-edit", func(w http.ResponseWriter, r *http.Request) {
		if !soPost(w, r) {
			return
		}
		var req struct {
			JID   string  `json:"jid"`
			Name  *string `json:"name"`
			Topic *string `json:"topic"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			erroJSON(w, http.StatusBadRequest, "JSON invalido: "+err.Error())
			return
		}
		c := clientePronto(w)
		if c == nil {
			return
		}
		grupo, err := jidDe(req.JID)
		if err != nil {
			erroJSON(w, http.StatusBadRequest, err.Error())
			return
		}
		feito := []string{}
		if req.Name != nil {
			if len([]rune(*req.Name)) > maxNomeGrupo {
				erroJSON(w, http.StatusBadRequest, fmt.Sprintf(
					"o nome tem %d caracteres e o WhatsApp aceita %d",
					len([]rune(*req.Name)), maxNomeGrupo))
				return
			}
			if err := c.SetGroupName(context.Background(), grupo, *req.Name); err != nil {
				erroJSON(w, http.StatusInternalServerError, "nome: "+err.Error())
				return
			}
			feito = append(feito, "name")
		}
		if req.Topic != nil {
			if err := c.SetGroupTopic(context.Background(), grupo, "", "", *req.Topic); err != nil {
				erroJSON(w, http.StatusInternalServerError, "topic: "+err.Error())
				return
			}
			feito = append(feito, "topic")
		}
		if len(feito) == 0 {
			erroJSON(w, http.StatusBadRequest, "informe name, topic, ou os dois")
			return
		}
		respJSON(w, http.StatusOK, map[string]any{"success": true, "changed": feito})
	})

	// ---- o numero tem WhatsApp? ------------------------------------------------------
	// POST {"phones":["5517999999999","1733331111"]}
	// E' assim que se descarta telefone fixo SEM gastar reputacao tentando enviar.
	http.HandleFunc("/api/on-whatsapp", func(w http.ResponseWriter, r *http.Request) {
		if !soPost(w, r) {
			return
		}
		var req struct {
			Phones []string `json:"phones"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			erroJSON(w, http.StatusBadRequest, "JSON invalido: "+err.Error())
			return
		}
		if len(req.Phones) == 0 {
			erroJSON(w, http.StatusBadRequest, "phones e' obrigatorio")
			return
		}
		c := clientePronto(w)
		if c == nil {
			return
		}
		res, err := c.IsOnWhatsApp(context.Background(), req.Phones)
		if err != nil {
			erroJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		fora := make([]map[string]any, 0, len(res))
		for _, p := range res {
			fora = append(fora, map[string]any{
				"query": p.Query, "tem_whatsapp": p.IsIn, "jid": p.JID.String(),
			})
		}
		respJSON(w, http.StatusOK, map[string]any{"success": true, "result": fora})
	})
}
