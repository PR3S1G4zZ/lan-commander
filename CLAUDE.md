# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Visión general

LAN Commander es una plataforma de administración remota para una LAN: un **Control Center** (app de escritorio Wails: backend Go + frontend Svelte 5/TypeScript) se conecta por WebSocket a **agentes** (binario Go, servicio Windows/systemd) instalados en los equipos gestionados. README.md (en español) tiene los diagramas, la instalación y las recomendaciones de despliegue; los specs y planes están en `docs/superpowers/`. `.planning/codebase/` contiene mapas del código generados (ARCHITECTURE, CONCERNS, TESTING…); pueden quedar desactualizados, verifica contra el código.

## Estructura: tres proyectos independientes

- `agent/` — módulo Go `github.com/mediacode/lan-commander/agent` (Go 1.26+). Sin CGO.
- `control-center/` — módulo Go `control-center` (Go 1.25+), con el frontend en `control-center/frontend/` (Vite 7, Svelte 5, Tailwind 4, Vitest).
- No hay módulo compartido: cada `go.mod` es independiente. Ejecuta los comandos Go **desde el directorio de cada módulo**.

## Comandos

Todo el build de release solo funciona en Windows (PowerShell + Wails + WebView2).

```powershell
# Build completo (agente win/linux, copia a installers/, frontend, Wails, release-manifest.sha256)
.\scripts\build-all.ps1
.\scripts\build-agent.ps1 -Target windows|linux|all
.\scripts\verify-release-manifest.ps1 -ManifestPath .\release-manifest.sha256

# Agente (lo que ejecuta CI)
cd agent
go test ./... -count=1
go test -race ./...
go vet ./...
go test ./internal/server -run TestNombre -count=1     # un solo test

# Control Center
cd control-center/frontend; npm ci; npm run build      # PRIMERO: main.go hace //go:embed all:frontend/dist
cd control-center
go test ./... -count=1; go test -race ./...; go vet ./...
wails dev                                              # desarrollo con hot reload (requiere Wails CLI v2.13)

# Frontend
cd control-center/frontend
npm test                                               # vitest run
npx vitest run src/lib/utils/transferState.test.ts     # un solo archivo
npm run check                                          # svelte-check
```

`frontend/dist/` no está versionado: en un checkout limpio `go test`/`go vet` de `control-center` fallan hasta que se ejecute `npm run build`. Los archivos de `frontend/wailsjs/` los genera Wails a partir de los métodos exportados de `App` y sí están versionados; si cambias la firma de un método de `app.go`, regenera/actualiza `wailsjs/go/main/App.{js,d.ts}` y `models.ts` (`wails dev`/`wails build` lo hacen).

## Arquitectura (lo que exige leer varios archivos)

**Protocolo.** Cada frame WebSocket es un `protocol.Message{ID, Type, Payload, Timestamp, Error}` en JSON. El protocolo está **duplicado a mano** en `agent/internal/protocol/types.go` y `control-center/backend/protocol/types.go` (ya han divergido, p. ej. `MsgCancelFile` solo existe en el agente). Al añadir o cambiar un mensaje, actualiza ambos archivos y los dos handlers (`agent/internal/server/handlers.go` y `control-center/app.go`); un desajuste falla en silencio (campos a cero), no con error. Los payloads se doble-serializan (`json.Marshal` → `Unmarshal` a struct tipada) en cada frontera.

**Agente** (`agent/cmd/lan-agent/main.go`): el mismo binario corre en primer plano, como servicio (`kardianos/service`: install/uninstall/start/stop) o en modo `--ui` (servidor HTTP solo en `127.0.0.1` + navegador, solo muestra estado). `internal/server` gestiona conexiones (par `readPump`/`writePump` por cliente, push de `system_update` cada 2 s) y `handlers.go` despacha a los módulos sin conocimiento del protocolo: `executor`, `filesystem`, `screenshot`, `system`; `discovery` anuncia `_lan-commander._tcp` por mDNS. Flags del agente: `--port` (9474), `--host`, `--name`, `--discovery`, `--auth-token`, `--auth-token-file` (o la variable `LAN_COMMANDER_AUTH_TOKEN`; los instaladores usan el archivo para que el token no quede en los argumentos del servicio), `--audit-log`, `--no-auth`, `--tls-cert/--tls-key`, `--ui`, `--managed-by-notice`. `--allow-from`/`-AllowFrom` no es un flag del agente: es opción de los instaladores y solo restringe el firewall.

