<script lang="ts">
	import { agents, selectedAgentId } from '../stores/agents';
	import { addNotification } from '../stores/ui';
	import { downloadFile, listDir, uploadFile } from '../utils/api';
	import { formatBytes, formatTime, getErrorMessage, getFileMeta } from '../utils/format';
	import { canDownloadEntry, createTransferState, normalizeTransferError, type TransferKind } from '../utils/transferState';
	import type { DirEntry, DirContents } from '../stores/agents';
	import Icon from './Icon.svelte';

	let currentPath = $state('/');
	let entries = $state<DirEntry[]>([]);
	let loading = $state(false);
	let breadcrumbs = $state<string[]>([]);
	let loadError = $state<string | null>(null);
	let activeAgentId = $state<string | null>(null);
	let navigationRequest = 0;
	let transferBusy = $state(false);
	const transferState = createTransferState();

	$effect(() => {
		const agentId = $selectedAgentId;
		if (agentId === activeAgentId) return;
		activeAgentId = agentId;
		navigationRequest++;
		currentPath = '/';
		entries = [];
		updateBreadcrumbs('/');
		loadError = null;
		if (agentId) void navigateTo('/', agentId);
		else loading = false;
	});

	function updateBreadcrumbs(path: string) {
		const parts = path.replace(/\\/g, '/').split('/').filter(Boolean);
		breadcrumbs = path === '/' ? ['Root'] : parts;
	}

	async function navigateTo(path: string, requestedAgentId: string | null = $selectedAgentId) {
		if (!requestedAgentId) return;
		const agent = $agents.find(item => item.id === requestedAgentId);
		if (!agent) return;
		if (!agent.connected) {
			if (requestedAgentId === $selectedAgentId) {
				entries = [];
				loadError = 'Agent is disconnected';
				loading = false;
			}
			return;
		}
		const requestId = ++navigationRequest;
		loading = true;
		currentPath = path;
		updateBreadcrumbs(path);
		loadError = null;
		try {
			const result = (await listDir(requestedAgentId, path)) as DirContents;
			if (requestId !== navigationRequest || requestedAgentId !== $selectedAgentId) return;
			entries = result.entries || [];
		} catch (err) {
			if (requestId !== navigationRequest || requestedAgentId !== $selectedAgentId) return;
			loadError = getErrorMessage(err);
			addNotification('error', `Failed to list directory: ${loadError}`);
			entries = [];
		} finally {
			if (requestId === navigationRequest) loading = false;
		}
	}

	function navigateToCrumb(index: number) {
		const parts = currentPath.replace(/\\/g, '/').split('/').filter(Boolean);
		navigateTo('/' + parts.slice(0, index + 1).join('/'));
	}

	function openEntry(entry: DirEntry) { if (entry.is_dir) navigateTo(entry.path); }
	function goBack() { navigateTo(currentPath.substring(0, currentPath.lastIndexOf('/')) || '/'); }

	function beginTransfer(kind: TransferKind): boolean {
		const started = transferState.begin(kind);
		transferBusy = transferState.busy;
		return started;
	}

	function endTransfer(kind: TransferKind): void {
		transferState.end(kind);
		transferBusy = transferState.busy;
	}

	async function downloadEntry(entry: DirEntry): Promise<void> {
		const agentId = $selectedAgentId;
		if (!agentId || !canDownloadEntry(entry) || !beginTransfer('download')) return;
		try {
			await downloadFile(agentId, entry.path);
			addNotification('success', `Downloaded ${entry.name}`);
		} catch (error) {
			addNotification('error', `Download failed: ${normalizeTransferError(error)}`);
		} finally {
			endTransfer('download');
		}
	}

	async function uploadToCurrentDirectory(): Promise<void> {
		const agentId = $selectedAgentId;
		if (!agentId || !beginTransfer('upload')) return;
		try {
			await uploadFile(agentId, currentPath);
			addNotification('success', 'File uploaded');
			await navigateTo(currentPath, agentId);
		} catch (error) {
			addNotification('error', `Upload failed: ${normalizeTransferError(error)}`);
		} finally {
			endTransfer('upload');
		}
	}
</script>

