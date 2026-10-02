import type { Session } from '../stores/agents';

/** Normalize backend session fields while deliberately excluding auth_token. */
export function normalizeSession(raw: Record<string, unknown>): Session {
	return {
		id: Number(raw.id ?? 0),
		name: String(raw.name ?? ''),
		host: String(raw.host ?? ''),
		port: Number(raw.port ?? 0),
		created_at: String(raw.created_at ?? raw.createdAt ?? ''),
		last_connected: String(raw.last_connected ?? raw.lastConnected ?? ''),
		tls: Boolean(raw.tls),
		ca_file: String(raw.ca_file ?? raw.caFile ?? ''),
		server_name: String(raw.server_name ?? raw.serverName ?? ''),
	};
}
