import type { DirEntry } from '../stores/agents';

export type TransferKind = 'download' | 'upload';

export function canDownloadEntry(entry: Pick<DirEntry, 'is_dir'>): boolean {
	return !entry.is_dir;
}

export function normalizeTransferError(error: unknown): string {
	if (error instanceof Error) return error.message;
	if (typeof error === 'string') return error;
	if (error && typeof error === 'object' && 'message' in error) {
		const message = (error as { message?: unknown }).message;
		if (typeof message === 'string' && message.trim()) return message;
	}
	return String(error);
}

export function createTransferState() {
	let active: TransferKind | null = null;

	return {
		get busy(): boolean {
			return active !== null;
		},
		get kind(): TransferKind | null {
			return active;
		},
		begin(kind: TransferKind): boolean {
			if (active !== null) return false;
			active = kind;
			return true;
		},
		end(kind: TransferKind): void {
			if (active === kind) active = null;
		},
	};
}