<div class="h-full flex flex-col bg-slate-900">
	<div class="flex items-center justify-between px-4 py-2 bg-slate-800 border-b border-slate-700">
		<div class="flex items-center gap-1 text-sm font-mono">
			<button aria-label="Open agent root directory" class="flex items-center text-amber-400 hover:text-amber-300 px-1 py-0.5 rounded bg-transparent border-none cursor-pointer disabled:opacity-40" onclick={() => navigateTo('/')} disabled={transferBusy}>
				<Icon name="folder" size={15} />
			</button>
			{#each breadcrumbs as crumb, i}
				<span class="text-slate-600">/</span>
				<button class="text-slate-400 hover:text-slate-200 px-1 py-0.5 rounded bg-transparent border-none cursor-pointer disabled:opacity-40 {i === breadcrumbs.length - 1 ? 'text-cyan-400' : ''}" onclick={() => navigateToCrumb(i)} disabled={transferBusy}>{crumb}</button>
			{/each}
		</div>
		<div class="flex items-center gap-2">
			<button aria-label="Upload file to current directory" class="flex items-center gap-1 px-3 py-1 text-xs rounded bg-cyan-700 hover:bg-cyan-600 text-white disabled:opacity-50 cursor-pointer border-none" onclick={uploadToCurrentDirectory} disabled={!$selectedAgentId || transferBusy}>
				<Icon name="plus" size={12} /> Upload
			</button>
			<button aria-label="Go to parent directory" class="flex items-center gap-1 px-3 py-1 text-xs rounded bg-slate-700 hover:bg-slate-600 text-slate-300 disabled:opacity-50 cursor-pointer border-none" onclick={goBack} disabled={currentPath === '/' || transferBusy}>
				<Icon name="arrow-left" size={12} /> Back
			</button>
		</div>
	</div>

	<div class="flex-1 overflow-y-auto">
		<div class="flex items-center px-4 py-2 text-xs text-slate-500 font-medium border-b border-slate-800 bg-slate-800/30">
			<span class="flex-1">Name</span>
			<span class="w-24 text-right">Size</span>
			<span class="w-32 text-right">Modified</span>
			<span class="w-20 text-right">Actions</span>
		</div>

		{#if loadError}
			<div class="flex flex-col items-center justify-center gap-2 py-12 text-sm text-red-300" role="alert">
				<Icon name="alert-triangle" size={20} />
				<span>{loadError}</span>
			</div>
		{:else if loading}
			<div class="flex items-center justify-center gap-2 py-12 text-sm text-slate-500">
				<div class="w-4 h-4 border-2 border-slate-600 border-t-cyan-500 rounded-full animate-spin"></div>
				Loading...
			</div>
		{:else if entries.length === 0}
			<div class="flex items-center justify-center py-12 text-sm text-slate-500">Empty directory</div>
		{:else}
			{#each entries as entry (entry.path)}
				{@const meta = getFileMeta(entry)}
				<div class="w-full flex items-center px-4 py-2 text-sm text-slate-300 border-b border-slate-800/30 hover:bg-slate-800/50">
					<button class="flex-1 flex items-center gap-2 min-w-0 bg-transparent border-none text-left text-slate-300 cursor-pointer p-0 disabled:opacity-40" onclick={() => openEntry(entry)} disabled={!entry.is_dir || transferBusy} aria-label={entry.is_dir ? `Open directory ${entry.name}` : entry.name}>
						<Icon name={meta.icon} size={15} class={meta.color} />
						<span class="truncate">{entry.name}</span>
					</button>
					<span class="w-24 text-right text-slate-500">{entry.is_dir ? '--' : formatBytes(entry.size)}</span>
					<span class="w-32 text-right text-slate-500">{formatTime(entry.mod_time)}</span>
					<span class="w-20 flex justify-end">
						{#if canDownloadEntry(entry)}
							<button aria-label={`Download ${entry.name}`} title="Download" class="p-1 rounded text-slate-400 hover:text-cyan-300 hover:bg-slate-700 disabled:opacity-40 cursor-pointer border-none bg-transparent" onclick={() => downloadEntry(entry)} disabled={transferBusy}>
								<Icon name="save" size={14} />
							</button>
						{/if}
					</span>
				</div>
			{/each}
		{/if}
	</div>
</div>
