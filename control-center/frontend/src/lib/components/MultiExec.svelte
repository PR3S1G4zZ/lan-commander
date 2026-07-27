<script lang="ts">
	import { agents, type CommandResult } from '../stores/agents';
	import { addNotification } from '../stores/ui';
	import { execCommandMulti } from '../utils/api';
	import Icon from './Icon.svelte';

	let command = $state('');
	let selectedAgentIds = $state<Set<string>>(new Set());
	let results = $state<Record<string, CommandResult>>({});
	let running = $state(false);

	function toggleAgent(id: string) {
		const newSet = new Set(selectedAgentIds);
		newSet.has(id) ? newSet.delete(id) : newSet.add(id);
		selectedAgentIds = newSet;
	}

	function selectAll() { selectedAgentIds = new Set($agents.filter(a => a.connected).map(a => a.id)); }
	function deselectAll() { selectedAgentIds = new Set(); }

	async function run() {
		if (!command.trim() || running) return;
		const ids = Array.from(selectedAgentIds);
		if (ids.length === 0) { addNotification('warning', 'Select at least one agent'); return; }
		running = true;
		results = {};
		try {
			const res = (await execCommandMulti(ids, command.trim(), 30)) as Record<string, CommandResult>;
			results = res;
			const ok = Object.values(res).filter(r => r.exit_code === 0).length;
			addNotification('info', `Executed on ${ids.length} agents: ${ok} succeeded`);
		} catch (err: any) { addNotification('error', `Multi-exec failed: ${err.message || err}`); }
		finally { running = false; }
	}
</script>

<div class="p-6 h-full overflow-y-auto box-border">
	<div class="mb-4">
		<h2 class="text-lg font-semibold text-slate-100">Multi-Execute</h2>
		<p class="text-sm text-slate-500">Run commands on multiple agents simultaneously</p>
	</div>

	<div class="mb-4 p-4 bg-slate-800/50 rounded-xl border border-slate-700">
		<div class="flex justify-between items-center mb-2">
			<span class="text-sm text-slate-400">Target Agents ({selectedAgentIds.size} selected)</span>
			<div class="flex gap-2">
				<button class="text-xs text-cyan-400 hover:text-cyan-300 bg-transparent border-none cursor-pointer" onclick={selectAll}>All</button>
				<button class="text-xs text-slate-500 hover:text-slate-400 bg-transparent border-none cursor-pointer" onclick={deselectAll}>None</button>
			</div>
		</div>
		<div class="flex flex-wrap gap-2">
			{#each $agents.filter(a => a.connected) as agent (agent.id)}
				<button class="flex items-center gap-1.5 px-3 py-1.5 rounded-full text-sm transition-all bg-slate-700 text-slate-300 hover:bg-slate-600 cursor-pointer border-none {selectedAgentIds.has(agent.id) ? 'bg-cyan-600/20 text-cyan-300 border border-cyan-500/30' : ''}" onclick={() => toggleAgent(agent.id)}>
					<span class="w-1.5 h-1.5 rounded-full bg-green-500"></span>
					{agent.name}
				</button>
			{/each}
			{#if $agents.filter(a => a.connected).length === 0}
				<span class="text-sm text-slate-500">No connected agents</span>
			{/if}
		</div>
	</div>

	<div class="mb-4 flex gap-2">
		<input type="text" bind:value={command} placeholder="Enter command..." disabled={running} onkeydown={(e) => e.key === 'Enter' && run()} class="flex-1 bg-slate-800 text-slate-200 rounded-xl px-4 py-3 text-sm font-mono outline-none border border-slate-700 focus:border-cyan-500 box-border" />
		<button class="flex items-center gap-1.5 px-6 py-3 rounded-xl font-medium bg-gradient-to-r from-cyan-600 to-blue-600 hover:from-cyan-500 hover:to-blue-500 text-white disabled:opacity-50 cursor-pointer border-none shadow-lg shadow-cyan-500/10" onclick={run} disabled={running}>
			{#if !running}<Icon name="play" size={14} />{/if} {running ? 'Running...' : 'Run'}
		</button>
	</div>

	{#if Object.keys(results).length > 0}
		<div>
			<h3 class="text-sm font-medium text-slate-300 mb-2">Results</h3>
			<div class="grid gap-3" style="grid-template-columns: repeat(auto-fill, minmax(400px, 1fr));">
				{#each $agents.filter(a => results[a.id]) as agent (agent.id)}
					{@const result = results[agent.id]}
					<div class="bg-slate-800/50 rounded-xl border p-4 {result.exit_code !== 0 ? 'border-red-900/50' : 'border-slate-700'}">
						<div class="flex items-center gap-2 mb-2 text-sm">
							<span class="w-2 h-2 rounded-full {result.exit_code === 0 ? 'bg-green-500' : 'bg-red-500'}"></span>
							<span class="font-medium text-slate-200">{agent.name}</span>
							<span class="text-xs text-slate-500">exit: {result.exit_code} ({result.duration_ms}ms)</span>
						</div>
						{#if result.stdout}<pre class="text-xs font-mono text-green-400 whitespace-pre-wrap bg-slate-900/50 rounded p-2 max-h-32 overflow-y-auto">{result.stdout}</pre>{/if}
						{#if result.stderr}<pre class="text-xs font-mono text-red-400 whitespace-pre-wrap bg-slate-900/50 rounded p-2 mt-1 max-h-32 overflow-y-auto">{result.stderr}</pre>{/if}
					</div>
				{/each}
			</div>
		</div>
	{/if}
</div>
