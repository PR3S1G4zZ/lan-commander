# Codebase Concerns

**Analysis Date:** 2026-08-27

## Testing Readiness: NOT READY

This project is **not ready to enter a formal testing phase**. There is effectively zero automated test coverage, no CI pipeline, and a documented verification ledger that does not match the actual repository state.

**What's missing:**
- No CI configuration anywhere in the repo (no `.github/workflows`, no `.gitlab-ci.yml`, no Azure Pipelines, nothing). `find . -path "*/.github/*"` and `find . -iname "*.yml"` both return empty.
- Only one Go test file in the entire codebase: `agent/internal/ui/probe_test.go` (23 lines). Every other Go package — `agent/internal/server/`, `agent/internal/executor/`, `agent/internal/filesystem/`, `agent/internal/discovery/`, `agent/internal/system/`, `control-center/backend/client/`, `control-center/backend/session/`, `control-center/backend/audit/`, `control-center/backend/scripting/`, `control-center/backend/discovery/`, `control-center/backend/wol/`, `control-center/app.go` — has **no tests**.
- The frontend (`control-center/frontend/`) has **zero test files** (no `*.test.ts`, no `*.spec.ts`) and no `test` script in `control-center/frontend/package.json` (scripts are only `dev`, `build`, `preview`, `check`). There is no Vitest/Jest config file present.
- No `.eslintrc*`, `.prettierrc*`, or `biome.json` found in `control-center/frontend/` — no enforced lint/format standard beyond `svelte-check`.

**Documentation/reality mismatch (high priority to investigate):**
`.superpowers/sdd/lan-commander-completion/progress.md` claims a "Final verification evidence" section stating:
- `go test ./...`, `go test -race ./...`, `go vet ./...` pass for both `agent` and `control-center`
- `npm test` (5 tests), `npm run check`, `npm run build` pass for the frontend
- A `securestore` package handles TLS/DPAPI credential storage
- A release manifest with five SHA-256-verified artifacts was generated

None of this is true of the current working tree: there are no Go tests beyond `probe_test.go`, no `npm test` script exists, no `securestore` package exists anywhere in the repo (`find . -iname "*securestore*"` returns nothing), and `git log --diff-filter=D` shows no test files were ever deleted. This strongly suggests either (a) the ledger describes work done in a separate branch/worktree that was never merged, or (b) the ledger is aspirational/fabricated. **This must be reconciled before trusting any "verified" claims in project documentation.** Treat `progress.md` as unreliable until cross-checked against actual git history.

**Before entering a testing phase, prioritize:**
1. Add unit tests for `agent/internal/executor/`, `agent/internal/filesystem/` (path safety), and `agent/internal/server/` auth handling — these are the highest-risk, security-relevant modules.
2. Add a `test` script and at least smoke tests for `control-center/frontend/src/lib/stores/` and `control-center/frontend/src/lib/utils/`.
3. Stand up a CI workflow (`go test ./...`, `go vet ./...`, `npm run check`, `npm run build`) so regressions are caught automatically — currently everything is manual.
4. Reconcile `.superpowers/sdd/lan-commander-completion/progress.md` against reality; remove or correct false verification claims.

## Security Considerations

**Unauthenticated agent access is the default:**
- Risk: `agent/cmd/lan-agent/main.go` defaults `-auth-token` to `""`. In `agent/internal/server/handlers.go` (`handleMessage`), when `s.authToken == ""` the server marks every connecting client `authed = true` immediately (see `agent/internal/server/server.go:167-169`). Any device on the LAN (or routed network) that can reach the agent's WebSocket port can execute arbitrary shell commands, read/write arbitrary files, and capture screenshots with no credential at all.
- Files: `agent/cmd/lan-agent/main.go`, `agent/internal/server/server.go`, `agent/internal/server/handlers.go`
- Current mitigation: none by default; operator must explicitly pass `-auth-token`.
- Recommendation: require a non-empty auth token by default (fail to start, or generate+print a random token on first run) rather than silently allowing unauthenticated access.

**Auth token comparison is not constant-time:**
- Risk: `agent/internal/server/handlers.go:65` compares `auth.Token != c.server.authToken` using Go's native `!=` string comparison, which short-circuits on first differing byte. This is a timing side-channel that could theoretically help an attacker brute-force the token over many attempts, especially since there is no rate limiting on auth attempts.
- Files: `agent/internal/server/handlers.go`
- Recommendation: use `crypto/subtle.ConstantTimeCompare` and add a basic rate limiter / backoff on repeated failed auth attempts per connection/IP.

