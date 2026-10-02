<script lang="ts">
	import { addNotification } from '../stores/ui';
	import { wakeOnLAN, getSessions, saveSession, saveSessionSecure, deleteSession, reconnectSession } from '../utils/api';
	import { getErrorMessage } from '../utils/format';
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
	let newSessionTls = $state(false);
	let newSessionCAFile = $state('');
	let newSessionServerName = $state('');
	let reconnectingSessionId = $state<number | null>(null);
	let sessionsError = $state<string | null>(null);

	$effect(() => { loadSessions(); });

	async function loadSessions() {
		try {
			sessionsList = (await getSessions()) as Session[];
			sessionsError = null;
			return true;
		} catch (err) {
			sessionsError = getErrorMessage(err);
			addNotification('error', `Failed to load sessions: ${sessionsError}`);
			return false;
		}
	}

	async function sendWoL() {
		if (!wolMac.trim()) { addNotification('warning', 'Enter a MAC address'); return; }
		wolSending = true;
		try { await wakeOnLAN(wolMac, wolBroadcast); addNotification('success', `WoL sent to ${wolMac}`); }
		catch (err) { addNotification('error', `WoL failed: ${getErrorMessage(err)}`); }
		finally { wolSending = false; }
	}

	async function removeSession(id: number) {
		try { await deleteSession(id); await loadSessions(); addNotification('success', 'Session deleted'); }
		catch (err) { addNotification('error', `Delete failed: ${getErrorMessage(err)}`); }
	}

	async function reconnect(id: number, name: string) {
		if (reconnectingSessionId !== null) return;
		reconnectingSessionId = id;
		try {
			const agentId = await reconnectSession(id);
			await loadSessions();
			addNotification('success', `Reconnected to ${name} (${agentId})`);
		} catch (err) { addNotification('error', `Reconnect failed: ${getErrorMessage(err)}`); }
		finally { reconnectingSessionId = null; }
	}

	async function addSession() {
		if (!newSessionHost.trim()) return;
		try {
			if (newSessionTls) {
				await saveSessionSecure(newSessionHost, newSessionPort, newSessionName || newSessionHost, newSessionToken, newSessionCAFile, newSessionServerName);
			} else {
				await saveSession(newSessionHost, newSessionPort, newSessionName || newSessionHost, newSessionToken);
			}
			await loadSessions();
			newSessionHost = '';
			newSessionToken = '';
			newSessionCAFile = '';
			newSessionServerName = '';
			newSessionTls = false;
			addNotification('success', 'Session saved');
		}
		catch (err) { addNotification('error', `Save failed: ${getErrorMessage(err)}`); }
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
			<input type="text" bind:value={wolMac} aria-label="Wake-on-LAN MAC address" placeholder="MAC (AA:BB:CC:DD:EE:FF)" class="bg-slate-700 text-slate-200 rounded-lg px-3 py-2 text-sm outline-none border border-slate-600 focus:border-cyan-500" />
			<input type="text" bind:value={wolBroadcast} aria-label="Wake-on-LAN broadcast IP" placeholder="Broadcast IP" class="bg-slate-700 text-slate-200 rounded-lg px-3 py-2 text-sm outline-none border border-slate-600 focus:border-cyan-500 w-40" />
			<button class="px-4 py-2 rounded-lg text-sm font-medium bg-gradient-to-r from-cyan-600 to-blue-600 hover:from-cyan-500 hover:to-blue-500 text-white disabled:opacity-50 cursor-pointer border-none shadow-lg shadow-cyan-500/10" onclick={sendWoL} disabled={wolSending}>{wolSending ? 'Sending...' : 'Send Magic Packet'}</button>
		</div>
	</section>

	<section class="mb-8">
		<h3 class="text-base font-semibold text-slate-200 mb-1 flex items-center gap-2">
			<Icon name="save" size={16} class="text-cyan-400" /> Saved Sessions
		</h3>
		<p class="text-sm text-slate-500 mb-3">Manage saved agent connections</p>
		<div class="flex gap-2 items-end flex-wrap mb-3">
			<input type="text" bind:value={newSessionHost} aria-label="Session host IP" placeholder="Host IP" class="bg-slate-700 text-slate-200 rounded-lg px-3 py-2 text-sm outline-none border border-slate-600 focus:border-cyan-500" />
			<input type="number" bind:value={newSessionPort} aria-label="Session port" class="bg-slate-700 text-slate-200 rounded-lg px-3 py-2 text-sm outline-none border border-slate-600 focus:border-cyan-500 w-24" />
			<input type="text" bind:value={newSessionName} aria-label="Session name" placeholder="Name" class="bg-slate-700 text-slate-200 rounded-lg px-3 py-2 text-sm outline-none border border-slate-600 focus:border-cyan-500" />
			<input type="password" bind:value={newSessionToken} aria-label="Session auth token" placeholder="Auth token" class="bg-slate-700 text-slate-200 rounded-lg px-3 py-2 text-sm outline-none border border-slate-600 focus:border-cyan-500" />
			<label class="flex items-center gap-2 text-sm text-slate-300 px-2 py-2">
				<input type="checkbox" bind:checked={newSessionTls} aria-label="Use TLS for this session" class="accent-cyan-500" /> Secure TLS
			</label>
			{#if newSessionTls}
				<input type="text" bind:value={newSessionCAFile} aria-label="Session CA certificate file" placeholder="CA certificate file (optional)" class="bg-slate-700 text-slate-200 rounded-lg px-3 py-2 text-sm outline-none border border-slate-600 focus:border-cyan-500" />
				<input type="text" bind:value={newSessionServerName} aria-label="TLS server name" placeholder="Server name (optional)" class="bg-slate-700 text-slate-200 rounded-lg px-3 py-2 text-sm outline-none border border-slate-600 focus:border-cyan-500" />
			{/if}
			<button class="px-4 py-2 rounded-lg text-sm font-medium bg-slate-700 hover:bg-slate-600 text-slate-100 cursor-pointer border-none" onclick={addSession}>Save</button>
		</div>

		{#if sessionsError}
			<div class="px-3 py-2 rounded-lg bg-red-500/10 border border-red-500/30 text-sm text-red-300" role="alert">
				Could not load saved sessions: {sessionsError}
			</div>
		{:else if sessionsList.length > 0}
			<div class="space-y-1">
				{#each sessionsList as session (session.id)}
					<div class="flex items-center gap-3 px-3 py-2 rounded-lg bg-slate-800/50 text-sm" aria-label={`Saved session ${session.name}${session.tls ? ', TLS enabled' : ''}`}>
						<span class="flex-1 text-slate-200 font-medium">{session.name}</span>
						{#if session.tls}<span class="text-[10px] uppercase tracking-wide px-1.5 py-0.5 rounded bg-emerald-500/10 text-emerald-300 border border-emerald-500/20" aria-label="TLS enabled">TLS</span>{/if}
						<span class="text-slate-400 font-mono">{session.host}:{session.port}</span>
						<span class="text-xs text-slate-500">{new Date(session.last_connected).toLocaleDateString()}</span>
						<button aria-label={`Reconnect session ${session.name}`} class="text-cyan-300 hover:text-cyan-200 transition-colors bg-transparent border-none cursor-pointer p-1 rounded hover:bg-slate-700 disabled:opacity-50" onclick={() => reconnect(session.id, session.name)} disabled={reconnectingSessionId !== null}>
							{#if reconnectingSessionId === session.id}<span class="text-xs">Connecting…</span>{:else}<Icon name="refresh" size={14} />{/if}
						</button>
						<button aria-label={`Delete session ${session.name}`} class="text-slate-500 hover:text-red-400 transition-colors bg-transparent border-none cursor-pointer p-1 rounded hover:bg-slate-700" onclick={() => removeSession(session.id)}>
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
			<div class="flex gap-2"><span class="text-slate-400">LAN Commander</span><span class="text-slate-200">v{__APP_VERSION__}</span></div>
			<div class="flex gap-2"><span class="text-slate-400">Agent</span><span class="text-slate-200">v{__APP_VERSION__} (Go)</span></div>
			<div class="flex gap-2"><span class="text-slate-400">Frontend</span><span class="text-slate-200">Svelte + Tailwind CSS</span></div>
			<div class="flex gap-2"><span class="text-slate-400">Backend</span><span class="text-slate-200">Wails v2 + Go</span></div>
		</div>
	</section>
</div>
