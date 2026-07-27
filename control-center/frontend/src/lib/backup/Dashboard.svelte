<script lang="ts">
	import { agents, selectedAgentId, type AgentInfo, type SystemInfo } from '../stores/agents';
	import { currentView } from '../stores/ui';
	import { formatBytes, formatPercent, formatUptime, getOsIcon } from '../utils/format';

	function getCpuColor(pct: number): string {
		if (pct > 90) return 'from-red-500 to-red-600';
		if (pct > 70) return 'from-yellow-500 to-orange-500';
		return 'from-cyan-500 to-blue-500';
	}

	function getMemColor(pct: number): string {
		if (pct > 90) return 'from-red-500 to-red-600';
		if (pct > 70) return 'from-yellow-500 to-orange-500';
		return 'from-emerald-500 to-teal-500';
	}

	function viewTerminal(agentId: string) {
		$selectedAgentId = agentId;
		$currentView = 'terminal';
	}

	function viewFiles(agentId: string) {
		$selectedAgentId = agentId;
		$currentView = 'files';
	}
</script>

<div class="p-6 h-full overflow-y-auto box-border">
	{#if $agents.filter(a => a.connected).length > 0}
		<div class="grid gap-4" style="grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));">
			{#each $agents.filter(a => a.connected) as agent (agent.id)}
				<div class="bg-slate-800/50 rounded-xl border border-slate-700 overflow-hidden hover:border-slate-600 transition-all duration-200">
					<div class="flex items-center gap-2 px-4 py-3 border-b border-slate-700/50">
						<div class="flex items-center gap-2 flex-1">
							<span class="w-2 h-2 rounded-full bg-green-500 flex-shrink-0"></span>
							<h3 class="text-sm font-semibold text-slate-100">{agent.name}</h3>
						</div>
						<span class="text-xs text-slate-400">{getOsIcon(agent.os)} {agent.os}</span>
						<span class="text-xs text-slate-600 font-mono">{agent.host}:{agent.port}</span>
					</div>

					{#if agent.systemInfo}
						{@const sys = agent.systemInfo}
						<div class="p-4 space-y-3">
							<div>
								<div class="flex justify-between text-xs text-slate-300 mb-1">
									<span>CPU</span>
									<span class="font-bold font-mono">{formatPercent(sys.cpu.percent)}</span>
								</div>
								<div class="w-full h-2 bg-slate-700 rounded-full overflow-hidden">
									<div class="h-full rounded-full bg-gradient-to-r {getCpuColor(sys.cpu.percent)}" style="width: {sys.cpu.percent}%; transition: width 0.5s ease-out;"></div>
								</div>
								<div class="text-xs text-slate-600 mt-0.5">{sys.cpu.cores} cores</div>
							</div>

							<div>
								<div class="flex justify-between text-xs text-slate-300 mb-1">
									<span>RAM</span>
									<span class="font-bold font-mono">{formatPercent(sys.memory.percent)}</span>
								</div>
								<div class="w-full h-2 bg-slate-700 rounded-full overflow-hidden">
									<div class="h-full rounded-full bg-gradient-to-r {getMemColor(sys.memory.percent)}" style="width: {sys.memory.percent}%; transition: width 0.5s ease-out;"></div>
								</div>
								<div class="text-xs text-slate-600 mt-0.5">{formatBytes(sys.memory.used)} / {formatBytes(sys.memory.total)}</div>
							</div>

							{#if sys.disks && sys.disks.length > 0}
								{@const disk = sys.disks[0]}
								<div>
									<div class="flex justify-between text-xs text-slate-300 mb-1">
										<span>Disk ({disk.mount})</span>
										<span class="font-bold font-mono">{formatPercent(disk.percent)}</span>
									</div>
									<div class="w-full h-2 bg-slate-700 rounded-full overflow-hidden">
										<div class="h-full rounded-full bg-gradient-to-r from-violet-500 to-purple-500" style="width: {disk.percent}%; transition: width 0.5s ease-out;"></div>
									</div>
									<div class="text-xs text-slate-600 mt-0.5">{formatBytes(disk.used)} / {formatBytes(disk.total)}</div>
								</div>
							{/if}
						</div>

						<div class="flex items-center justify-between px-4 py-2 bg-slate-800/30 border-t border-slate-700/50">
							<span class="text-xs text-slate-500">↑ {formatUptime(sys.uptime)}</span>
							<div class="flex gap-1">
								<button class="px-2 py-1 text-xs rounded bg-slate-700 hover:bg-slate-600 text-slate-300 transition-colors cursor-pointer border-none" onclick={() => viewTerminal(agent.id)} title="Terminal">&gt;_</button>
								<button class="px-2 py-1 text-xs rounded bg-slate-700 hover:bg-slate-600 text-slate-300 transition-colors cursor-pointer border-none" onclick={() => viewFiles(agent.id)} title="Files">📁</button>
							</div>
						</div>
					{:else}
						<div class="flex flex-col items-center justify-center py-8 text-sm text-slate-500">
							<div class="w-6 h-6 border-2 border-slate-600 border-t-cyan-500 rounded-full animate-spin mb-2"></div>
							<span>Connecting...</span>
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{:else}
		<div class="flex flex-col items-center justify-center h-full text-center">
			<div class="text-6xl mb-4">⚡</div>
			<h2 class="text-2xl font-bold text-slate-200 mb-2">LAN Commander</h2>
			<p class="text-slate-400">No agents connected</p>
			<p class="text-sm text-slate-600 mt-2">Install <code class="bg-slate-800 px-1 rounded">lan-agent</code> on your machines or add them manually</p>
		</div>
	{/if}
</div>
