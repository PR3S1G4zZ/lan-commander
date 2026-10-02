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

    Mostrar el aviso de gestion en la interfaz visual:
         .\install-agent.ps1 -ManagedByNotice "Nombre de la organizacion"

    Restringir el firewall a la IP del equipo administrador (recomendado):
        .\install-agent.ps1 -AllowFrom "192.168.1.10"

    El firewall solo se abre en los perfiles Dominio y Privado. Si la red esta
    clasificada como Publica, hay que pedirlo de forma explicita:
        .\install-agent.ps1 -FirewallProfile Domain,Private,Public

    El token se guarda en %ProgramData%\LAN Commander Agent\agent.token (solo
    SYSTEM y Administradores) y el servicio lo lee con --auth-token-file, de modo
    que no aparece en los argumentos del servicio ni en el registro de Windows.
    El registro local de auditoria queda en audit.log, en la misma carpeta.

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
    [ValidateSet("Domain", "Private", "Public")]
    [string[]]$FirewallProfile = @("Domain", "Private"),
    [string]$ManagedByNotice = "",
    [switch]$NoAuth,
    [switch]$Uninstall
)

$ErrorActionPreference = "Stop"
$exeName   = "lan-agent.exe"
$exeDest   = Join-Path $InstallDir $exeName
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$fwRuleName = "LAN Commander Agent"
$dataDir    = Join-Path $env:ProgramData "LAN Commander Agent"
$tokenFile  = Join-Path $dataDir "agent.token"
$auditFile  = Join-Path $dataDir "audit.log"

if ($Uninstall) {
    Write-Host "Deteniendo y desinstalando el servicio..." -ForegroundColor Yellow
    if (Test-Path $exeDest) {
        & $exeDest stop 2>$null | Out-Null
        & $exeDest uninstall 2>$null | Out-Null
    }
    Remove-NetFirewallRule -DisplayName $fwRuleName -ErrorAction SilentlyContinue
    Remove-ItemProperty -Path "HKLM:\Software\Microsoft\Windows\CurrentVersion\Run" -Name "LANCommanderUI" -ErrorAction SilentlyContinue
    Remove-Item -Recurse -Force $InstallDir -ErrorAction SilentlyContinue
    # El token se elimina; el registro de auditoria se conserva como evidencia.
    Remove-Item -Force $tokenFile -ErrorAction SilentlyContinue
    Write-Host "Agente desinstalado." -ForegroundColor Green
    if (Test-Path $auditFile) {
        Write-Host "  Se conserva el registro de auditoria en $dataDir (borralo a mano si ya no lo necesitas)." -ForegroundColor Yellow
    }
    exit 0
}

Write-Host "Instalando LAN Commander Agent..." -ForegroundColor Cyan

