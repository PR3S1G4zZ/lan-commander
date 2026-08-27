# LAN Commander - Verify Release Manifest
# Verifica hashes SHA-256 y detecta archivos faltantes o modificados.

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateNotNullOrEmpty()]
    [string]$ManifestPath
)

$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($ManifestPath)) {
    throw "ManifestPath cannot be empty or whitespace."
}

function Resolve-ExistingManifestPath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "Manifest was not found or is not a file: $Path"
    }

    try {
        return (Resolve-Path -LiteralPath $Path).Path
    } catch {
        throw "Manifest path is invalid: $Path"
    }
}

function Resolve-ManifestArtifactPath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$ManifestDirectory,

        [Parameter(Mandatory = $true)]
        [string]$RelativePath
    )

    if ($RelativePath.Contains('\')) {
        throw "Manifest entry must use '/' as the path separator: $RelativePath"
    }
    if ([System.IO.Path]::IsPathRooted($RelativePath) -or $RelativePath -match '(^|/)\.\.(/|$)') {
        throw "Manifest entry path must be a safe relative path: $RelativePath"
    }

    $filePath = $RelativePath.Replace('/', [System.IO.Path]::DirectorySeparatorChar)
    $candidate = [System.IO.Path]::GetFullPath((Join-Path $ManifestDirectory $filePath))

    $rootWithSeparator = $ManifestDirectory
    if (-not $rootWithSeparator.EndsWith([string][System.IO.Path]::DirectorySeparatorChar)) {
        $rootWithSeparator += [System.IO.Path]::DirectorySeparatorChar
    }

    $rootUri = [System.Uri]::new($rootWithSeparator)
    $candidateUri = [System.Uri]::new($candidate)
    if (-not $rootUri.IsBaseOf($candidateUri)) {
        throw "Manifest entry resolves outside the manifest directory: $RelativePath"
    }

    return $candidate
}

$manifestFullPath = Resolve-ExistingManifestPath -Path $ManifestPath
$manifestDirectory = Split-Path -Parent $manifestFullPath
$lines = [System.IO.File]::ReadAllLines($manifestFullPath)
$problems = New-Object 'System.Collections.Generic.List[string]'
$seenPaths = @{}
$verifiedCount = 0

for ($index = 0; $index -lt $lines.Count; $index++) {
    $lineNumber = $index + 1
    $line = $lines[$index]
    if ([string]::IsNullOrWhiteSpace($line)) {
        $problems.Add("Line $lineNumber is empty; every manifest line must contain a SHA-256 hash and a relative path.")
        continue
    }

    $match = [System.Text.RegularExpressions.Regex]::Match($line, '^(?<Hash>[0-9A-Fa-f]{64})  (?<Path>.+)$')
    if (-not $match.Success) {
        $problems.Add("Line $lineNumber has invalid format; expected '<64 hex chars>  <relative/path>'.")
        continue
    }

    $expectedHash = $match.Groups['Hash'].Value.ToLowerInvariant()
    $relativePath = $match.Groups['Path'].Value
    $pathKey = $relativePath.ToLowerInvariant()
    if ($seenPaths.ContainsKey($pathKey)) {
        $problems.Add("Line $lineNumber duplicates artifact path: $relativePath")
        continue
    }
    $seenPaths[$pathKey] = $true

    try {
        $artifactFullPath = Resolve-ManifestArtifactPath -ManifestDirectory $manifestDirectory -RelativePath $relativePath
    } catch {
        $problems.Add("Line ${lineNumber}: $($_.Exception.Message)")
        continue
    }

    if (-not (Test-Path -LiteralPath $artifactFullPath -PathType Leaf)) {
        $problems.Add("Missing artifact for line ${lineNumber}: $relativePath")
        continue
    }

    try {
        $actualHash = (Get-FileHash -LiteralPath $artifactFullPath -Algorithm SHA256).Hash.ToLowerInvariant()
    } catch {
        $problems.Add("Could not hash artifact on line $lineNumber ($relativePath): $($_.Exception.Message)")
        continue
    }

    if (-not [string]::Equals($expectedHash, $actualHash, [System.StringComparison]::Ordinal)) {
        $problems.Add("Modified artifact detected: $relativePath (expected $expectedHash, found $actualHash)")
        continue
    }

    $verifiedCount++
}

if ($lines.Count -eq 0) {
    $problems.Add("Manifest is empty: $manifestFullPath")
}

if ($problems.Count -gt 0) {
    $details = ($problems | ForEach-Object { " - $_" }) -join "`n"
    throw "Release manifest verification failed:`n$details"
}

Write-Host ("Release manifest verified successfully: {0} artifact(s)" -f $verifiedCount) -ForegroundColor Green
