#!/usr/bin/env bash
#
# Prueba de humo del agente: lo arranca de verdad con --auth-token-file y
# --audit-log, comprueba /health y que se crea el registro de auditoria, y lo
# detiene. Sirve para validar un binario nuevo en la arquitectura que lo ejecuta.
#
# Uso: scripts/smoke-agent.sh <ruta-al-binario-del-agente> [puerto]
#
set -euo pipefail

AGENT="${1:?Uso: $0 <binario-del-agente> [puerto]}"
PORT="${2:-19474}"

WORK="$(mktemp -d)"
AGENT_PID=""
cleanup() {
	if [[ -n "${AGENT_PID}" ]]; then
		kill "${AGENT_PID}" 2>/dev/null || true
		sleep 1
		# Git Bash en Windows no siempre termina el proceso nativo con SIGTERM.
		kill -9 "${AGENT_PID}" 2>/dev/null || true
		wait "${AGENT_PID}" 2>/dev/null || true
	fi
	rm -rf "${WORK}" 2>/dev/null || true
}
trap cleanup EXIT

printf '%s\n' "token-de-prueba-$$" > "${WORK}/agent.token"

(cd "${WORK}" && exec "${AGENT}" --port "${PORT}" --host 127.0.0.1 --discovery=false \
	--auth-token-file agent.token --audit-log audit.log > agent.out 2>&1) &
AGENT_PID=$!

healthy=0
for _ in $(seq 1 40); do
	if curl --silent --fail "http://127.0.0.1:${PORT}/health" 2>/dev/null | grep -q '"status":"ok"'; then
		healthy=1
		break
	fi
	if ! kill -0 "${AGENT_PID}" 2>/dev/null; then
		break
	fi
	sleep 0.5
done

if [[ "${healthy}" -ne 1 ]]; then
	echo "El agente no respondio en /health. Salida:" >&2
	cat "${WORK}/agent.out" >&2 || true
	exit 1
fi

if [[ ! -f "${WORK}/audit.log" ]]; then
	echo "El agente arranco pero no creo el registro de auditoria." >&2
	exit 1
fi

# Sin token en la linea de comandos: el secreto no debe aparecer en los argumentos.
if grep -q "token-de-prueba" "${WORK}/agent.out"; then
	echo "El token aparece en la salida del agente." >&2
	exit 1
fi

echo "Agente OK en el puerto ${PORT}: /health responde y la auditoria esta activa."
