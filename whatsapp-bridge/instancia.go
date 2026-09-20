package main

// Permite rodar MAIS DE UM bridge na mesma maquina, cada um com a sua conta de WhatsApp.
//
// Ate' 18/09/2026 tudo era fixo: porta 8080 no codigo e a pasta `store/` sempre ao lado do
// exe. Duas contas exigiam duas maquinas.
//
// Tres variaveis de ambiente resolvem, e **o default e' identico ao comportamento antigo**,
// entao a instancia que ja' roda nao muda em nada:
//
//	WA_BRIDGE_PORT   porta do REST      default 8080
//	WA_BRIDGE_DATA   pasta de dados     default: a pasta do proprio exe
//	WA_BRIDGE_NAME   apelido            default: vazio, e o tray diz so' "WhatsApp Bridge"
//
// `store/`, `bridge.log` e `qr.png` sao relativos ao diretorio de trabalho, e o bridge faz
// chdir para WA_BRIDGE_DATA logo no inicio. Ou seja, **duas instancias com WA_BRIDGE_DATA
// diferente ja' tem banco, midia e log separados sem mais nenhuma mudanca**. O que faltava
// mesmo era a porta, porque a checagem de instancia unica batia sempre em 8080 e a segunda
// instancia saia calada achando que ja' havia outra.
//
// O apelido existe para nao confundir na bandeja: com dois icones iguais, mandar mensagem
// pela conta errada e' questao de tempo.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// portaDoBridge le' WA_BRIDGE_PORT. Valor invalido nao e' silenciado: cai no default e avisa
// no log, porque bridge subindo em porta inesperada e' pior do que bridge que nao sobe.
func portaDoBridge() int {
	bruto := strings.TrimSpace(os.Getenv("WA_BRIDGE_PORT"))
	if bruto == "" {
		return 8080
	}
	p, err := strconv.Atoi(bruto)
	if err != nil || p < 1 || p > 65535 {
		fmt.Printf("[bridge] WA_BRIDGE_PORT=%q nao e' uma porta valida, usando 8080\n", bruto)
		return 8080
	}
	return p
}

// pastaDeDados devolve onde ficam store/, bridge.log e qr.png.
func pastaDeDados() string {
	if d := strings.TrimSpace(os.Getenv("WA_BRIDGE_DATA")); d != "" {
		abs, err := filepath.Abs(d)
		if err == nil {
			if mk := os.MkdirAll(abs, 0755); mk == nil {
				return abs
			}
		}
		fmt.Printf("[bridge] WA_BRIDGE_DATA=%q nao pode ser usada, caindo na pasta do exe\n", d)
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(exe)
	}
	return "."
}

// nomeDaInstancia e' o apelido que aparece na bandeja e no log.
func nomeDaInstancia() string {
	return strings.TrimSpace(os.Getenv("WA_BRIDGE_NAME"))
}

// rotuloTray monta "WhatsApp Bridge" ou "WhatsApp Bridge [primo]".
func rotuloTray(sufixo string) string {
	base := "WhatsApp Bridge"
	if n := nomeDaInstancia(); n != "" {
		base = fmt.Sprintf("%s [%s]", base, n)
	}
	if sufixo != "" {
		return base + " - " + sufixo
	}
	return base
}
