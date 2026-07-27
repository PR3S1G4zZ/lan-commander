<script lang="ts">
	import { selectedAgent } from '../stores/agents';
	import { addNotification } from '../stores/ui';
	import { listDir } from '../utils/api';
	import { formatBytes, formatTime, getFileIcon } from '../utils/format';
	import type { DirEntry, DirContents } from '../stores/agents';

	let currentPath = $state('/');
	let entries = $state<DirEntry[]>([]);
	let loading = $state(false);
	let breadcrumbs = $state<string[]>([]);

	$effect(() => { if ($selectedAgent) navigateTo('/'); });

	function updateBreadcrumbs(path: string) {
		const parts = path.replace(/\\/g, '/').split('/').filter(Boolean);
		breadcrumbs = path === '/' ? ['Root'] : parts;
	}

	async function navigateTo(path: string) {
		if (!$selectedAgent) return;
		loading = true;
		currentPath = path;
		updateBreadcrumbs(path);
		try {
			const result = (await listDir($selectedAgent, path)) as DirContents;
			entries = result.entries || [];
		} catch (err: any) {
			addNotification('error', `Failed to list directory: ${err.message || err}`);
			entries = [];
		} finally { loading = false; }
	}

	function navigateToCrumb(index: number) {
		const parts = currentPath.replace(/\\/g, '/').split('/').filter(Boolean);
		navigateTo('/' + parts.slice(0, index + 1).join('/'));
	}

	function openEntry(entry: DirEntry) { if (entry.is_dir) navigateTo(entry.path); }
	function goBack() { navigateTo(currentPath.substring(0, currentPath.lastIndexOf('/')) || '/'); }
</script>

<div class="h-full flex flex-col bg-slate-900">
	<div class="flex items-center justify-between px-4 py-2 bg-slate-800 border-b border-slate-700">
		<div class="flex items-center gap-1 text-sm font-mono">
			<button class="text-slate-400 hover:text-slate-200 px-1 py-0.5 rounded bg-transparent border-none cursor-pointer" onclick={() => navigateTo('/')}>📁</button>
			{#each breadcrumbs as crumb, i}
				<span class="text-slate-600">/</span>
				<button class="text-slate-400 hover:text-slate-200 px-1 py-0.5 rounded bg-transparent border-none cursor-pointer {i === breadcrumbs.length - 1 ? 'text-cyan-400' : ''}" onclick={() => navigateToCrumb(i)}>{crumb}</button>
			{/each}
		</div>
		<button class="px-3 py-1 text-xs rounded bg-slate-700 hover:bg-slate-600 text-slate-300 disabled:opacity-50 cursor-pointer border-none" onclick={goBack} disabled={currentPath === '/'}>← Back</button>
	</div>

	<div class="flex-1 overflow-y-auto">
		<div class="flex items-center px-4 py-2 text-xs text-slate-500 font-medium border-b border-slate-800 bg-slate-800/30">
			<span class="flex-1">Name</span>
			<span class="w-24 text-right">Size</span>
			<span class="w-32 text-right">Modified</span>
		</div>

		{#if loading}
			<div class="flex items-center justify-center gap-2 py-12 text-sm text-slate-500">
				<div class="w-4 h-4 border-2 border-slate-600 border-t-cyan-500 rounded-full animate-spin"></div>
				Loading...
			</div>
		{:else if entries.length === 0}
			<div class="flex items-center justify-center py-12 text-sm text-slate-500">Empty directory</div>
		{:else}
			{#each entries as entry (entry.path)}
				<button class="w-full flex items-center px-4 py-2 text-sm text-slate-300 hover:bg-slate-800/50 cursor-pointer border-b border-slate-800/30 bg-transparent border-none text-left" onclick={() => openEntry(entry)}>
					<span class="flex-1 flex items-center gap-2">
						<span>{getFileIcon(entry)}</span>
						<span class="truncate">{entry.name}</span>
					</span>
					<span class="w-24 text-right text-slate-500">{entry.is_dir ? '--' : formatBytes(entry.size)}</span>
					<span class="w-32 text-right text-slate-500">{formatTime(entry.mod_time)}</span>
				</button>
			{/each}
		{/if}
	</div>
</div>
