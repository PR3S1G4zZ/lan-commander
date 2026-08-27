import { describe, expect, it } from 'vitest';
import { canDownloadEntry, createTransferState, normalizeTransferError } from './transferState';

describe('transfer state helpers', () => {
	it('allows downloads for files but not directories', () => {
		expect(canDownloadEntry({ is_dir: false })).toBe(true);
		expect(canDownloadEntry({ is_dir: true })).toBe(false);
	});

	it('allows only one active transfer and releases it by kind', () => {
		const state = createTransferState();
		expect(state.busy).toBe(false);
		expect(state.begin('download')).toBe(true);
		expect(state.busy).toBe(true);
		expect(state.begin('upload')).toBe(false);
		state.end('upload');
		expect(state.busy).toBe(true);
		state.end('download');
		expect(state.busy).toBe(false);
	});

	it('normalizes Wails and plain errors for notifications', () => {
		expect(normalizeTransferError(new Error('network failed'))).toBe('network failed');
		expect(normalizeTransferError({ message: 'checksum mismatch' })).toBe('checksum mismatch');
		expect(normalizeTransferError('cancelled')).toBe('cancelled');
	});
});
