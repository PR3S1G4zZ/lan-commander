<script lang="ts">
	import { addNotification } from '../stores/ui';
	import { connectAgent, connectAgentSecure } from '../utils/api';
	import { getErrorMessage } from '../utils/format';
	import Icon from './Icon.svelte';

	let { onclose = () => {} }: { onclose: () => void } = $props();
	let host = $state('');
	let port = $state(9474);
	let authToken = $state('');
	let useTLS = $state(false);
	let caFile = $state('');
	let serverName = $state('');
	let connecting = $state(false);

	async function connect() {
		if (!host.trim()) { addNotification('warning', 'Enter a host'); return; }
		connecting = true;
		try {
			if (useTLS) {
				await connectAgentSecure(host, port, authToken, caFile, serverName);
			} else {
				await connectAgent(host, port, authToken);
			}
			addNotification('success', `Connected to ${host}:${port}`);
			onclose();
		} catch (err) { addNotification('error', `Connection failed: ${getErrorMessage(err)}`); }
		finally { connecting = false; }
	}
</script>

<div class="fixed inset-0 flex items-center justify-center z-50">
	<button type="button" aria-label="Close connection dialog" class="absolute inset-0 bg-black/60 border-none cursor-default" onclick={onclose}></button>
	<div class="relative bg-slate-800 rounded-xl border border-slate-700 w-full max-w-md shadow-2xl" role="dialog" aria-modal="true" aria-labelledby="connect-agent-title">
		<div class="flex justify-between items-center px-6 py-4 border-b border-slate-700">
			<h3 id="connect-agent-title" class="text-lg font-semibold text-slate-100 flex items-center gap-2">
				<Icon name="plug" size={17} class="text-cyan-400" /> Connect Agent
			</h3>
			<button type="button" class="text-slate-400 hover:text-slate-200 bg-transparent border-none cursor-pointer p-1 rounded hover:bg-slate-700 transition-colors" onclick={onclose} aria-label="Close">
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
				<input type="number" bind:value={port} min="1" max="65535" class="bg-slate-700 text-slate-200 rounded-lg px-3 py-2 text-sm outline-none border border-slate-600 focus:border-cyan-500" />
			</label>
			<label class="flex flex-col gap-1">
				<span class="text-sm text-slate-400">Auth Token (optional)</span>
				<input type="password" bind:value={authToken} placeholder="Leave empty if no auth" class="bg-slate-700 text-slate-200 rounded-lg px-3 py-2 text-sm outline-none border border-slate-600 focus:border-cyan-500" />
			</label>
			<label class="flex items-center gap-2 text-sm text-slate-300">
				<input type="checkbox" bind:checked={useTLS} class="accent-cyan-500" />
				<span>Use verified TLS (wss)</span>
			</label>
			{#if useTLS}
				<label class="flex flex-col gap-1">
					<span class="text-sm text-slate-400">CA file (optional)</span>
					<input type="text" bind:value={caFile} placeholder="C:\\certs\\lan-ca.pem" aria-label="CA certificate file" class="bg-slate-700 text-slate-200 rounded-lg px-3 py-2 text-sm outline-none border border-slate-600 focus:border-cyan-500" />
				</label>
				<label class="flex flex-col gap-1">
					<span class="text-sm text-slate-400">TLS server name (optional)</span>
					<input type="text" bind:value={serverName} placeholder="agent.example.internal" aria-label="TLS server name" class="bg-slate-700 text-slate-200 rounded-lg px-3 py-2 text-sm outline-none border border-slate-600 focus:border-cyan-500" />
				</label>
			{/if}
		</div>

		<div class="flex justify-end gap-2 px-6 py-4 border-t border-slate-700">
			<button type="button" class="px-4 py-2 rounded-lg text-sm font-medium text-slate-400 hover:text-slate-200 bg-transparent border-none cursor-pointer" onclick={onclose}>Cancel</button>
			<button type="button" class="px-4 py-2 rounded-lg text-sm font-medium bg-gradient-to-r from-cyan-600 to-blue-600 hover:from-cyan-500 hover:to-blue-500 text-white disabled:opacity-50 cursor-pointer border-none shadow-lg shadow-cyan-500/10" onclick={connect} disabled={connecting}>{connecting ? 'Connecting...' : 'Connect'}</button>
		</div>
	</div>
</div>
