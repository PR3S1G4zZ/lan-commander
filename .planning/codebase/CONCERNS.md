# Codebase Concerns

**Analysis Date:** 2026-08-27
<!-- refreshed: 2026-08-27 (post-merge of origin/main PR #1 + local security hardening) -->

## Testing Readiness: READY (with residual gaps noted below)

The picture in the previous version of this document (based on the `main` branch before it was synced with `origin/main`) was stale. `main` was 2 commits behind `origin/main`, missing an already-merged PR (`#1`, commits `6d78226` + `2b57b4d`) that added a full test suite, CI, and several security fixes. After merging (`7f826c6`) and applying two follow-up fixes locally (`7054264`, `4d30c29`), the project has been verified end-to-end on this machine:

```
agent:            go test ./... -count=1   PASS
                  go test -race ./...      PASS
                  go vet ./...             clean
control-center:   go test ./... -count=1   PASS
                  go test -race ./...      PASS
                  go vet ./...             clean
frontend:         npm test (5 tests)       PASS
                  npm run check             0 errors, 0 warnings
                  npm run build             succeeds
```

This is a real green baseline (run locally, not just claimed) — the project can enter a formal testing/UAT phase.

**What now has coverage (previously zero):**
- `agent/cmd/lan-agent/main_test.go`, `agent/internal/filesystem/fs_test.go`, `agent/internal/server/{handlers,server}_test.go`
- `control-center/{app_connection,app_dialog}_test.go`, `backend/{audit,client,securestore,session,transfer}/*_test.go`
- `control-center/frontend/src/lib/utils/{selectionState,transferState}.test.ts`
- `.github/workflows/ci.yml` — matrix CI (Go tests + race + vet for `agent` and `control-center`, frontend `npm test`/`check`/build) on push/PR

**Still not covered (residual gaps, lower priority than before):**
- `agent/internal/executor/`, `agent/internal/discovery/`, `agent/internal/system/`, `agent/internal/screenshot/` — no test files yet. `executor.Execute` remains the highest-value target here (timeout handling, output capping).
- `control-center/backend/{discovery,protocol,scripting,wol}/` — no test files. `scripting/engine.go`'s `text/template` variable substitution is the most worth testing (see Security Considerations below).
- Frontend: only `selectionState`/`transferState` utils are tested; the Svelte components themselves (`Dashboard`, `Terminal`, `FileBrowser`, etc.) have no component tests.

**Documentation/reality mismatch: resolved.**
The earlier version of this document flagged `.superpowers/sdd/lan-commander-completion/progress.md` as unreliable because it claimed tests/securestore/CI that didn't exist in the working tree. Root cause: that work existed on `origin/main` (PR #1) but had never been pulled into the local `main` checkout. After merging, `progress.md`'s claims check out against the actual code. No further reconciliation needed — treat that ledger as accurate again.

## Security Considerations

**Resolved by the origin/main merge:**
- **Unauthenticated agent access is no longer the default.** `agent/cmd/lan-agent/main.go` now requires `--auth-token` to start, unless `--no-auth` is passed explicitly (documented as "only for an isolated laboratory network"). Previously the agent defaulted to `authToken == ""`, which auto-authed every connecting client.
- **Control-center now uses `wss://` when appropriate.** `control-center/backend/client/client.go` builds the WebSocket URL scheme dynamically instead of hardcoding `"ws"`.
- **CORS/Origin check tightened.** `agent/internal/server/server.go`'s `CheckOrigin` now requires an empty `Origin` header (native clients don't send one; browsers always do), mitigating cross-site WebSocket hijacking (CSWSH).
- **Auth tokens are no longer stored in plaintext on Windows.** `control-center/backend/session/session.go` now routes `AuthToken` through `control-center/backend/securestore` before persisting to SQLite; on Windows this uses `CryptProtectData`/`CryptUnprotectData` (DPAPI, user-scoped). On non-Windows platforms `securestore.Default()` returns an `unavailableStore` that errors (`ErrUnavailable`) rather than silently falling back to plaintext — a safe fail-closed choice, but it means saved sessions with tokens currently cannot be persisted on Linux/macOS control-center builds. Worth tracking as a follow-up if cross-platform session persistence matters.

