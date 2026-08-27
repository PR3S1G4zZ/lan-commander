<#
LAN Commander - instalador grafico para Windows.

Este asistente solicita la configuracion y delega la instalacion privilegiada
al instalador principal. Requiere que lan-agent.exe y lan-agent-ui.exe esten
junto a ambos scripts.
#>
param([switch]$Uninstall)

$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

function Show-Installer {
    $form = New-Object Windows.Forms.Form
    $form.Text = "LAN Commander - Instalador"
    $form.Size = New-Object Drawing.Size(560, 570)
    $form.StartPosition = "CenterScreen"
    $form.FormBorderStyle = "FixedDialog"
    $form.MaximizeBox = $false

    $title = New-Object Windows.Forms.Label
    $title.Text = "Instalar LAN Commander Agent"
    $title.Font = New-Object Drawing.Font("Segoe UI", 16, [Drawing.FontStyle]::Bold)
    $title.Location = New-Object Drawing.Point(24, 22)
    $title.AutoSize = $true
    $form.Controls.Add($title)

    $subtitle = New-Object Windows.Forms.Label
    $subtitle.Text = "Instala el servicio protegido y la aplicacion de escritorio."
    $subtitle.Location = New-Object Drawing.Point(26, 58)
    $subtitle.AutoSize = $true
    $form.Controls.Add($subtitle)

    function Add-Field($labelText, $top, $default, $password = $false) {
        $label = New-Object Windows.Forms.Label
        $label.Text = $labelText
        $label.Location = New-Object Drawing.Point(26, $top)
        $label.AutoSize = $true
        $form.Controls.Add($label)
        $box = New-Object Windows.Forms.TextBox
        $box.Text = $default
        $box.Location = New-Object Drawing.Point(230, ($top - 4))
        $box.Width = 290
        if ($password) { $box.UseSystemPasswordChar = $true }
        $form.Controls.Add($box)
        return $box
    }

    $port = Add-Field "Puerto del agente" 100 "9474"
    $token = Add-Field "Token (opcional)" 140 "" $true
    $notice = Add-Field "Organizacion" 180 ""
    $allowFrom = Add-Field "IP del administrador" 220 ""
    $tlsCert = Add-Field "Certificado TLS" 260 ""
    $tlsKey = Add-Field "Clave privada TLS" 300 "" $true

    $secure = New-Object Windows.Forms.CheckBox
    $secure.Text = "Usar TLS (requiere los dos campos anteriores)"
    $secure.Location = New-Object Drawing.Point(26, 342)
    $secure.AutoSize = $true
    $form.Controls.Add($secure)

    $hint = New-Object Windows.Forms.Label
    $hint.Text = "El instalador registra el servicio, firewall y la app visual al iniciar sesion."
    $hint.Location = New-Object Drawing.Point(26, 380)
    $hint.Size = New-Object Drawing.Size(500, 38)
    $hint.ForeColor = [Drawing.Color]::DimGray
    $form.Controls.Add($hint)

    $install = New-Object Windows.Forms.Button
    $install.Text = "Instalar"
    $install.Location = New-Object Drawing.Point(340, 450)
    $install.Width =  90
    $install.DialogResult = [Windows.Forms.DialogResult]::OK
    $form.Controls.Add($install)
    $cancel = New-Object Windows.Forms.Button
    $cancel.Text = "Cancelar"
    $cancel.Location = New-Object Drawing.Point(440, 450)
    $cancel.Width =  90
    $cancel.DialogResult = [Windows.Forms.DialogResult]::Cancel
    $form.Controls.Add($cancel)
    $form.AcceptButton = $install
    $form.CancelButton = $cancel

    if ($form.ShowDialog() -ne [Windows.Forms.DialogResult]::OK) { return $null }
    return @{ Port = $port.Text; Token = $token.Text; Notice = $notice.Text; AllowFrom = $allowFrom.Text; TlsCert = $tlsCert.Text; TlsKey = $tlsKey.Text; Secure = $secure.Checked }
}

if ($Uninstall) {
    Start-Process powershell.exe -Verb RunAs -Wait -ArgumentList @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', (Join-Path $PSScriptRoot 'install-agent.ps1'), '-Uninstall')
    exit $LASTEXITCODE
}

$config = Show-Installer
if ($null -eq $config) { exit 0 }
if ($config.Secure -and (-not $config.TlsCert -or -not $config.TlsKey)) {
    [Windows.Forms.MessageBox]::Show('TLS requiere indicar el certificado y la clave privada.', 'LAN Commander', 'OK', 'Warning') | Out-Null
    exit 1
}
$args = @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', (Join-Path $PSScriptRoot 'install-agent.ps1'), '-Port', $config.Port)
if ($config.Token) { $args += @('-AuthToken', $config.Token) }
if ($config.Notice) { $args += @('-ManagedByNotice', $config.Notice) }
if ($config.AllowFrom) { $args += @('-AllowFrom', $config.AllowFrom) }
if ($config.TlsCert) { $args += @('-TlsCert', $config.TlsCert, '-TlsKey', $config.TlsKey) }
if ($config.Secure) { $args += '-Secure' }
Start-Process powershell.exe -Verb RunAs -Wait -ArgumentList $args