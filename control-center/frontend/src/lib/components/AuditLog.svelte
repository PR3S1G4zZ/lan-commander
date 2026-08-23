<script lang="ts">
	import { type AuditEntry } from '../stores/agents';
	import { getAuditLogs } from '../utils/api';
	import { addNotification } from '../stores/ui';
	import { getErrorMessage } from '../utils/format';

	let logs = $state<AuditEntry[]>([]);
	let loading = $state(false);
	let filterAction = $state('');
	let searchText = $state('');
	let loadError = $state<string | null>(null);

	$effect(() => { refresh(); });

	async function refresh() {
		loading = true;
		loadError = null;
		try {
			logs = (await getAuditLogs(200)) as AuditEntry[];
		} catch (err) {
			loadError = getErrorMessage(err);
			addNotification('error', `Failed to load audit log: ${loadError}`);
		}
		finally { loading = false; }
	}

	const filteredLogs = $derived(logs.filter(log => {
		if (filterAction && !log.action.toLowerCase().includes(filterAction.toLowerCase())) return false;
		if (searchText && !JSON.stringify(log).toLowerCase().includes(searchText.toLowerCase())) return false;
		return true;
	}));
</script>

<div class="h-full flex flex-col">
	<div class="p-4 border-b border-slate-700">
		<h2 class="text-lg font-semibold text-slate-100 mb-2">Audit Log</h2>
		<div class="flex gap-2">
			<input type="text" bind:value={searchText} aria-label="Search audit log" placeholder="Search..." class="bg-slate-700 text-slate-200 rounded px-3 py-1.5 text-sm outline-none border border-slate-600 focus:border-cyan-500" />
			<input type="text" bind:value={filterAction} aria-label="Filter audit actions" placeholder="Filter action..." class="bg-slate-700 text-slate-200 rounded px-3 py-1.5 text-sm outline-none border border-slate-600 focus:border-cyan-500" />
			<button aria-label="Refresh audit log" class="px-3 py-1.5 rounded bg-slate-700 hover:bg-slate-600 text-slate-300 text-sm cursor-pointer border-none" onclick={refresh}>⟳</button>
		</div>
	</div>

	<div class="flex-1 overflow-y-auto">
		<div class="flex items-center px-4 py-2 text-xs text-slate-500 font-medium border-b border-slate-800 bg-slate-800/30">
			<span class="w-40 flex-shrink-0">Time</span>
			<span class="w-32 flex-shrink-0">Action</span>
			<span class="w-32 flex-shrink-0">Agent</span>
			<span class="flex-1">Details</span>
			<span class="w-20 flex-shrink-0">Status</span>
		</div>

		{#if loadError}
			<div class="flex flex-col items-center justify-center gap-2 py-12 text-sm text-red-300" role="alert">
				<span>Could not load audit log</span>
				<span class="text-xs text-red-400/80">{loadError}</span>
			</div>
		{:else if loading}
			<div class="flex items-center justify-center py-12 text-sm text-slate-500">Loading...</div>
		{:else if filteredLogs.length === 0}
			<div class="flex items-center justify-center py-12 text-sm text-slate-500">No entries found</div>
		{:else}
			{#each filteredLogs as log (log.id)}
				<div class="flex items-center px-4 py-2 text-sm border-b border-slate-800/50 hover:bg-slate-800/30">
					<span class="w-40 flex-shrink-0 text-xs font-mono text-slate-400">{log.timestamp ? new Date(log.timestamp).toLocaleString() : '-'}</span>
					<span class="w-32 flex-shrink-0 text-slate-300">{log.action}</span>
					<span class="w-32 flex-shrink-0 text-slate-400">{log.agent_id || '-'}</span>
					<span class="flex-1 text-slate-400 truncate">{log.details || '-'}</span>
					<span class="w-20 flex-shrink-0">
						<span class="px-2 py-0.5 rounded text-xs font-medium {log.status === 'success' ? 'bg-green-500/20 text-green-400' : log.status === 'error' ? 'bg-red-500/20 text-red-400' : 'bg-slate-500/20 text-slate-400'}">{log.status}</span>
					</span>
				</div>
			{/each}
		{/if}
	</div>
</div>
