# Builds the bridge.
#
# -H=windowsgui is NOT optional: without it the binary is a console app and Windows pops a
# black CMD window every time the logon task starts it. The code already expects a GUI build
# and redirects stdout/stderr to bridge.log (see main.go, "a -H=windowsgui build has no
# console"), and QR pairing writes qr.png and opens it in the default viewer, so nothing is
# lost by dropping the console.
#
# Usage:  .\build.ps1            build + swap the running binary
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

$conn = Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
if ($conn) {
  Write-Host "stopping the running bridge (pid $($conn.OwningProcess)) ..."
  Stop-Process -Id $conn.OwningProcess -Force
  Start-Sleep -Seconds 3
}
if (Test-Path "whatsapp-bridge.exe") { Move-Item "whatsapp-bridge.exe" "whatsapp-bridge.exe.prev" -Force }
Move-Item $new "whatsapp-bridge.exe" -Force
Start-Process -FilePath (Join-Path $PSScriptRoot "whatsapp-bridge.exe") -WorkingDirectory $PSScriptRoot
Start-Sleep -Seconds 8

$conn = Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
if ($conn) { Write-Host "bridge up, pid $($conn.OwningProcess)" } else { throw "bridge did not come back up" }
