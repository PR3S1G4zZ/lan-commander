<!-- refreshed: 2026-08-27 -->
# Architecture

**Analysis Date:** 2026-08-27

## System Overview

LAN Commander is a two-binary system: a lightweight cross-platform **agent** that runs on each managed machine, and a desktop **control center** (Wails app) that connects to one or more agents over WebSocket to monitor and control them on a local network.

```text
┌───────────────────────────────────────────────────────────────────┐
│                     control-center (Wails desktop app)             │
├───────────────────────────────┬───────────────────────────────────┤
│  Frontend (Svelte 5 + TS)      │  Backend (Go)                     │
│  `control-center/frontend/src` │  `control-center/app.go` (bindings)│
│  Stores: agents, sessions, ui  │  Managers: client, discovery,     │
│  Components: Dashboard,        │  session, audit, scripting, wol   │
│  Terminal, FileBrowser, etc.   │  `control-center/backend/*`       │
└───────────────┬─────────────────────────────┬───────────────────┘
                │ Wails JS<->Go bindings        │ WebSocket (ws://host:port/ws)
                │ (auto-generated,               │ JSON protocol.Message frames
                │  `frontend/wailsjs/go/main`)   │
                ▼                                ▼
┌───────────────────────────────────────────────────────────────────┐
│                         agent (Go, per managed host)                │
│  `agent/cmd/lan-agent/main.go` — entry point / OS service wrapper   │
│  `agent/internal/server` — WebSocket server, message dispatch       │
│  `agent/internal/{executor,filesystem,screenshot,system}`           │
│  `agent/internal/discovery` — mDNS advertisement                    │
│  `agent/internal/ui` — optional in-session tray/UI mode             │
└───────────────────────────────────────────────────────────────────┘
```

## Component Responsibilities

| Component | Responsibility | File |
|-----------|----------------|------|
| Agent entry point / service wrapper | CLI flags, install/start/stop as OS service, launches server or UI mode | `agent/cmd/lan-agent/main.go` |
| Agent WebSocket server | Accepts connections, auth handshake, per-client read/write pumps, periodic system-info push | `agent/internal/server/server.go` |
| Agent message handlers | Dispatches protocol messages to executor/filesystem/screenshot/system | `agent/internal/server/handlers.go` |
| Command executor | Runs shell commands with timeout, captures stdout/stderr/exit code | `agent/internal/executor/executor.go` |
| Filesystem operations | Directory listing, chunked file read/write, SHA-256 checksum | `agent/internal/filesystem/fs.go` |
| Screenshot capture | Captures the desktop framebuffer as PNG | `agent/internal/screenshot/capture.go` |
| System monitor | Polls CPU/memory/disk/network stats | `agent/internal/system/monitor.go` |
| mDNS discovery (agent side) | Advertises the agent's presence on the LAN | `agent/internal/discovery/mdns.go` |
| Agent tray/UI mode | Optional foreground UI when run with `--ui` | `agent/internal/ui/app.go`, `agent/internal/ui/probe*.go` |
| Wails app bootstrap | Registers Go struct methods as JS-callable bindings, wires startup/shutdown | `control-center/main.go` |
| Control-center App struct | Owns all backend managers; every exported method is a frontend-callable binding | `control-center/app.go` |
| Agent connection manager | Multi-agent WebSocket client, request/response correlation, reconnect logic | `control-center/backend/client/client.go` |
| mDNS discovery (control-center side) | Discovers agents broadcasting on the LAN, triggers auto-connect | `control-center/backend/discovery/discover.go` |
| Session persistence | SQLite-backed storage of saved agent connections | `control-center/backend/session/session.go` |
| Audit logging | SQLite-backed action log for every user/system operation | `control-center/backend/audit/audit.go` |
| Script engine | Stores/executes multi-line scripts against an agent, one command at a time | `control-center/backend/scripting/engine.go` |
| Wake-on-LAN | Sends WOL magic packets | `control-center/backend/wol/wol.go` |
| Shared wire protocol (control-center copy) | Message/payload struct definitions mirroring the agent's protocol | `control-center/backend/protocol/types.go` |
| Shared wire protocol (agent copy) | Message/payload struct definitions | `agent/internal/protocol/types.go` |
| Frontend state stores | Svelte writable/derived stores for agents, sessions, UI view state | `control-center/frontend/src/lib/stores/*.ts` |
| Frontend API layer | Wraps Wails auto-generated bindings, normalizes snake_case→camelCase | `control-center/frontend/src/lib/utils/api.ts` |
| Frontend UI components | Dashboard, Terminal, FileBrowser, MultiExec, ScriptEditor, AuditLog, Settings, Sidebar | `control-center/frontend/src/lib/components/*.svelte` |

## Pattern Overview

**Overall:** Client-server agent architecture with a message-passing wire protocol (WebSocket + JSON envelopes), fronted by a Wails desktop shell that exposes Go backend methods directly to a Svelte SPA via generated JS bindings.

**Key Characteristics:**
- Two independently buildable Go modules (`agent/`, `control-center/`) each with their own `go.mod`, connected only by a shared JSON wire protocol (duplicated, not imported, across `agent/internal/protocol/types.go` and `control-center/backend/protocol/types.go`)
- Every WebSocket message is a `protocol.Message{ID, Type, Payload, Timestamp, Error}` envelope; request/response correlation on the control-center side is done by matching `msg.ID` against a `pending` map of channels (`control-center/backend/client/client.go:56-71`)
- The Wails `App` struct (`control-center/app.go`) is the single seam between frontend and backend — every public method becomes a callable binding; there is no HTTP API surface exposed to the frontend
- The frontend never talks to agents directly; the WebSocket connection lives entirely in the Go backend, and the frontend polls Go-bound methods (`GetAgents`, `GetSystemInfo`) on a 2-second interval (`control-center/frontend/src/App.svelte:16-59`)
- SQLite is used for two independent local stores (sessions, audit log), each with its own manager type and no shared schema

## Layers

**Agent transport layer:**
- Purpose: Accept WebSocket connections, authenticate, encode/decode `protocol.Message` frames
- Location: `agent/internal/server/`
- Contains: `Server`, `Client`, per-connection read/write pumps
- Depends on: `agent/internal/protocol`, `agent/internal/system` (for periodic push)
- Used by: `agent/cmd/lan-agent/main.go`

**Agent capability modules:**
- Purpose: Implement individual remote-management operations (exec, filesystem, screenshot, system info)
- Location: `agent/internal/{executor,filesystem,screenshot,system}/`
- Contains: Pure Go functions with no protocol awareness — handlers translate protocol payloads to/from these calls
- Depends on: OS-level APIs, `os/exec`
- Used by: `agent/internal/server/handlers.go`

**Control-center backend managers:**
- Purpose: Own long-lived state (connections, sessions, audit log, scripts) and expose it through the `App` struct
- Location: `control-center/backend/{client,discovery,session,audit,scripting,wol}/`
- Contains: One manager type per concern, each independently constructed in `App.startup`
- Depends on: `control-center/backend/protocol`
- Used by: `control-center/app.go`

**Wails binding layer:**
- Purpose: Translate frontend calls into backend manager calls; the only layer aware of both sides
- Location: `control-center/app.go`
- Contains: One exported method per frontend-visible operation (`ConnectAgent`, `ExecCommand`, `TransferFile`, `RunScript`, etc.), each wrapping the call with audit logging
- Depends on: All backend managers
- Used by: `control-center/main.go` (via `wails.Run(options.App{Bind: []interface{}{app}}))`), and indirectly the frontend via generated bindings

**Frontend state layer:**
- Purpose: Hold client-side reactive state (agent list, selection, UI mode, sessions)
- Location: `control-center/frontend/src/lib/stores/`
- Contains: Svelte `writable`/`derived` stores, plain TS interfaces (no framework-specific DTOs)
- Depends on: `control-center/frontend/src/lib/utils/api.ts`
- Used by: `control-center/frontend/src/App.svelte` and all `lib/components/*.svelte`

**Frontend API layer:**
- Purpose: Single choke point for calling into Go; normalizes casing mismatch between Go JSON tags (snake_case) and TS conventions (camelCase)
- Location: `control-center/frontend/src/lib/utils/api.ts`
- Contains: `callBinding()` wrapper + one typed function per Go binding
- Depends on: `control-center/frontend/wailsjs/go/main/App.js` (generated)
- Used by: Components and stores

**Frontend components:**
- Purpose: Render views (Dashboard, Terminal, FileBrowser, MultiExec, ScriptEditor, AuditLog, Settings, Sidebar)
- Location: `control-center/frontend/src/lib/components/`
- Contains: Svelte 5 components using runes (`$state`, `$effect`)
- Depends on: stores, API layer
- Used by: `control-center/frontend/src/App.svelte`

## Data Flow

### Command execution path (control center → agent)

1. User submits a command in the Terminal component (`control-center/frontend/src/lib/components/Terminal.svelte`)
2. Frontend calls `execCommand(agentId, command, timeout)` (`control-center/frontend/src/lib/utils/api.ts:64-66`), which invokes the Wails binding `App.ExecCommand`
3. `App.ExecCommand` (`control-center/app.go:309-331`) logs the audit entry, then calls `a.requestCommandResult`
4. `requestCommandResult` (`control-center/app.go:780-793`) builds an `ExecCommandPayload` and calls `clientMgr.SendAndParse`
5. `Manager.SendRequest` (`control-center/backend/client/client.go:276-323`) generates a message ID, registers a `pendingRequest` channel, sends the JSON frame over the WebSocket, and blocks on the channel or timeout
6. Agent's `Server.handleWebSocket`/`readPump` receives the frame (`agent/internal/server/server.go:212-249`), dispatches via `handleMessage` (`agent/internal/server/handlers.go:14-49`) to `handleExecCommand`
7. `handleExecCommand` calls `executor.Execute` (`agent/internal/executor/executor.go`) and sends a `command_result` response with the same message ID
8. Control-center's `readPump` (`control-center/backend/client/client.go:415-479`) matches the response by `msg.ID` against `pending`, delivers it to the waiting channel, unblocking step 4
9. Result flows back up through the binding to the frontend, which renders it in the Terminal

### System info polling (push + pull, dual path)

1. **Push path:** Agent's `writePump` ticks every `PushInterval` (2s) and calls `pushSystemUpdate` (`agent/internal/server/server.go:33,251-289`), sending unsolicited `system_update` messages
2. Control-center's `readPump` intercepts `system_update` messages internally via `handleSystemUpdate` (`control-center/backend/client/client.go:508-534`), updating `AgentConnection.info.SystemInfo` in place — these are not correlated to a pending request and are also forwarded to `onAgentMessage` for audit logging
3. **Pull path:** Frontend's `App.svelte` polling loop (`control-center/frontend/src/App.svelte:19-59`) calls `getSystemInfo(agentId)` every 2s for each connected agent, merging results into the `agents` store while preserving `cpuHistory`

**State Management:**
- Backend: in-memory `Manager.agents map[string]*AgentConnection` guarded by `sync.RWMutex`; durable state (sessions, audit log) persisted to SQLite files in the OS user-config directory (`control-center/app.go:50-71`)
- Frontend: Svelte stores (`agents`, `selectedAgentId`, `sessions`, `ui`) are the sole source of truth for rendering; refreshed by polling, not push/subscribe

## Key Abstractions

**protocol.Message:**
- Purpose: Universal envelope for every WebSocket frame in both directions
- Examples: `agent/internal/protocol/types.go`, `control-center/backend/protocol/types.go` (duplicated definitions, must be kept in sync manually)
- Pattern: `{ID, Type, Payload interface{}, Timestamp, Error}`; `Type` is a string constant (`exec_command`, `command_result`, `system_update`, etc.); `Payload` is double-marshaled (`json.Marshal` then `json.Unmarshal` into a typed struct) at each handler boundary

**Manager (client.go):**
- Purpose: Central registry of live agent connections with request/response correlation
- Examples: `control-center/backend/client/client.go:63-83`
- Pattern: Map of `agentID -> *AgentConnection`, plus a separate `pending map[msgID]*pendingRequest` for in-flight requests; every `SendRequest` call races a response channel against a `time.Timer`

**AgentConnection:**
- Purpose: Per-agent WebSocket state with reconnect support
- Examples: `control-center/backend/client/client.go:43-50`
- Pattern: Wraps `*websocket.Conn` with a cancelable context; on read error, spawns `reconnect()` which retries up to `maxReconnectRetries` with `reconnectDelay` backoff and re-runs the auth handshake before installing the new connection

**Wails Binding Method:**
- Purpose: Uniform frontend-callable operation shape
- Examples: All exported methods on `App` in `control-center/app.go`
- Pattern: Validate inputs → call backend manager → `audit.Log(...)` on both success and failure → return `(result, error)`

## Entry Points

**Agent binary:**
- Location: `agent/cmd/lan-agent/main.go`
- Triggers: OS process start; CLI flags select service verb (`install`/`uninstall`/`start`/`stop`/`restart`) vs. foreground run vs. `--ui` desktop mode
- Responsibilities: Parse flags, construct `server.NewServer`, optionally start mDNS advertisement (`agent/internal/discovery`), block on `srv.Start(ctx)`; when run as a service, implements `service.Interface` (`kardianos/service`) so the same binary installs as a Windows Service or systemd unit

**Control-center desktop app:**
- Location: `control-center/main.go`
- Triggers: OS process start (double-click / installer shortcut)
- Responsibilities: Embed the built frontend (`//go:embed all:frontend/dist`), construct `NewApp()`, call `wails.Run` with `OnStartup`/`OnShutdown` hooks and the `App` struct bound for JS access

**App.startup:**
- Location: `control-center/app.go:74-142`
- Triggers: Called once by the Wails runtime after the window is created
- Responsibilities: Resolve per-user data directory, open SQLite session/audit DBs, construct the script engine, WOL sender, client manager, and discovery service; restore and reconnect saved sessions in a background goroutine; start mDNS discovery

## Architectural Constraints

- **Threading:** Agent server uses one goroutine pair (`readPump`/`writePump`) per client connection plus a shared `system.Monitor`; control-center uses one goroutine pair per agent connection (`readPump`/`heartbeat`) inside `Manager`, plus ad-hoc goroutines for multi-agent parallel exec (`ExecCommandMulti`, `control-center/app.go:336-378`) using `sync.WaitGroup`
- **Global state:** `Manager.agents`, `Manager.pending` (client.go) are the only long-lived shared mutable maps; both are mutex-protected. `Server.clients` (agent/internal/server/server.go) is similarly guarded
- **Protocol duplication:** The wire protocol is defined twice — `agent/internal/protocol/types.go` and `control-center/backend/protocol/types.go` — with no shared module. Message type constants and payload structs must be changed in both places in lockstep or messages silently fail to decode
- **No shared Go module:** `agent/` and `control-center/` are separate Go modules (each has its own `go.mod`/`go.sum`); no code is shared via import, only via convention
- **Frontend has a parallel "backup" tree:** `control-center/frontend/src/lib/backup/` mirrors `lib/components/` and `lib/utils/api.ts`/`lib/stores/` files; only `lib/components/`, `lib/stores/`, `lib/utils/` under the non-backup paths are wired into `App.svelte`

## Anti-Patterns

### Duplicated wire protocol definitions

**What happens:** `MsgExecCommand`, `AuthPayload`, `SystemInfoPayload`, etc. are defined independently in `agent/internal/protocol/types.go` and `control-center/backend/protocol/types.go`.
**Why it's wrong:** Adding or renaming a message type or field in one file without updating the other causes silent JSON decode failures at runtime (fields become zero-valued instead of erroring), since both sides use `json.Marshal`/`json.Unmarshal` through `interface{}` payloads.
**Do this instead:** When adding/changing a protocol message, update both `agent/internal/protocol/types.go` and `control-center/backend/protocol/types.go` in the same change, and grep for the message type string in both handler files (`agent/internal/server/handlers.go`, `control-center/app.go`) to confirm both sides handle it.

### Stale "backup" component tree in frontend

**What happens:** `control-center/frontend/src/lib/backup/` contains near-duplicate `.svelte`/`.ts` files (`App.svelte`, `Dashboard.svelte`, `agents.ts`, etc.) alongside the live `lib/components/`, `lib/stores/` trees.
**Why it's wrong:** Editing a file in `lib/backup/` has no effect on the running app (only `lib/components/*` is imported by `App.svelte`), which risks wasted edits or confusion about which copy is authoritative.
**Do this instead:** Treat `control-center/frontend/src/lib/backup/` as dead code; make all UI changes under `control-center/frontend/src/lib/components/`, `lib/stores/`, `lib/utils/`. Confirm the active import graph by checking `App.svelte`'s imports before editing.

## Error Handling

**Strategy:** Errors are returned as Go `error` values throughout the backend; at the WebSocket boundary they are converted to `protocol.Message{Type: "error", Error: string}` frames rather than being thrown. Wails bindings return `(T, error)` tuples, which surface as rejected Promises in the frontend.

**Patterns:**
- Agent handlers call `c.sendError(msg.ID, "...")` on any failure (`agent/internal/server/handlers.go`), always including the original request ID so the control-center's `pending` map can resolve the waiting caller with an error
- `Manager.SendRequest` treats a response with a non-empty `Error` field as a Go error (`control-center/backend/client/client.go:313-317`)
- `App` binding methods wrap every operation with `a.audit.Log(action, agentID, "user"/"system", detail, audit.StatusSuccess|StatusError)` both on success and failure paths (see every method in `control-center/app.go`)
- File transfer (`TransferFile`, `control-center/app.go:447-555`) writes to a temp file and only `os.Rename`s into place after a full SHA-256 checksum match, to avoid partial/corrupt downloads on error

## Cross-Cutting Concerns

**Logging:** Standard library `log` package throughout both binaries, prefixed by component tag (e.g. `[server]`, `[client]`, `app:`); no structured logging or log levels.

**Validation:** Manual input validation at the top of each `App` binding method and each protocol handler (empty-string/range checks); no schema/validation library used.

**Authentication:** Optional shared-secret token (`--auth-token` flag on the agent). If set, the agent greets new connections with `auth_required`; the client must send an `auth` message with a matching token before any other message type is processed (`agent/internal/server/handlers.go:14-31,51-80`). Sessions persist the token in SQLite (`control-center/backend/session/session.go`) so saved connections can reconnect automatically.

---

*Architecture analysis: 2026-08-27*
