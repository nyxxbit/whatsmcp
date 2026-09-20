# Rodar mais de uma conta de WhatsApp na mesma máquina

Até 18/09/2026 o bridge tinha a porta 8080 e a pasta `store/` fixas no código, então duas
contas exigiam duas máquinas. Agora não.

**A instância que já existe não mudou em nada.** Todo valor novo tem default igual ao
comportamento antigo: sem variável de ambiente nenhuma, o bridge sobe na 8080 com os dados na
pasta do próprio exe, exatamente como antes.

## O que foi alterado

| arquivo | o que mudou |
|---|---|
| `whatsapp-bridge/instancia.go` | **novo.** Lê as três variáveis e monta o rótulo da bandeja |
| `whatsapp-bridge/main.go` | `chdir` vai para a pasta de dados; a trava de instância única e o servidor REST usam a porta configurada; a bandeja mostra o apelido |
| `whatsapp-mcp-server/whatsapp.py` | `MESSAGES_DB_PATH` e `WHATSAPP_API_BASE_URL` passam a aceitar variável de ambiente |
| `scripts/instancia.ps1` | **novo.** Sobe, reporta ou para uma instância adicional |
| `.gitignore` | `instancias/` fica de fora do repositório: são mensagens de outra pessoa |

### As três variáveis

    WA_BRIDGE_PORT    porta do REST        default 8080
    WA_BRIDGE_DATA    pasta de dados       default: a pasta do exe
    WA_BRIDGE_NAME    apelido da conta     default: vazio

`store/`, `bridge.log` e `qr.png` já eram relativos ao diretório de trabalho, e o bridge faz
`chdir` para `WA_BRIDGE_DATA` logo no início. Por isso **duas instâncias com pastas diferentes
já têm banco, mídia e log separados sem mais nenhuma mudança**.

O que de fato faltava era a porta: a trava de instância única sempre testava a 8080, então o
segundo bridge **saía calado**, achando que já havia outro rodando. Era esse o bloqueio.

## Como usar

### Subir a segunda conta

```powershell
cd C:\Users\gabri\whatsapp-mcp\scripts
.\instancia.ps1 primo
```

Sobe na porta **8081** com os dados em `whatsapp-mcp\instancias\primo\`. No primeiro acesso ele
gera `qr.png` e abre no visualizador: escanear em **Aparelhos conectados** no celular da conta
nova. Outra porta: `.\instancia.ps1 primo -Porta 8082`. Parar: `.\instancia.ps1 primo -Parar`.

Rodando na mão, sem o script:

```powershell
$env:WA_BRIDGE_PORT='8081'
$env:WA_BRIDGE_DATA='C:\Users\gabri\whatsapp-mcp\instancias\primo'
$env:WA_BRIDGE_NAME='primo'
Start-Process C:\Users\gabri\whatsapp-mcp\whatsapp-bridge\whatsapp-bridge.exe
```

### Apontar um MCP para essa conta

No `claude_desktop_config.json` (ou equivalente do cliente), o servidor MCP da conta nova leva
as duas variáveis:

```json
{
  "mcpServers": {
    "whatsapp-primo": {
      "command": "uv",
      "args": ["--directory", "C:\\Users\\gabri\\whatsapp-mcp\\whatsapp-mcp-server",
               "run", "main.py"],
      "env": {
        "WHATSAPP_API_BASE_URL": "http://localhost:8081/api",
        "WHATSAPP_MESSAGES_DB": "C:\\Users\\gabri\\whatsapp-mcp\\instancias\\primo\\store\\messages.db"
      }
    }
  }
}
```

O MCP da conta principal **não leva `env` nenhum** e continua na 8080.

> [!critico] **As duas variáveis andam juntas, sempre.** Apontar o banco de uma conta para a API
> da outra faz o assistente **ler as mensagens de uma e enviar pela outra**. É o pior erro
> possível aqui, não dá aviso nenhum e só aparece quando a mensagem já chegou na pessoa errada.

## Conferir que está tudo separado

```powershell
Get-NetTCPConnection -LocalPort 8080,8081 -State Listen | Select LocalPort,OwningProcess
Get-ChildItem C:\Users\gabri\whatsapp-mcp\instancias\primo -Recurse -File
```

Devem aparecer dois processos distintos, e a pasta do primo com `store\messages.db` e
`store\whatsapp.db` próprios. Instância ainda não pareada responde **503** na API, que é o
esperado, e não erro de conexão.

## Endpoints disponíveis nas duas

Todos valem para qualquer instância, trocando a porta:

    POST /api/send                 enviar texto ou arquivo
    POST /api/download             baixar mídia
    POST /api/revoke               apagar mensagem enviada
    POST /api/pair-phone           parear por código, sem QR
    GET  /api/menu-options         ler menu de bot
    POST /api/select-option        clicar em opção de menu
    POST /api/sync-labels          sincronizar etiquetas
    POST /api/group-create         criar grupo
    GET  /api/groups               listar grupos
    GET  /api/group-info           um grupo em detalhe
    POST /api/group-participants   adicionar, remover, promover, rebaixar
    POST /api/group-edit           renomear, mudar descrição
    POST /api/on-whatsapp          o número tem WhatsApp

Os seis últimos são de 18/09/2026. O `on-whatsapp` importa: é como se descarta telefone fixo
**sem** tentar enviar. Sondar número disparando mensagem queima a reputação da conta, e em
10/09/2026 umas dez mensagens frias em meia hora derrubaram o aparelho.

## Duas armadilhas de quem for mexer nisso

**Compile sempre com `whatsapp-bridge\build.ps1`.** Sem `-ldflags "-H=windowsgui"` o binário
vira aplicação de console e o Windows abre uma janela preta a cada inicialização. O script
confere o subsystem do PE depois de compilar (2 = GUI) e recusa o binário se estiver errado.

**Mídia do WhatsApp expira no servidor em uma a três semanas**, e repareamento invalida para
sempre tudo que chegou pela sessão anterior. Numa instância nova isso não é problema, mas
repareamento na conta antiga é.
