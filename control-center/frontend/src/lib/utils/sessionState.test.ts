import { describe, expect, it } from 'vitest';
import { normalizeSession } from './sessionState';

describe('normalizeSession', () => {
	it('normalizes saved TLS fields without exposing the stored token', () => {
		const session = normalizeSession({
			id: 4, name: 'secure host', host: 'host.local', port: 9474,
			auth_token: 'must-not-reach-ui', tls: true, ca_file: 'root.pem', server_name: 'agent.local',
		});
		expect(session).toMatchObject({ tls: true, ca_file: 'root.pem', server_name: 'agent.local' });
		expect(session).not.toHaveProperty('auth_token');
	});
});
