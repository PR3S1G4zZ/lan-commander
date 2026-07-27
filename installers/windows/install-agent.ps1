#Requires -RunAsAdministrator
<#
    LAN Commander Agent - instalador para Windows.

    El agente SIEMPRE queda protegido con un token de autenticacion. Si no se
    indica uno, el instalador genera uno aleatorio y lo muestra al final: hay
    que copiarlo en el Control Center para poder conectarse a este equipo.

    Uso normal (genera token automaticamente, puerto 9474 por defecto):
        .\install-agent.ps1

    Con un token propio (el mismo para toda la flota):
        .\install-agent.ps1 -AuthToken "un-secreto"

    Puerto distinto:
        .\install-agent.ps1 -Port 9500

    Restringir el firewall a la IP del equipo administrador (recomendado):
        .\install-agent.ps1 -AllowFrom "192.168.1.10"

    Instalar SIN autenticacion (inseguro, solo para pruebas en red aislada):
        .\install-agent.ps1 -NoAuth

    Desinstalar:
        .\install-agent.ps1 -Uninstall
#>
param(
    [string]$Port = "9474",
    [string]$AuthToken = "",
    [string]$InstallDir = "$env:ProgramFiles\LAN Commander Agent",
    [string]$AllowFrom = "",
    [switch]$NoAuth,
    [switch]$Uninstall
)

$ErrorActionPreference = "Stop"
$exeName   = "lan-agent.exe"
$exeDest   = Join-Path $InstallDir $exeName
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$fwRuleName = "LAN Commander Agent"

if ($Uninstall) {
    Write-Host "Deteniendo y desinstalando el servicio..." -ForegroundColor Yellow
    if (Test-Path $exeDest) {
        & $exeDest stop 2>$null | Out-Null
        & $exeDest uninstall 2>$null | Out-Null
    }
    Remove-NetFirewallRule -DisplayName $fwRuleName -ErrorAction SilentlyContinue
    Remove-Item -Recurse -Force $InstallDir -ErrorAction SilentlyContinue
    Write-Host "Agente desinstalado." -ForegroundColor Green
    exit 0
}

Write-Host "Instalando LAN Commander Agent..." -ForegroundColor Cyan

# --- Autenticacion ---
# Sin token, cualquier equipo de la LAN puede ejecutar comandos como SYSTEM en
# esta maquina. Por eso el token es obligatorio salvo que se pida -NoAuth.
$generatedToken = $false
if ($NoAuth) {
    Write-Host ""
    Write-Host "  ADVERTENCIA: instalando SIN autenticacion (-NoAuth)." -ForegroundColor Red
    Write-Host "  Cualquier equipo de la red podra ejecutar comandos como SYSTEM aqui." -ForegroundColor Red
    Write-Host ""
    $AuthToken = ""
} elseif ($AuthToken -eq "") {
    $bytes = New-Object 'System.Byte[]' 24
    [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
    $AuthToken = [System.Convert]::ToBase64String($bytes).TrimEnd('=').Replace('+', '-').Replace('/', '_')
    $generatedToken = $true
}

$sourceExe = Join-Path $scriptDir $exeName
if (-not (Test-Path $sourceExe)) {
    Write-Host "No se encontro $exeName junto a este script." -ForegroundColor Red
    exit 1
}

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Copy-Item -Path $sourceExe -Destination $exeDest -Force

# Abrir el puerto en el Firewall de Windows (entrada, TCP).
# Se recrea siempre para que -AllowFrom tome efecto en reinstalaciones.
Remove-NetFirewallRule -DisplayName $fwRuleName -ErrorAction SilentlyContinue
$fwParams = @{
    DisplayName = $fwRuleName
    Direction   = "Inbound"
    Protocol    = "TCP"
    LocalPort   = $Port
    Action      = "Allow"
}
if ($AllowFrom -ne "") {
    $fwParams["RemoteAddress"] = $AllowFrom -split ',' | ForEach-Object { $_.Trim() }
}
New-NetFirewallRule @fwParams | Out-Null
if ($AllowFrom -ne "") {
    Write-Host "  Regla de firewall creada (puerto $Port/TCP, solo desde $AllowFrom)" -ForegroundColor Green
} else {
    Write-Host "  Regla de firewall creada (puerto $Port/TCP, abierta a toda la red local)" -ForegroundColor Yellow
}

# Si ya habia una instalacion previa, la reinstalamos limpio para tomar los nuevos parametros
& $exeDest stop 2>$null | Out-Null
& $exeDest uninstall 2>$null | Out-Null

$installArgs = @("install", "--port", $Port)
if ($AuthToken -ne "") { $installArgs += @("--auth-token", $AuthToken) }

& $exeDest @installArgs
& $exeDest start

Write-Host ""
Write-Host "Listo. El agente quedo instalado como servicio de Windows ('LANCommanderAgent')," -ForegroundColor Green
Write-Host "arranca solo con el sistema (sin ventana visible) y escucha en el puerto $Port." -ForegroundColor Green
Write-Host "Deberia aparecer solo en el Control Center via descubrimiento en red (mDNS)." -ForegroundColor Green

if ($generatedToken) {
    Write-Host ""
    Write-Host "=====================================================================" -ForegroundColor Cyan
    Write-Host " TOKEN DE ACCESO (guardalo, no se vuelve a mostrar):" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "   $AuthToken" -ForegroundColor White
    Write-Host ""
    Write-Host " Cargalo en el Control Center al agregar este equipo. Sin el token" -ForegroundColor Cyan
    Write-Host " el agente rechaza cualquier conexion." -ForegroundColor Cyan
    Write-Host "=====================================================================" -ForegroundColor Cyan
} elseif (-not $NoAuth) {
    Write-Host ""
    Write-Host "El agente usa el token que indicaste en -AuthToken." -ForegroundColor Green
}
