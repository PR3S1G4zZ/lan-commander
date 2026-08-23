export function clearSelectionAfterDisconnect(
	disconnectedAgentId: string,
	selectedAgentId: string | null,
): string | null {
	return disconnectedAgentId === selectedAgentId ? null : selectedAgentId;
}
