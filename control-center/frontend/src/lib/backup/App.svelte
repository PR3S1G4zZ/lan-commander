<script lang="ts">
	import { agents, selectedAgentId, type AgentInfo } from './lib/stores/agents';
	import { currentView, sidebarCollapsed, addNotification, type ViewType } from './lib/stores/ui';
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

	let polling = $state(false);

	// Poll agents and system info every 2 seconds
	$effect(() => {
		const interval = setInterval(async () => {
			if (polling) return;
			polling = true;
			try {
				const agentList = (await getAgents()) as AgentInfo[];
				if (agentList && agentList.length > 0) {
					$agents = agentList;
					for (const agent of agentList.filter((a: AgentInfo) => a.connected)) {
						try {
							const sysInfo = await getSystemInfo(agent.id);
							if (sysInfo) {
								$agents = $agents.map((a: AgentInfo) => {
									if (a.id === agent.id) {
										const history = a.cpuHistory || [];
										const cpuPct = (sysInfo as any).cpu?.percent || 0;
										history.push(cpuPct);
										if (history.length > 60) history.shift();
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
		{ id: 'dashboard', label: 'Dashboard', icon: '📊' },
		{ id: 'terminal', label: 'Terminal', icon: '>_' },
		{ id: 'files', label: 'Files', icon: '📁' },
		{ id: 'multi-exec', label: 'Multi-Exec', icon: '⚡' },
		{ id: 'scripts', label: 'Scripts', icon: '📜' },
		{ id: 'audit', label: 'Audit', icon: '📋' },
		{ id: 'settings', label: 'Settings', icon: '⚙️' },
	];
</script>

<div class="h-screen w-screen overflow-hidden bg-slate-900 text-slate-200">
	<Notifications />
	<div class="flex h-full">
		<Sidebar />
		<main class="flex-1 flex flex-col overflow-hidden">
			<nav class="flex items-center justify-between px-4 py-2 bg-slate-900/80 border-b border-slate-800 backdrop-blur-sm">
				<div class="flex items-center gap-3">
					<button class="text-slate-400 hover:text-slate-200 text-lg bg-transparent border-none cursor-pointer" onclick={() => $sidebarCollapsed = !$sidebarCollapsed}>☰</button>
					<span class="text-sm font-medium text-slate-300">
						{views.find(v => v.id === $currentView)?.icon}
						{views.find(v => v.id === $currentView)?.label}
					</span>
				</div>
				<div class="flex items-center gap-1">
					{#if $selectedAgentId}
						{@const agent = $agents.find(a => a.id === $selectedAgentId)}
						{#if agent}
							<span class="flex items-center gap-1.5 px-3 py-1 rounded-full bg-slate-800 text-xs text-slate-300 mr-2">
								<span class="w-1.5 h-1.5 rounded-full bg-green-500"></span>
								{agent.name}
								<span class="text-xs text-slate-600 ml-1">{agent.host}:{agent.port}</span>
							</span>
						{/if}
					{/if}
					{#each views as view}
						<button
							class="px-2.5 py-1.5 rounded-lg text-sm transition-colors bg-transparent border-none cursor-pointer {$currentView === view.id ? 'bg-slate-800 text-cyan-400' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-800'}"
							onclick={() => $currentView = view.id}
							title={view.label}
						>{view.icon}</button>
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
