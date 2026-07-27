# LAN Commander - Build Agent (cross-platform)

param(
    [string]$Target = "all" # windows, linux, or all
)

Write-Host "LAN Commander - Building Agent" -ForegroundColor Cyan

Set-Location -LiteralPath "$PSScriptRoot\..\agent"

if ($Target -eq "windows" -or $Target -eq "all") {
    Write-Host "`nBuilding for Windows..." -ForegroundColor Yellow
    go build -ldflags="-s -w" -o build\lan-agent.exe .\cmd\lan-agent\
    if ($?) { Write-Host "  ✅ lan-agent.exe (Windows)" -ForegroundColor Green }
}

if ($Target -eq "linux" -or $Target -eq "all") {
    Write-Host "`nBuilding for Linux..." -ForegroundColor Yellow
    $env:GOOS="linux"; $env:GOARCH="amd64"
    go build -ldflags="-s -w" -o build\lan-agent-linux .\cmd\lan-agent\
    if ($?) { Write-Host "  ✅ lan-agent-linux (Linux)" -ForegroundColor Green }
    $env:GOOS=""; $env:GOARCH=""
}

Write-Host "`nDone!" -ForegroundColor Cyan