**Autenticación.** El agente se niega a arrancar sin `--auth-token` salvo `--no-auth` explícito. Tras `auth_required` el cliente debe enviar `auth` en 15 s (`AuthTimeout`, no se renueva con `keep_alive`); la comparación es en tiempo constante y se cierra la conexión tras 5 intentos fallidos. `server/ipguard.go` limita 16 conexiones simultáneas por IP y bloquea 1 min a la IP con 20 fallos/min. `CheckOrigin` solo acepta `Origin` vacío (anti-CSWSH).

**Control Center.** `control-center/app.go` es la única costura frontend↔backend: cada método exportado de `App` es un binding Wails. Convención: validar entrada → llamar al manager de `backend/*` → `audit.Log(...)` tanto en éxito como en error. `App.startup` construye los managers: `client` (conexiones multiagente, correlación petición/respuesta por `msg.ID` con un mapa `pending`, reconexión con reintentos), `discovery` (mDNS), `session` y `audit` (dos SQLite independientes vía `modernc.org/sqlite` en el directorio de config del usuario), `scripting`, `wol`, `transfer`, `securestore`. `app.go` es muy grande y de alto impacto: léelo entero antes de editarlo.

**Frontend.** Nunca habla con los agentes; solo llama a bindings Go a través de `src/lib/utils/api.ts` (único punto de entrada, normaliza snake_case de Go → camelCase). Estado en stores Svelte (`agents`, `sessions`, `ui`), refrescado por polling cada 2 s en `App.svelte`, no por eventos. La lógica pura testeable vive en `lib/utils/*State.ts` (con tests Vitest); los componentes `.svelte` no tienen tests.

**Transferencia de archivos.** Por bloques de hasta 64 KB: el agente lee/escribe por offset y calcula SHA-256 completo; el Control Center escribe en un temporal y hace `os.Rename` atómico solo tras verificar el checksum (`backend/transfer` y `TransferFile` en `app.go`).

**Tokens guardados.** `backend/securestore` usa DPAPI en Windows; en otras plataformas falla cerrado (`ErrUnavailable`), por lo que no se pueden persistir sesiones con token fuera de Windows.

## Decisiones de producto (no son bugs)

- El acceso al sistema de archivos del agente **no está confinado a una raíz**: `filesystem.safePath()` solo rechaza `..`. Es intencional (herramienta de administración completa, protegida por token); no añadir `--allowed-root` sin que el usuario lo pida.
- `executor` es una primitiva de shell remota sin restricciones por diseño.
- TLS es opcional (el cliente debe activarlo en el diálogo de conexión; el agente necesita cert/clave).

## Release

La versión vive en cuatro sitios que `scripts/check-version.sh` obliga a mantener iguales: `agent/internal/version/version.go`, `control-center/backend/appinfo/appinfo.go`, `control-center/frontend/package.json` y `control-center/wails.json` (`info.productVersion`). Para publicar: sube las cuatro, escribe `docs/releases/vX.Y.Z.md` y empuja el tag `vX.Y.Z`; `.github/workflows/release.yml` llama a `packages.yml` (instaladores Windows amd64/arm64, AppImage, agentes) y solo publica si todas sus verificaciones pasan. `packages.yml` también corre en PR y en `release/**` para revisar los paquetes sin publicar.

Los datos del usuario (`lan-commander.db`, `audit.db`, `scripts/`) viven en `os.UserConfigDir()/LAN Commander`, fuera de la instalación; no los muevas dentro de la carpeta de instalación ni del AppImage. `backend/dbbackup` copia las bases a `*.bak-<versión>` antes de migrar. Los identificadores del instalador NSIS (`PRODUCT_EXECUTABLE`, `UNINST_KEY_NAME`) no deben cambiar entre versiones o las actualizaciones dejarían de reconocer la instalación previa.

## CI

`.github/workflows/ci.yml`: job `go` (matriz agent/control-center en `windows-latest`: test, `-race`, vet; para control-center construye antes el frontend), job `frontend` (`npm test`, `check`, `build`, `npm audit --omit=dev --audit-level=high`), job `scripts` (parseo de PowerShell y `bash -n` del instalador Linux) y `release-build` (`build-all.ps1` + manifiesto SHA-256). El manifiesto no está firmado.

## Convenciones del repo

- README, comentarios de scripts y documentación están en español; el código y los identificadores, en inglés.
- Los artefactos (`agent/build/`, `installers/*/lan-agent*`, `control-center/build/bin/`, `release-manifest.sha256`) y `*.db` están en `.gitignore`; no los versiones.
