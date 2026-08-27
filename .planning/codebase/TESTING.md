# Testing Patterns

**Analysis Date:** 2026-08-27
<!-- refreshed: 2026-08-27 (post-merge of origin/main PR #1) -->

The previous version of this document reflected `main` before it was synced with `origin/main`, where only one test file (`agent/internal/ui/probe_test.go`) existed. After merging PR #1 (commit `7f826c6`), the project has real test suites across both Go modules and the frontend, plus a CI workflow that runs them. Verified green locally on 2026-08-27 (see `CONCERNS.md`).

## Test Framework

**Go:**
- Still the standard library `testing` package — no third-party framework (no testify/ginkgo) was introduced by the merge. Assertions remain plain `if got != want { t.Fatalf(...) }`.
- `httptest.NewTLSServer` / `httptest.NewServer` + `gorilla/websocket` are used to spin up real WebSocket servers in tests (see `control-center/backend/client/client_test.go`, `agent/internal/server/handlers_test.go`) rather than mocking the transport — the established pattern is integration-style tests against a real (local, in-process) WebSocket connection.

**Frontend:**
- Vitest (`vitest run` via `npm test`), added by the merge. Config lives in `control-center/frontend/vite.config.ts` (`test` block) — no separate `vitest.config.ts`.
- No component-testing library (`@testing-library/svelte` etc.) yet — only plain-function unit tests via `describe`/`it`/`expect` from `vitest`.

**Run Commands:**
```bash
# Go — from each module root
cd agent && go test ./... -count=1
cd agent && go test -race ./...
cd agent && go vet ./...

cd control-center && go test ./... -count=1
cd control-center && go test -race ./...
cd control-center && go vet ./...

# Frontend
cd control-center/frontend && npm ci
cd control-center/frontend && npm test        # vitest run
cd control-center/frontend && npm run check   # svelte-check
cd control-center/frontend && npm run build   # vite build
```
This is exactly what `.github/workflows/ci.yml` runs (Go job is a matrix over `agent`/`control-center` on `windows-latest`; frontend job on `ubuntu-latest`).

## Test File Organization

**Location:** Co-located with source, standard Go/Vitest convention (`<file>_test.go`, `<file>.test.ts`).

**New test files added by the merge:**
```
agent/
├── cmd/lan-agent/main_test.go            # flag parsing / validateAgentFlags
├── internal/filesystem/fs_test.go        # safePath, chunked read/write, checksums
├── internal/server/handlers_test.go      # message dispatch, file transfer handlers
└── internal/server/server_test.go        # connection lifecycle, auth handshake

control-center/
├── app_connection_test.go                # ConnectAgent / disconnect bindings
├── app_dialog_test.go                    # native dialog bindings
└── backend/
    ├── audit/audit_test.go
    ├── client/client_test.go             # reconnect, TLS greeting, pending requests
    ├── securestore/securestore_test.go
    ├── session/session_test.go           # SQLite persistence + token protect/restore
    └── transfer/transfer_test.go         # new transfer package

control-center/frontend/src/lib/utils/
├── selectionState.test.ts
└── transferState.test.ts
```

Still no test file for: `agent/internal/{executor,discovery,screenshot,system}/`, `control-center/backend/{discovery,protocol,scripting,wol}/`, and any `.svelte` component.

## Test Structure

**Go patterns observed in the new suites:**
- Table-driven tests are still not the dominant style — most new tests are one `TestXxx` function per behavior/scenario with a descriptive name (e.g. `TestSendFileUsesRemoteTemporaryPathAndCommitsValidatedFinalChunk` in `agent/internal/server/handlers_test.go`), following existing repo convention from `probe_test.go`.
- `t.Helper()` + small local helper functions (e.g. `sendFileMessage`, `handlerChecksum` in `handlers_test.go`; `tlsGreetingServer` in `client_test.go`) are used to build reusable test fixtures inline rather than a separate fixtures package.
- `t.Cleanup(...)` is the standard teardown mechanism (e.g. `t.Cleanup(server.Close)`), consistent with `t.Setenv` usage already established in `probe_test.go`.

**Frontend pattern (Vitest):**
```ts
import { describe, expect, it } from 'vitest';
import { clearSelectionAfterDisconnect } from './selectionState';

describe('clearSelectionAfterDisconnect', () => {
	it('clears the selected agent when that agent disconnects', () => {
		expect(clearSelectionAfterDisconnect('agent-1', 'agent-1')).toBeNull();
	});
});
```
Standard `describe`/`it`/`expect` blocks, one behavior per `it`. No component rendering yet — only pure-function utilities (`selectionState.ts`, `transferState.ts`) are tested this way.

## Mocking

**Go:** Still no mocking framework. The new tests favor **real dependencies over mocks**:
- WebSocket tests spin up an actual `httptest` server + `gorilla/websocket` connection rather than mocking `*websocket.Conn`.
- `securestore_test.go` presumably exercises the real DPAPI path on Windows (via `securestore.Default()`) and/or a test-only `Store` implementation — check this file directly before assuming behavior on non-Windows CI runners (the CI Go job runs on `windows-latest`, so DPAPI is exercised in CI).
- `session_test.go` uses a real (temp-file or in-memory) SQLite database via `modernc.org/sqlite`, not a mock DB layer.

**Frontend:** No mocking — pure functions tested directly.

## Coverage

**Requirements:** Still no enforced coverage threshold or `go tool cover` gate in CI — `.github/workflows/ci.yml` runs `go test`, `go test -race`, and `go vet`, not `go test -cover` with a minimum.

**View Coverage:**
```bash
cd agent && go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out
cd control-center && go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out
```

## CI Pipeline

`.github/workflows/ci.yml` (added by the merge):
- Triggers: push to `main`/`fix/**`, all pull requests, manual `workflow_dispatch`.
- `go` job: matrix over `{agent, control-center}`, runs on `windows-latest` (matches production target OS). For `control-center`, builds the embedded frontend first (`npm ci && npm run build`) since `main.go` uses `//go:embed all:frontend/dist`. Then `go test ./... -count=1`, `go test -race ./...`, `go vet ./...`.
- `frontend` job: runs on `ubuntu-latest`. `npm ci`, `npm test`, then type/Svelte checks (`npm run check`), consistent with local verification.

## Gaps Summary (priority order, post-merge)

1. `agent/internal/executor/executor.go` — still zero coverage; highest-value remaining gap (the RCE-capable code path).
2. `control-center/backend/scripting/engine.go` — `processVariables()` template substitution untested, including edge cases with shell-metacharacter-bearing variable values.
3. `agent/internal/discovery/`, `control-center/backend/discovery/` — mDNS, both sides untested (lower risk, LAN-local only).
4. Svelte components (`Dashboard`, `Terminal`, `FileBrowser`, `MultiExec`, `ScriptEditor`, etc.) — no component-level tests; only two utility modules (`selectionState`, `transferState`) have coverage so far.

---

*Testing analysis: 2026-08-27 (post-merge refresh)*
