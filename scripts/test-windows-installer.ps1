# LAN Commander - Prueba del instalador de Windows
#
# Dos modos:
#   -Mode Inspect   Comprueba que un .exe es de la arquitectura esperada.
#   -Mode Cycle     Instala en silencio, reinstala encima (actualizacion), desinstala
#                   y verifica que los datos del usuario sobreviven a todo el ciclo.
#
# Ejemplos:
#   .\test-windows-installer.ps1 -Mode Inspect -Architecture arm64 -AppExe .\lan-commander.exe
#   .\test-windows-installer.ps1 -Mode Cycle -Architecture amd64 -Installer .\setup.exe
#
# El modo Cycle instala de verdad: usalo solo en un runner de CI o en una maquina de pruebas.

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet("Inspect", "Cycle")]
    [string]$Mode,

    [Parameter(Mandatory = $true)]
    [ValidateSet("amd64", "arm64")]
    [string]$Architecture,

    [string]$AppExe,
    [string]$Installer
)

$ErrorActionPreference = "Stop"

function Get-PeArchitecture {
    param([Parameter(Mandatory = $true)][string]$Path)

    $stream = [System.IO.File]::OpenRead($Path)
    try {
        $reader = New-Object System.IO.BinaryReader($stream)
        if ($reader.ReadUInt16() -ne 0x5A4D) { throw "$Path no es un ejecutable de Windows (falta la firma MZ)." }
        $stream.Seek(0x3C, [System.IO.SeekOrigin]::Begin) | Out-Null
        $peOffset = $reader.ReadInt32()
        $stream.Seek($peOffset, [System.IO.SeekOrigin]::Begin) | Out-Null
        if ($reader.ReadUInt32() -ne 0x00004550) { throw "$Path no tiene una cabecera PE valida." }
        $machine = $reader.ReadUInt16()
    } finally {
        $stream.Dispose()
    }

    switch ($machine) {
        0x8664 { return "amd64" }
        0xAA64 { return "arm64" }
        0x014C { return "x86" }
        default { return ("desconocida (0x{0:X4})" -f $machine) }
    }
}

function Assert-Architecture {
    param([string]$Path, [string]$Expected)
    $actual = Get-PeArchitecture -Path $Path
    if ($actual -ne $Expected) {
        throw "$Path es $actual, pero se esperaba $Expected."
    }
    Write-Host "  OK: $(Split-Path -Leaf $Path) es $actual" -ForegroundColor Green
}

if ($Mode -eq "Inspect") {
    if (-not $AppExe -or -not (Test-Path -LiteralPath $AppExe -PathType Leaf)) {
        throw "-AppExe debe apuntar a un archivo existente."
    }
    Assert-Architecture -Path $AppExe -Expected $Architecture
    exit 0
}

# --- Modo Cycle -------------------------------------------------------------
if (-not $Installer -or -not (Test-Path -LiteralPath $Installer -PathType Leaf)) {
    throw "-Installer debe apuntar a un archivo existente."
}

function Find-Installation {
    $roots = @(
        "HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall",
        "HKLM:\Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall",
        "HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall"
    )
    foreach ($root in $roots) {
        if (-not (Test-Path $root)) { continue }
        foreach ($key in Get-ChildItem $root) {
            $props = Get-ItemProperty -LiteralPath $key.PSPath
            if (-not $props.UninstallString) { continue }
            $uninstaller = ($props.UninstallString -replace '"', '').Trim()
            $dir = Split-Path -Parent $uninstaller
            if ($dir -and (Test-Path -LiteralPath (Join-Path $dir "lan-commander.exe"))) {
                return [pscustomobject]@{ Dir = $dir; Uninstaller = $uninstaller }
            }
        }
    }
    return $null
}

function Invoke-Installer {
    $process = Start-Process -FilePath $Installer -ArgumentList "/S" -Wait -PassThru
    if ($process.ExitCode -ne 0) {
        throw "El instalador termino con codigo $($process.ExitCode)."
    }
}

$dataDir = Join-Path $env:APPDATA "LAN Commander"
$sentinel = Join-Path $dataDir "upgrade-sentinel.txt"
New-Item -ItemType Directory -Force -Path $dataDir | Out-Null
Set-Content -LiteralPath $sentinel -Value "keep-me" -Encoding ASCII

function Assert-DataIntact {
    param([string]$Stage)
    if (-not (Test-Path -LiteralPath $sentinel)) {
        throw "Los datos del usuario desaparecieron tras: $Stage."
    }
    if ((Get-Content -LiteralPath $sentinel -Raw).Trim() -ne "keep-me") {
        throw "Los datos del usuario cambiaron tras: $Stage."
    }
    Write-Host "  OK: los datos del usuario siguen intactos tras $Stage" -ForegroundColor Green
}

try {
    Write-Host "[1/4] Instalacion limpia..." -ForegroundColor Yellow
    Invoke-Installer
    $installation = Find-Installation
    if (-not $installation) { throw "No se encontro la instalacion en el registro tras instalar." }
    Assert-Architecture -Path (Join-Path $installation.Dir "lan-commander.exe") -Expected $Architecture
    Assert-DataIntact -Stage "la instalacion"

    Write-Host "[2/4] Actualizacion (instalar encima de la version existente)..." -ForegroundColor Yellow
    Invoke-Installer
    if (-not (Find-Installation)) { throw "La instalacion desaparecio tras actualizar." }
    Assert-DataIntact -Stage "la actualizacion"

    Write-Host "[3/4] Desinstalacion..." -ForegroundColor Yellow
    # "_?=" hace que el desinstalador no se copie a %TEMP% y por tanto se espere a que termine.
    $process = Start-Process -FilePath $installation.Uninstaller -ArgumentList @("/S", "_?=$($installation.Dir)") -Wait -PassThru
    if ($process.ExitCode -ne 0) { throw "El desinstalador termino con codigo $($process.ExitCode)." }
    if (Test-Path -LiteralPath (Join-Path $installation.Dir "lan-commander.exe")) {
        throw "lan-commander.exe sigue instalado tras desinstalar."
    }
    Assert-DataIntact -Stage "la desinstalacion"

    Write-Host "[4/4] Ciclo completo correcto." -ForegroundColor Cyan
} finally {
    Remove-Item -LiteralPath $sentinel -Force -ErrorAction SilentlyContinue
}
