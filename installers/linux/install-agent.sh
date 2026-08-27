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
FIREWALL_STATE_DIR="/var/lib/lan-commander"
FIREWALL_STATE_FILE="${FIREWALL_STATE_DIR}/firewall-rule"
FIREWALL_RULE_COMMENT="LAN Commander Agent (lan-commander)"

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

if ! [[ "${PORT}" =~ ^[0-9]{1,5}$ ]]; then
	echo "El puerto debe ser un numero entre 1 y 65535." >&2
	exit 1
fi
PORT_NUMBER=$((10#${PORT}))
if (( PORT_NUMBER < 1 || PORT_NUMBER > 65535 )); then
	echo "El puerto debe ser un numero entre 1 y 65535." >&2
	exit 1
fi
PORT="${PORT_NUMBER}"

if [[ -n "${ALLOW_FROM}" ]] && ! [[ "${ALLOW_FROM}" =~ ^[0-9A-Fa-f:.\/]+$ ]]; then
	echo "--allow-from debe ser una direccion IP o una red CIDR." >&2
	exit 1
fi

if [[ "${NO_AUTH}" -eq 1 && -n "${AUTH_TOKEN}" ]]; then
	echo "--no-auth no se puede combinar con --auth-token." >&2
	exit 1
fi

if [[ "${EUID}" -ne 0 ]]; then
	echo "Este script debe ejecutarse como root: sudo ./install-agent.sh" >&2
	exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEST="${INSTALL_DIR}/${BIN_NAME}"

validate_firewall_state_port() {
	[[ "$1" =~ ^[0-9]{1,5}$ ]] || return 1
	local state_port_number=$((10#$1))
	(( state_port_number >= 1 && state_port_number <= 65535 ))
}

read_firewall_state() {
	FIREWALL_STATE_BACKEND=""
	FIREWALL_STATE_PORT=""
	FIREWALL_STATE_ALLOW_FROM=""

	if [[ ! -f "${FIREWALL_STATE_FILE}" ]]; then
		return 0
	fi

	while IFS='=' read -r key value || [[ -n "${key:-}" ]]; do
		case "${key}" in
		backend) FIREWALL_STATE_BACKEND="${value}" ;;
		port) FIREWALL_STATE_PORT="${value}" ;;
		allow_from) FIREWALL_STATE_ALLOW_FROM="${value}" ;;
		*)
			echo "Estado de firewall no reconocido en ${FIREWALL_STATE_FILE}." >&2
			return 1
			;;
		esac
	done < "${FIREWALL_STATE_FILE}"

	if [[ "${FIREWALL_STATE_BACKEND}" != "ufw" && "${FIREWALL_STATE_BACKEND}" != "firewalld" ]]; then
		echo "Backend de firewall no reconocido en ${FIREWALL_STATE_FILE}." >&2
		return 1
	fi
	if ! validate_firewall_state_port "${FIREWALL_STATE_PORT}"; then
		echo "Puerto no valido en ${FIREWALL_STATE_FILE}." >&2
		return 1
	fi
	if [[ -n "${FIREWALL_STATE_ALLOW_FROM}" ]] && ! [[ "${FIREWALL_STATE_ALLOW_FROM}" =~ ^[0-9A-Fa-f:.\/]+$ ]]; then
		echo "Origen no valido en ${FIREWALL_STATE_FILE}." >&2
		return 1
	fi
}

write_firewall_state() {
	local backend="$1"
	local port="$2"
	local allow_from="$3"
	local temporary_file="${FIREWALL_STATE_FILE}.$$"

	install -d -m 755 "${FIREWALL_STATE_DIR}"
	{
		printf 'backend=%s\n' "${backend}"
		printf 'port=%s\n' "${port}"
		printf 'allow_from=%s\n' "${allow_from}"
	} > "${temporary_file}"
	chmod 600 "${temporary_file}"
	mv -f "${temporary_file}" "${FIREWALL_STATE_FILE}"
}

build_firewalld_rule() {
	local port="$1"
	local allow_from="$2"
	if [[ -n "${allow_from}" ]]; then
		printf 'rule family=ipv4 source address=%s port port=%s protocol=tcp accept' "${allow_from}" "${port}"
	else
		printf 'rule family=ipv4 port port=%s protocol=tcp accept' "${port}"
	fi
}

