# LAN Commander - Build Agent

param(
    [ValidateSet("windows", "linux", "all")]
    [string]$Target = "all"
)

$ErrorActionPreference = "Stop"
$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$agentDir = Join-Path $repoRoot "agent"

$windowsBuildOutput = Join-Path $agentDir "build\lan-agent.exe"
$linuxBuildOutput = Join-Path $agentDir "build\lan-agent-linux"
$windowsPayload = Join-Path $repoRoot "installers\windows\lan-agent.exe"
$linuxPayload = Join-Path $repoRoot "installers\linux\lan-agent-linux"
$releaseManifestPath = Join-Path $repoRoot "release-manifest.sha256"
$manifestGenerator = Join-Path $repoRoot "scripts\generate-release-manifest.ps1"
$manifestVerifier = Join-Path $repoRoot "scripts\verify-release-manifest.ps1"
$releaseArtifacts = New-Object 'System.Collections.Generic.List[string]'

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

Push-Location $agentDir
try {
    Remove-OutputFile -Path $releaseManifestPath

    if ($Target -eq "windows" -or $Target -eq "all") {
        Remove-OutputFile -Path $windowsBuildOutput
        Remove-OutputFile -Path $windowsPayload

        Write-Host "Compilando agente Windows..." -ForegroundColor Yellow
        Invoke-RequiredNativeCommand -FilePath "go" -ArgumentList @(
            "build",
            "-ldflags=-s -w",
            "-o",
            $windowsBuildOutput,
            (Join-Path $agentDir "cmd\lan-agent")
        )
        Assert-BuildOutput -Path $windowsBuildOutput
        Copy-Item -LiteralPath $windowsBuildOutput -Destination $windowsPayload -Force
        $releaseArtifacts.Add($windowsBuildOutput)
        $releaseArtifacts.Add($windowsPayload)
        Write-Host "OK: Windows" -ForegroundColor Green
    }

    if ($Target -eq "linux" -or $Target -eq "all") {
        Remove-OutputFile -Path $linuxBuildOutput
        Remove-OutputFile -Path $linuxPayload

        Write-Host "Compilando agente Linux amd64..." -ForegroundColor Yellow
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
        $releaseArtifacts.Add($linuxBuildOutput)
        $releaseArtifacts.Add($linuxPayload)
        Write-Host "OK: Linux" -ForegroundColor Green
    }
} finally {
    Pop-Location
}

if ($releaseArtifacts.Count -eq 0) {
    throw "No release artifacts were built for target '$Target'."
}
if (-not (Test-Path -LiteralPath $manifestGenerator -PathType Leaf)) {
    throw "Release manifest generator was not found: $manifestGenerator"
}
Write-Host "Generando manifiesto SHA-256..." -ForegroundColor Yellow
& $manifestGenerator -ManifestPath $releaseManifestPath -ArtifactPath $releaseArtifacts.ToArray()
if (-not $?) {
    throw "Release manifest generation failed: $releaseManifestPath"
}
Write-Host "OK: release-manifest.sha256" -ForegroundColor Green
if (-not (Test-Path -LiteralPath $manifestVerifier -PathType Leaf)) {
    throw "Release manifest verifier was not found: $manifestVerifier"
}
& $manifestVerifier -ManifestPath $releaseManifestPath
if (-not $?) {
    throw "Release manifest verification failed: $releaseManifestPath"
}
Write-Host "OK: release-manifest.sha256 verificado" -ForegroundColor Green

Write-Host "Build del agente terminado." -ForegroundColor Cyan
