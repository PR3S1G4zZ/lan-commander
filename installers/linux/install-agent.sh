#!/usr/bin/env bash
#
# LAN Commander Agent - instalador para Linux.
#
# El agente SIEMPRE queda protegido con un token de autenticacion. Si no se
# indica uno, el instalador genera uno aleatorio y lo muestra al final: hay que
# copiarlo en el Control Center para poder conectarse a este equipo.
#
# Uso normal (genera token automaticamente, puerto 9474 por defecto):
#   sudo ./install-agent.sh
#
# Con un token propio (el mismo para toda la flota):
#   sudo ./install-agent.sh --auth-token "un-secreto"
#
# Puerto distinto:
#   sudo ./install-agent.sh --port 9500
#
# Mostrar el aviso de gestion en la interfaz visual:
#   sudo ./install-agent.sh --managed-by-notice "Nombre de la organizacion"
#
# Restringir el firewall a la IP del equipo administrador (recomendado):
#   sudo ./install-agent.sh --allow-from 192.168.1.10
#
# Instalar SIN autenticacion (inseguro, solo para pruebas en red aislada):
#   sudo ./install-agent.sh --no-auth
#
# Desinstalar:
#   sudo ./install-agent.sh --uninstall
#
set -euo pipefail

PORT="9474"
AUTH_TOKEN=""
ALLOW_FROM=""
MANAGED_BY_NOTICE=""
NO_AUTH=0
UNINSTALL=0
GENERATED_TOKEN=0
INSTALL_DIR="/usr/local/bin"
BIN_NAME="lan-agent"
SRC_BIN="lan-agent-linux"

while [[ $# -gt 0 ]]; do
	case "$1" in
	--port)
		PORT="$2"
		shift 2
		;;
	--auth-token)
		AUTH_TOKEN="$2"
		shift 2
		;;
	--allow-from)
		ALLOW_FROM="$2"
		shift 2
		;;
	--managed-by-notice)
		MANAGED_BY_NOTICE="$2"
		shift 2
		;;
	--no-auth)
		NO_AUTH=1
		shift
		;;
	--uninstall)
		UNINSTALL=1
		shift
		;;
	*)
		echo "Argumento desconocido: $1"
		exit 1
		;;
	esac
done

if [[ "${EUID}" -ne 0 ]]; then
	echo "Este script debe ejecutarse como root: sudo ./install-agent.sh" >&2
	exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEST="${INSTALL_DIR}/${BIN_NAME}"

if [[ "${UNINSTALL}" -eq 1 ]]; then
	echo "Deteniendo y desinstalando el servicio..."
	"${DEST}" stop >/dev/null 2>&1 || true
	"${DEST}" uninstall >/dev/null 2>&1 || true
	rm -f "${DEST}"
	rm -f /etc/xdg/autostart/lan-commander-ui.desktop
	echo "Agente desinstalado."
	exit 0
fi

echo "Instalando LAN Commander Agent..."

# --- Autenticacion ---
# Sin token, cualquier equipo de la LAN puede ejecutar comandos como root en
# esta maquina. Por eso el token es obligatorio salvo que se pida --no-auth.
if [[ "${NO_AUTH}" -eq 1 ]]; then
	echo ""
	echo "  ADVERTENCIA: instalando SIN autenticacion (--no-auth)."
	echo "  Cualquier equipo de la red podra ejecutar comandos como root aqui."
	echo ""
	AUTH_TOKEN=""
elif [[ -z "${AUTH_TOKEN}" ]]; then
	if command -v openssl >/dev/null 2>&1; then
		AUTH_TOKEN="$(openssl rand -base64 24 | tr '+/' '-_' | tr -d '=')"
	else
		AUTH_TOKEN="$(head -c 24 /dev/urandom | base64 | tr '+/' '-_' | tr -d '=')"
	fi
	GENERATED_TOKEN=1
fi

