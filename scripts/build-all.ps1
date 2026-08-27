# LAN Commander - Build All
# Compila los agentes y el Control Center.

$ErrorActionPreference = "Stop"
$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$agentDir = Join-Path $repoRoot "agent"
$controlCenterDir = Join-Path $repoRoot "control-center"

$windowsBuildOutput = Join-Path $agentDir "build\lan-agent.exe"
$linuxBuildOutput = Join-Path $agentDir "build\lan-agent-linux"
$windowsPayload = Join-Path $repoRoot "installers\windows\lan-agent.exe"
$linuxPayload = Join-Path $repoRoot "installers\linux\lan-agent-linux"
$controlCenterOutput = Join-Path $controlCenterDir "build\bin\lan-commander.exe"
$releaseManifestPath = Join-Path $repoRoot "release-manifest.sha256"
$manifestGenerator = Join-Path $repoRoot "scripts\generate-release-manifest.ps1"
$manifestVerifier = Join-Path $repoRoot "scripts\verify-release-manifest.ps1"

function Remove-OutputFile {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    if (Test-Path -LiteralPath $Path) {
        Remove-Item -LiteralPath $Path -Force
    }
}

function Invoke-RequiredNativeCommand {
    param(
        [Parameter(Mandatory = $true)]
        [string]$FilePath,

        [Parameter(Mandatory = $true)]
        [string[]]$ArgumentList
    )

    & $FilePath @ArgumentList
    $exitCode = $LASTEXITCODE
    if ($exitCode -ne 0) {
        throw "$FilePath failed with exit code $exitCode."
    }
}

function Assert-BuildOutput {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "Build succeeded but expected output was not created: $Path"
    }
}

Write-Host "LAN Commander - Build All" -ForegroundColor Cyan

# Elimina artefactos anteriores antes de iniciar cualquier compilacion. Asi,
# un fallo no puede dejar ni publicar un binario de una ejecucion anterior.
Remove-OutputFile -Path $windowsBuildOutput
Remove-OutputFile -Path $linuxBuildOutput
Remove-OutputFile -Path $windowsPayload
Remove-OutputFile -Path $linuxPayload
Remove-OutputFile -Path $controlCenterOutput
Remove-OutputFile -Path $releaseManifestPath

Push-Location $agentDir
try {
    Write-Host "[1/3] Compilando agente Windows..." -ForegroundColor Yellow
    Invoke-RequiredNativeCommand -FilePath "go" -ArgumentList @(
        "build",
        "-ldflags=-s -w",
        "-o",
        $windowsBuildOutput,
        (Join-Path $agentDir "cmd\lan-agent")
    )
    Assert-BuildOutput -Path $windowsBuildOutput
    Copy-Item -LiteralPath $windowsBuildOutput -Destination $windowsPayload -Force
    Write-Host "  OK: agente Windows y payload del instalador" -ForegroundColor Green

    Write-Host "[2/3] Compilando agente Linux amd64..." -ForegroundColor Yellow
    $previousGoOS = $env:GOOS
    $previousGoARCH = $env:GOARCH
    try {
        $env:GOOS = "linux"
        $env:GOARCH = "amd64"
        Invoke-RequiredNativeCommand -FilePath "go" -ArgumentList @(
            "build",
            "-ldflags=-s -w",
            "-o",
            $linuxBuildOutput,
            (Join-Path $agentDir "cmd\lan-agent")
        )
    } finally {
        if ($null -eq $previousGoOS) {
            Remove-Item Env:GOOS -ErrorAction SilentlyContinue
        } else {
            $env:GOOS = $previousGoOS
        }
        if ($null -eq $previousGoARCH) {
            Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
        } else {
            $env:GOARCH = $previousGoARCH
        }
    }
    Assert-BuildOutput -Path $linuxBuildOutput
    Copy-Item -LiteralPath $linuxBuildOutput -Destination $linuxPayload -Force
    Write-Host "  OK: agente Linux y payload del instalador" -ForegroundColor Green
} finally {
    Pop-Location
}

Write-Host "[3/3] Compilando Control Center..." -ForegroundColor Yellow
$wails = Join-Path $env:USERPROFILE "go\bin\wails.exe"
if (-not (Test-Path -LiteralPath $wails -PathType Leaf)) {
    $wails = "wails"
}
Push-Location $controlCenterDir
try {
    Invoke-RequiredNativeCommand -FilePath $wails -ArgumentList @(
        "build",
        "-clean",
        "-f",
        "-platform",
        "windows/amd64",
        "-o",
        "lan-commander.exe"
    )
    Assert-BuildOutput -Path $controlCenterOutput
} finally {
    Pop-Location
}
Write-Host "  OK: Control Center" -ForegroundColor Green

Write-Host "[4/4] Generando manifiesto SHA-256..." -ForegroundColor Yellow
if (-not (Test-Path -LiteralPath $manifestGenerator -PathType Leaf)) {
    throw "Release manifest generator was not found: $manifestGenerator"
}
& $manifestGenerator -ManifestPath $releaseManifestPath -ArtifactPath @(
    $windowsBuildOutput,
    $linuxBuildOutput,
    $windowsPayload,
    $linuxPayload,
    $controlCenterOutput
)
if (-not $?) {
    throw "Release manifest generation failed: $releaseManifestPath"
}
Write-Host "  OK: release-manifest.sha256" -ForegroundColor Green
if (-not (Test-Path -LiteralPath $manifestVerifier -PathType Leaf)) {
    throw "Release manifest verifier was not found: $manifestVerifier"
}
& $manifestVerifier -ManifestPath $releaseManifestPath
if (-not $?) {
    throw "Release manifest verification failed: $releaseManifestPath"
}
Write-Host "  OK: release-manifest.sha256 verificado" -ForegroundColor Green

Write-Host "Build terminado." -ForegroundColor Cyan
Write-Host "  agent\build\lan-agent.exe"
Write-Host "  agent\build\lan-agent-linux"
Write-Host "  installers\windows\lan-agent.exe"
Write-Host "  installers\linux\lan-agent-linux"
Write-Host "  control-center\build\bin\lan-commander.exe"
Write-Host "  release-manifest.sha256"
Write-Host "PENDIENTE DE RELEASE: firmar el manifiesto con material externo confiable y verificar esa firma." -ForegroundColor Yellow
