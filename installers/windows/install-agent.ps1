#Requires -RunAsAdministrator
<#
LAN Commander Agent installer for Windows. Installs the privileged service and
the separate desktop application used by logged-in users.
#>
param(
    [ValidateRange(1, 65535)][int]$Port = 9474,
    [string]$AuthToken = "",
    [string]$InstallDir = "$env:ProgramFiles\LAN Commander Agent",
    [string]$AllowFrom = "",
    [string]$ManagedByNotice = "",
    [string]$TlsCert = "",
    [string]$TlsKey = "",
    [switch]$Secure,
    [switch]$NoAuth,
    [switch]$Uninstall
)

$ErrorActionPreference = "Stop"
$exeName = "lan-agent.exe"
$uiName = "lan-agent-ui.exe"
$exeDest = Join-Path $InstallDir $exeName
$uiDest = Join-Path $InstallDir $uiName
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$fwRuleName = "LAN Commander Agent"
$runKey = "HKLM:\Software\Microsoft\Windows\CurrentVersion\Run"

if ($Uninstall) {
    Write-Host "Desinstalando LAN Commander..." -ForegroundColor Yellow
    if (Test-Path $exeDest) {
        & $exeDest stop 2>$null | Out-Null
        & $exeDest uninstall 2>$null | Out-Null
    }
    Remove-NetFirewallRule -DisplayName $fwRuleName -ErrorAction SilentlyContinue
    Remove-ItemProperty -Path $runKey -Name "LANCommanderUI" -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $InstallDir -Recurse -Force -ErrorAction SilentlyContinue
    Write-Host "LAN Commander desinstalado." -ForegroundColor Green
    exit 0
}

if (($TlsCert -and -not $TlsKey) -or ($TlsKey -and -not $TlsCert)) { throw "TlsCert y TlsKey deben especificarse juntos." }
if ($Secure -and (-not $TlsCert -or -not $TlsKey)) { throw "-Secure requiere -TlsCert y -TlsKey." }
if ($ManagedByNotice -match '[\r\n"]') { throw "ManagedByNotice no puede contener comillas ni saltos de linea." }

$sourceExe = Join-Path $scriptDir $exeName
$sourceUi = Join-Path $scriptDir $uiName
if (-not (Test-Path $sourceExe)) { throw "No se encontro $exeName junto a este script." }
if (-not (Test-Path $sourceUi)) { throw "No se encontro $uiName junto a este script." }

Write-Host "Instalando LAN Commander Agent..." -ForegroundColor Cyan
$generatedToken = $false
if ($NoAuth) {
    Write-Host "ADVERTENCIA: instalando SIN autenticacion." -ForegroundColor Red
    $AuthToken = ""
} elseif ($AuthToken -eq "") {
    $bytes = New-Object 'System.Byte[]' 24
    [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
    $AuthToken = [System.Convert]::ToBase64String($bytes).TrimEnd('=').Replace('+', '-').Replace('/', '_')
    $generatedToken = $true
}

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Copy-Item -LiteralPath $sourceExe -Destination $exeDest -Force
Copy-Item -LiteralPath $sourceUi -Destination $uiDest -Force

# La app visual corre como el usuario que inicia sesion; el servicio corre separado.
New-Item -Path $runKey -Force | Out-Null
$uiCommand = "`"$uiDest`" --port $Port"
if ($Secure) { $uiCommand += " --secure" }
if ($ManagedByNotice) { $uiCommand += " --managed-by-notice `"$ManagedByNotice`"" }
Set-ItemProperty -Path $runKey -Name "LANCommanderUI" -Value $uiCommand

Remove-NetFirewallRule -DisplayName $fwRuleName -ErrorAction SilentlyContinue
$fwParams = @{ DisplayName = $fwRuleName; Direction = "Inbound"; Protocol = "TCP"; LocalPort = $Port; Action = "Allow" }
if ($AllowFrom) { $fwParams["RemoteAddress"] = $AllowFrom -split ',' | ForEach-Object { $_.Trim() } }
New-NetFirewallRule @fwParams | Out-Null

& $exeDest stop 2>$null | Out-Null
& $exeDest uninstall 2>$null | Out-Null
$installArgs = @("install", "--port", $Port)
if ($AuthToken) { $installArgs += @("--auth-token", $AuthToken) }
if ($TlsCert) { $installArgs += @("--tls-cert", $TlsCert, "--tls-key", $TlsKey) }
& $exeDest @installArgs
& $exeDest start

Write-Host "Servicio instalado y aplicacion de escritorio registrada." -ForegroundColor Green
Write-Host "El agente escucha en TCP $Port y la aplicacion se abre al iniciar sesion." -ForegroundColor Green
if ($Secure) { Write-Host "TLS habilitado." -ForegroundColor Green }
if ($generatedToken) {
    Write-Host "TOKEN DE ACCESO (guardalo; no se vuelve a mostrar):" -ForegroundColor Cyan
    Write-Host $AuthToken -ForegroundColor White
}
