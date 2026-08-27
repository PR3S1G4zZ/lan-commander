# Codebase Structure

**Analysis Date:** 2026-08-27

## Directory Layout

```
lan-commander/
├── agent/                          # Standalone Go module: per-host management agent
│   ├── cmd/lan-agent/main.go       # Agent binary entry point / OS service wrapper
│   ├── internal/
│   │   ├── discovery/mdns.go       # mDNS service advertisement (agent side)
│   │   ├── executor/executor.go    # Shell command execution
│   │   ├── filesystem/fs.go        # Directory listing, chunked file I/O, checksums
│   │   ├── protocol/types.go       # Wire protocol message/payload structs (agent copy)
│   │   ├── screenshot/capture.go   # Desktop screenshot capture
│   │   ├── server/                 # WebSocket server + message dispatch
│   │   │   ├── server.go
│   │   │   └── handlers.go
│   │   ├── system/monitor.go       # CPU/memory/disk/network stats polling
│   │   └── ui/                     # Optional foreground tray/UI mode (--ui flag)
│   │       ├── app.go
│   │       ├── probe.go / probe_other.go / probe_windows.go / probe_test.go
│   └── build/                      # Build output (gitignored / generated)
├── control-center/                 # Standalone Go module: Wails desktop app
│   ├── main.go                     # Wails app bootstrap, embeds frontend/dist
│   ├── app.go                      # App struct: all frontend-bindable methods
│   ├── wails.json                  # Wails build configuration
│   ├── backend/
│   │   ├── audit/audit.go          # SQLite-backed audit log
│   │   ├── client/client.go        # Multi-agent WebSocket connection manager
│   │   ├── discovery/discover.go   # mDNS discovery (control-center side)
│   │   ├── protocol/types.go       # Wire protocol structs (control-center copy)
│   │   ├── scripting/engine.go     # Script storage + line-by-line execution
│   │   ├── session/session.go      # SQLite-backed saved-connection sessions
│   │   └── wol/wol.go              # Wake-on-LAN magic packet sender
│   ├── build/                      # Wails build output (gitignored / generated)
│   └── frontend/                   # Svelte 5 + TypeScript + Vite SPA
│       ├── index.html
│       ├── vite.config.ts / svelte.config.js / tsconfig*.json / postcss.config.mjs
│       ├── src/
│       │   ├── main.ts             # Svelte app mount point
│       │   ├── App.svelte          # Root component: layout, view routing, polling loop
│       │   ├── style.css
│       │   ├── assets/             # Fonts, images
│       │   └── lib/
│       │       ├── components/     # Active UI components (Dashboard, Terminal, etc.)
│       │       ├── stores/         # Svelte writable/derived stores (agents, sessions, ui)
│       │       ├── utils/          # api.ts (Wails binding wrapper), format.ts
│       │       └── backup/         # STALE, unused duplicate of components/stores/utils
│       └── wailsjs/                # Auto-generated Wails JS bindings (do not hand-edit)
│           ├── go/main/App.d.ts / App.js / models.ts
│           └── runtime/
├── installers/
│   ├── linux/                      # install-agent.sh, prebuilt lan-agent-linux binary
│   └── windows/                    # install-agent.ps1, prebuilt lan-agent.exe
├── scripts/
│   ├── build-agent.ps1             # Build the agent binary
│   └── build-all.ps1               # Build agent + control-center
├── docs/                           # Project documentation (incl. docs/superpowers)
├── .planning/                      # GSD planning artifacts (this document's home)
├── .superpowers/                   # Superpowers skill/plugin data
└── README.md
```

## Directory Purposes

**`agent/`:**
- Purpose: Everything needed to build and run the per-host management agent
- Contains: Go source under `internal/`, a single `cmd/` entry point, its own `go.mod`
- Key files: `agent/cmd/lan-agent/main.go`, `agent/internal/server/server.go`

**`agent/internal/`:**
- Purpose: Implementation packages, intentionally not importable outside the `agent` module (Go `internal/` convention)
- Contains: One package per capability (discovery, executor, filesystem, protocol, screenshot, server, system, ui)
- Key files: `agent/internal/server/handlers.go` (message routing), `agent/internal/protocol/types.go` (wire format)

