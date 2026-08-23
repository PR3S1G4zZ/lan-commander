<script lang="ts">
	import { agents, selectedAgentId, type AgentInfo } from './lib/stores/agents';
	import { currentView, sidebarCollapsed, type ViewType } from './lib/stores/ui';
	import { addNotification } from './lib/stores/ui';
	import { disconnectAgent, getAgents, getSystemInfo } from './lib/utils/api';
	import { getErrorMessage } from './lib/utils/format';
	import { clearSelectionAfterDisconnect } from './lib/utils/selectionState';
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

	let polling = false;
	let backendError = $state<string | null>(null);
	let disconnecting = $state(false);
	const systemInfoRequests = new Set<string>();

	async function disconnectSelectedAgent(): Promise<void> {
		const agent = $agents.find((item: AgentInfo) => item.id === $selectedAgentId);
		if (!agent || disconnecting) return;
		if (!globalThis.confirm(`Disconnect from ${agent.name}?`)) return;

		disconnecting = true;
		try {
			await disconnectAgent(agent.id);
			$selectedAgentId = clearSelectionAfterDisconnect(agent.id, $selectedAgentId);
			addNotification('success', `Disconnected from ${agent.name}`);
		} catch (err) {
			addNotification('error', `Disconnect failed: ${getErrorMessage(err)}`);
		} finally {
			disconnecting = false;
		}
	}

	async function refreshSystemInfo(agentId: string): Promise<void> {
		if (systemInfoRequests.has(agentId)) return;
		systemInfoRequests.add(agentId);
		try {
			const sysInfo = await getSystemInfo(agentId);
			if (!sysInfo) throw new Error('Agent returned no system information');

			const currentAgent = $agents.find((agent: AgentInfo) => agent.id === agentId);
			if (!currentAgent?.connected) return;
			const cpuPct = (sysInfo as any).cpu?.percent || 0;
			const history = [...(currentAgent.cpuHistory || []), cpuPct].slice(-60);
			$agents = $agents.map((agent: AgentInfo) => agent.id === agentId
				? { ...agent, systemInfo: sysInfo as any, systemInfoError: null, cpuHistory: history }
				: agent
			);
		} catch (err) {
			const message = getErrorMessage(err);
			$agents = $agents.map((agent: AgentInfo) => agent.id === agentId && agent.connected
				? { ...agent, systemInfoError: message }
				: agent
			);
		} finally {
			systemInfoRequests.delete(agentId);
		}
	}

	async function pollAgents(): Promise<void> {
		if (polling) return;
		polling = true;
		try {
			const agentList = await getAgents();
			backendError = null;
			const previous = new Map($agents.map((agent: AgentInfo) => [agent.id, agent]));
			$agents = agentList.map((incoming: AgentInfo) => {
				const prev = previous.get(incoming.id);
				if (!prev) return incoming;
				return {
					...incoming,
					systemInfo: incoming.systemInfo ?? prev.systemInfo,
					systemInfoError: incoming.systemInfo ? null : (prev.systemInfoError ?? null),
					cpuHistory: prev.cpuHistory ?? [],
				};
			});

			// Start each request independently. A slow agent does not block the
			// successful results from updating the dashboard for other agents.
			const requests = agentList
				.filter((agent: AgentInfo) => agent.connected)
				.map((agent: AgentInfo) => refreshSystemInfo(agent.id));
			void Promise.allSettled(requests);
		} catch (err) {
			backendError = getErrorMessage(err);
			$agents = $agents.map((agent: AgentInfo) => ({
				...agent,
				connected: false,
				systemInfoError: backendError,
			}));
		} finally {
			polling = false;
		}
	}

	// Poll the agent list every 2 seconds; system info requests run concurrently.
	$effect(() => {
		void pollAgents();
		const interval = setInterval(() => void pollAgents(), 2000);
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
			{#if backendError}
				<div class="flex items-center gap-2 px-4 py-2 bg-red-500/10 border-b border-red-500/30 text-sm text-red-300" role="alert" aria-live="polite">
					<Icon name="alert-triangle" size={15} />
					<span>Backend unavailable: {backendError}</span>
				</div>
			{/if}
			<nav class="flex items-center justify-between px-4 py-2 bg-slate-900/80 border-b border-slate-800 backdrop-blur-sm">
				<div class="flex items-center gap-3">
					<button aria-label={$sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'} class="flex items-center justify-center text-slate-400 hover:text-slate-200 bg-transparent border-none cursor-pointer p-1 rounded-lg hover:bg-slate-800 transition-colors" onclick={() => $sidebarCollapsed = !$sidebarCollapsed}>
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
							{@const statusLabel = agent.connected ? 'Connected' : agent.systemInfoError ? 'Error' : 'Disconnected'}
							{@const statusColor = agent.connected ? 'bg-emerald-500 shadow-[0_0_6px_theme(colors.emerald.500)]' : agent.systemInfoError ? 'bg-red-500' : 'bg-slate-500'}
							<span class="flex items-center gap-1.5 px-3 py-1 rounded-full bg-slate-800 text-xs text-slate-300 mr-2">
								<span class="w-1.5 h-1.5 rounded-full {statusColor}"></span>
								{agent.name}
								<span class="text-xs {agent.connected ? 'text-emerald-400' : agent.systemInfoError ? 'text-red-400' : 'text-slate-500'}">{statusLabel}</span>
								<span class="text-xs text-slate-500 ml-1">{agent.host}:{agent.port}</span>
							</span>
							<button
								aria-label={`Disconnect from ${agent.name}`}
								class="px-2 py-1 rounded text-xs text-red-300 hover:text-red-200 hover:bg-red-500/10 disabled:opacity-50 cursor-pointer bg-transparent border border-red-500/30"
								onclick={disconnectSelectedAgent}
								disabled={disconnecting || !agent.connected}
								title="Disconnect"
							>
								{disconnecting ? 'Disconnecting…' : 'Disconnect'}
							</button>
						{/if}
					{/if}
					{#each views as view}
						<button
							class="flex items-center justify-center p-2 rounded-lg transition-colors bg-transparent border-none cursor-pointer {$currentView === view.id ? 'bg-slate-800 text-cyan-400' : 'text-slate-500 hover:text-slate-300 hover:bg-slate-800'}"
							onclick={() => $currentView = view.id}
							title={view.label}
							aria-label={view.label}
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
