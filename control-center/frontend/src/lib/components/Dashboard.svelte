<script lang="ts">
	import { onDestroy } from 'svelte';
	import { agents, selectedAgentId, type AgentInfo, type ScreenshotData } from '../stores/agents';
	import { addNotification, currentView } from '../stores/ui';
	import { requestScreenshot } from '../utils/api';
	import { formatBytes, formatPercent, formatUptime, getErrorMessage, getOsMeta } from '../utils/format';
	import Icon from './Icon.svelte';

	let screenshotLoadingAgentId = $state<string | null>(null);
	let screenshotUrl = $state<string | null>(null);
	let screenshotAgentName = $state('');
	let screenshotError = $state<string | null>(null);

	function releaseScreenshotUrl() {
		if (screenshotUrl) URL.revokeObjectURL(screenshotUrl);
		screenshotUrl = null;
	}

	onDestroy(releaseScreenshotUrl);

	function decodeScreenshotData(data: ScreenshotData['data']): Uint8Array {
		if (typeof data !== 'string') return Uint8Array.from(data);
		const binary = atob(data);
		return Uint8Array.from(binary, character => character.charCodeAt(0));
	}

	async function captureScreenshot(agent: AgentInfo) {
		screenshotLoadingAgentId = agent.id;
		screenshotError = null;
		try {
			const screenshot = (await requestScreenshot(agent.id)) as ScreenshotData;
			const bytes = decodeScreenshotData(screenshot.data);
			if (bytes.byteLength === 0) throw new Error('Agent returned an empty screenshot');
			const format = (screenshot.format || 'png').toLowerCase();
			const mimeType = format === 'jpeg' || format === 'jpg' ? 'image/jpeg' : 'image/png';
			releaseScreenshotUrl();
			const blobBytes = new ArrayBuffer(bytes.byteLength);
			new Uint8Array(blobBytes).set(bytes);
			screenshotUrl = URL.createObjectURL(new Blob([blobBytes], { type: mimeType }));
			screenshotAgentName = agent.name;
		} catch (err) {
			screenshotError = getErrorMessage(err);
			addNotification('error', `Screenshot failed: ${screenshotError}`);
		} finally {
			screenshotLoadingAgentId = null;
		}
	}

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
	{#if screenshotError}
		<div class="flex items-center gap-2 mb-4 px-3 py-2 rounded-lg bg-red-500/10 border border-red-500/30 text-sm text-red-300" role="alert">
			<Icon name="alert-triangle" size={15} /> Screenshot error: {screenshotError}
		</div>
	{/if}
	{#if $agents.filter(a => a.connected).length > 0}
		<div class="grid gap-4" style="grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));">
			{#each $agents.filter(a => a.connected) as agent (agent.id)}
				{@const osMeta = getOsMeta(agent.os)}
				<div class="bg-slate-800/50 rounded-xl border border-slate-700 overflow-hidden hover:border-slate-600 transition-all duration-200">
					<div class="flex items-center gap-2 px-4 py-3 border-b border-slate-700/50">
						<div class="flex items-center gap-2 flex-1">
							<span class="w-2 h-2 rounded-full bg-emerald-500 shadow-[0_0_6px_theme(colors.emerald.500)] flex-shrink-0"></span>
							<h3 class="text-sm font-semibold text-slate-100">{agent.name}</h3>
						</div>
						<span class="text-[10px] font-bold tracking-wide px-1.5 py-0.5 rounded {osMeta.color}">{osMeta.label}</span>
						<span class="text-xs text-slate-500 font-mono">{agent.host}:{agent.port}</span>
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
							<span class="flex items-center gap-1 text-xs text-slate-500">
								<Icon name="chevron-up" size={11} /> {formatUptime(sys.uptime)}
							</span>
							<div class="flex gap-1">
								<button aria-label={`Open terminal for ${agent.name}`} class="p-1.5 rounded bg-slate-700 hover:bg-slate-600 text-slate-300 transition-colors cursor-pointer border-none" onclick={() => viewTerminal(agent.id)} title="Terminal">
									<Icon name="terminal" size={13} />
								</button>
								<button aria-label={`Open files for ${agent.name}`} class="p-1.5 rounded bg-slate-700 hover:bg-slate-600 text-slate-300 transition-colors cursor-pointer border-none" onclick={() => viewFiles(agent.id)} title="Files">
									<Icon name="folder" size={13} />
								</button>
								<button aria-label={`Capture screenshot from ${agent.name}`} class="p-1.5 rounded bg-slate-700 hover:bg-slate-600 text-slate-300 disabled:opacity-50 cursor-pointer border-none" onclick={() => captureScreenshot(agent)} title="Capture screenshot" disabled={screenshotLoadingAgentId !== null}>
									<Icon name="image" size={13} />
								</button>
							</div>
						</div>
					{:else if agent.systemInfoError}
						<div class="flex flex-col items-center justify-center gap-2 py-8 text-sm text-red-300" role="alert">
							<Icon name="alert-triangle" size={22} />
							<span>System information unavailable</span>
							<span class="text-xs text-red-400/80 text-center px-4">{agent.systemInfoError}</span>
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
			<div class="flex items-center justify-center w-16 h-16 rounded-2xl bg-gradient-to-br from-cyan-500 to-blue-600 mb-4 shadow-lg shadow-cyan-500/25">
				<Icon name="logo" size={32} class="text-white" />
			</div>
			<h2 class="text-2xl font-bold text-slate-200 mb-2">LAN Commander</h2>
			<p class="text-slate-400">No agents connected</p>
			<p class="text-sm text-slate-600 mt-2">Install <code class="bg-slate-800 px-1 rounded">lan-agent</code> on your machines or add them manually</p>
		</div>
	{/if}
</div>

{#if screenshotUrl}
	<div class="fixed inset-0 z-50 flex items-center justify-center p-6 bg-black/70" role="dialog" aria-modal="true" aria-labelledby="screenshot-title">
		<div class="relative max-w-5xl max-h-full rounded-xl border border-slate-700 bg-slate-900 p-4 shadow-2xl">
			<div class="flex items-center justify-between gap-4 mb-3">
				<h2 id="screenshot-title" class="text-sm font-semibold text-slate-100">Screenshot — {screenshotAgentName}</h2>
				<button type="button" aria-label="Close screenshot" class="p-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 cursor-pointer border-none" onclick={releaseScreenshotUrl}>
					<Icon name="x" size={15} />
				</button>
			</div>
			<img src={screenshotUrl} alt={`Screenshot from ${screenshotAgentName}`} class="max-w-full max-h-[75vh] object-contain" />
		</div>
	</div>
{/if}
