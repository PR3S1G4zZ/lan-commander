# LAN Commander - Build All
# Este script compila todo el proyecto

Write-Host "╔══════════════════════════════════════════╗" -ForegroundColor Cyan
Write-Host "║      LAN Commander - Build All          ║" -ForegroundColor Cyan
Write-Host "╚══════════════════════════════════════════╝" -ForegroundColor Cyan

# 1. Build Agent (Windows)
Write-Host "`n[1/3] Building Agent (Windows)..." -ForegroundColor Yellow
Set-Location -LiteralPath "$PSScriptRoot\..\agent"
go build -ldflags="-s -w" -o build\lan-agent.exe .\cmd\lan-agent\
if ($?) { Write-Host "  ✅ lan-agent.exe built" -ForegroundColor Green } else { Write-Host "  ❌ FAILED" -ForegroundColor Red }

# 2. Build Agent (Linux) - cross-compile
Write-Host "`n[2/3] Building Agent (Linux)..." -ForegroundColor Yellow
$env:GOOS="linux"; $env:GOARCH="amd64"
go build -ldflags="-s -w" -o build\lan-agent-linux .\cmd\lan-agent\
if ($?) { Write-Host "  ✅ lan-agent-linux built" -ForegroundColor Green } else { Write-Host "  ❌ FAILED" -ForegroundColor Red }
$env:GOOS=""; $env:GOARCH=""

# 3. Build Control Center
Write-Host "`n[3/3] Building Control Center..." -ForegroundColor Yellow
$env:Path += ";$env:USERPROFILE\go\bin"
Set-Location -LiteralPath "$PSScriptRoot\..\control-center"
wails build -f -o build\bin\lan-commander.exe
if ($?) { Write-Host "  ✅ lan-commander.exe built" -ForegroundColor Green } else { Write-Host "  ❌ FAILED" -ForegroundColor Red }

Write-Host "`n╔══════════════════════════════════════════╗" -ForegroundColor Cyan
Write-Host "║          Build Complete!                ║" -ForegroundColor Cyan
Write-Host "╚══════════════════════════════════════════╝" -ForegroundColor Cyan
Write-Host "Output files:" -ForegroundColor White
Write-Host "  agent\build\lan-agent.exe       - Windows Agent" -ForegroundColor White
Write-Host "  agent\build\lan-agent-linux     - Linux Agent" -ForegroundColor White
Write-Host "  control-center\build\bin\lan-commander.exe - Control Center" -ForegroundColor White