**`control-center/`:**
- Purpose: Desktop application module (Wails v2) combining a Go backend with an embedded Svelte frontend
- Contains: `main.go`/`app.go` at the module root, `backend/` packages, `frontend/` SPA, `wails.json` config
- Key files: `control-center/app.go` (frontend-facing API surface), `control-center/main.go`

**`control-center/backend/`:**
- Purpose: Backend manager packages, one per concern, each owning its own state and constructed once in `App.startup`
- Contains: `audit`, `client`, `discovery`, `protocol`, `scripting`, `session`, `wol`
- Key files: `control-center/backend/client/client.go` (largest/most complex — connection lifecycle, request correlation)

**`control-center/frontend/src/lib/components/`:**
- Purpose: Active, imported Svelte UI views/panels
- Contains: `Dashboard.svelte`, `Terminal.svelte`, `FileBrowser.svelte`, `MultiExec.svelte`, `ScriptEditor.svelte`, `AuditLog.svelte`, `Settings.svelte`, `Sidebar.svelte`, `ConnectionDialog.svelte`, `Notifications.svelte`, `Icon.svelte`
- Key files: Referenced directly from `control-center/frontend/src/App.svelte`

**`control-center/frontend/src/lib/stores/`:**
- Purpose: Shared reactive state (Svelte stores) used across components
- Contains: `agents.ts` (agent list, selection, derived counts, TS interfaces for all wire payload types), `sessions.ts`, `ui.ts` (current view, sidebar state)
- Key files: `control-center/frontend/src/lib/stores/agents.ts`

**`control-center/frontend/src/lib/utils/`:**
- Purpose: Cross-cutting helpers
- Contains: `api.ts` (the only file that imports Wails-generated bindings; normalizes Go snake_case → TS camelCase), `format.ts`
- Key files: `control-center/frontend/src/lib/utils/api.ts`

**`control-center/frontend/src/lib/backup/`:**
- Purpose: **Stale/unused.** Duplicate copies of components, stores, and utils that are not imported anywhere in the active app
- Contains: Older versions of `App.svelte`, `Dashboard.svelte`, `agents.ts`, `api.ts`, `format.ts`, etc.
- Key files: None load-bearing — do not edit files here expecting runtime effect

**`control-center/frontend/wailsjs/`:**
- Purpose: Auto-generated JS/TS bindings mirroring `App` struct methods and Go model types
- Contains: `go/main/App.d.ts`, `go/main/App.js`, `go/main/models.ts`, `runtime/`
- Key files: Regenerated by `wails generate module` — never hand-edit

**`installers/`:**
- Purpose: End-user installation scripts and prebuilt agent binaries for distribution
- Contains: `linux/install-agent.sh` + `lan-agent-linux`, `windows/install-agent.ps1` + `lan-agent.exe`
- Key files: `installers/windows/install-agent.ps1`, `installers/linux/install-agent.sh`

**`scripts/`:**
- Purpose: Local/CI build automation (PowerShell)
- Contains: `build-agent.ps1`, `build-all.ps1`
- Key files: `scripts/build-all.ps1`

## Key File Locations

**Entry Points:**
- `agent/cmd/lan-agent/main.go`: Agent process entry, CLI flags, service install/run
- `control-center/main.go`: Wails app entry, embeds frontend, registers bindings
- `control-center/frontend/src/main.ts`: Svelte SPA mount point

**Configuration:**
- `control-center/wails.json`: Wails build/dev configuration
- `control-center/frontend/vite.config.ts`, `svelte.config.js`, `tsconfig.json`, `postcss.config.mjs`: Frontend build config
- Agent runtime config is CLI-flag-driven only (no config file) — see flags in `agent/cmd/lan-agent/main.go:33-46`

**Core Logic:**
- `agent/internal/server/handlers.go`: Every remote-management operation an agent supports
- `control-center/app.go`: Every operation the desktop UI can invoke (single API surface)
- `control-center/backend/client/client.go`: WebSocket client/connection-pool logic

