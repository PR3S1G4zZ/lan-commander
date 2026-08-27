# Coding Conventions

**Analysis Date:** 2026-08-27

## Naming Patterns

**Files:**
- Go: lowercase, single-word package-matching names — `client.go`, `handlers.go`, `types.go`, `executor.go`, `monitor.go` in `agent/internal/*` and `control-center/backend/*`
- Svelte components: PascalCase — `control-center/frontend/src/lib/components/Dashboard.svelte`, `Sidebar.svelte`, `ConnectionDialog.svelte`
- TS modules: lowercase, purpose-named — `control-center/frontend/src/lib/utils/api.ts`, `control-center/frontend/src/lib/stores/agents.ts`
- A `backup/` mirror directory exists under `control-center/frontend/src/lib/backup/` containing older/duplicate copies of components (e.g. `backup/App.svelte`, `backup/agents.ts`) — do not edit these; they are stale duplicates, not backups managed by tooling. When adding new code, always work in `lib/components/`, `lib/stores/`, `lib/utils/`, never `lib/backup/`.

**Go identifiers:**
- Exported types/functions: PascalCase — `AgentInfo`, `Manager`, `NewManager`, `Execute`, `ListDir`
- Unexported: camelCase — `handshake`, `readPump`, `writeJSON`, `cappedWriter`
- Constants grouped in `const (...)` blocks with doc comments, e.g. `agent/internal/executor/executor.go:15-20` (`MaxOutputSize`, `MaxTimeout`), `control-center/backend/client/client.go:19-26` (timeouts/retry constants)
- Errors as sentinel vars: `var ErrAuthRequired = errors.New(...)` (`control-center/backend/client/client.go:164`)

**TypeScript identifiers:**
- Interfaces: PascalCase — `AgentInfo`, `SystemInfo`, `CommandResult` (`control-center/frontend/src/lib/stores/agents.ts`)
- Functions/variables: camelCase — `getAgents`, `connectAgent`, `normalizeAgent`
- Svelte 5 runes used for local state: `let x = $state(...)`, `const y = $derived(...)` (see `control-center/frontend/src/lib/components/Sidebar.svelte:8-17`)

**JSON wire format vs TS/Go field names:**
- Go structs use snake_case JSON tags (`last_seen`, `system_info`, `exit_code`) throughout `agent/internal/protocol/types.go` and `control-center/backend/protocol/types.go`.
- TypeScript store interfaces mix both: some fields keep snake_case to mirror the wire payload directly (`fs_type`, `is_dir`, `exit_code`, `agent_version` in `control-center/frontend/src/lib/stores/agents.ts`), while UI-facing convenience fields use camelCase (`lastSeen`, `systemInfo`, `cpuHistory` on `AgentInfo`).
- The boundary-normalization pattern lives in `control-center/frontend/src/lib/utils/api.ts:34-47` (`normalizeAgent`): raw Wails/Go response objects are explicitly mapped field-by-field into the camelCase `AgentInfo` shape, accepting either `raw.last_seen` or `raw.lastSeen` as input. Follow this pattern for any new response type that crosses the Go→JS boundary — do not let snake_case leak into components uncontrolled.

## Code Style

**Formatting:**
- Go: standard `gofmt` formatting (tabs for indentation, standard brace placement). No `.golangci.yml` or custom linter config found — relies on `go vet`/`gofmt` defaults.
- TypeScript/Svelte: tabs for indentation (see `Sidebar.svelte`, `agents.ts`). No ESLint or Prettier config file present in `control-center/frontend/` — no automated style enforcement; match existing tab-indented style manually.

**Linting:**
- No `.eslintrc*`, `.prettierrc*`, `biome.json`, or `.golangci*` files found anywhere in the repo. Style consistency is maintained by convention only — inspect neighboring files before writing new code.

## Import Organization

