import type { AgentInfo } from '../stores/agents';

export function reconcileSelectedAgentIds(selectedIds: Iterable<string>, agents: AgentInfo[]): Set<string> {
	const connectedIds = new Set(agents.filter(agent => agent.connected).map(agent => agent.id));
	return new Set(Array.from(selectedIds).filter(id => connectedIds.has(id)));
}

export function getRunnableAgentIds(selectedIds: Iterable<string>, agents: AgentInfo[]): string[] {
	return Array.from(reconcileSelectedAgentIds(selectedIds, agents));
}