# --- Autenticacion ---
# Sin token, cualquier equipo de la LAN puede ejecutar comandos como SYSTEM en
# esta maquina. Por eso el token es obligatorio salvo que se pida -NoAuth.
$generatedToken = $false
$reusedToken = $false
if ($NoAuth -and $AuthToken -ne "") {
    throw "-NoAuth no se puede combinar con -AuthToken."
}
if ($NoAuth) {
    Write-Host ""
    Write-Host "  ADVERTENCIA: instalando SIN autenticacion (-NoAuth)." -ForegroundColor Red
    Write-Host "  Cualquier equipo de la red podra ejecutar comandos como SYSTEM aqui." -ForegroundColor Red
    Write-Host ""
    $AuthToken = ""
} elseif ($AuthToken -eq "" -and (Test-Path $tokenFile)) {
    # Reinstalacion o actualizacion: se conserva el token existente para no
    # invalidar las sesiones ya guardadas en el Control Center.
    $AuthToken = (Get-Content -Raw -Path $tokenFile).Trim()
    $reusedToken = $($AuthToken -ne "")
}
if (-not $NoAuth -and $AuthToken -eq "") {
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

# Si ya habia una instalacion previa hay que parar el servicio ANTES de copiar:
# con el .exe en uso, Copy-Item falla y la reinstalacion quedaria a medias.
if (Test-Path $exeDest) {
    & $exeDest stop 2>$null | Out-Null
    & $exeDest uninstall 2>$null | Out-Null
}

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Copy-Item -Path $sourceExe -Destination $exeDest -Force

# Carpeta de datos con acceso solo para SYSTEM y Administradores. Se usan SIDs
# (S-1-5-18 y S-1-5-32-544) para que funcione en cualquier idioma de Windows.
New-Item -ItemType Directory -Force -Path $dataDir | Out-Null
& icacls.exe $dataDir /inheritance:r /grant:r "*S-1-5-18:(OI)(CI)F" "*S-1-5-32-544:(OI)(CI)F" | Out-Null
if ($LASTEXITCODE -ne 0) { throw "No se pudo restringir el acceso a $dataDir." }

if ($AuthToken -ne "") {
    [System.IO.File]::WriteAllText($tokenFile, $AuthToken, (New-Object System.Text.UTF8Encoding($false)))
    Write-Host "  Token guardado en $tokenFile (acceso restringido)" -ForegroundColor Green
} else {
    Remove-Item -Force $tokenFile -ErrorAction SilentlyContinue
}
# Registrar la interfaz para la sesión gráfica de cada usuario. El servicio y
# la interfaz son procesos separados por el aislamiento de sesión de Windows.
$runKey = "HKLM:\Software\Microsoft\Windows\CurrentVersion\Run"
New-Item -Path $runKey -Force | Out-Null
$uiCommand = "`"$exeDest`" --ui --port $Port"
if ($ManagedByNotice -ne "") {
    $safeNotice = $ManagedByNotice.Replace('"', '\"')
    $uiCommand += " --managed-by-notice `"$safeNotice`""
}
Set-ItemProperty -Path $runKey -Name "LANCommanderUI" -Value $uiCommand
Write-Host "  Interfaz visual registrada para iniciar sesion" -ForegroundColor Green

# Abrir el puerto en el Firewall de Windows (entrada, TCP).
# Se recrea siempre para que -AllowFrom tome efecto en reinstalaciones.
Remove-NetFirewallRule -DisplayName $fwRuleName -ErrorAction SilentlyContinue
$fwParams = @{
    DisplayName = $fwRuleName
    Direction   = "Inbound"
    Protocol    = "TCP"
    LocalPort   = $Port
    Action      = "Allow"
    Profile     = $FirewallProfile
}
if ($AllowFrom -ne "") {
    $fwParams["RemoteAddress"] = $AllowFrom -split ',' | ForEach-Object { $_.Trim() }
}
New-NetFirewallRule @fwParams | Out-Null
$profiles = $FirewallProfile -join ","
if ($AllowFrom -ne "") {
    Write-Host "  Regla de firewall creada (puerto $Port/TCP, perfiles $profiles, solo desde $AllowFrom)" -ForegroundColor Green
} else {
    Write-Host "  Regla de firewall creada (puerto $Port/TCP, perfiles $profiles, abierta a toda la red)" -ForegroundColor Yellow
}

# Los argumentos del servicio quedan en el registro de Windows, legibles por
# cualquier usuario local: por eso el token va en un archivo y no aqui.
$installArgs = @("install", "--port", $Port, "--audit-log", $auditFile)
if ($AuthToken -ne "") { $installArgs += @("--auth-token-file", $tokenFile) }
elseif ($NoAuth) { $installArgs += "--no-auth" }

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
} elseif ($reusedToken) {
    Write-Host ""
    Write-Host "Se conservo el token de la instalacion anterior (no cambia al actualizar)." -ForegroundColor Green
} elseif (-not $NoAuth) {
    Write-Host ""
    Write-Host "El agente usa el token que indicaste en -AuthToken." -ForegroundColor Green
}
