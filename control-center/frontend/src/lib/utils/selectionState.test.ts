import { describe, expect, it } from 'vitest';
import { clearSelectionAfterDisconnect } from './selectionState';

describe('clearSelectionAfterDisconnect', () => {
	it('clears the selected agent when that agent disconnects', () => {
		expect(clearSelectionAfterDisconnect('agent-1', 'agent-1')).toBeNull();
	});

	it('keeps a different selected agent untouched', () => {
		expect(clearSelectionAfterDisconnect('agent-1', 'agent-2')).toBe('agent-2');
	});
});
