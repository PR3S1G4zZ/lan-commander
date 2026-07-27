<script lang="ts">
	import { agents, selectedAgentId, agentCounts, type AgentInfo } from '../stores/agents';
	import { sidebarCollapsed, currentView } from '../stores/ui';
	import { getOsMeta, formatTimeAgo } from '../utils/format';
	import ConnectionDialog from './ConnectionDialog.svelte';
	import Icon from './Icon.svelte';

	let showConnectionDialog = $state(false);
	let searchQuery = $state('');

	const filteredAgents = $derived(
		$agents.filter(a =>
			a.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
			a.host.includes(searchQuery) ||
			a.os.toLowerCase().includes(searchQuery.toLowerCase())
		)
	);

	function selectAgent(id: string) {
		$selectedAgentId = id;
		$currentView = 'dashboard';
	}

	function getStatusColor(agent: AgentInfo): string {
		if (agent.connected) return 'bg-emerald-500 shadow-[0_0_6px_theme(colors.emerald.500)]';
		if (agent.lastSeen && Date.now() - new Date(agent.lastSeen).getTime() < 30000) return 'bg-amber-500';
		return 'bg-red-500';
	}
</script>

<aside class="bg-slate-800 border-r border-slate-700 flex flex-col h-full transition-all duration-200" class:w-64={!$sidebarCollapsed} class:w-12={$sidebarCollapsed}>
	<div class="flex items-center justify-between px-3 py-3 border-b border-slate-700">
		{#if !$sidebarCollapsed}
			<div class="flex items-center gap-2">
				<span class="flex items-center justify-center w-6 h-6 rounded-md bg-gradient-to-br from-cyan-500 to-blue-600 text-white shadow-lg shadow-cyan-500/25">
					<Icon name="logo" size={15} />
				</span>
				<span class="font-bold text-slate-100 text-sm">LAN Commander</span>
			</div>
		{/if}
		<button class="text-slate-400 hover:text-slate-200 p-1 rounded bg-transparent border-none cursor-pointer" onclick={() => $sidebarCollapsed = !$sidebarCollapsed}>
			<Icon name={$sidebarCollapsed ? 'chevron-right' : 'chevron-left'} size={16} />
		</button>
	</div>

	{#if !$sidebarCollapsed}
		<div class="p-2 relative">
			<Icon name="search" size={14} class="absolute left-4.5 top-1/2 -translate-y-1/2 text-slate-500 pointer-events-none" />
			<input
				type="text"
				placeholder="Search agents..."
				bind:value={searchQuery}
				class="w-full bg-slate-700 text-slate-200 rounded px-3 py-1.5 pl-8 text-sm outline-none border border-slate-600 focus:border-cyan-500 box-border"
			/>
		</div>

		<div class="flex items-center px-3 py-2 text-xs text-slate-400">
			<span class="w-2 h-2 rounded-full inline-block mr-2 flex-shrink-0 bg-emerald-500 shadow-[0_0_6px_theme(colors.emerald.500)]"></span>
			<span>{$agentCounts.connected} connected</span>
			<span class="ml-2 text-slate-500">/ {$agentCounts.total} total</span>
		</div>

		<div class="flex-1 overflow-y-auto px-2 space-y-1">
			{#each filteredAgents as agent (agent.id)}
				{@const osMeta = getOsMeta(agent.os)}
				<button
					class="w-full flex items-center gap-2 px-3 py-2 rounded-lg text-left text-sm transition-colors cursor-pointer text-slate-300 hover:bg-slate-700/50 border-none"
					class:bg-slate-700={$selectedAgentId === agent.id}
					class:text-slate-100={$selectedAgentId === agent.id}
					onclick={() => selectAgent(agent.id)}
				>
					<span class="w-2 h-2 rounded-full flex-shrink-0 {getStatusColor(agent)}"></span>
					<span class="flex-1 truncate font-medium">{agent.name}</span>
					<span class="text-[10px] font-bold tracking-wide px-1.5 py-0.5 rounded {osMeta.color}">{osMeta.label}</span>
					{#if !agent.connected}
						<span class="text-xs text-slate-500">{formatTimeAgo(agent.lastSeen)}</span>
					{/if}
				</button>
			{/each}

			{#if filteredAgents.length === 0}
				<div class="text-center py-8 text-sm text-slate-500">
					<p>No agents found</p>
					<p class="text-xs text-slate-600">Click + to add manually</p>
				</div>
			{/if}
		</div>

		<div class="p-3 border-t border-slate-700">
			<button class="w-full flex items-center justify-center gap-2 py-2 rounded-lg text-sm font-medium bg-gradient-to-r from-cyan-600 to-blue-600 hover:from-cyan-500 hover:to-blue-500 text-white transition-all cursor-pointer border-none shadow-lg shadow-cyan-500/10" onclick={() => showConnectionDialog = true}>
				<Icon name="plus" size={15} /> Add Agent
			</button>
		</div>
	{:else}
		<div class="flex flex-col items-center gap-2 py-3">
			{#each $agents as agent (agent.id)}
				<button
					class="w-8 h-8 flex items-center justify-center rounded hover:bg-slate-700 cursor-pointer border-none {$selectedAgentId === agent.id ? 'bg-slate-700 ring-1 ring-cyan-500' : ''}"
					onclick={() => selectAgent(agent.id)}
					title={agent.name}
				>
					<span class="w-2 h-2 rounded-full {agent.connected ? 'bg-emerald-500' : 'bg-red-500'}"></span>
				</button>
			{/each}
		</div>
	{/if}
</aside>

{#if showConnectionDialog}
	<ConnectionDialog onclose={() => showConnectionDialog = false} />
{/if}