remove_firewall_rule() {
	if [[ ! -f "${FIREWALL_STATE_FILE}" ]]; then
		return 0
	fi

	if ! read_firewall_state; then
		return 1
	fi

	case "${FIREWALL_STATE_BACKEND}" in
	ufw)
		if ! command -v ufw >/dev/null 2>&1; then
			echo "No se encontro ufw para retirar la regla registrada." >&2
			return 1
		fi
		if [[ -n "${FIREWALL_STATE_ALLOW_FROM}" ]]; then
			if ! ufw delete allow from "${FIREWALL_STATE_ALLOW_FROM}" to any port "${FIREWALL_STATE_PORT}" proto tcp comment "${FIREWALL_RULE_COMMENT}" >/dev/null; then
				echo "No se pudo retirar la regla ufw registrada." >&2
				return 1
			fi
		else
			if ! ufw delete allow "${FIREWALL_STATE_PORT}"/tcp comment "${FIREWALL_RULE_COMMENT}" >/dev/null; then
				echo "No se pudo retirar la regla ufw registrada." >&2
				return 1
			fi
		fi
		;;
	firewalld)
		if ! command -v firewall-cmd >/dev/null 2>&1; then
			echo "No se encontro firewall-cmd para retirar la regla registrada." >&2
			return 1
		fi
		local state_rule
		state_rule="$(build_firewalld_rule "${FIREWALL_STATE_PORT}" "${FIREWALL_STATE_ALLOW_FROM}")"
		if ! firewall-cmd --permanent --remove-rich-rule="${state_rule}" >/dev/null; then
			echo "No se pudo retirar la regla firewalld registrada." >&2
			return 1
		fi
		if ! firewall-cmd --reload >/dev/null; then
			echo "Se retiro la regla firewalld, pero fallo la recarga." >&2
			return 1
		fi
		;;
	*)
		echo "Backend de firewall no soportado: ${FIREWALL_STATE_BACKEND}" >&2
		return 1
		;;
	esac

	rm -f "${FIREWALL_STATE_FILE}"
	rmdir "${FIREWALL_STATE_DIR}" 2>/dev/null || true
}

configure_firewall() {
	# Quitar primero la regla que esta instalacion registro, si existe. Nunca
	# se elimina una regla solo por coincidir el puerto: el estado y la marca
	# identifican exclusivamente la regla creada por este instalador.
	if ! remove_firewall_rule; then
		echo "No se pudo retirar la regla de firewall anterior; se cancela la instalacion para no acumular reglas." >&2
		return 1
	fi

	if command -v ufw >/dev/null 2>&1 && ufw status | grep -q "Status: active"; then
		if [[ -n "${ALLOW_FROM}" ]]; then
			ufw allow from "${ALLOW_FROM}" to any port "${PORT}" proto tcp comment "${FIREWALL_RULE_COMMENT}" >/dev/null
		else
			ufw allow "${PORT}"/tcp comment "${FIREWALL_RULE_COMMENT}" >/dev/null
		fi
		write_firewall_state "ufw" "${PORT}" "${ALLOW_FROM}"
		if [[ -n "${ALLOW_FROM}" ]]; then
			echo "  Regla ufw creada (puerto ${PORT}/tcp, solo desde ${ALLOW_FROM})"
		else
			echo "  Regla ufw creada (puerto ${PORT}/tcp, abierta a toda la red local)"
		fi
	elif command -v firewall-cmd >/dev/null 2>&1 && systemctl is-active --quiet firewalld 2>/dev/null; then
		local rule
		rule="$(build_firewalld_rule "${PORT}" "${ALLOW_FROM}")"
		firewall-cmd --permanent --add-rich-rule="${rule}" >/dev/null
		write_firewall_state "firewalld" "${PORT}" "${ALLOW_FROM}"
		firewall-cmd --reload >/dev/null
		if [[ -n "${ALLOW_FROM}" ]]; then
			echo "  Regla firewalld creada (puerto ${PORT}/tcp, solo desde ${ALLOW_FROM})"
		else
			echo "  Regla firewalld creada (puerto ${PORT}/tcp, abierta a toda la red local)"
		fi
	else
		echo "  No se detecto ufw/firewalld activo; si usas otro firewall abre el puerto ${PORT}/tcp manualmente."
	fi
}

if [[ "${UNINSTALL}" -eq 1 ]]; then
	echo "Deteniendo y desinstalando el servicio..."
	"${DEST}" stop >/dev/null 2>&1 || true
	"${DEST}" uninstall >/dev/null 2>&1 || true
	if ! remove_firewall_rule; then
		echo "Advertencia: no se pudo retirar la regla de firewall registrada." >&2
	fi
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
configure_firewall

# Si ya habia una instalacion previa, la reinstalamos limpio para tomar los nuevos parametros
"${DEST}" stop >/dev/null 2>&1 || true
"${DEST}" uninstall >/dev/null 2>&1 || true

INSTALL_ARGS=(install --port "${PORT}")
if [[ -n "${AUTH_TOKEN}" ]]; then
	INSTALL_ARGS+=(--auth-token "${AUTH_TOKEN}")
elif [[ "${NO_AUTH}" -eq 1 ]]; then
	INSTALL_ARGS+=(--no-auth)
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
