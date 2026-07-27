<script lang="ts">
	import { addNotification } from '../stores/ui';
	import { connectAgent } from '../utils/api';
	import Icon from './Icon.svelte';

	let { onclose = () => {} }: { onclose: () => void } = $props();
	let host = $state('');
	let port = $state(9474);
	let authToken = $state('');
	let connecting = $state(false);

	async function connect() {
		if (!host.trim()) { addNotification('warning', 'Enter a host'); return; }
		connecting = true;
		try { await connectAgent(host, port, authToken); addNotification('success', `Connected to ${host}:${port}`); onclose(); }
		catch (err: any) { addNotification('error', `Connection failed: ${err.message || err}`); }
		finally { connecting = false; }
	}
</script>

<div class="fixed inset-0 bg-black/60 flex items-center justify-center z-50" onclick={onclose}>
	<div class="bg-slate-800 rounded-xl border border-slate-700 w-full max-w-md shadow-2xl" onclick={(e) => e.stopPropagation()}>
		<div class="flex justify-between items-center px-6 py-4 border-b border-slate-700">
			<h3 class="text-lg font-semibold text-slate-100 flex items-center gap-2">
				<Icon name="plug" size={17} class="text-cyan-400" /> Connect Agent
			</h3>
			<button class="text-slate-400 hover:text-slate-200 bg-transparent border-none cursor-pointer p-1 rounded hover:bg-slate-700 transition-colors" onclick={onclose}>
				<Icon name="x" size={16} />
			</button>
		</div>

		<div class="px-6 py-4 space-y-4">
			<label class="flex flex-col gap-1">
				<span class="text-sm text-slate-400">Host / IP</span>
				<input type="text" bind:value={host} placeholder="192.168.1.10" class="bg-slate-700 text-slate-200 rounded-lg px-3 py-2 text-sm outline-none border border-slate-600 focus:border-cyan-500" />
			</label>
			<label class="flex flex-col gap-1">
				<span class="text-sm text-slate-400">Port</span>
				<input type="number" bind:value={port} class="bg-slate-700 text-slate-200 rounded-lg px-3 py-2 text-sm outline-none border border-slate-600 focus:border-cyan-500" />
			</label>
			<label class="flex flex-col gap-1">
				<span class="text-sm text-slate-400">Auth Token (optional)</span>
				<input type="password" bind:value={authToken} placeholder="Leave empty if no auth" class="bg-slate-700 text-slate-200 rounded-lg px-3 py-2 text-sm outline-none border border-slate-600 focus:border-cyan-500" />
			</label>
		</div>

		<div class="flex justify-end gap-2 px-6 py-4 border-t border-slate-700">
			<button class="px-4 py-2 rounded-lg text-sm font-medium text-slate-400 hover:text-slate-200 bg-transparent border-none cursor-pointer" onclick={onclose}>Cancel</button>
			<button class="px-4 py-2 rounded-lg text-sm font-medium bg-gradient-to-r from-cyan-600 to-blue-600 hover:from-cyan-500 hover:to-blue-500 text-white disabled:opacity-50 cursor-pointer border-none shadow-lg shadow-cyan-500/10" onclick={connect} disabled={connecting}>{connecting ? 'Connecting...' : 'Connect'}</button>
		</div>
	</div>
</div>
