# Sobe uma instancia ADICIONAL do bridge, com outra conta de WhatsApp.
#
# A instancia principal nao e tocada: ela continua na porta 8080 com os dados na pasta do exe.
# Esta aqui usa porta e pasta proprias, entao o banco, a midia e o log ficam separados.
#
#   .\instancia.ps1 primo            sobe (ou apenas reporta, se ja estiver no ar)
#   .\instancia.ps1 primo -Porta 8082
#   .\instancia.ps1 primo -Parar
param(
  [Parameter(Mandatory = $true)][string]$Nome,
  [int]$Porta = 8081,
  [switch]$Parar
)
$ErrorActionPreference = "Stop"
$raiz  = Split-Path -Parent $PSScriptRoot
$exe   = Join-Path $raiz "whatsapp-bridge\whatsapp-bridge.exe"
$dados = Join-Path $raiz "instancias\$Nome"

$conn = Get-NetTCPConnection -LocalPort $Porta -State Listen -EA SilentlyContinue | Select-Object -First 1
if ($Parar) {
  if ($conn) { Stop-Process -Id $conn.OwningProcess -Force; "instancia '$Nome' parada (porta $Porta)" }
  else { "nada rodando na porta $Porta" }
  return
}
if ($conn) { "instancia '$Nome' ja esta no ar na porta $Porta (pid $($conn.OwningProcess))"; return }
if (-not (Test-Path $exe)) { throw "bridge nao compilado: $exe" }
New-Item -ItemType Directory -Force -Path $dados | Out-Null

$env:WA_BRIDGE_PORT = "$Porta"
$env:WA_BRIDGE_DATA = $dados
$env:WA_BRIDGE_NAME = $Nome
Start-Process -FilePath $exe -WorkingDirectory (Split-Path $exe)
Remove-Item Env:WA_BRIDGE_PORT, Env:WA_BRIDGE_DATA, Env:WA_BRIDGE_NAME
Start-Sleep -Seconds 8

$conn = Get-NetTCPConnection -LocalPort $Porta -State Listen -EA SilentlyContinue | Select-Object -First 1
if (-not $conn) { throw "a instancia nao subiu; veja $dados\bridge.log" }
"instancia '$Nome' no ar na porta $Porta (pid $($conn.OwningProcess))"
"dados em $dados"
if (Test-Path "$dados\qr.png") { "primeiro acesso: escaneie $dados\qr.png em Aparelhos conectados" }