if [[ ! -f "${SCRIPT_DIR}/${SRC_BIN}" ]]; then
	echo "No se encontro ${SRC_BIN} junto a este script." >&2
	exit 1
fi

install -m 755 "${SCRIPT_DIR}/${SRC_BIN}" "${DEST}"
# Registrar la interfaz visual para sesiones gráficas XDG. El servicio systemd
# continúa siendo un proceso separado y privilegiado.
UI_EXEC="${DEST} --ui --port ${PORT}"
if [[ -n "${MANAGED_BY_NOTICE}" ]]; then
	UI_EXEC="${UI_EXEC} --managed-by-notice \"${MANAGED_BY_NOTICE}\""
fi
install -d -m 755 /etc/xdg/autostart
cat > /etc/xdg/autostart/lan-commander-ui.desktop <<EOF
[Desktop Entry]
Type=Application
Name=LAN Commander
Comment=Interfaz de usuario del agente LAN Commander
Exec=${UI_EXEC}
Terminal=false
NoDisplay=false
X-GNOME-Autostart-enabled=true
EOF
echo "  Interfaz visual registrada para iniciar sesion"

# Firewall (best-effort segun lo que haya instalado)
if command -v ufw >/dev/null 2>&1 && ufw status | grep -q "Status: active"; then
	if [[ -n "${ALLOW_FROM}" ]]; then
		ufw allow from "${ALLOW_FROM}" to any port "${PORT}" proto tcp >/dev/null
		echo "  Regla ufw creada (puerto ${PORT}/tcp, solo desde ${ALLOW_FROM})"
	else
		ufw allow "${PORT}"/tcp >/dev/null
		echo "  Regla ufw creada (puerto ${PORT}/tcp, abierta a toda la red local)"
	fi
elif command -v firewall-cmd >/dev/null 2>&1 && systemctl is-active --quiet firewalld 2>/dev/null; then
	if [[ -n "${ALLOW_FROM}" ]]; then
		firewall-cmd --permanent --add-rich-rule="rule family=ipv4 source address=${ALLOW_FROM} port port=${PORT} protocol=tcp accept" >/dev/null
		echo "  Regla firewalld creada (puerto ${PORT}/tcp, solo desde ${ALLOW_FROM})"
	else
		firewall-cmd --permanent --add-port="${PORT}"/tcp >/dev/null
		echo "  Regla firewalld creada (puerto ${PORT}/tcp, abierta a toda la red local)"
	fi
	firewall-cmd --reload >/dev/null
else
	echo "  No se detecto ufw/firewalld activo; si usas otro firewall abre el puerto ${PORT}/tcp manualmente."
fi

# Si ya habia una instalacion previa, la reinstalamos limpio para tomar los nuevos parametros
"${DEST}" stop >/dev/null 2>&1 || true
"${DEST}" uninstall >/dev/null 2>&1 || true

INSTALL_ARGS=(install --port "${PORT}")
if [[ -n "${AUTH_TOKEN}" ]]; then
	INSTALL_ARGS+=(--auth-token "${AUTH_TOKEN}")
fi

"${DEST}" "${INSTALL_ARGS[@]}"
"${DEST}" start

echo ""
echo "Listo. El agente quedo instalado como servicio systemd ('LANCommanderAgent'),"
echo "arranca solo con el sistema y escucha en el puerto ${PORT}."
echo "Deberia aparecer solo en el Control Center via descubrimiento en red (mDNS)."

if [[ "${GENERATED_TOKEN}" -eq 1 ]]; then
	echo ""
	echo "====================================================================="
	echo " TOKEN DE ACCESO (guardalo, no se vuelve a mostrar):"
	echo ""
	echo "   ${AUTH_TOKEN}"
	echo ""
	echo " Cargalo en el Control Center al agregar este equipo. Sin el token"
	echo " el agente rechaza cualquier conexion."
	echo "====================================================================="
elif [[ "${NO_AUTH}" -eq 0 ]]; then
	echo ""
	echo "El agente usa el token que indicaste en --auth-token."
fi
