# LAN Commander - Build Agent and desktop UI

param(
    [ValidateSet("windows", "linux", "all")]
    [string]$Target = "all"
)

$ErrorActionPreference = "Stop"
$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$agentDir = Join-Path $repoRoot "agent"
$agentUiDir = Join-Path $repoRoot "agent-ui"

$windowsBuildOutput = Join-Path $agentDir "build\lan-agent.exe"
$linuxBuildOutput = Join-Path $agentDir "build\lan-agent-linux"
$windowsPayload = Join-Path $repoRoot "installers\windows\lan-agent.exe"
$linuxPayload = Join-Path $repoRoot "installers\linux\lan-agent-linux"
$uiWindowsBuildOutput = Join-Path $agentUiDir "build\bin\lan-agent-ui.exe"
$uiLinuxBuildOutput = Join-Path $agentUiDir "build\bin\lan-agent-ui"
$uiWindowsPayload = Join-Path $repoRoot "installers\windows\lan-agent-ui.exe"
$uiLinuxPayload = Join-Path $repoRoot "installers\linux\lan-agent-ui"
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
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "Build succeeded but expected output was not created: $Path"
    }
}

function Add-ReleaseArtifact {
    param([Parameter(Mandatory = $true)][string]$Path)
    Assert-BuildOutput -Path $Path
    [void]$releaseArtifacts.Add($Path)
}

function Build-Ui {
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
            if (-not (Test-Path -LiteralPath node_modules -PathType Container)) {
                Invoke-RequiredNativeCommand -FilePath "npm.cmd" -ArgumentList @("install")
            }
            Invoke-RequiredNativeCommand -FilePath "npm.cmd" -ArgumentList @("run", "build")
        } finally { Pop-Location }

        $previousGoCache = $env:GOCACHE
        $previousGoOS = $env:GOOS
        $previousGoARCH = $env:GOARCH
        try {
            $env:GOCACHE = Join-Path $repoRoot ".codex-cache\go-agent-ui"
            if ($ForLinux) { $env:GOOS = "linux"; $env:GOARCH = "amd64" }
            Invoke-RequiredNativeCommand -FilePath "go" -ArgumentList @(
                "build", "-ldflags=-s -w", "-o", $OutputPath, "."
            )
        } finally {
            if ($null -eq $previousGoCache) { Remove-Item Env:GOCACHE -ErrorAction SilentlyContinue } else { $env:GOCACHE = $previousGoCache }
            if ($null -eq $previousGoOS) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue } else { $env:GOOS = $previousGoOS }
            if ($null -eq $previousGoARCH) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue } else { $env:GOARCH = $previousGoARCH }
        }
        Add-ReleaseArtifact -Path $OutputPath
        Copy-Item -LiteralPath $OutputPath -Destination $PayloadPath -Force
        Add-ReleaseArtifact -Path $PayloadPath
    } finally { Pop-Location }
}

function Build-AgentWindows {
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $windowsBuildOutput) | Out-Null
    Push-Location $agentDir
    try {
        Invoke-RequiredNativeCommand -FilePath "go" -ArgumentList @("build", "-ldflags=-s -w", "-o", $windowsBuildOutput, ".\cmd\lan-agent")
    } finally { Pop-Location }
    Add-ReleaseArtifact -Path $windowsBuildOutput
    Copy-Item -LiteralPath $windowsBuildOutput -Destination $windowsPayload -Force
    Add-ReleaseArtifact -Path $windowsPayload
}

function Build-AgentLinux {
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $linuxBuildOutput) | Out-Null
    Push-Location $agentDir
    try {
        $previousGoOS = $env:GOOS
        $previousGoARCH = $env:GOARCH
        try {
            $env:GOOS = "linux"; $env:GOARCH = "amd64"
            Invoke-RequiredNativeCommand -FilePath "go" -ArgumentList @("build", "-ldflags=-s -w", "-o", $linuxBuildOutput, ".\cmd\lan-agent")
        } finally {
            if ($null -eq $previousGoOS) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue } else { $env:GOOS = $previousGoOS }
            if ($null -eq $previousGoARCH) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue } else { $env:GOARCH = $previousGoARCH }
        }
    } finally { Pop-Location }
    Add-ReleaseArtifact -Path $linuxBuildOutput
    Copy-Item -LiteralPath $linuxBuildOutput -Destination $linuxPayload -Force
    Add-ReleaseArtifact -Path $linuxPayload
}

Remove-OutputFile -Path $releaseManifestPath
if ($Target -eq "windows" -or $Target -eq "all") {
    Remove-OutputFile -Path $windowsBuildOutput
    Remove-OutputFile -Path $windowsPayload
    Remove-OutputFile -Path $uiWindowsBuildOutput
    Remove-OutputFile -Path $uiWindowsPayload

    Write-Host "Compilando agente Windows..." -ForegroundColor Yellow
    Build-AgentWindows
    Write-Host "Compilando interfaz de escritorio Windows..." -ForegroundColor Yellow
    Build-Ui -OutputPath $uiWindowsBuildOutput -PayloadPath $uiWindowsPayload -ForLinux $false
}

if ($Target -eq "linux" -or $Target -eq "all") {
    Remove-OutputFile -Path $linuxBuildOutput
    Remove-OutputFile -Path $linuxPayload
    Remove-OutputFile -Path $uiLinuxBuildOutput
    Remove-OutputFile -Path $uiLinuxPayload

    Write-Host "Compilando agente Linux amd64..." -ForegroundColor Yellow
    Build-AgentLinux
    if ($env:OS -eq "Windows_NT") {
        Write-Host "La interfaz Linux debe compilarse en un host Linux; se omite en Windows." -ForegroundColor DarkYellow
    } else {
        Write-Host "Compilando interfaz de escritorio Linux amd64..." -ForegroundColor Yellow
        Build-Ui -OutputPath $uiLinuxBuildOutput -PayloadPath $uiLinuxPayload -ForLinux $true
    }
}

if ($releaseArtifacts.Count -eq 0) { throw "No release artifacts were built for target '$Target'." }
if (-not (Test-Path -LiteralPath $manifestGenerator -PathType Leaf)) { throw "Release manifest generator was not found: $manifestGenerator" }
Write-Host "Generando manifiesto SHA-256..." -ForegroundColor Yellow
& $manifestGenerator -ManifestPath $releaseManifestPath -ArtifactPath $releaseArtifacts.ToArray()
if (-not $?) { throw "Release manifest generation failed: $releaseManifestPath" }
if (-not (Test-Path -LiteralPath $manifestVerifier -PathType Leaf)) { throw "Release manifest verifier was not found: $manifestVerifier" }
& $manifestVerifier -ManifestPath $releaseManifestPath
if (-not $?) { throw "Release manifest verification failed: $releaseManifestPath" }
Write-Host "Build del agente terminado." -ForegroundColor Cyan
