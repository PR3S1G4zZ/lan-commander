# LAN Commander Completion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Finish the approved LAN Commander gaps across active UI operations, secure sessions, automated tests, and release verification.

**Architecture:** Preserve the existing WebSocket protocol and chunked transfer implementation. Add thin Wails dialog bindings, a testable client options/secure-store boundary, and standalone release-manifest scripts. Keep the existing local agent status UI unchanged in this pass.

**Tech Stack:** Go 1.25+/1.26+, Wails v2.13, Svelte 5, TypeScript, Vitest, SQLite, Gorilla WebSocket, PowerShell.

**Spec:** `docs/superpowers/specs/2026-08-22-lan-commander-completion-design.md`

## Global Constraints

- Preserve the pre-existing dirty working tree; stage only files belonging to each task.
- No `git reset --hard`, `git checkout --`, broad deletion, or credential generation.
- Use 64 KiB chunks and atomic finalization for file transfers.
- Never enable `InsecureSkipVerify`.
- Existing `Connect` and laboratory `--no-auth` compatibility must remain available.
- Every new production behavior gets a failing test before implementation.

---

### Task 1: Transfer service boundaries

**Files:**
- Create: `control-center/backend/transfer/transfer.go`
- Create: `control-center/backend/transfer/transfer_test.go`
- Modify: `control-center/app.go`

**Interfaces:**
- Produces `Download(ctx, requester, remotePath, localPath) error` and `Upload(ctx, requester, localPath, remotePath) error`.
- `requester` sends a protocol request and returns a protocol response.

- [x] Write failing tests for checksum-verified atomic download, empty non-final chunks, and upload chunk boundaries.
- [x] Run `go test ./backend/transfer -count=1` and confirm the expected failures.
- [x] Implement the minimal streaming helpers using a requester interface and 64 KiB chunks.
- [x] Refactor `App.TransferFile` to delegate to the helper while preserving its audit behavior.
- [x] Add `App.UploadFileFromPath` for testable local-path uploads.
- [x] Run the focused transfer tests and then `go test ./...`.

### Task 2: Native dialog bindings

**Files:**
- Modify: `control-center/app.go`
- Modify: `control-center/frontend/src/lib/utils/api.ts`
- Regenerate: `control-center/frontend/wailsjs/go/main/App.*`

- [x] Write a failing test for rejecting empty agent/path arguments before opening a dialog.
- [x] Add `DownloadFile(agentID, remotePath)` using `runtime.SaveFileDialog` and `UploadFile(agentID, remoteDirectory)` using `runtime.OpenFileDialog`.
- [x] Delegate both bindings to the tested path-based methods.
- [x] Regenerate Wails bindings and run Go tests/build.

### Task 3: Active file-browser controls

**Files:**
- Modify: `control-center/frontend/src/lib/components/FileBrowser.svelte`
- Modify: `control-center/frontend/src/lib/utils/format.ts`
- Create/Modify: `control-center/frontend/src/lib/utils/transferState.ts`
- Test: `control-center/frontend/src/lib/utils/transferState.test.ts`

- [x] Write failing Vitest tests for file-only download visibility, transfer busy state, and error normalization.
- [x] Add download buttons to file rows and an upload button for the current remote directory.
- [x] Add busy/error/success notifications without changing directory navigation semantics.
- [x] Run the focused frontend test, `npm run check`, and `npm run build`.

### Task 4: Active disconnection flow

**Files:**
- Modify: `control-center/frontend/src/lib/components/Sidebar.svelte`
- Modify: `control-center/frontend/src/App.svelte`
- Modify: `control-center/frontend/src/lib/utils/api.ts`
- Test: `control-center/frontend/src/lib/utils/transferState.test.ts`

- [x] Write failing tests for disconnect state cleanup and selected-agent clearing.
- [x] Add a visible disconnect action for the selected agent with confirmation.
- [x] Call `DisconnectAgent`, clear the selected agent, and report errors through notifications.
- [x] Run frontend checks and build.

### Task 5: Secure client options and origin policy

**Files:**
- Modify: `control-center/backend/client/client.go`
- Create/Modify: `control-center/backend/client/client_test.go`
- Modify: `agent/internal/server/server.go`
- Modify: `agent/internal/server/server_test.go`

- [x] Write failing tests for `ws` versus `wss`, CA loading, hostname verification, reconnect option reuse, and origin rejection.
- [x] Add `ConnectOptions`, secure dialer construction, and TLS-aware reconnects without changing the legacy wrapper.
- [x] Reject non-empty browser origins in the agent upgrader.
- [x] Run focused tests, race tests, and `go vet`.

### Task 6: Protected session tokens

**Files:**
- Create: `control-center/backend/securestore/securestore.go`
- Create: `control-center/backend/securestore/securestore_windows.go`
- Create: `control-center/backend/securestore/securestore_other.go`
- Create: `control-center/backend/securestore/securestore_test.go`
- Modify: `control-center/backend/session/session.go`
- Create/Modify: `control-center/backend/session/session_test.go`

- [x] Write failing tests for round-trip protection and migration of existing plaintext rows.
- [x] Implement Windows DPAPI protection and a fail-closed unsupported-platform implementation.
- [x] Add additive SQLite columns/migration for protected token storage.
- [x] Run session/securestore tests and the full Control Center Go suite.

### Task 7: Frontend test harness and secure connection UI

**Files:**
- Modify: `control-center/frontend/package.json`
- Modify: `control-center/frontend/vite.config.ts`
- Create: `control-center/frontend/src/lib/utils/transferState.ts`
- Create: `control-center/frontend/src/lib/utils/transferState.test.ts`
- Modify: `control-center/frontend/src/lib/components/ConnectionDialog.svelte`
- Modify: `control-center/frontend/src/lib/utils/api.ts`

- [x] Add Vitest scripts/dependencies and make a focused test fail for transfer state.
- [x] Implement the state helper and verify the focused test.
- [x] Add secure connection fields and call the new Wails binding.
- [x] Run `npm ci`, focused tests, `npm run check`, and `npm run build`.

### Task 8: Release manifest and naming

**Files:**
- Create: `scripts/generate-release-manifest.ps1`
- Create: `scripts/verify-release-manifest.ps1`
- Modify: `control-center/wails.json`
- Modify: `scripts/build-all.ps1`
- Modify: `README.md`

- [x] Write a failing PowerShell verification case for a changed artifact hash.
- [x] Implement deterministic SHA-256 manifest generation and verification with explicit artifact paths.
- [x] Align Wails default output and build-script output on `lan-commander.exe`.
- [x] Document unsigned-versus-signed release status and required external signing material.
- [x] Run both scripts against a temporary manifest and verify failure on tampering.

### Task 9: Final verification

**Files:**
- No production files; verification only.

- [x] Run `go test ./...`, `go test -race ./...`, and `go vet ./...` in both Go modules.
- [x] Run `npm test`, `npm run check`, and `npm run build` in the frontend.
- [x] Run `wails build` and verify `control-center/build/bin/lan-commander.exe`.
- [x] Run PowerShell syntax checks and release-manifest verification.
- [x] Report Linux/real-install limitations explicitly and preserve unrelated working-tree changes.
