# Technology Stack

**Analysis Date:** 2026-08-27

## Languages

**Primary:**
- Go 1.26.3 (`agent/go.mod`) - Agent service (background daemon that runs on managed PCs)
- Go 1.25.0 (`control-center/go.mod`) - Control Center backend (Wails desktop app)
- TypeScript 5.6.x (`control-center/frontend/tsconfig.json`) - Control Center frontend logic
- Svelte 5.55.x (`.svelte` files under `control-center/frontend/src`) - Control Center UI components

**Secondary:**
- CSS / Tailwind CSS 4.3.x (`control-center/frontend/src/style.css`) - Styling
- PowerShell (`scripts/build-agent.ps1`, `scripts/build-all.ps1`) - Build automation
- Shell/batch installers (`installers/windows`, `installers/linux`) - Install/uninstall scripts for agent service

## Runtime

**Environment:**
- Go toolchain 1.25–1.26 (two modules with slightly different `go` directives — see below)
- Node.js (version not pinned via `.nvmrc`; required for the Vite/Svelte frontend build)
- WebView2 runtime (Windows) / WebKitGTK (Linux) — required by Wails at runtime for the Control Center desktop shell

**Package Manager:**
- Go modules (`go.mod` / `go.sum`) for both `agent/` and `control-center/`
- npm for `control-center/frontend` (lockfile: `control-center/frontend/package-lock.json` present)

## Frameworks

**Core:**
- Wails v2.13.0 (`github.com/wailsapp/wails/v2`, `control-center/go.mod`) - Go-to-desktop bridge; wraps the Svelte frontend in a native WebView window and exposes Go methods to JS (`control-center/app.go`, `control-center/main.go`)
- Svelte 5 + `@sveltejs/vite-plugin-svelte` (`control-center/frontend/package.json`) - Frontend component framework
- Vite 8 (`control-center/frontend/vite.config.ts`) - Frontend dev server / bundler
- Gorilla WebSocket (`github.com/gorilla/websocket` v1.5.3, used in both `agent/go.mod` and `control-center/go.mod`) - Underlying transport for the agent↔control-center protocol (`agent/internal/server/server.go`, `control-center/backend/client/client.go`)

**Testing:**
- Go standard `testing` package - only test file found is `agent/internal/ui/probe_test.go`; no Go test framework dependency beyond stdlib
- `svelte-check` v4 (`control-center/frontend/package.json`) - TypeScript/Svelte type checking, run via `npm run check`

**Build/Dev:**
- Wails CLI (build system for `control-center`, driven by `control-center/wails.json`)
- Vite (`control-center/frontend/vite.config.ts`, `postcss.config.mjs`)
- PowerShell build scripts (`scripts/build-agent.ps1`, `scripts/build-all.ps1`) - Cross-compile agent binaries and package installers

## Key Dependencies

**Critical (agent, `agent/go.mod`):**
- `github.com/gorilla/websocket` v1.5.3 - WebSocket server for remote command/file/screenshot protocol
- `github.com/hashicorp/mdns` v1.0.7 - mDNS service advertisement so the agent is discoverable on the LAN
- `github.com/shirou/gopsutil/v4` v4.26.6 - Cross-platform CPU/memory/disk/network system stats (`agent/internal/system/monitor.go`)
- `github.com/kbinani/screenshot` - Cross-platform screen capture (`agent/internal/screenshot/capture.go`)
- `github.com/google/uuid` v1.6.0 - Message/session ID generation
- `github.com/kardianos/service` (indirect) - Runs the agent as a native Windows Service / systemd/launchd service
- `fyne.io/fyne/v2` v2.8.0 (indirect) - Backing GUI toolkit for the agent's local status UI (`agent/internal/ui/app.go`)

**Critical (control-center, `control-center/go.mod`):**
- `github.com/wailsapp/wails/v2` v2.13.0 - Desktop app shell/bridge
- `modernc.org/sqlite` v1.53.0 - Pure-Go (cgo-free) SQLite driver, backs session persistence and audit log storage
- `github.com/gorilla/websocket` v1.5.3 - WebSocket client to connect to agents (`control-center/backend/client/client.go`)
- `github.com/hashicorp/mdns` v1.0.7 - mDNS discovery of agents on the LAN (`control-center/backend/discovery/discover.go`)
- `github.com/google/uuid` v1.6.0 - ID generation

**Infrastructure:**
- `database/sql` + `modernc.org/sqlite` - Embedded local database, no external DB server
- Standard library `net`, `net/http`, `crypto/tls` - Custom WebSocket server/client and optional TLS (agent side)

## Configuration

**Environment:**
- No `.env` files detected in the repo; configuration is via CLI flags/service install parameters, not env vars
- Agent listens on TCP port 9474 by default (per `README.md` architecture diagram); auth via a shared token (`agent/internal/server/server.go` `authToken` field, `protocol.AuthPayload`)
- TLS is optional per agent instance: if `certFile`/`keyFile` are supplied to `NewServer`, the agent serves WSS over TLS 1.2+; otherwise plain WS (`agent/internal/server/server.go:64-141`)

**Build:**
- `control-center/wails.json` - Wails project config (frontend build hooks: `npm install`, `npm run build`, `npm run dev`)
- `control-center/frontend/vite.config.ts`, `tsconfig.json`, `tsconfig.node.json`, `svelte.config.js`, `postcss.config.mjs` - Frontend build/type-check config
- `agent/go.mod`, `control-center/go.mod` - Go module manifests (note: two independent Go modules, not a workspace — `agent` targets Go 1.26.3, `control-center` targets Go 1.25.0)

## Platform Requirements

**Development:**
- Go 1.25+ toolchain
- Node.js + npm (for `control-center/frontend`)
- Wails CLI installed globally for building the desktop shell
- Windows and/or Linux build targets (cross-compilation via `scripts/build-all.ps1`)

**Production:**
- **Control Center:** Windows or Linux desktop, packaged as a native Wails app (WebView2 on Windows, WebKitGTK on Linux) — see `installers/windows`, `installers/linux`
- **Agent:** runs as a background service (Windows Service via `kardianos/service`, or systemd unit on Linux per `README.md`), listening on TCP 9474 for WebSocket connections; optional local status UI opened in the user's default browser (`--ui` flag, `agent/internal/ui`)
- No cloud dependency — fully LAN-local, peer-to-peer between Control Center and Agents (per `README.md`: "no depender de SSH ni de un servicio en la nube")

---

*Stack analysis: 2026-08-27*
