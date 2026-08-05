# LAN Commander - Build Agent

param(
    [ValidateSet("windows", "linux", "all")]
    [string]$Target = "all"
)

$ErrorActionPreference = "Stop"
$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$agentDir = Join-Path $repoRoot "agent"

Push-Location $agentDir
try {
    if ($Target -eq "windows" -or $Target -eq "all") {
        Write-Host "Compilando agente Windows..." -ForegroundColor Yellow
        go build -ldflags="-s -w" -o build\lan-agent.exe .\cmd\lan-agent
        Copy-Item build\lan-agent.exe ..\installers\windows\lan-agent.exe -Force
        Write-Host "OK: Windows" -ForegroundColor Green
    }

    if ($Target -eq "linux" -or $Target -eq "all") {
        Write-Host "Compilando agente Linux amd64..." -ForegroundColor Yellow
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
        Write-Host "OK: Linux" -ForegroundColor Green
    }
} finally {
    Pop-Location
}

Write-Host "Build del agente terminado." -ForegroundColor Cyan