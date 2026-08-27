import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import {
	connectAgent,
	connectAgentSecure,
	execCommand,
	execCommandMulti,
	getAgents,
	getAuditLogs,
	wakeOnLAN,
	saveSession,
	saveSessionSecure,
} from './api';

type Binding = (...args: unknown[]) => unknown;
type BindingCall = { name: string; args: unknown[] };
type WailsWindow = {
	go: {
		main: {
			App: Record<string, Binding>;
		};
	};
};

let originalWindowDescriptor: PropertyDescriptor | undefined;

function installBindings(bindings: Record<string, Binding>): BindingCall[] {
	const calls: BindingCall[] = [];
	const app: Record<string, Binding> = {};

	for (const [name, binding] of Object.entries(bindings)) {
		app[name] = (...args: unknown[]) => {
			calls.push({ name, args });
			return binding(...args);
		};
	}

	Object.defineProperty(globalThis, 'window', {
		configurable: true,
		enumerable: true,
		value: { go: { main: { App: app } } } satisfies WailsWindow,
		writable: true,
	});

	return calls;
}

describe('Wails API wrappers', () => {
	beforeEach(() => {
		originalWindowDescriptor = Object.getOwnPropertyDescriptor(globalThis, 'window');
	});

	afterEach(() => {
		if (originalWindowDescriptor) {
			Object.defineProperty(globalThis, 'window', originalWindowDescriptor);
		} else {
			Reflect.deleteProperty(globalThis, 'window');
		}
	});

	it('normalizes snake_case and camelCase agent responses at the API boundary', async () => {
		const snakeSystemInfo = { hostname: 'snake-host' };
		const camelSystemInfo = { hostname: 'camel-host' };
		const rawAgents = [
			{
				id: 'snake-agent',
				host: '10.0.0.1',
				port: '9001',
				name: 'Snake Agent',
				os: 'Linux',
				arch: 'amd64',
				connected: true,
				last_seen: '2026-01-02T03:04:05.000Z',
				lastSeen: 'ignored-snake-fallback',
				system_info: snakeSystemInfo,
				systemInfo: camelSystemInfo,
			},
			{
				id: 'camel-agent',
				host: '10.0.0.2',
				port: 9002,
				name: 'Camel Agent',
				os: 'Windows',
				arch: 'x64',
				connected: false,
				lastSeen: '2026-01-02T03:05:05.000Z',
				systemInfo: camelSystemInfo,
			},
		];
		const calls = installBindings({ GetAgents: () => Promise.resolve(rawAgents) });

		expect(await getAgents()).toEqual([
			{
				id: 'snake-agent',
				host: '10.0.0.1',
				port: 9001,
				name: 'Snake Agent',
				os: 'Linux',
				arch: 'amd64',
				connected: true,
				secure: false,
				lastSeen: '2026-01-02T03:04:05.000Z',
				systemInfo: snakeSystemInfo,
				systemInfoError: null,
				cpuHistory: [],
			},
			{
				id: 'camel-agent',
				host: '10.0.0.2',
				port: 9002,
				name: 'Camel Agent',
				os: 'Windows',
				arch: 'x64',
				connected: false,
				secure: false,
				lastSeen: '2026-01-02T03:05:05.000Z',
				systemInfo: camelSystemInfo,
				systemInfoError: null,
				cpuHistory: [],
			},
		]);
		expect(calls).toEqual([{ name: 'GetAgents', args: [] }]);
	});

	it('returns an empty agent list for non-array backend responses', async () => {
		for (const response of [null, { agents: [] }]) {
			installBindings({ GetAgents: () => Promise.resolve(response) });
			expect(await getAgents()).toEqual([]);
		}
	});

	it('propagates backend errors to callers', async () => {
		const failure = new Error('backend unavailable');
		installBindings({ GetAgents: () => Promise.reject(failure) });

		await expect(getAgents()).rejects.toBe(failure);
	});

	it('supplies defaults for optional wrapper arguments', async () => {
		const calls = installBindings({
			ConnectAgent: () => Promise.resolve('connected'),
			ConnectAgentSecure: () => Promise.resolve('secure'),
			ExecCommand: () => Promise.resolve('command'),
			ExecCommandMulti: () => Promise.resolve('multi-command'),
			WakeOnLAN: () => Promise.resolve('wake'),
			SaveSession: () => Promise.resolve('session'),
			SaveSessionSecure: () => Promise.resolve('secure-session'),
			GetAuditLogs: () => Promise.resolve('audit'),
		});

		expect(await connectAgent('host.lan', 9000)).toBe('connected');
		expect(await connectAgentSecure('host.lan', 9000)).toBe('secure');
		expect(await execCommand('agent-1', 'hostname')).toBe('command');
		expect(await execCommandMulti(['agent-1', 'agent-2'], 'hostname')).toBe('multi-command');
		expect(await wakeOnLAN('00:11:22:33:44:55')).toBe('wake');
		expect(await saveSession('host.lan', 9000, 'Lab host')).toBe('session');
		expect(await saveSessionSecure('host.lan', 9000, 'Lab host')).toBe('secure-session');
		expect(await getAuditLogs()).toBe('audit');

		expect(calls).toEqual([
			{ name: 'ConnectAgent', args: ['host.lan', 9000, ''] },
			{ name: 'ConnectAgentSecure', args: ['host.lan', 9000, '', '', ''] },
			{ name: 'ExecCommand', args: ['agent-1', 'hostname', 0] },
			{ name: 'ExecCommandMulti', args: [['agent-1', 'agent-2'], 'hostname', 0] },
			{ name: 'WakeOnLAN', args: ['00:11:22:33:44:55', '255.255.255.255'] },
			{ name: 'SaveSession', args: ['host.lan', 9000, 'Lab host', ''] },
			{ name: 'SaveSessionSecure', args: ['host.lan', 9000, 'Lab host', '', '', ''] },
			{ name: 'GetAuditLogs', args: [100] },
		]);
	});
});
