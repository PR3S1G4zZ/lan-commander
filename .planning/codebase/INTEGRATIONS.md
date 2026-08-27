# External Integrations

**Analysis Date:** 2026-08-27

## APIs & External Services

This project has **no third-party cloud API integrations**. It is designed explicitly to avoid cloud dependencies — all communication is peer-to-peer over the local network between the Control Center (admin desktop app) and Agents (managed PCs).

**LAN Protocol (custom):**
- Control Center ↔ Agent communication - custom JSON-over-WebSocket protocol
  - Client: `github.com/gorilla/websocket` (both sides)
  - Protocol definitions: `agent/internal/protocol/types.go`, `control-center/backend/protocol/types.go`
  - Default port: TCP 9474
  - Message types include: `exec_command`, `list_dir`, `get_file`, `send_file`, `screenshot`, `system_info`, `auth`, `keep_alive`, `script_run`
  - Auth: shared bearer-style token sent via `MsgAuth` (`AuthPayload{Token, Username}`), validated by `agent/internal/server/server.go`

## Data Storage

**Databases:**
- SQLite (embedded, file-based, no server) - `control-center/backend/session/session.go`, `control-center/backend/audit/audit.go`
  - Driver: `modernc.org/sqlite` (pure-Go, no cgo dependency)
  - Connection: local file `lan-commander.db`, created next to the executable (or in a caller-supplied dir) — see `session.Manager.Open()` in `control-center/backend/session/session.go:38-71`
  - Tables: `sessions` (saved agent connections: name, host, port, auth_token, timestamps) — `control-center/backend/session/session.go:84-98`; audit log table opened via `Logger.OpenDB()` in `control-center/backend/audit/audit.go`
  - No migrations framework; tables created with `CREATE TABLE IF NOT EXISTS` on startup

**File Storage:**
- Local filesystem only — no object storage / cloud storage integration
  - Agent exposes file browsing/transfer over the WebSocket protocol (`agent/internal/filesystem/fs.go`)
  - File transfer is chunked with SHA-256 checksum verification (per `README.md`, `FileChunkPayload.Checksum` in `agent/internal/protocol/types.go`)
  - Saved scripts persisted to disk under a `scripts/` directory (`control-center/backend/scripting/engine.go`)

**Caching:**
- None detected — audit log uses an in-memory ring buffer (`control-center/backend/audit/audit.go`) with optional SQLite persistence, not a caching layer

## Authentication & Identity

**Auth Provider:**
- None — custom, LAN-local shared-token authentication
  - Implementation: Agent optionally requires a token (`authToken` field on `agent/internal/server/server.go` `Server`); if set, agent sends `MsgAuthRequired` on connect and expects an `MsgAuth` message with a matching token before marking the client `authed`
  - Control Center stores per-session tokens in SQLite (`Session.AuthToken` field, `control-center/backend/session/session.go`)
  - No OAuth, SSO, or external identity provider integration
  - Optional transport security: TLS 1.2+ via locally supplied cert/key files (`agent/internal/server/server.go:129-141`), not a managed CA/ACME integration

## Monitoring & Observability

**Error Tracking:**
- None — no Sentry/Bugsnag/etc. integration detected

**Logs:**
- Standard library `log` package used throughout (e.g., `agent/internal/server/server.go`, `control-center/backend/discovery/discover.go`) — logs to stdout/stderr, no external log aggregation
- Application-level audit trail (distinct from technical logs): `control-center/backend/audit/audit.go` — in-memory ring buffer (default capacity 1000 entries) with optional SQLite persistence, tracks actions like commands run, files transferred, per agent/user, with success/error/warning status

## CI/CD & Deployment

**Hosting:**
- Not applicable — this is a distributed desktop/service application, not a hosted web service
- Distribution via native installers: `installers/windows`, `installers/linux`
- Release artifacts verified via checksum manifest: `release-manifest.sha256` (repo root)

**CI Pipeline:**
- None detected — no `.github/workflows`, `.gitlab-ci.yml`, or similar CI config found in the repo
- Builds are performed locally via PowerShell scripts: `scripts/build-agent.ps1` (cross-compiles the agent binary for Windows/Linux), `scripts/build-all.ps1` (builds agent + Control Center + packages installers)

## Environment Configuration

**Required env vars:**
- None detected — no `.env` files present; configuration is passed via CLI flags to the agent binary (e.g., service install mode, `--ui` flag for local status UI) and via the Control Center's session store (host/port/token entered by the admin at connect time)

**Secrets location:**
- No `.env`/credentials files found in the repository
- Auth tokens are runtime values entered by the operator and persisted locally in the Control Center's SQLite database (`sessions.auth_token` column) — not committed to source control
- TLS certs/keys (if used) are supplied as local file paths at agent startup, not embedded in the repo

## Webhooks & Callbacks

**Incoming:**
- Agent exposes a plain HTTP `/health` endpoint (`agent/internal/server/server.go:90-94`) returning `{"status":"ok"}` — used for liveness checks, not a webhook receiver
- No inbound webhook endpoints from external services

**Outgoing:**
- None — no outbound webhook calls to external services
- Agent advertises itself via mDNS (`_lan-commander._tcp` service, `control-center/backend/discovery/discover.go:15`) so Control Center instances can discover it on the LAN; this is service discovery, not a webhook
- Wake-on-LAN magic packets sent via UDP broadcast on port 9 (`control-center/backend/wol/wol.go`) to remotely power on managed machines — LAN broadcast, not an external API call

---

*Integration audit: 2026-08-27*