**Testing:**
- `agent/internal/ui/probe_test.go`: Only Go test file found in the repository

## Naming Conventions

**Files:**
- Go: lowercase, single-word or short compound package-scoped filenames (`server.go`, `handlers.go`, `types.go`, `client.go`) — one primary type/concern per file, matching the containing package directory name
- Svelte components: PascalCase `.svelte` (e.g. `FileBrowser.svelte`, `ScriptEditor.svelte`)
- TypeScript modules: lowercase (`api.ts`, `format.ts`, `agents.ts`, `ui.ts`)

**Directories:**
- Go: singular, lowercase, noun-based package names describing one capability (`executor`, `filesystem`, `screenshot`, `discovery`, `scripting`, `session`, `audit`, `wol`)
- Frontend: lowercase, grouped by role under `lib/` (`components/`, `stores/`, `utils/`)

**Protocol message type constants (both `protocol/types.go` files):**
- `Msg<PascalCase>` Go identifier mapping to a `snake_case` string value, e.g. `MsgExecCommand = "exec_command"`, `MsgSystemUpdate = "system_update"`

**Wails bindings (`control-center/app.go` → frontend):**
- Go method: `PascalCase` (e.g. `ExecCommand`, `TransferFile`, `RunScript`)
- Frontend wrapper function in `api.ts`: `camelCase` (e.g. `execCommand`, `transferFile`, `runScript`)

## Where to Add New Code

**New agent capability (e.g. a new remote operation):**
1. Add message type constants to **both** `agent/internal/protocol/types.go` and `control-center/backend/protocol/types.go`
2. Implement the capability as a new package under `agent/internal/<capability>/` (pure functions, no protocol awareness)
3. Add a `handle<Capability>` method in `agent/internal/server/handlers.go` and wire it into the `switch` in `handleMessage`
4. Add a corresponding `App` binding method in `control-center/app.go` that calls `clientMgr.SendAndParse`
5. Add a wrapper function in `control-center/frontend/src/lib/utils/api.ts`
6. Consume it from a component under `control-center/frontend/src/lib/components/`

**New frontend view/panel:**
- Component: `control-center/frontend/src/lib/components/<Name>.svelte`
- Register in the `views` array and `{#if}` chain in `control-center/frontend/src/App.svelte:61-69,108-115`
- Add any new shared state to `control-center/frontend/src/lib/stores/ui.ts` (view type) or a new store file

**New backend manager (persistent/stateful concern):**
- Implementation: `control-center/backend/<name>/<name>.go`
- Construct and store on `App` in `control-center/app.go` `startup()`, tear down in `shutdown()`
- Expose operations as `App` binding methods with audit logging, following the pattern of existing methods

**Utilities:**
- Agent-side shared helpers: new file under the relevant `agent/internal/<package>/`
- Control-center backend shared helpers: `control-center/backend/<package>/`
- Frontend shared helpers: `control-center/frontend/src/lib/utils/`

## Special Directories

**`agent/build/`, `control-center/build/`:**
- Purpose: Compiled binaries / packaging output
- Generated: Yes
- Committed: No (build artifacts)

**`control-center/frontend/wailsjs/`:**
- Purpose: Auto-generated Go↔JS binding glue
- Generated: Yes (via `wails generate module`)
- Committed: Yes (checked in so the frontend builds without a Go toolchain step)

**`control-center/frontend/src/lib/backup/`:**
- Purpose: Stale duplicate of `components/`/`stores/`/`utils/`, not part of the active import graph
- Generated: No
- Committed: Yes — treat as dead code, not a template to copy from

**`installers/`:**
- Purpose: Ships prebuilt platform binaries (`lan-agent.exe`, `lan-agent-linux`) alongside install scripts
- Generated: Binaries are build output committed for distribution; scripts are hand-written
- Committed: Yes

**`.planning/`, `.superpowers/`:**
- Purpose: GSD workflow planning artifacts and Superpowers skill data — not application code
- Generated: Mixed (planning docs are written by GSD tooling)
- Committed: Yes

---

*Structure analysis: 2026-08-27*
