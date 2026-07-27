<script lang="ts">
	import { agents, selectedAgentId, type AgentInfo } from './lib/stores/agents';
	import { currentView, sidebarCollapsed, type ViewType } from './lib/stores/ui';
	import { getAgents, getSystemInfo } from './lib/utils/api';
	import Sidebar from './lib/components/Sidebar.svelte';
	import Dashboard from './lib/components/Dashboard.svelte';
	import Terminal from './lib/components/Terminal.svelte';
	import FileBrowser from './lib/components/FileBrowser.svelte';
	import MultiExec from './lib/components/MultiExec.svelte';
	import ScriptEditor from './lib/components/ScriptEditor.svelte';
	import AuditLog from './lib/components/AuditLog.svelte';
	import Settings from './lib/components/Settings.svelte';
	import Notifications from './lib/components/Notifications.svelte';
	import Icon from './lib/components/Icon.svelte';

	let polling = $state(false);

	// Poll agents and system info every 2 seconds
	$effect(() => {
		const interval = setInterval(async () => {
			if (polling) return;
			polling = true;
			try {
				const agentList = await getAgents();
				{
					// Merge instead of replacing: the fresh payload has no
					// cpuHistory/systemInfo, so a wholesale assignment would
					// discard everything accumulated on previous polls.
					const previous = new Map($agents.map((a: AgentInfo) => [a.id, a]));
					$agents = agentList.map((incoming: AgentInfo) => {
						const prev = previous.get(incoming.id);
						if (!prev) return incoming;
						return {
							...incoming,
							systemInfo: incoming.systemInfo ?? prev.systemInfo,
							cpuHistory: prev.cpuHistory ?? [],
						};
					});
					for (const agent of agentList.filter((a: AgentInfo) => a.connected)) {
						try {
							const sysInfo = await getSystemInfo(agent.id);
							if (sysInfo) {
								$agents = $agents.map((a: AgentInfo) => {
									if (a.id === agent.id) {
										const cpuPct = (sysInfo as any).cpu?.percent || 0;
										const history = [...(a.cpuHistory || []), cpuPct].slice(-60);
										return { ...a, systemInfo: sysInfo as any, cpuHistory: history };
									}
									return a;
								});
							}
						} catch { /* agent disconnected */ }
					}
				}
			} catch { /* backend not ready yet */ }
			finally { polling = false; }
		}, 2000);
		return () => clearInterval(interval);
	});

	const views: { id: ViewType; label: string; icon: string }[] = [
		{ id: 'dashboard', label: 'Dashboard', icon: 'grid' },
		{ id: 'terminal', label: 'Terminal', icon: 'terminal' },
		{ id: 'files', label: 'Files', icon: 'folder' },
		{ id: 'multi-exec', label: 'Multi-Exec', icon: 'zap' },
		{ id: 'scripts', label: 'Scripts', icon: 'file-code' },
		{ id: 'audit', label: 'Audit', icon: 'clipboard-list' },
		{ id: 'settings', label: 'Settings', icon: 'sliders' },
	];
</script>

<div class="h-screen w-screen overflow-hidden bg-slate-900 text-slate-200">
	<Notifications />
	<div class="flex h-full">
		<Sidebar />
		<main class="flex-1 flex flex-col overflow-hidden">
			<nav class="flex items-center justify-between px-4 py-2 bg-slate-900/80 border-b border-slate-800 backdrop-blur-sm">
				<div class="flex items-center gap-3">
					<button class="flex items-center justify-center text-slate-400 hover:text-slate-200 bg-transparent border-none cursor-pointer p-1 rounded-lg hover:bg-slate-800 transition-colors" onclick={() => $sidebarCollapsed = !$sidebarCollapsed}>
						<Icon name="menu" size={18} />
					</button>
					<span class="flex items-center gap-1.5 text-sm font-medium text-slate-300">
						<Icon name={views.find(v => v.id === $currentView)?.icon ?? 'grid'} size={16} class="text-cyan-400" />
						{views.find(v => v.id === $currentView)?.label}
					</span>
				</div>
				<div class="flex items-center gap-1">
					{#if $selectedAgentId}
						{@const agent = $agents.find(a => a.id === $selectedAgentId)}
						{#if agent}
							<span class="flex items-center gap-1.5 px-3 py-1 rounded-full bg-slate-800 text-xs text-slate-300 mr-2">
								<span class="w-1.5 h-1.5 rounded-full bg-emerald-500 shadow-[0_0_6px_theme(colors.emerald.500)]"></span>
								{agent.name}
								<span class="text-xs text-slate-500 ml-1">{agent.host}:{agent.port}</span>
							</span>
						{/if}
					{/if}
					{#each views as view}
						<button
							class="flex items-center justify-center p-2 rounded-lg transition-colors bg-transparent border-none cursor-pointer {$currentView === view.id ? 'bg-slate-800 text-cyan-400' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-800'}"
							onclick={() => $currentView = view.id}
							title={view.label}
						><Icon name={view.icon} size={17} /></button>
					{/each}
				</div>
			</nav>
			<div class="flex-1 overflow-hidden">
				{#if $currentView === 'dashboard'}<Dashboard />
				{:else if $currentView === 'terminal'}<Terminal />
				{:else if $currentView === 'files'}<FileBrowser />
				{:else if $currentView === 'multi-exec'}<MultiExec />
				{:else if $currentView === 'scripts'}<ScriptEditor />
				{:else if $currentView === 'audit'}<AuditLog />
				{:else if $currentView === 'settings'}<Settings />
				{/if}
			</div>
		</main>
	</div>
</div>
