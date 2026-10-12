# Builds the bridge.
#
# -H=windowsgui is NOT optional: without it the binary is a console app and Windows pops a
# black CMD window every time the logon task starts it. The code already expects a GUI build
# and redirects stdout/stderr to bridge.log (see main.go, "a -H=windowsgui build has no
# console"), and QR pairing writes qr.png and opens it in the default viewer, so nothing is
# lost by dropping the console.
#
# Usage:  .\build.ps1            build + swap the running binary (if the new one does not come
#                                up within 30 s, the previous binary is put back and started)
#         .\build.ps1 -NoSwap    build only, leave the running bridge alone
param([switch]$NoSwap)

$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

$new = "whatsapp-bridge-new.exe"
Write-Host "building $new ..."
go build -ldflags "-H=windowsgui" -o $new .
if ($LASTEXITCODE -ne 0) { throw "go build failed" }

# Verify the PE subsystem instead of trusting the flag: 2 = GUI, 3 = console.
$fs = [IO.File]::OpenRead((Resolve-Path $new))
$br = New-Object IO.BinaryReader($fs)
$fs.Position = 0x3C
$pe = $br.ReadUInt32()
$fs.Position = $pe + 24 + 68
$sub = $br.ReadUInt16()
$br.Close(); $fs.Close()
if ($sub -ne 2) { Remove-Item $new -Force; throw "subsystem is $sub, expected 2 (GUI)" }
Write-Host "subsystem OK: GUI, no console window"

if ($NoSwap) { Write-Host "built $new (not swapped)"; return }

function Get-Listener { Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1 }

# Polls instead of sleeping a fixed time. Returns the listener, or $null on timeout.
function Wait-Listener([int]$seconds) {
  $deadline = (Get-Date).AddSeconds($seconds)
  do {
    $c = Get-Listener
    if ($c) { return $c }
    Start-Sleep -Milliseconds 500
  } while ((Get-Date) -lt $deadline)
  return $null
}

$conn = Get-Listener
if ($conn) {
  $old = $conn.OwningProcess
  Write-Host "stopping the running bridge (pid $old) ..."
  Stop-Process -Id $old -Force
  # The bridge quits SILENTLY when port 8080 already answers (single-instance check in main.go),
  # so the new binary must only start once the old process is gone and the port is closed.
  # A fixed 3 s sleep was not enough on 2026-10-12: the new binary found 8080 still taken, quit
  # without writing a single log line, and the bridge stayed down until a manual rollback.
  try { Wait-Process -Id $old -Timeout 30 -ErrorAction Stop } catch { }
  $deadline = (Get-Date).AddSeconds(30)
  while ((Get-Listener) -and (Get-Date) -lt $deadline) { Start-Sleep -Milliseconds 250 }
  if (Get-Listener) { throw "port 8080 still taken after stopping pid $old; nothing was swapped" }
}
if (Test-Path "whatsapp-bridge.exe") { Move-Item "whatsapp-bridge.exe" "whatsapp-bridge.exe.prev" -Force }
Move-Item $new "whatsapp-bridge.exe" -Force
Start-Process -FilePath (Join-Path $PSScriptRoot "whatsapp-bridge.exe") -WorkingDirectory $PSScriptRoot

$conn = Wait-Listener 30
if ($conn) { Write-Host "bridge up, pid $($conn.OwningProcess)"; return }

# The new binary did not come up: put the previous one back so the line is not left down.
Write-Host "new bridge did not come up, restoring the previous binary ..."
Move-Item "whatsapp-bridge.exe" "whatsapp-bridge-failed.exe" -Force
Copy-Item "whatsapp-bridge.exe.prev" "whatsapp-bridge.exe" -Force
Start-Process -FilePath (Join-Path $PSScriptRoot "whatsapp-bridge.exe") -WorkingDirectory $PSScriptRoot
$conn = Wait-Listener 30
if ($conn) { throw "new bridge did not come up; previous binary restored (pid $($conn.OwningProcess)), failed one kept as whatsapp-bridge-failed.exe" }
throw "new bridge did not come up AND the previous binary did not come back either"
