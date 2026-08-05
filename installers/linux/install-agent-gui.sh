#!/usr/bin/env bash
# Asistente grafico opcional para escritorios Linux con Zenity.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if ! command -v zenity >/dev/null 2>&1; then
  echo "Este asistente requiere zenity. Instala zenity o usa install-agent.sh." >&2
  exit 1
fi

PORT="$(zenity --entry --title="LAN Commander" --text="Puerto del agente:" --entry-text="9474")" || exit 0
TOKEN="$(zenity --password --title="LAN Commander" --text="Token opcional (vacio para generar uno):")" || exit 0
NOTICE="$(zenity --entry --title="LAN Commander" --text="Organizacion que gestiona este equipo (opcional):")" || exit 0
ALLOW_FROM="$(zenity --entry --title="LAN Commander" --text="IP del administrador (opcional):")" || exit 0

ARGS=(--port "$PORT")
[[ -n "$TOKEN" ]] && ARGS+=(--auth-token "$TOKEN")
[[ -n "$NOTICE" ]] && ARGS+=(--managed-by-notice "$NOTICE")
[[ -n "$ALLOW_FROM" ]] && ARGS+=(--allow-from "$ALLOW_FROM")

if ! pkexec "$SCRIPT_DIR/install-agent.sh" "${ARGS[@]}"; then
  zenity --error --title="LAN Commander" --text="No se pudo completar la instalacion."
  exit 1
fi
zenity --info --title="LAN Commander" --text="Agente y aplicacion de escritorio instalados correctamente."
