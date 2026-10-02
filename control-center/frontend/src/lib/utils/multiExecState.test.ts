import { describe, expect, it } from 'vitest';
import type { AgentInfo } from '../stores/agents';
import { getRunnableAgentIds, reconcileSelectedAgentIds } from './multiExecState';

const agent = (id: string, connected: boolean): AgentInfo => ({
	id, connected, host: '', port: 0, name: id, os: '', arch: '', lastSeen: '', systemInfo: null, systemInfoError: null, cpuHistory: [],
});

describe('multi-exec agent selection', () => {
	it('removes disconnected and removed agents from selection', () => {
		expect(reconcileSelectedAgentIds(['online', 'offline', 'removed'], [agent('online', true), agent('offline', false)]))
			.toEqual(new Set(['online']));
	});

	it('only returns currently connected execution targets', () => {
		expect(getRunnableAgentIds(['connected', 'stale'], [agent('connected', true), agent('stale', false)]))
			.toEqual(['connected']);
	});
});
