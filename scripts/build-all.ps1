# LAN Commander - Build All
# Compila el agente, su interfaz visual y el Control Center.

$ErrorActionPreference = "Stop"
$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$agentDir = Join-Path $repoRoot "agent"
$agentUiDir = Join-Path $repoRoot "agent-ui"
$controlCenterDir = Join-Path $repoRoot "control-center"

$windowsBuildOutput = Join-Path $agentDir "build\lan-agent.exe"
$linuxBuildOutput = Join-Path $agentDir "build\lan-agent-linux"
$windowsPayload = Join-Path $repoRoot "installers\windows\lan-agent.exe"
$linuxPayload = Join-Path $repoRoot "installers\linux\lan-agent-linux"
$uiWindowsBuildOutput = Join-Path $agentUiDir "build\bin\lan-agent-ui.exe"
$uiLinuxBuildOutput = Join-Path $agentUiDir "build\bin\lan-agent-ui"
$uiWindowsPayload = Join-Path $repoRoot "installers\windows\lan-agent-ui.exe"
$uiLinuxPayload = Join-Path $repoRoot "installers\linux\lan-agent-ui"
$controlCenterOutput = Join-Path $controlCenterDir "build\bin\lan-commander.exe"
$releaseManifestPath = Join-Path $repoRoot "release-manifest.sha256"
$manifestGenerator = Join-Path $repoRoot "scripts\generate-release-manifest.ps1"
$manifestVerifier = Join-Path $repoRoot "scripts\verify-release-manifest.ps1"
$releaseArtifacts = New-Object 'System.Collections.Generic.List[string]'

function Remove-OutputFile {
    param([Parameter(Mandatory = $true)][string]$Path)
    if (Test-Path -LiteralPath $Path) { Remove-Item -LiteralPath $Path -Force }
}

function Invoke-RequiredNativeCommand {
    param(
        [Parameter(Mandatory = $true)][string]$FilePath,
        [Parameter(Mandatory = $true)][string[]]$ArgumentList
    )
    & $FilePath @ArgumentList
    if ($LASTEXITCODE -ne 0) { throw "$FilePath failed with exit code $LASTEXITCODE." }
}

function Assert-BuildOutput {
    param([Parameter(Mandatory = $true)][string]$Path)
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { throw "Expected build output was not created: $Path" }
}

function Add-ReleaseArtifact {
    param([Parameter(Mandatory = $true)][string]$Path)
    Assert-BuildOutput -Path $Path
    [void]$releaseArtifacts.Add($Path)
}

function Restore-TargetEnvironment {
    param(
        [AllowNull()][string]$GoOS,
        [AllowNull()][string]$GoARCH,
        [AllowNull()][string]$GoCache
    )
    if ($null -eq $GoOS) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue } else { $env:GOOS = $GoOS }
    if ($null -eq $GoARCH) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue } else { $env:GOARCH = $GoARCH }
    if ($null -eq $GoCache) { Remove-Item Env:GOCACHE -ErrorAction SilentlyContinue } else { $env:GOCACHE = $GoCache }
}

function Build-AgentWindows {
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $windowsBuildOutput) | Out-Null
    Push-Location $agentDir
    try { Invoke-RequiredNativeCommand -FilePath "go" -ArgumentList @("build", "-ldflags=-s -w", "-o", $windowsBuildOutput, ".\cmd\lan-agent") }
    finally { Pop-Location }
    Add-ReleaseArtifact -Path $windowsBuildOutput
    Copy-Item -LiteralPath $windowsBuildOutput -Destination $windowsPayload -Force
    Add-ReleaseArtifact -Path $windowsPayload
}

function Build-AgentLinux {
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $linuxBuildOutput) | Out-Null
    $oldGoOS = $env:GOOS; $oldGoARCH = $env:GOARCH
    try {
        $env:GOOS = "linux"; $env:GOARCH = "amd64"
        Push-Location $agentDir
        try { Invoke-RequiredNativeCommand -FilePath "go" -ArgumentList @("build", "-ldflags=-s -w", "-o", $linuxBuildOutput, ".\cmd\lan-agent") }
        finally { Pop-Location }
    } finally { Restore-TargetEnvironment -GoOS $oldGoOS -GoARCH $oldGoARCH -GoCache $env:GOCACHE }
    Add-ReleaseArtifact -Path $linuxBuildOutput
    Copy-Item -LiteralPath $linuxBuildOutput -Destination $linuxPayload -Force
    Add-ReleaseArtifact -Path $linuxPayload
}

