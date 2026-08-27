import { get } from 'svelte/store';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import {
	agentCounts,
	agents,
	connectedAgents,
	selectedAgent,
	selectedAgentId,
	type AgentInfo,
} from './agents';

function makeAgent(id: string, connected: boolean, overrides: Partial<AgentInfo> = {}): AgentInfo {
	return {
		id,
		host: `${id}.lan`,
		port: 9000,
		name: id,
		os: 'linux',
		arch: 'amd64',
		connected,
		lastSeen: '2026-01-02T03:04:05.000Z',
		systemInfo: null,
		systemInfoError: null,
		cpuHistory: [],
		...overrides,
	};
}

describe('agent store derivations', () => {
	beforeEach(() => {
		agents.set([]);
		selectedAgentId.set(null);
	});

	afterEach(() => {
		agents.set([]);
		selectedAgentId.set(null);
	});

	it('returns null when no agent is selected or the selected id is missing', () => {
		agents.set([makeAgent('agent-1', true)]);

		expect(get(selectedAgent)).toBeNull();

		selectedAgentId.set('missing');
		expect(get(selectedAgent)).toBeNull();
	});

	it('returns the selected agent and follows replacement updates', () => {
		const first = makeAgent('agent-1', true);
		const second = makeAgent('agent-2', false);
		agents.set([first, second]);
		selectedAgentId.set('agent-2');

		expect(get(selectedAgent)).toBe(second);

		const updatedSecond = { ...second, connected: true };
		agents.set([first, updatedSecond]);
		expect(get(selectedAgent)).toBe(updatedSecond);
	});

	it('derives only connected agents', () => {
		const offline = makeAgent('offline', false);
		const online = makeAgent('online', true);
		const alsoOnline = makeAgent('also-online', true);
		agents.set([offline, online, alsoOnline]);

		expect(get(connectedAgents)).toEqual([online, alsoOnline]);
	});

	it('reports zero counts for an empty agent collection', () => {
		expect(get(agentCounts)).toEqual({ total: 0, connected: 0, offline: 0 });
	});

	it('counts connected and offline agents independently', () => {
		agents.set([
			makeAgent('online-1', true),
			makeAgent('offline-1', false),
			makeAgent('online-2', true),
		]);

		expect(get(agentCounts)).toEqual({ total: 3, connected: 2, offline: 1 });
	});
});
