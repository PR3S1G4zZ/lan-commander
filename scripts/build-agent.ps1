# LAN Commander - Build Agent and desktop UI

param(
    [ValidateSet("windows", "linux", "all")]
    [string]$Target = "all"
)

$ErrorActionPreference = "Stop"
$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$agentDir = Join-Path $repoRoot "agent"
$agentUiDir = Join-Path $repoRoot "agent-ui"

function Build-AgentWindows {
    Push-Location $agentDir
    try {
        go build -ldflags="-s -w" -o build\lan-agent.exe .\cmd\lan-agent
        Copy-Item build\lan-agent.exe (Join-Path $repoRoot "installers\windows\lan-agent.exe") -Force
    } finally { Pop-Location }
}

function Build-AgentLinux {
    Push-Location $agentDir
    try {
        $oldGoOS = $env:GOOS; $oldGoARCH = $env:GOARCH
        try {
            $env:GOOS = "linux"; $env:GOARCH = "amd64"
            go build -ldflags="-s -w" -o build\lan-agent-linux .\cmd\lan-agent
        } finally { $env:GOOS = $oldGoOS; $env:GOARCH = $oldGoARCH }
        Copy-Item build\lan-agent-linux (Join-Path $repoRoot "installers\linux\lan-agent-linux") -Force
    } finally { Pop-Location }
}

function Build-UiWindows {
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
    } finally { Pop-Location }
}

function Build-UiLinux {
    if ($env:OS -eq "Windows_NT") { Write-Warning "La UI Linux debe compilarse en Linux; se omite en Windows."; return }
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
}

if ($Target -eq "windows" -or $Target -eq "all") {
    Write-Host "Compilando agente Windows..." -ForegroundColor Yellow
    Build-AgentWindows
    Write-Host "Compilando interfaz de escritorio Windows..." -ForegroundColor Yellow
    Build-UiWindows
    Write-Host "OK: agente e interfaz Windows" -ForegroundColor Green
}

if ($Target -eq "linux" -or $Target -eq "all") {
    Write-Host "Compilando agente Linux amd64..." -ForegroundColor Yellow
    Build-AgentLinux
    Write-Host "Compilando interfaz de escritorio Linux amd64..." -ForegroundColor Yellow
    Build-UiLinux
    Write-Host "OK: agente e interfaz Linux" -ForegroundColor Green
}

Write-Host "Build del agente terminado." -ForegroundColor Green