function Build-AgentUi {
    param(
        [Parameter(Mandatory = $true)][string]$OutputPath,
        [Parameter(Mandatory = $true)][string]$PayloadPath,
        [Parameter(Mandatory = $true)][bool]$ForLinux
    )
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $OutputPath) | Out-Null
    Push-Location $agentUiDir
    try {
        Push-Location frontend
        try {
            if (-not (Test-Path -LiteralPath node_modules -PathType Container)) { Invoke-RequiredNativeCommand -FilePath "npm.cmd" -ArgumentList @("install") }
            Invoke-RequiredNativeCommand -FilePath "npm.cmd" -ArgumentList @("run", "build")
        } finally { Pop-Location }

        $oldGoOS = $env:GOOS; $oldGoARCH = $env:GOARCH; $oldGoCache = $env:GOCACHE
        try {
            $env:GOCACHE = Join-Path $repoRoot ".codex-cache\go-agent-ui"
            if ($ForLinux) { $env:GOOS = "linux"; $env:GOARCH = "amd64" }
            Invoke-RequiredNativeCommand -FilePath "go" -ArgumentList @("build", "-ldflags=-s -w", "-o", $OutputPath, ".")
        } finally { Restore-TargetEnvironment -GoOS $oldGoOS -GoARCH $oldGoARCH -GoCache $oldGoCache }
    } finally { Pop-Location }
    Add-ReleaseArtifact -Path $OutputPath
    Copy-Item -LiteralPath $OutputPath -Destination $PayloadPath -Force
    Add-ReleaseArtifact -Path $PayloadPath
}

Write-Host "LAN Commander - Build All" -ForegroundColor Cyan
foreach ($path in @($windowsBuildOutput, $linuxBuildOutput, $windowsPayload, $linuxPayload, $uiWindowsBuildOutput, $uiLinuxBuildOutput, $uiWindowsPayload, $uiLinuxPayload, $controlCenterOutput, $releaseManifestPath)) {
    Remove-OutputFile -Path $path
}

Write-Host "[1/5] Compilando agente Windows..." -ForegroundColor Yellow
Build-AgentWindows
Write-Host "[2/5] Compilando agente Linux amd64..." -ForegroundColor Yellow
Build-AgentLinux
Write-Host "[3/5] Compilando interfaz de escritorio Windows..." -ForegroundColor Yellow
Build-AgentUi -OutputPath $uiWindowsBuildOutput -PayloadPath $uiWindowsPayload -ForLinux $false

if ($env:OS -eq "Windows_NT") {
    Write-Host "[4/5] UI Linux: compilar en un host Linux (se omite en Windows)." -ForegroundColor DarkYellow
} else {
    Write-Host "[4/5] Compilando interfaz de escritorio Linux amd64..." -ForegroundColor Yellow
    Build-AgentUi -OutputPath $uiLinuxBuildOutput -PayloadPath $uiLinuxPayload -ForLinux $true
}

Write-Host "[5/5] Compilando Control Center..." -ForegroundColor Yellow
$wails = Join-Path $env:USERPROFILE "go\bin\wails.exe"
if (-not (Test-Path -LiteralPath $wails -PathType Leaf)) { $wails = "wails" }
Push-Location $controlCenterDir
try {
    Invoke-RequiredNativeCommand -FilePath $wails -ArgumentList @("build", "-clean", "-f", "-platform", "windows/amd64", "-o", "lan-commander.exe")
} finally { Pop-Location }
Add-ReleaseArtifact -Path $controlCenterOutput

if (-not (Test-Path -LiteralPath $manifestGenerator -PathType Leaf)) { throw "Release manifest generator was not found: $manifestGenerator" }
Write-Host "Generando manifiesto SHA-256..." -ForegroundColor Yellow
& $manifestGenerator -ManifestPath $releaseManifestPath -ArtifactPath $releaseArtifacts.ToArray()
if (-not $?) { throw "Release manifest generation failed: $releaseManifestPath" }
if (-not (Test-Path -LiteralPath $manifestVerifier -PathType Leaf)) { throw "Release manifest verifier was not found: $manifestVerifier" }
& $manifestVerifier -ManifestPath $releaseManifestPath
if (-not $?) { throw "Release manifest verification failed: $releaseManifestPath" }
Write-Host "Build terminado y manifiesto verificado." -ForegroundColor Cyan