**Control-center never actually connects over TLS ("wss://"):**
- Risk: The agent server supports TLS (`agent/internal/server/server.go` `createListener()` loads cert/key and can `tls.Listen`), but the control-center client (`control-center/backend/client/client.go:101-116` and the reconnect path around line 587-591) always builds the WebSocket URL with `Scheme: "ws"` — never `"wss"`. This means even when an operator configures TLS on the agent, the actual desktop client shipped in this repo has no code path that uses it. All traffic — including the auth token sent in the `auth` message and all command/file transfer payloads — travels as plaintext WebSocket frames on the LAN.
- Files: `control-center/backend/client/client.go`, `agent/internal/server/server.go`
- Recommendation: add a `useTLS`/`wss` option wired through `Connect()`, and use `tls.Config` (with certificate pinning or a user-approved fingerprint, since these are typically self-signed LAN certs) on the dialer.

**Auth tokens stored in plaintext:**
- Risk: `control-center/backend/session/session.go` persists `AuthToken` as a plain `TEXT` column in a local SQLite database (`lan-commander.db`) with no encryption. Anyone with filesystem access to the operator's machine (or a backup of it) can read every saved agent's credentials.
- Files: `control-center/backend/session/session.go`
- Recommendation: encrypt tokens at rest (e.g., OS keychain/DPAPI, or an app-level encryption key), especially since the `progress.md` ledger references a `securestore`/DPAPI mechanism that does not actually exist in the code.

**Filesystem access has no root confinement:**
- Risk: `agent/internal/filesystem/fs.go` `safePath()` only rejects paths containing `..` and cleans/absolutizes the path — it does not confine access to any allowed root directory. Once authenticated (or if auth is disabled, as above), a client can read/write/list any file the agent process's OS user can access anywhere on disk (e.g., `C:\Windows\System32\config`, SSH keys, browser profiles), not just an intended "shared" directory.
- Files: `agent/internal/filesystem/fs.go`, `agent/internal/server/handlers.go` (`handleListDir`, `handleGetFile`, `handleSendFile`)
- Recommendation: introduce a configurable allowed-roots list and reject paths outside it, in addition to the existing traversal check.

**Arbitrary command execution via `executor.Execute`:**
- Risk: `agent/internal/executor/executor.go` builds a full shell command line by joining `cmd` and `args` with spaces and passing the whole thing to `powershell -Command` / `cmd.exe /c` / `/bin/bash -c`. There is no allowlist, sandboxing, or restriction on what commands can run — by design this is a full remote-shell feature, but it means a compromised or malicious client (especially combined with the no-auth-by-default issue above) has unrestricted code execution on the host.
- Files: `agent/internal/executor/executor.go`
- Impact: this is core product functionality, not incidental, but it raises the stakes of the auth gaps above considerably — this is effectively an unauthenticated RCE service in default configuration.

**CORS/Origin check disabled on WebSocket upgrade:**
- Risk: `agent/internal/server/server.go:74-76` — `CheckOrigin: func(r *http.Request) bool { return true }` accepts WebSocket upgrades from any origin. Combined with the default no-auth-token setup, a malicious webpage loaded in a browser on the same LAN could open a WebSocket to the agent and issue commands (a CSWSH — cross-site WebSocket hijacking — style attack), since browsers do not block cross-origin WebSocket connections by default.
- Files: `agent/internal/server/server.go`
- Recommendation: validate `Origin` header or bind the WebSocket server to only accept connections initiated by the known control-center client (e.g., a custom header/handshake secret), not just any WebSocket-capable caller.

**Script variable substitution uses `text/template` directly on shell input:**
- Risk: `control-center/backend/scripting/engine.go` `processVariables()` parses the raw script line as a Go `text/template` and executes it with user-supplied `vars`. If a variable value itself contains `{{...}}` template syntax, or if the script content includes uncontrolled template actions, this could allow unexpected template execution/expansion before the string is handed to a shell. It's a `text/template` (not `html/template`), which has no escaping semantics appropriate for shell contexts, so this is really just string templating with a Turing-incomplete surface, but it's still worth validating/sanitizing variable values before injecting them into a command string ultimately passed to `bash -c` / `powershell -Command`.
- Files: `control-center/backend/scripting/engine.go`
- Recommendation: escape or reject variable values containing shell metacharacters before substitution, or move to a placeholder-based (non-template-engine) substitution to reduce surface area.

