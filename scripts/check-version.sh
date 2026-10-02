#!/usr/bin/env bash
#
# Comprueba que la version de LAN Commander es la misma en todos los sitios donde
# se declara, e imprime esa version (sin prefijo "v") en la ultima linea.
#
# Uso:
#   scripts/check-version.sh            # solo comprueba y muestra la version
#   scripts/check-version.sh v1.1.0     # ademas exige que coincida con el tag
#
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
EXPECTED="${1:-}"
EXPECTED="${EXPECTED#v}"

go_version() {
	sed -n 's/^var Version = "\([^"]*\)".*/\1/p' "$1" | head -n 1 | tr -d '\r'
}

AGENT="$(go_version "${ROOT}/agent/internal/version/version.go")"
CENTER="$(go_version "${ROOT}/control-center/backend/appinfo/appinfo.go")"
FRONTEND="$(sed -n 's/^[[:space:]]*"version":[[:space:]]*"\([^"]*\)".*/\1/p' "${ROOT}/control-center/frontend/package.json" | head -n 1 | tr -d '\r')"
WAILS="$(sed -n 's/^[[:space:]]*"productVersion":[[:space:]]*"\([^"]*\)".*/\1/p' "${ROOT}/control-center/wails.json" | head -n 1 | tr -d '\r')"

status=0
report() {
	printf '  %-34s %s\n' "$1" "${2:-<no encontrada>}" >&2
}

echo "Versiones declaradas:" >&2
report "agent/internal/version" "${AGENT}"
report "control-center/backend/appinfo" "${CENTER}"
report "frontend/package.json" "${FRONTEND}"
report "control-center/wails.json" "${WAILS}"

for value in "${AGENT}" "${CENTER}" "${FRONTEND}" "${WAILS}"; do
	if [[ -z "${value}" ]]; then
		echo "Falta una version en alguno de los archivos." >&2
		status=1
	fi
done

if [[ "${AGENT}" != "${CENTER}" || "${AGENT}" != "${FRONTEND}" || "${AGENT}" != "${WAILS}" ]]; then
	echo "Las versiones no coinciden: actualiza los cuatro archivos a la vez." >&2
	status=1
fi

if ! [[ "${AGENT}" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]]; then
	echo "La version '${AGENT}' no tiene el formato MAJOR.MINOR.PATCH[-sufijo]." >&2
	status=1
fi

if [[ -n "${EXPECTED}" && "${AGENT}" != "${EXPECTED}" ]]; then
	echo "El tag pide la version ${EXPECTED}, pero el codigo declara ${AGENT}." >&2
	status=1
fi

if [[ "${status}" -ne 0 ]]; then
	exit "${status}"
fi
echo "${AGENT}"
