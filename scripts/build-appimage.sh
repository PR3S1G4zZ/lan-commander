#!/usr/bin/env bash
#
# Empaqueta el Control Center de Linux como AppImage.
#
# Uso:
#   scripts/build-appimage.sh <version> <binario-wails> <salida.AppImage>
#
# <binario-wails> es el ejecutable que genera "wails build -platform linux/amd64".
# El AppImage NO incluye WebKitGTK: usa el del sistema (libwebkit2gtk-4.1 y
# libgtk-3), igual que cualquier aplicacion Wails. Los datos de la aplicacion
# viven en ~/.config/LAN Commander, fuera del AppImage, y por eso sobreviven a
# las actualizaciones.
#
set -euo pipefail

if [[ $# -ne 3 ]]; then
	echo "Uso: $0 <version> <binario-wails> <salida.AppImage>" >&2
	exit 2
fi

VERSION="$1"
BINARY="$2"
OUTPUT="$3"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ICON="${ROOT}/control-center/build/appicon.png"

# appimagetool fijado por version y SHA-256 para que el build sea reproducible y
# no ejecute un binario distinto del revisado.
APPIMAGETOOL_VERSION="1.9.0"
APPIMAGETOOL_URL="https://github.com/AppImage/appimagetool/releases/download/${APPIMAGETOOL_VERSION}/appimagetool-x86_64.AppImage"
APPIMAGETOOL_SHA256="46fdd785094c7f6e545b61afcfb0f3d98d8eab243f644b4b17698c01d06083d1"

[[ -f "${BINARY}" ]] || { echo "No existe el binario: ${BINARY}" >&2; exit 1; }
[[ -f "${ICON}" ]] || { echo "No existe el icono: ${ICON}" >&2; exit 1; }
[[ -n "${VERSION}" ]] || { echo "La version no puede estar vacia." >&2; exit 1; }

WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT

APPDIR="${WORK}/LANCommander.AppDir"
install -d "${APPDIR}/usr/bin"
install -m 755 "${BINARY}" "${APPDIR}/usr/bin/lan-commander"
install -m 644 "${ICON}" "${APPDIR}/lan-commander.png"
install -m 644 "${ICON}" "${APPDIR}/.DirIcon"

cat > "${APPDIR}/lan-commander.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=LAN Commander
GenericName=Remote administration
Comment=Control Center para administrar equipos de la red local
Exec=lan-commander
Icon=lan-commander
Terminal=false
Categories=Network;System;
X-AppImage-Version=${VERSION}
EOF

cat > "${APPDIR}/AppRun" <<'EOF'
#!/bin/sh
HERE="$(dirname "$(readlink -f "$0")")"
exec "${HERE}/usr/bin/lan-commander" "$@"
EOF
chmod 755 "${APPDIR}/AppRun"

TOOL="${WORK}/appimagetool"
curl --fail --silent --show-error --location --retry 3 -o "${TOOL}" "${APPIMAGETOOL_URL}"
echo "${APPIMAGETOOL_SHA256}  ${TOOL}" | sha256sum --check --status || {
	echo "El SHA-256 de appimagetool no coincide con el esperado; se aborta." >&2
	exit 1
}
chmod +x "${TOOL}"

mkdir -p "$(dirname "${OUTPUT}")"
# Los runners de CI no tienen FUSE: se ejecuta la herramienta extraida.
ARCH=x86_64 "${TOOL}" --appimage-extract-and-run --no-appstream "${APPDIR}" "${OUTPUT}"
chmod 755 "${OUTPUT}"
echo "AppImage generado: ${OUTPUT}"