**Go:**
- Standard library imports first, blank line, then third-party, then blank line, then internal `github.com/mediacode/lan-commander/...` or `control-center/backend/...` packages. Example: `control-center/backend/client/client.go:3-17` (stdlib → `control-center/backend/protocol` → `github.com/google/uuid`, `github.com/gorilla/websocket`).
- Internal packages for the agent live under module path `github.com/mediacode/lan-commander/agent/internal/...` (see `agent/go.mod`); control-center backend uses local module `control-center/backend/...` (see `control-center/go.mod`) — these are two separate Go modules, not one monorepo module.

**TypeScript:**
- Type-only imports use `import type { ... }` (e.g. `control-center/frontend/src/lib/utils/api.ts:7`).
- Relative imports only; no path aliases configured in `tsconfig.json`.
- Svelte components import stores first, then utils, then child components (see `Sidebar.svelte:2-6`).

## Error Handling

**Go:**
- Errors wrapped with `fmt.Errorf("...: %w", err)` to preserve context and support `errors.Is`/`errors.As` — used consistently in `control-center/backend/client/client.go` (`Connect`, `handshake`, `SendRequest`) and `agent/internal/executor/executor.go` (`ValidateShell`).
- WebSocket message handlers in `agent/internal/server/handlers.go` follow a strict pattern per handler: marshal payload → unmarshal into typed struct → on any error call `c.sendError(msg.ID, "...")` and `return` early. New message handlers should follow this exact 4-step shape (see `handleExecCommand`, `handleListDir`, `handleGetFile`, `handleSendFile` for the template).
- Sentinel errors exported for callers to check via `errors.Is` (e.g. `client.ErrAuthRequired`).
- Comments above non-obvious error-handling decisions explain *why*, not just *what* — e.g. the handshake-before-readPump comment in `control-center/backend/client/client.go:121-124` and `:166-168`.

**TypeScript:**
- `api.ts` throws plain `Error` objects with descriptive messages when a Wails binding is missing (`control-center/frontend/src/lib/utils/api.ts:20-25`).
- Dynamic import of generated Wails bindings wrapped in `try/catch` with a `console.warn` fallback so dev-mode works before bindings are generated (`control-center/frontend/src/lib/utils/api.ts:12-18`).

## Comments

**When to Comment:**
- Package-level and exported-symbol doc comments follow Go convention (`// FuncName does X.`) throughout `agent/internal/*` and `control-center/backend/*`.
- Non-obvious concurrency/ordering decisions get an explanatory comment block directly above the code (see the handshake-ordering comments in `control-center/backend/client/client.go`).
- TSDoc-style `/** ... */` block used at file top of `api.ts` to describe module purpose.

**Inline field comments (Go):**
- Struct fields frequently carry an inline comment explaining units or defaults, e.g. `Timeout int json:"timeout,omitempty"` `// seconds, 0 = no timeout` (`agent/internal/protocol/types.go:46`), `ChunkSize int` `// default 64KB` (`:57`).

## Function Design

**Go:**
- Handler functions in `agent/internal/server/handlers.go` are short (10-40 lines), single-purpose, and named `handleXxx`.
- `Manager` methods in `client.go` favor small, focused methods (`Connect`, `Disconnect`, `SendMessage`, `SendRequest`, `SendAndParse`) rather than one large orchestrator.

**TypeScript:**
- `api.ts` functions are thin one-line wrappers around `callBinding(...)` — no business logic in the API layer; logic belongs in Go backend or Svelte components/stores.

## Module Design

**Go:**
- Package-per-concern under `internal/`: `discovery`, `executor`, `filesystem`, `protocol`, `screenshot`, `server`, `system`, `ui` (agent); `audit`, `client`, `discovery`, `protocol`, `scripting`, `session`, `wol` (control-center backend).
- `protocol` package is duplicated independently in both modules (`agent/internal/protocol/types.go` and `control-center/backend/protocol/types.go`) rather than shared — message type constants and payload structs must be kept in sync manually when the wire protocol changes.

**TypeScript:**
- Svelte stores (`lib/stores/*.ts`) hold shared reactive state and TypeScript interfaces mirroring the Go payload shapes.
- `lib/utils/api.ts` is the sole boundary layer to the Go backend; components should call these wrapper functions rather than importing Wails bindings directly.

---

*Convention analysis: 2026-08-27*
