# LAN Commander - Build All
# Compila los agentes y el Control Center.

$ErrorActionPreference = "Stop"
$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$agentDir = Join-Path $repoRoot "agent"
$controlCenterDir = Join-Path $repoRoot "control-center"

Write-Host "LAN Commander - Build All" -ForegroundColor Cyan

Push-Location $agentDir
try {
    Write-Host "[1/3] Compilando agente Windows..." -ForegroundColor Yellow
    go build -ldflags="-s -w" -o build\lan-agent.exe .\cmd\lan-agent
    Copy-Item build\lan-agent.exe ..\installers\windows\lan-agent.exe -Force
    Write-Host "  OK: agente Windows y payload del instalador" -ForegroundColor Green

    Write-Host "[2/3] Compilando agente Linux amd64..." -ForegroundColor Yellow
    $previousGoOS = $env:GOOS
    $previousGoARCH = $env:GOARCH
    try {
        $env:GOOS = "linux"
        $env:GOARCH = "amd64"
        go build -ldflags="-s -w" -o build\lan-agent-linux .\cmd\lan-agent
    } finally {
        $env:GOOS = $previousGoOS
        $env:GOARCH = $previousGoARCH
    }
    Copy-Item build\lan-agent-linux ..\installers\linux\lan-agent-linux -Force
    Write-Host "  OK: agente Linux y payload del instalador" -ForegroundColor Green
} finally {
    Pop-Location
}

Write-Host "[3/3] Compilando Control Center..." -ForegroundColor Yellow
$wails = Join-Path $env:USERPROFILE "go\bin\wails.exe"
if (-not (Test-Path $wails)) {
    $wails = "wails"
}
Push-Location $controlCenterDir
try {
    & $wails build -f -o lan-commander.exe
} finally {
    Pop-Location
}
Write-Host "  OK: Control Center" -ForegroundColor Green

Write-Host "Build terminado." -ForegroundColor Cyan
Write-Host "  agent\build\lan-agent.exe"
Write-Host "  agent\build\lan-agent-linux"
Write-Host "  installers\windows\lan-agent.exe"
Write-Host "  installers\linux\lan-agent-linux"
Write-Host "  control-center\build\bin\lan-commander.exe"