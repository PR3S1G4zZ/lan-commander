#!/usr/bin/env bash
# LAN Commander Agent installer for Linux/systemd.
# The privileged service and the separate desktop UI are installed together.
set -euo pipefail

PORT="9474"
AUTH_TOKEN=""
ALLOW_FROM=""
MANAGED_BY_NOTICE=""
TLS_CERT=""
TLS_KEY=""
SECURE=0
NO_AUTH=0
UNINSTALL=0
GENERATED_TOKEN=0
INSTALL_DIR="/usr/local/bin"
BIN_NAME="lan-agent"
SRC_BIN="lan-agent-linux"
UI_NAME="lan-agent-ui"
FIREWALL_STATE_DIR="/var/lib/lan-commander"
FIREWALL_STATE_FILE="${FIREWALL_STATE_DIR}/firewall-rule"
FIREWALL_RULE_COMMENT="LAN Commander Agent (lan-commander)"

while [[ $# -gt 0 ]]; do
	case "$1" in
	--port) PORT="${2:?Falta el puerto}"; shift 2 ;;
	--auth-token) AUTH_TOKEN="${2:?Falta el token}"; shift 2 ;;
	--allow-from) ALLOW_FROM="${2:?Falta la IP de origen}"; shift 2 ;;
	--managed-by-notice) MANAGED_BY_NOTICE="${2:?Falta el nombre de la organizacion}"; shift 2 ;;
	--tls-cert) TLS_CERT="${2:?Falta el certificado TLS}"; shift 2 ;;
	--tls-key) TLS_KEY="${2:?Falta la clave TLS}"; shift 2 ;;
	--secure) SECURE=1; shift ;;
	--no-auth) NO_AUTH=1; shift ;;
	--uninstall) UNINSTALL=1; shift ;;
	*) echo "Argumento desconocido: $1" >&2; exit 1 ;;
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
if [[ -n "${TLS_CERT}" && -z "${TLS_KEY}" ]] || [[ -z "${TLS_CERT}" && -n "${TLS_KEY}" ]]; then
	echo "--tls-cert y --tls-key deben usarse juntos." >&2
	exit 1
fi
if (( SECURE == 1 )) && [[ -z "${TLS_CERT}" || -z "${TLS_KEY}" ]]; then
	echo "--secure requiere --tls-cert y --tls-key." >&2
	exit 1
fi
if [[ "${MANAGED_BY_NOTICE}" == *'"'* || "${MANAGED_BY_NOTICE}" == *'\'* || "${MANAGED_BY_NOTICE}" == *$'\n'* ]]; then
	echo "--managed-by-notice contiene caracteres no permitidos." >&2
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
UI_DEST="${INSTALL_DIR}/${UI_NAME}"

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
	# Remove only the rule recorded by this installer. Never remove a rule just
	# because another firewall rule happens to use the same port.
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
if [[ -f "${DEST}" ]]; then
		"${DEST}" stop >/dev/null 2>&1 || true
		"${DEST}" uninstall >/dev/null 2>&1 || true
	fi
	if ! remove_firewall_rule; then
		echo "Advertencia: no se pudo retirar la regla de firewall registrada." >&2
	fi
	rm -f "${DEST}" "${UI_DEST}" /etc/xdg/autostart/lan-commander-ui.desktop
	echo "Agente desinstalado."
	exit 0
fi

if [[ ! -f "${SCRIPT_DIR}/${SRC_BIN}" ]]; then
	echo "No se encontro ${SRC_BIN} junto a este script." >&2
	exit 1
fi
if [[ ! -f "${SCRIPT_DIR}/${UI_NAME}" ]]; then
	echo "No se encontro ${UI_NAME} junto a este script." >&2
	exit 1
fi

echo "Instalando LAN Commander Agent..."
if [[ "${NO_AUTH}" -eq 1 ]]; then
	echo "ADVERTENCIA: instalando SIN autenticacion (--no-auth)."
	AUTH_TOKEN=""
elif [[ -z "${AUTH_TOKEN}" ]]; then
	if command -v openssl >/dev/null 2>&1; then
		AUTH_TOKEN="$(openssl rand -base64 24 | tr '+/' '-_' | tr -d '=')"
	else
		AUTH_TOKEN="$(head -c 24 /dev/urandom | base64 | tr '+/' '-_' | tr -d '=')"
	fi
	GENERATED_TOKEN=1
fi

install -m 755 "${SCRIPT_DIR}/${SRC_BIN}" "${DEST}"
install -m 755 "${SCRIPT_DIR}/${UI_NAME}" "${UI_DEST}"

# The UI runs as the logged-in user while the agent remains a privileged
# service. It checks the local health endpoint directly from the Wails app.
install -d -m 755 /etc/xdg/autostart
UI_EXEC="${UI_DEST} --port ${PORT}"
if (( SECURE == 1 )); then UI_EXEC+=" --secure"; fi
if [[ -n "${MANAGED_BY_NOTICE}" ]]; then UI_EXEC+=" --managed-by-notice \"${MANAGED_BY_NOTICE}\""; fi
cat > /etc/xdg/autostart/lan-commander-ui.desktop <<EOF
[Desktop Entry]
Type=Application
Name=LAN Commander
Comment=Aplicacion de escritorio del agente LAN Commander
Exec=${UI_EXEC}
Terminal=false
NoDisplay=false
X-GNOME-Autostart-enabled=true
EOF

configure_firewall

# Reinstall the service cleanly so changed arguments take effect.
"${DEST}" stop >/dev/null 2>&1 || true
"${DEST}" uninstall >/dev/null 2>&1 || true
INSTALL_ARGS=(install --port "${PORT}")
if [[ -n "${AUTH_TOKEN}" ]]; then
	INSTALL_ARGS+=(--auth-token "${AUTH_TOKEN}")
elif [[ "${NO_AUTH}" -eq 1 ]]; then
	INSTALL_ARGS+=(--no-auth)
fi
if [[ -n "${TLS_CERT}" ]]; then
	INSTALL_ARGS+=(--tls-cert "${TLS_CERT}" --tls-key "${TLS_KEY}")
fi

"${DEST}" "${INSTALL_ARGS[@]}"
"${DEST}" start
systemctl enable LANCommanderAgent.service >/dev/null 2>&1 || true

echo "Servicio y aplicacion de escritorio instalados."
if [[ "${GENERATED_TOKEN}" -eq 1 ]]; then
	echo "TOKEN DE ACCESO (guardalo; no se vuelve a mostrar):"
	echo "${AUTH_TOKEN}"
elif [[ "${NO_AUTH}" -eq 0 ]]; then
	echo "El agente usa el token que indicaste en --auth-token."
fi
