<script lang="ts">
	import { addNotification } from '../stores/ui';
	import { wakeOnLAN, getSessions, saveSession, deleteSession } from '../utils/api';
	import type { Session } from '../stores/agents';
	import Icon from './Icon.svelte';

	let wolMac = $state('');
	let wolBroadcast = $state('255.255.255.255');
	let wolSending = $state(false);
	let sessionsList = $state<Session[]>([]);
	let newSessionHost = $state('');
	let newSessionPort = $state(9474);
	let newSessionName = $state('');
	let newSessionToken = $state('');
let newSessionSecure = $state(false);

	$effect(() => { loadSessions(); });

	async function loadSessions() { try { sessionsList = (await getSessions()) as Session[]; } catch {} }

	async function sendWoL() {
		if (!wolMac.trim()) { addNotification('warning', 'Enter a MAC address'); return; }
		wolSending = true;
		try { await wakeOnLAN(wolMac, wolBroadcast); addNotification('success', `WoL sent to ${wolMac}`); }
		catch (err: any) { addNotification('error', `WoL failed: ${err.message || err}`); }
		finally { wolSending = false; }
	}

	async function removeSession(id: number) {
		try { await deleteSession(id); await loadSessions(); addNotification('success', 'Session deleted'); }
		catch (err: any) { addNotification('error', `Delete failed: ${err.message || err}`); }
	}

	async function addSession() {
		if (!newSessionHost.trim()) return;
		try {
			await saveSession(newSessionHost, newSessionPort, newSessionName || newSessionHost, newSessionToken, newSessionSecure);
			await loadSessions();
			newSessionHost = '';
			newSessionToken = '';
			newSessionSecure = false;
			addNotification('success', 'Session saved');
		}
		catch (err: any) { addNotification('error', `Save failed: ${err.message || err}`); }
	}
</script>

<div class="p-6 overflow-y-auto h-full box-border">
	<h2 class="text-xl font-bold text-slate-100 mb-6">Settings</h2>

	<section class="mb-8">
		<h3 class="text-base font-semibold text-slate-200 mb-1 flex items-center gap-2">
			<Icon name="plug" size={16} class="text-cyan-400" /> Wake-on-LAN
		</h3>
		<p class="text-sm text-slate-500 mb-3">Send magic packet to wake up a sleeping machine</p>
		<div class="flex gap-2 items-end flex-wrap">
			<input type="text" bind:value={wolMac} placeholder="MAC (AA:BB:CC:DD:EE:FF)" class="bg-slate-700 text-slate-200 rounded-lg px-3 py-2 text-sm outline-none border border-slate-600 focus:border-cyan-500" />
			<input type="text" bind:value={wolBroadcast} placeholder="Broadcast IP" class="bg-slate-700 text-slate-200 rounded-lg px-3 py-2 text-sm outline-none border border-slate-600 focus:border-cyan-500 w-40" />
			<button class="px-4 py-2 rounded-lg text-sm font-medium bg-gradient-to-r from-cyan-600 to-blue-600 hover:from-cyan-500 hover:to-blue-500 text-white disabled:opacity-50 cursor-pointer border-none shadow-lg shadow-cyan-500/10" onclick={sendWoL} disabled={wolSending}>{wolSending ? 'Sending...' : 'Send Magic Packet'}</button>
		</div>
	</section>

	<section class="mb-8">
		<h3 class="text-base font-semibold text-slate-200 mb-1 flex items-center gap-2">
			<Icon name="save" size={16} class="text-cyan-400" /> Saved Sessions
		</h3>
		<p class="text-sm text-slate-500 mb-3">Manage saved agent connections</p>
		<div class="flex gap-2 items-end flex-wrap mb-3">
			<input type="text" bind:value={newSessionHost} placeholder="Host IP" class="bg-slate-700 text-slate-200 rounded-lg px-3 py-2 text-sm outline-none border border-slate-600 focus:border-cyan-500" />
			<input type="number" bind:value={newSessionPort} class="bg-slate-700 text-slate-200 rounded-lg px-3 py-2 text-sm outline-none border border-slate-600 focus:border-cyan-500 w-24" />
			<input type="text" bind:value={newSessionName} placeholder="Name" class="bg-slate-700 text-slate-200 rounded-lg px-3 py-2 text-sm outline-none border border-slate-600 focus:border-cyan-500" />
			<input type="password" bind:value={newSessionToken} placeholder="Auth token" class="bg-slate-700 text-slate-200 rounded-lg px-3 py-2 text-sm outline-none border border-slate-600 focus:border-cyan-500" />
			<label class="flex items-center gap-2 text-sm text-slate-300 pb-2"><input type="checkbox" bind:checked={newSessionSecure} /> TLS</label>
			<button class="px-4 py-2 rounded-lg text-sm font-medium bg-slate-700 hover:bg-slate-600 text-slate-100 cursor-pointer border-none" onclick={addSession}>Save</button>
		</div>

		{#if sessionsList.length > 0}
			<div class="space-y-1">
				{#each sessionsList as session (session.id)}
					<div class="flex items-center gap-3 px-3 py-2 rounded-lg bg-slate-800/50 text-sm">
						<span class="flex-1 text-slate-200 font-medium">{session.name}</span>
						<span class="text-slate-400 font-mono">{session.host}:{session.port}</span><span class="text-xs text-cyan-400">{session.secure ? 'TLS' : 'LAN'}</span>
						<span class="text-xs text-slate-500">{new Date(session.last_connected).toLocaleDateString()}</span>
						<button class="text-slate-500 hover:text-red-400 transition-colors bg-transparent border-none cursor-pointer p-1 rounded hover:bg-slate-700" onclick={() => removeSession(session.id)}>
							<Icon name="x" size={13} />
						</button>
					</div>
				{/each}
			</div>
		{:else}
			<p class="text-sm text-slate-500">No saved sessions</p>
		{/if}
	</section>

	<section class="mb-8">
		<h3 class="text-base font-semibold text-slate-200 mb-1 flex items-center gap-2">
			<Icon name="info" size={16} class="text-cyan-400" /> About
		</h3>
		<div class="space-y-1 text-sm">
			<div class="flex gap-2"><span class="text-slate-400">LAN Commander</span><span class="text-slate-200">v1.0.0</span></div>
			<div class="flex gap-2"><span class="text-slate-400">Agent</span><span class="text-slate-200">v1.0.0 (Go)</span></div>
			<div class="flex gap-2"><span class="text-slate-400">Frontend</span><span class="text-slate-200">Svelte + Tailwind CSS</span></div>
			<div class="flex gap-2"><span class="text-slate-400">Backend</span><span class="text-slate-200">Wails v2 + Go</span></div>
		</div>
	</section>
</div>
