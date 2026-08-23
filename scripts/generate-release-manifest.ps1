# LAN Commander - Generate Release Manifest
# Genera un manifiesto SHA-256 determinista para artefactos de release.

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateNotNullOrEmpty()]
    [string]$ManifestPath,

    [Parameter(Mandatory = $false)]
    [ValidateNotNullOrEmpty()]
    [string]$SigningCommand,

    [Parameter(Mandatory = $true)]
    [ValidateNotNullOrEmpty()]
    [string[]]$ArtifactPath
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($ManifestPath)) {
    throw "ManifestPath cannot be empty or whitespace."
}
if ($null -eq $ArtifactPath -or $ArtifactPath.Count -eq 0) {
    throw "At least one ArtifactPath is required."
}

function Resolve-ManifestOutputPath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    try {
        $fullPath = [System.IO.Path]::GetFullPath($Path)
    } catch {
        throw "Manifest path is invalid: $Path"
    }

    $parent = Split-Path -Parent $fullPath
    if ([string]::IsNullOrWhiteSpace($parent) -or -not (Test-Path -LiteralPath $parent -PathType Container)) {
        throw "Manifest parent directory does not exist: $parent"
    }

    if (Test-Path -LiteralPath $fullPath -PathType Container) {
        throw "Manifest path points to a directory, not a file: $fullPath"
    }

    return $fullPath
}

function Get-RelativeArtifactPath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ManifestDirectory,

        [Parameter(Mandatory = $true)]
        [string]$ArtifactFullPath
    )

    $rootWithSeparator = $ManifestDirectory
    if (-not $rootWithSeparator.EndsWith([string][System.IO.Path]::DirectorySeparatorChar)) {
        $rootWithSeparator += [System.IO.Path]::DirectorySeparatorChar
    }

    $rootUri = [System.Uri]::new($rootWithSeparator)
    $artifactUri = [System.Uri]::new($ArtifactFullPath)
    if (-not $rootUri.IsBaseOf($artifactUri)) {
        throw "Artifact must be inside the manifest directory so its relative path is portable: $ArtifactFullPath"
    }

    $relativePath = [System.Uri]::UnescapeDataString($rootUri.MakeRelativeUri($artifactUri).ToString())
    $relativePath = $relativePath.Replace('\', '/')
    if ([string]::IsNullOrWhiteSpace($relativePath) -or $relativePath -match '(^|/)\.\.(/|$)') {
        throw "Artifact path does not produce a safe relative manifest path: $ArtifactFullPath"
    }

    return $relativePath
}

$manifestFullPath = Resolve-ManifestOutputPath -Path $ManifestPath
$manifestDirectory = Split-Path -Parent $manifestFullPath
$entries = New-Object 'System.Collections.Generic.List[object]'
$seenPaths = @{}

foreach ($artifact in $ArtifactPath) {
    if ([string]::IsNullOrWhiteSpace($artifact)) {
        throw "ArtifactPath cannot contain an empty path."
    }

    if (-not (Test-Path -LiteralPath $artifact -PathType Leaf)) {
        throw "Artifact was not found or is not a file: $artifact"
    }

    $artifactFullPath = (Resolve-Path -LiteralPath $artifact).Path
    if ([string]::Equals($artifactFullPath, $manifestFullPath, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "The manifest cannot also be an artifact: $artifactFullPath"
    }

    $relativePath = Get-RelativeArtifactPath -ManifestDirectory $manifestDirectory -ArtifactFullPath $artifactFullPath
    $pathKey = $relativePath.ToLowerInvariant()
    if ($seenPaths.ContainsKey($pathKey)) {
        throw "Duplicate artifact path after normalization: $relativePath"
    }
    $seenPaths[$pathKey] = $true

    $hash = (Get-FileHash -LiteralPath $artifactFullPath -Algorithm SHA256).Hash.ToLowerInvariant()
    $entries.Add([pscustomobject]@{
        RelativePath = $relativePath
        Hash         = $hash
    })
}

$entries.Sort([System.Comparison[object]]{
    param($left, $right)
    return [string]::Compare($left.RelativePath, $right.RelativePath, [System.StringComparison]::Ordinal)
})

$manifestLines = foreach ($entry in $entries) {
    "{0}  {1}" -f $entry.Hash, $entry.RelativePath
}
$manifestText = ($manifestLines -join "`n") + "`n"
[System.IO.File]::WriteAllText(
    $manifestFullPath,
    $manifestText,
    [System.Text.UTF8Encoding]::new($false)
)

Write-Host ("Generated deterministic SHA-256 manifest: {0} ({1} artifact(s))" -f $manifestFullPath, $entries.Count) -ForegroundColor Green

if ([string]::IsNullOrWhiteSpace($SigningCommand)) {
    Write-Warning "Manifest is unsigned. Use -SigningCommand with externally managed signing material, and verify its public trust material separately."
} else {
    Write-Host ("Running external signing command: {0}" -f $SigningCommand) -ForegroundColor Yellow
    & $SigningCommand $manifestFullPath
    if (-not $?) {
        throw "External signing command failed: $SigningCommand"
    }
    Write-Host "External signing command completed. Signature trust remains external to this repository." -ForegroundColor Green
}
