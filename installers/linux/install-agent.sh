#!/usr/bin/env bash
# LAN Commander Agent installer for Linux/systemd.
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
UI_NAME="lan-agent-ui"
CONFIG_DIR="/etc/lan-commander"
FIREWALL_STATE="${CONFIG_DIR}/firewall.conf"

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

if [[ ${EUID} -ne 0 ]]; then echo "Ejecuta como root: sudo ./install-agent.sh" >&2; exit 1; fi
if ! [[ "$PORT" =~ ^[0-9]+$ ]] || (( PORT < 1 || PORT > 65535 )); then echo "Puerto invalido: $PORT" >&2; exit 1; fi
if [[ -n "$TLS_CERT" && -z "$TLS_KEY" ]] || [[ -z "$TLS_CERT" && -n "$TLS_KEY" ]]; then echo "--tls-cert y --tls-key deben usarse juntos" >&2; exit 1; fi
if (( SECURE == 1 )) && [[ -z "$TLS_CERT" || -z "$TLS_KEY" ]]; then echo "--secure requiere --tls-cert y --tls-key" >&2; exit 1; fi
if [[ "$MANAGED_BY_NOTICE" == *'"'* || "$MANAGED_BY_NOTICE" == *'\'* || "$MANAGED_BY_NOTICE" == *$'\n'* ]]; then echo "--managed-by-notice contiene caracteres no permitidos" >&2; exit 1; fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEST="${INSTALL_DIR}/${BIN_NAME}"
UI_DEST="${INSTALL_DIR}/${UI_NAME}"

remove_firewall_rules() {
  [[ -f "$FIREWALL_STATE" ]] || return 0
  local firewall_port="$PORT"
  local firewall_allow_from="$ALLOW_FROM"
  local saved_port
  local saved_allow_from
  saved_port="$(sed -n 's/^port=//p' "$FIREWALL_STATE" | head -n 1)"
  saved_allow_from="$(sed -n 's/^allow_from=//p' "$FIREWALL_STATE" | head -n 1)"
  [[ -n "$saved_port" ]] && firewall_port="$saved_port"
  [[ -n "$saved_allow_from" ]] && firewall_allow_from="$saved_allow_from"
  if grep -q '^ufw=yes' "$FIREWALL_STATE" && command -v ufw >/dev/null 2>&1; then
    ufw delete allow "${firewall_port}/tcp" >/dev/null 2>&1 || true
    if [[ -n "$firewall_allow_from" ]]; then ufw delete allow from "$firewall_allow_from" to any port "$firewall_port" proto tcp >/dev/null 2>&1 || true; fi
  fi
  if grep -q '^firewalld=yes' "$FIREWALL_STATE" && command -v firewall-cmd >/dev/null 2>&1; then
    firewall-cmd --permanent --remove-port="${firewall_port}/tcp" >/dev/null 2>&1 || true
    if [[ -n "$firewall_allow_from" ]]; then firewall-cmd --permanent --remove-rich-rule="rule family=ipv4 source address=${firewall_allow_from} port port=${firewall_port} protocol=tcp accept" >/dev/null 2>&1 || true; fi
    firewall-cmd --reload >/dev/null 2>&1 || true
  fi
  rm -f "$FIREWALL_STATE"
}

if (( UNINSTALL == 1 )); then
  echo "Desinstalando LAN Commander..."
  "$DEST" stop >/dev/null 2>&1 || true
  "$DEST" uninstall >/dev/null 2>&1 || true
  remove_firewall_rules
  rm -f "$DEST" "$UI_DEST" /etc/xdg/autostart/lan-commander-ui.desktop
  rmdir "$CONFIG_DIR" 2>/dev/null || true
  echo "LAN Commander desinstalado."
  exit 0
fi

if [[ ! -f "${SCRIPT_DIR}/${BIN_NAME}-linux" ]]; then echo "No se encontro ${BIN_NAME}-linux" >&2; exit 1; fi
if [[ ! -f "${SCRIPT_DIR}/${UI_NAME}" ]]; then echo "No se encontro ${UI_NAME}" >&2; exit 1; fi

echo "Instalando LAN Commander Agent..."
if (( NO_AUTH == 1 )); then
  echo "ADVERTENCIA: instalando SIN autenticacion."
  AUTH_TOKEN=""
elif [[ -z "$AUTH_TOKEN" ]]; then
  AUTH_TOKEN="$(openssl rand -base64 24 2>/dev/null | tr '+/' '-_' | tr -d '=')"
  GENERATED_TOKEN=1
fi

install -m 755 "${SCRIPT_DIR}/${BIN_NAME}-linux" "$DEST"
install -m 755 "${SCRIPT_DIR}/${UI_NAME}" "$UI_DEST"
install -d -m 755 /etc/xdg/autostart "$CONFIG_DIR"

UI_EXEC="${UI_DEST} --port ${PORT}"
if (( SECURE == 1 )); then UI_EXEC+=" --secure"; fi
if [[ -n "$MANAGED_BY_NOTICE" ]]; then UI_EXEC+=" --managed-by-notice \"${MANAGED_BY_NOTICE}\""; fi
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

remove_firewall_rules
firewall_kind=""
if command -v ufw >/dev/null 2>&1 && ufw status | grep -q "Status: active"; then
  if [[ -n "$ALLOW_FROM" ]]; then ufw allow from "$ALLOW_FROM" to any port "$PORT" proto tcp >/dev/null; else ufw allow "${PORT}/tcp" >/dev/null; fi
  firewall_kind="ufw"
elif command -v firewall-cmd >/dev/null 2>&1 && systemctl is-active --quiet firewalld 2>/dev/null; then
  if [[ -n "$ALLOW_FROM" ]]; then firewall-cmd --permanent --add-rich-rule="rule family=ipv4 source address=${ALLOW_FROM} port port=${PORT} protocol=tcp accept" >/dev/null; else firewall-cmd --permanent --add-port="${PORT}/tcp" >/dev/null; fi
  firewall-cmd --reload >/dev/null
  firewall_kind="firewalld"
else
  echo "Aviso: no se detecto un firewall compatible; abre TCP ${PORT} manualmente."
fi
printf 'ufw=%s\nfirewalld=%s\nport=%s\nallow_from=%s\n' "$([[ "$firewall_kind" == ufw ]] && echo yes || echo no)" "$([[ "$firewall_kind" == firewalld ]] && echo yes || echo no)" "$PORT" "$ALLOW_FROM" > "$FIREWALL_STATE"

"$DEST" stop >/dev/null 2>&1 || true
"$DEST" uninstall >/dev/null 2>&1 || true
INSTALL_ARGS=(install --port "$PORT")
if [[ -n "$AUTH_TOKEN" ]]; then INSTALL_ARGS+=(--auth-token "$AUTH_TOKEN"); fi
if [[ -n "$TLS_CERT" ]]; then INSTALL_ARGS+=(--tls-cert "$TLS_CERT" --tls-key "$TLS_KEY"); fi
"$DEST" "${INSTALL_ARGS[@]}"
"$DEST" start
systemctl enable LANCommanderAgent.service >/dev/null 2>&1 || true

echo "Servicio y aplicacion de escritorio instalados."
if (( GENERATED_TOKEN == 1 )); then echo "TOKEN DE ACCESO (guardalo; no se vuelve a mostrar):"; echo "$AUTH_TOKEN"; fi
