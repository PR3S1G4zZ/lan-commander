# LAN Commander - Build All

$ErrorActionPreference = "Stop"
$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$agentDir = Join-Path $repoRoot "agent"
$agentUiDir = Join-Path $repoRoot "agent-ui"
$controlCenterDir = Join-Path $repoRoot "control-center"

Write-Host "LAN Commander - Build All" -ForegroundColor Cyan

Push-Location $agentDir
try {
    Write-Host "[1/5] Compilando agente Windows..." -ForegroundColor Yellow
    go build -ldflags="-s -w" -o build\lan-agent.exe .\cmd\lan-agent
    Copy-Item build\lan-agent.exe (Join-Path $repoRoot "installers\windows\lan-agent.exe") -Force
    Write-Host "  OK" -ForegroundColor Green

    Write-Host "[2/5] Compilando agente Linux amd64..." -ForegroundColor Yellow
    $oldGoOS = $env:GOOS; $oldGoARCH = $env:GOARCH
    try {
        $env:GOOS = "linux"; $env:GOARCH = "amd64"
        go build -ldflags="-s -w" -o build\lan-agent-linux .\cmd\lan-agent
    } finally { $env:GOOS = $oldGoOS; $env:GOARCH = $oldGoARCH }
    Copy-Item build\lan-agent-linux (Join-Path $repoRoot "installers\linux\lan-agent-linux") -Force
    Write-Host "  OK" -ForegroundColor Green
} finally { Pop-Location }

Write-Host "[3/5] Compilando interfaz de escritorio Windows..." -ForegroundColor Yellow
Push-Location $agentUiDir
try {
    Push-Location frontend
    try {
        if (-not (Test-Path node_modules)) { npm.cmd install }
        npm.cmd run build
    } finally { Pop-Location }
    $env:GOCACHE = Join-Path $repoRoot ".codex-cache\go-agent-ui"
    go build -ldflags="-s -w" -o build\bin\lan-agent-ui.exe .
    Copy-Item build\bin\lan-agent-ui.exe (Join-Path $repoRoot "installers\windows\lan-agent-ui.exe") -Force
    Write-Host "  OK" -ForegroundColor Green
} finally { Pop-Location }

if ($env:OS -ne "Windows_NT") {
    Write-Host "[4/5] Compilando interfaz de escritorio Linux amd64..." -ForegroundColor Yellow
    Push-Location $agentUiDir
    try {
        Push-Location frontend
        try {
            if (-not (Test-Path node_modules)) { npm.cmd install }
            npm.cmd run build
        } finally { Pop-Location }
        $env:GOCACHE = Join-Path $repoRoot ".codex-cache\go-agent-ui"
        $env:GOOS = "linux"; $env:GOARCH = "amd64"
        go build -ldflags="-s -w" -o build\bin\lan-agent-ui .
        Copy-Item build\bin\lan-agent-ui (Join-Path $repoRoot "installers\linux\lan-agent-ui") -Force
    } finally { Pop-Location }
} else { Write-Host "[4/5] UI Linux: compilar en un host Linux (se omite en Windows)." -ForegroundColor DarkYellow }

Write-Host "[5/5] Compilando Control Center..." -ForegroundColor Yellow
Push-Location $controlCenterDir
try {
    if (-not (Test-Path (Join-Path $controlCenterDir "frontend\node_modules"))) { npm.cmd --prefix frontend install }
    npm.cmd --prefix frontend run build
    $env:GOCACHE = Join-Path $repoRoot ".codex-cache\go-control"
    go build -ldflags="-s -w" -o build\bin\lan-commander.exe .
} finally { Pop-Location }
Write-Host "Build terminado." -ForegroundColor Cyan