## Tech Debt

**Duplicate/backup frontend source tree left in the repository:**
- Issue: `control-center/frontend/src/lib/backup/` contains a full parallel copy of nearly every component in `control-center/frontend/src/lib/components/` (`App.svelte`, `AuditLog.svelte`, `ConnectionDialog.svelte`, `Dashboard.svelte`, `FileBrowser.svelte`, `MultiExec.svelte`, `Notifications.svelte`, `ScriptEditor.svelte`, `Settings.svelte`, `Sidebar.svelte`, `Terminal.svelte`, plus `agents.ts`, `api.ts`, `format.ts`, `sessions.ts`, `ui.ts`). It is unclear whether this is dead code, a rollback safety net, or an artifact of the delegated-agent workflow described in `progress.md`.
- Files: `control-center/frontend/src/lib/backup/*`
- Impact: doubles the frontend surface area for anyone reading the codebase, risks accidental imports from the wrong tree, and bloats the repo/diff noise.
- Fix approach: delete `backup/` if it is confirmed superseded by `components/`, or move it out of the source tree (e.g., into a `docs/` reference or git history) if it must be retained for reference.

**Build artifacts committed to the repository:**
- Issue: `agent/build/lan-agent-linux` and `agent/build/lan-agent.exe` are compiled binaries checked directly into version control, alongside `control-center/frontend/dist/` build output (`App-*.js`, `index-*.js`, `index-*.css`).
- Files: `agent/build/lan-agent-linux`, `agent/build/lan-agent.exe`, `control-center/frontend/dist/assets/*`
- Impact: bloats repo size over time, risks stale/mismatched binaries being mistaken for current builds, and complicates diffing.
- Fix approach: add these paths to `.gitignore` and rely on the release pipeline (`scripts/`) to produce them on demand.

**No dependency lockfile verification / update process visible:**
- Issue: `agent/go.sum` and `control-center/frontend/package-lock.json` exist, but there's no automated process (CI) that verifies `go.mod`/`go.sum` integrity or runs `npm audit` / `go list -m -u` to catch outdated or vulnerable dependencies.
- Files: `agent/go.mod`, `agent/go.sum`, `control-center/go.mod`, `control-center/frontend/package.json`, `control-center/frontend/package-lock.json`
- Fix approach: add a periodic dependency-audit step once CI is introduced.

**In-memory audit log capped at 1000 entries with silent eviction:**
- Issue: `control-center/backend/audit/audit.go` `Logger` keeps a ring buffer of at most `capacity` (default 1000) entries in memory; once exceeded, the oldest entries are silently dropped from the in-memory slice (though SQLite persistence via `OpenDB`/`dbEnabled` retains full history if enabled). If `OpenDB` is never called, audit history beyond the last 1000 actions is permanently lost with no warning to the operator.
- Files: `control-center/backend/audit/audit.go`
- Impact: a long-running session could silently lose early audit trail entries if DB persistence isn't explicitly wired up by the caller.
- Fix approach: verify `control-center/app.go` always calls `OpenDB` on startup, or surface a UI warning when audit persistence is disabled.

**No rate limiting or backpressure beyond fixed buffer sizes:**
- Issue: `agent/internal/server/server.go` uses a fixed `send chan []byte, 64` per client (`Client.send`) and silently drops messages when full (`sendMsg`, "Send buffer full, dropping message"). There's no mechanism to detect or recover from a client that's falling behind other than silent data loss — a dropped `command_result` or `file_chunk` message could leave the control-center UI in an inconsistent state (e.g., a stuck "in progress" transfer) with no timeout/retry visible in `client.go`.
- Files: `agent/internal/server/server.go`, `control-center/backend/client/client.go`
- Impact: potential silent hangs in file transfers or command execution under load or on a congested LAN link.
- Fix approach: add explicit timeout/retry handling on the control-center side for pending requests that never receive a final response, and consider surfacing dropped-message conditions back to the client instead of only logging server-side.

