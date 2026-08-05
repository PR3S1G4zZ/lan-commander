# Instalación gráfica

LAN Commander se distribuye como aplicaciones de escritorio separadas:

- `lan-commander.exe`: Control Center del administrador.
- `lan-agent.exe`: servicio privilegiado, sin ventana interactiva.
- `lan-agent-ui.exe`: aplicación visual del equipo gestionado.

La interfaz del agente ya no abre una página en el navegador. El instalador registra `lan-agent-ui` para que se inicie con la sesión gráfica del usuario y muestre estado, versión, puerto, transporte y organización responsable.

## Windows

En `installers/windows` deben estar `lan-agent.exe`, `lan-agent-ui.exe` y los scripts.

Para el asistente gráfico:

```powershell
powershell.exe -ExecutionPolicy Bypass -File .\install-agent-gui.ps1
```

El asistente configura puerto, token, organización y dirección autorizada. También se puede usar el instalador automatizable:

```powershell
.\install-agent.ps1 -ManagedByNotice "Mi organización" -AllowFrom "192.168.1.10"
```

La instalación registra el servicio `LANCommanderAgent`, crea la regla de firewall y agrega la aplicación de escritorio al inicio de sesión. Para quitarlo:

```powershell
powershell.exe -ExecutionPolicy Bypass -File .\install-agent-gui.ps1 -Uninstall
```

## Linux

En `installers/linux` deben estar `lan-agent-linux`, `lan-agent-ui` y los scripts. En un escritorio con Zenity se puede usar el asistente:

```bash
chmod +x install-agent-gui.sh
sudo ./install-agent-gui.sh
```

En servidores o escritorios sin Zenity se usa el instalador de consola:

```bash
chmod +x install-agent.sh
sudo ./install-agent.sh --managed-by-notice "Mi organización"
```

El instalador configura systemd, firewall disponible y `/etc/xdg/autostart/lan-commander-ui.desktop`. En un servidor sin sesión gráfica el servicio sigue funcionando; simplemente no se inicia la aplicación visual.

## TLS

El agente acepta certificados TLS y el Control Center permite marcar `Usar TLS (wss://)` al conectar. El certificado debe ser confiable para el equipo administrador; no se recomienda desactivar la validación en producción.
