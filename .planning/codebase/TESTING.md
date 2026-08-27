# Testing Patterns

**Analysis Date:** 2026-08-27

## Test Framework

**Runner:**
- Go standard library `testing` package. No third-party test framework (no testify, no ginkgo) is used or imported.
- No test config file exists beyond the standard `go test` toolchain (`agent/go.mod`).

**Assertion Library:**
- None — plain `if got != want { t.Fatalf(...) }` style assertions, standard Go idiom.

**Run Commands:**
```bash
cd agent && go test ./...              # Run all agent tests
cd agent && go test ./internal/ui/...  # Run only the ui package tests
cd agent && go test -v ./...           # Verbose output
```
There is no `go test` invocation configured for `control-center/backend` — it currently has zero test files.

**Frontend:**
- No test runner configured in `control-center/frontend/package.json` (no vitest, jest, or `@testing-library` dependency). No `*.test.ts`/`*.spec.ts` files exist anywhere under `control-center/frontend/src`.

## Test File Organization

**Location:**
- Co-located with source: `agent/internal/ui/probe_test.go` sits next to `agent/internal/ui/probe.go`, `probe_windows.go`, `probe_other.go`.

**Naming:**
- Standard Go convention: `<file>_test.go`, package matches the file under test (`package ui`).

**Structure:**
```
agent/
└── internal/
    └── ui/
        ├── probe.go
        ├── probe_windows.go
        ├── probe_other.go
        └── probe_test.go   # only test file in the entire repo
```

## Test Structure

**Suite Organization:**
```go
// agent/internal/ui/probe_test.go
func TestAvailableWithDisplaySet(t *testing.T) {
	t.Setenv("DISPLAY", ":0")
	if !Available() {
		t.Fatal("Available() = false with DISPLAY set, want true")
	}
}

func TestAvailableHeadless(t *testing.T) {
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	got := Available()
	want := runtime.GOOS == "windows"
	if got != want {
		t.Fatalf("Available() = %v on %s without display, want %v", got, runtime.GOOS, want)
	}
}
```

**Patterns:**
- One `TestXxx` function per behavior/scenario rather than table-driven tests (only 2 test functions exist total, so no table-driven convention has been established yet — table-driven tests using `[]struct{ name string; ... }` + `t.Run(tt.name, ...)` are the idiomatic Go approach and should be introduced for new multi-case tests).
- `t.Setenv(...)` used to isolate environment-dependent behavior instead of manual save/restore — this is the established pattern for any test needing to control env vars (auto-restored by the testing framework after each test).
- Failure messages follow the `got X, want Y` convention (`t.Fatalf("Available() = %v ..., want %v", got, want)`).

## Mocking

**Framework:** None present.

**Patterns:**
- No mocking library or hand-rolled mock/stub exists anywhere in the codebase (no `mock`, `fake`, `stub` identifiers found in Go or TS sources).
- The `client.Manager` in `control-center/backend/client/client.go` and the `executor.Execute` function in `agent/internal/executor/executor.go` are the two most test-relevant units but have no tests; both are pure/testable via dependency injection of a `net/url`/`websocket.Dialer` or `os/exec.Command` boundary if tests are added later.

**What to Mock (recommended, not yet implemented):**
- WebSocket connections in `client.Manager.Connect`/`handshake` — would need an interface around `*websocket.Conn` to allow substituting a fake conn in unit tests.
- `os/exec.Command` invocations in `agent/internal/executor/executor.go` — currently calls `exec.CommandContext` directly with no seam for substitution; introducing a `execCommand` function variable would enable testing `Execute` without spawning real shells.

**What NOT to Mock:**
- Simple pure functions like `detectShell` (`agent/internal/executor/executor.go:74`) — directly unit-testable without mocking since they take primitives and return primitives.

## Fixtures and Factories

**Test Data:**
- None exist. No fixtures directory, no factory functions, no golden files.

**Location:**
- Not applicable — no fixtures directory present.

## Coverage

**Requirements:** None enforced. No coverage threshold, CI gate, or `go tool cover` invocation found in any script (`scripts/build-agent.ps1`, `scripts/build-all.ps1` only build; they do not run tests).

**View Coverage:**
```bash
cd agent && go test -cover ./...
cd agent && go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out
```

## Test Types

**Unit Tests:**
- Only unit-level tests exist, and only for `agent/internal/ui` (display-availability probing). Scope: single-function behavior under different env-var states, cross-platform via `runtime.GOOS` branching.

**Integration Tests:**
- None. The WebSocket protocol between agent (`agent/internal/server`) and control-center (`control-center/backend/client`) has no integration test exercising a real handshake/message round trip, despite being the most complex and stateful part of the system (reconnect logic, pending-request tracking, heartbeat).

**E2E Tests:**
- Not used. No Playwright/Cypress/Wails e2e harness configured for the Svelte + Wails desktop UI.

## Common Patterns

**Async Testing:**
Not applicable — no async Go tests (no goroutine/channel testing patterns) and no frontend async tests exist yet. If added for `client.Manager.SendRequest` (channel + timer based), follow Go's standard pattern of using `context.WithTimeout` in the test itself to bound `t.Fatal` on hangs.

**Error Testing:**
Not applicable — no test currently asserts on an error path (e.g. `ErrAuthRequired`, malformed JSON payloads in `handlers.go`). These are natural first candidates: `agent/internal/server/handlers.go` handlers all short-circuit to `c.sendError(...)` on invalid JSON, which is currently unverified by any test.

## Gaps Summary (highest priority first)

1. `control-center/backend/client/client.go` — connection/handshake/reconnect logic is entirely untested; highest risk given its concurrency (goroutines, mutexes, channels).
2. `agent/internal/server/handlers.go` — message dispatch and per-type payload validation untested; a `handleMessage` table-driven test over all `MsgXxx` types would catch protocol drift.
3. `agent/internal/executor/executor.go` — `Execute`, `detectShell`, `cappedWriter` are pure enough to unit test cheaply (especially `cappedWriter.Write` truncation logic) but have zero coverage.
4. `control-center/frontend` — zero test infrastructure; no vitest/testing-library installed. Adding tests requires first installing a test runner and updating `package.json` scripts.

---

*Testing analysis: 2026-08-27*