**Fixed locally (this session, commit `7054264`):**
- **Auth token comparison is now constant-time.** `agent/internal/server/handlers.go` uses `crypto/subtle.ConstantTimeCompare` instead of `!=`.
- **Auth attempt limiting added.** `agent/internal/server/server.go` adds `MaxAuthAttempts = 5`; `Client.authAttempts` (atomic counter) closes the connection after 5 failed `auth` messages on a single WebSocket connection. This is per-connection, not per-IP — an attacker can still reconnect and retry, but each connection now costs a fresh TCP/WS handshake instead of allowing unlimited attempts on one socket.

**Confirmed intentional, not a bug (product decision, 2026-08-27):**
- **Filesystem access has no root confinement.** `agent/internal/filesystem/fs.go`'s `safePath()` only rejects `..` traversal; an authenticated client can still read/write anywhere the agent process's OS user can. This was raised and explicitly confirmed as intentional — LAN Commander is meant to be a full remote-administration tool (comparable to RDP/SSH+GUI), gated by the auth token, not a sandboxed file-sharing tool. No `--allowed-root` flag was added. Revisit only if the product direction changes.

**Still open (lower severity, not addressed this session):**
- **`control-center/backend/scripting/engine.go`'s `processVariables()`** still parses script lines as raw `text/template` with user-supplied vars — no escaping of shell metacharacters before the result is handed to `bash -c`/`powershell -Command`. Low exploitability (requires the operator to already have exec access to the agent) but worth hardening if scripts are ever shared/imported from untrusted sources.
- **`agent/internal/executor/executor.go`** remains an intentionally unrestricted remote-shell primitive (by design — this is the product's core feature) with no test coverage yet.

## Tech Debt

**Resolved this session:**
- `control-center/frontend/src/lib/backup/` (16 files, dead duplicate of `components/`/`stores/`/`utils/`) — deleted in commit `4d30c29`. Verified `npm run check`/`build`/`test` still pass with it removed.
- Committed build binaries (`agent/build/lan-agent.exe`, `lan-agent-linux`) and `frontend/dist/` — already removed from tracking by the origin/main merge (`.gitignore` now covers `agent/build/`, `installers/windows/lan-agent.exe`, `installers/linux/lan-agent-linux`); confirmed via `git ls-files` that none remain tracked.

**Still open:**
- **No dependency audit process in CI.** `.github/workflows/ci.yml` runs tests/vet/build but not `npm audit` or `go list -m -u`. Add as a periodic/scheduled job if supply-chain hygiene becomes a priority.
- **Protocol duplication.** The wire protocol is still defined independently in `agent/internal/protocol/types.go` and `control-center/backend/protocol/types.go` (no shared Go module). Unchanged by the merge — still requires manual lockstep updates on both sides when the protocol changes.
- **In-memory audit log ring buffer (1000 entries)** in `control-center/backend/audit/audit.go` still silently evicts beyond capacity if SQLite persistence isn't wired up. Unchanged by the merge; verify `App.startup` always calls `OpenDB` if this matters.

## Fragile Areas

**`control-center/app.go` (~1000+ lines after the merge, was 804):** Still the single Wails binding surface aggregating session, client, audit, scripting, transfer, and WoL orchestration. Now has `app_connection_test.go` and `app_dialog_test.go` covering connection/dialog flows specifically, but the file as a whole remains a wide-blast-radius change point. Read fully before editing.

**`control-center/backend/client/client.go` (601 lines, was 622 — refactored during the merge):** Connection lifecycle, reconnect, and pending-request tracking. Now has `client_test.go` (170 lines) covering this — a meaningful improvement over the previous zero-coverage state, but still the most concurrency-sensitive file in the project; extend tests before modifying reconnect/timeout logic further.

## Test Coverage Gaps (priority order, post-merge)

1. `agent/internal/executor/executor.go` — `Execute`, `detectShell`, `cappedWriter` are cheap to unit test (pure-ish functions) and remain uncovered; highest-value remaining gap given this is the RCE-capable code path.
2. `control-center/backend/scripting/engine.go` — `processVariables()` template substitution has no test exercising malicious/edge-case variable values.
3. `agent/internal/discovery/`, `control-center/backend/discovery/` — mDNS advertisement/discovery, both sides untested; lower risk (LAN-local, no remote-write surface).
4. Frontend Svelte components — no component-level tests yet, only `selectionState`/`transferState` utils. Lower priority than backend gaps given current CI already gates `npm run check` (type safety) and `npm run build`.

---

*Concerns audit: 2026-08-27 (post-merge refresh)*