## Fragile Areas

**`control-center/app.go` (804 lines) is a large, monolithic Wails binding surface:**
- Files: `control-center/app.go`
- Why fragile: as the single Go↔frontend bridge, this file likely aggregates session, client, audit, scripting, and WoL orchestration in one place (largest Go file in the project by a wide margin — next largest is `client.go` at 622 lines). Changes here have a wide blast radius across the entire UI.
- Safe modification: read the full file before editing; consider decomposing into per-domain binding files (session bindings, transfer bindings, script bindings) if it continues to grow.
- Test coverage: none — this file has zero associated tests, so regressions here are only caught by manual UAT.

**`control-center/backend/client/client.go` (622 lines) manages all agent WebSocket lifecycle, reconnection, and pending-request tracking:**
- Files: `control-center/backend/client/client.go`
- Why fragile: implements manual reconnect logic (`maxReconnectRetries`, `reconnectDelay`) and a pending-request map (`pendingMu`, `pending`) with per-request timers — this kind of hand-rolled concurrent state machine is a common source of goroutine leaks, deadlocks, or races, and it has zero automated race-detector coverage (`go test -race` is claimed in `progress.md` but no test files exist to run it against).
- Safe modification: add tests before modifying reconnect/timeout logic; run `go run -race` manually against real agent connections when touching this file until real tests exist.

## Missing Critical Features / Gaps

**No CI/CD automation of any kind:**
- Problem: every verification step (build, test, lint) described in `progress.md` is manual and unverifiable from the repository alone. There's no `.github/workflows`, no pre-commit hook config, nothing that runs on push/PR.
- Blocks: confident iteration — every change currently relies entirely on the developer remembering to run `go build`, `go vet`, `npm run check`, and `npm run build` locally before committing.

**No `.gitignore` coverage for build outputs (partially):**
- Problem: despite a `.gitignore` at the repo root and one in `control-center/.gitignore`, compiled binaries (`agent/build/lan-agent.exe`, `agent/build/lan-agent-linux`) and frontend `dist/` output are present in the tracked tree (see Tech Debt above).
- Blocks: clean diffs and accurate size tracking of the actual source contribution.

## Test Coverage Gaps

**Agent core (all untested):**
- What's not tested: `agent/internal/executor/` (command execution, timeout enforcement, output capping), `agent/internal/filesystem/` (path safety/traversal protection — the single most security-critical function, `safePath()`), `agent/internal/server/` (auth handshake, message routing, WebSocket lifecycle), `agent/internal/discovery/` (mDNS advertisement), `agent/internal/system/` (system info monitoring).
- Files: `agent/internal/executor/executor.go`, `agent/internal/filesystem/fs.go`, `agent/internal/server/server.go`, `agent/internal/server/handlers.go`, `agent/internal/discovery/mdns.go`, `agent/internal/system/monitor.go`
- Risk: these modules mediate remote code execution and filesystem access — the highest-impact code in the project — with zero regression protection.
- Priority: High

**Control-center backend (all untested):**
- What's not tested: `control-center/backend/client/client.go` (connection/reconnect/pending-request logic), `control-center/backend/session/session.go` (SQLite persistence), `control-center/backend/audit/audit.go` (ring buffer + DB dual-write), `control-center/backend/scripting/engine.go` (variable templating, script execution loop), `control-center/backend/discovery/discover.go` (mDNS parsing), `control-center/backend/wol/wol.go` (Wake-on-LAN packet construction), `control-center/app.go` (Wails bindings).
- Files: entire `control-center/backend/` tree and `control-center/app.go`
- Risk: session/audit persistence bugs would be silent (SQLite errors are only logged, not surfaced), and scripting engine bugs could produce unexpected shell commands.
- Priority: High

**Frontend (entirely untested):**
- What's not tested: all Svelte components and stores in `control-center/frontend/src/lib/components/` and `control-center/frontend/src/lib/stores/` — no unit or component tests exist, and no test runner is configured.
- Files: `control-center/frontend/src/lib/**`
- Risk: UI regressions (e.g., broken file transfer progress, script editor state) would only be caught by manual clicking.
- Priority: Medium (lower than backend RCE/filesystem paths, but still a gap before any "testing phase" claim is credible)

---

*Concerns audit: 2026-08-27*
