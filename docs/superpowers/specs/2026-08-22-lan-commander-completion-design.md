# LAN Commander Completion Design


**Date:** 2026-08-22
**Status:** Approved for implementation by the user

## Goal

Close the most important gaps identified in the LAN Commander review: make file transfer and disconnection usable from the active Control Center UI, add a secure TLS/session path, improve automated coverage, and make release artifacts verifiable.

## Current constraints

- The checkout already contains a large uncommitted local patch. It must be preserved; no reset, checkout, or broad cleanup is allowed.
- The Control Center is a Wails desktop application and the agent is a Go service for Windows and Linux.
- Existing unauthenticated mode remains an explicit laboratory-only option.
- A trusted release signature cannot be produced without release signing material. The implementation will generate and verify SHA-256 manifests and provide an optional signing hook, but will not invent a trust root.
- Linux installation and service tests require a Linux host or functioning WSL; Windows-only validation will state that limitation explicitly.

## Design

### 1. Control Center file operations and disconnection

Keep the existing chunked protocol and checksum validation. Add user-facing Wails bindings that open native dialogs and delegate to streaming backend methods:

- `DownloadFile(agentID, remotePath)` opens a save dialog and calls the existing atomic `TransferFile` implementation.
- `UploadFile(agentID, remoteDirectory)` opens an open-file dialog and streams the selected local file to the agent in 64 KiB chunks using `MsgSendFile`.
- `DisconnectAgent(agentID)` is already available in Go and will be exposed from the active header/sidebar UI with confirmation, state cleanup, and notification.

The file browser will show actions only for files, disable conflicting actions while a transfer is running, show progress where the Wails binding can report it, and surface failures through the existing notification store. Every successful or failed operation remains audited.

### 2. TLS and protected session data

Introduce a `client.ConnectOptions` value so new secure connections and reconnects share exactly the same dial configuration. Existing `Connect` remains a compatibility wrapper for plain LAN connections; the UI adds a secure connection path with a CA file and optional server name. The client uses `wss`, system roots plus the supplied CA, and hostname verification; it never enables `InsecureSkipVerify`.

Persist TLS settings in the sessions table through an additive migration. Protect saved auth tokens using Windows DPAPI behind a small `securestore` interface; tests use the same interface with an in-memory implementation. On unsupported platforms, the code fails closed rather than silently storing a false sense of security.

The agent WebSocket upgrader will reject non-empty browser origins by default. Native Wails connections do not send an Origin header, while the local status UI remains HTTP-only on loopback.

### 3. Tests and release verification

- Add Go tests for transfer validation/streaming, TLS dial configuration, session migration/protection, and origin policy.
- Add a small Vitest suite for the frontend transfer/disconnect state helpers and error normalization.
- Add `scripts/generate-release-manifest.ps1` and `scripts/verify-release-manifest.ps1` for SHA-256 manifests. The generation script accepts an optional external signing command but reports unsigned output clearly.
- Align the Wails output name, build scripts, and README on `lan-commander.exe`.

## Out of scope for this completion pass

The native tray and desktop-notification experience described in the older Fyne design remains a separate product slice. The current local agent UI will remain status-only until that native UI is designed and tested on both operating systems; no misleading partial tray implementation will be added.

## Acceptance criteria

1. A user can download a remote file, upload a local file, and disconnect a selected agent from the active UI.
2. Transfers are streamed, checksum-verified, audited, and leave no partial final file after failure.
3. Secure sessions use TLS certificate verification and reconnect with the same TLS settings.
4. Saved tokens are not stored as plaintext on Windows.
5. Existing plain-LAN and laboratory flows continue to work.
6. Go tests, race tests, `go vet`, frontend checks/tests/build, Wails build, release-manifest generation/verification, and PowerShell syntax checks pass.
