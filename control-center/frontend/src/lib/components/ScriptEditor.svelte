<script lang="ts">
	import { agents, type Script } from '../stores/agents';
	import { addNotification } from '../stores/ui';
	import { getScripts, saveScript, runScript } from '../utils/api';
	import Icon from './Icon.svelte';

	let scripts = $state<Script[]>([]);
	let selectedScript = $state<string | null>(null);
	let scriptName = $state('');
	let scriptContent = $state('');
	let targetAgentId = $state<string | null>(null);
	let running = $state(false);
	let output = $state('');

	$effect(() => { loadScripts(); });

	async function loadScripts() {
		try { scripts = (await getScripts()) as Script[]; } catch {}
	}

	function selectScript(name: string) {
		const s = scripts.find(sc => sc.name === name);
		if (s) { selectedScript = name; scriptName = s.name; scriptContent = s.content; }
	}

	async function save() {
		if (!scriptName.trim() || !scriptContent.trim()) { addNotification('warning', 'Name and content required'); return; }
		try { await saveScript(scriptName, scriptContent); addNotification('success', 'Script saved'); await loadScripts(); }
		catch (err: any) { addNotification('error', `Save failed: ${err.message || err}`); }
	}

	function newScript() { selectedScript = null; scriptName = ''; scriptContent = ''; output = ''; }

	async function run() {
		if (!scriptContent.trim() || !targetAgentId) { addNotification('warning', 'Select agent and write script'); return; }
		running = true; output = '';
		try {
			const result: any = await runScript(targetAgentId, scriptName || 'unnamed', scriptContent);
			output = `Exit: ${result.exit_code}\n${result.stdout || ''}${result.stderr ? '\nSTDERR:\n' + result.stderr : ''}`;
		} catch (err: any) { output = `Error: ${err.message || err}`; }
		finally { running = false; }
	}
</script>

<div class="h-full overflow-hidden flex flex-col">
	<div class="p-4 border-b border-slate-700">
		<h2 class="text-lg font-semibold text-slate-100">Script Editor</h2>
		<p class="text-sm text-slate-500">Write and run scripts on agents</p>
	</div>

	<div class="flex flex-1 overflow-hidden">
		<div class="w-48 bg-slate-800/50 border-r border-slate-700 overflow-y-auto">
			<div class="flex justify-between items-center p-2 border-b border-slate-700">
				<span class="text-sm font-medium text-slate-300">Scripts</span>
				<button class="flex items-center gap-1 text-xs px-2 py-1 rounded bg-cyan-600 hover:bg-cyan-500 text-white cursor-pointer border-none" onclick={newScript}>
					<Icon name="plus" size={12} /> New
				</button>
			</div>
			{#each scripts as script}
				<button class="w-full flex items-center gap-1.5 text-left px-3 py-2 text-sm text-slate-300 hover:bg-slate-700/50 transition-colors bg-transparent border-none cursor-pointer {selectedScript === script.name ? 'bg-slate-700 text-cyan-300' : ''}" onclick={() => selectScript(script.name)}>
					<Icon name="file-code" size={14} class="text-slate-500 shrink-0" /> <span class="truncate">{script.name}</span>
				</button>
			{/each}
			{#if scripts.length === 0}<p class="text-xs text-slate-500 p-2">No saved scripts</p>{/if}
		</div>

		<div class="flex-1 flex flex-col">
			<div class="flex items-center gap-2 p-2 bg-slate-800/30 border-b border-slate-700 flex-wrap">
				<input type="text" bind:value={scriptName} placeholder="Script name..." class="bg-slate-700 text-slate-200 rounded px-3 py-1.5 text-sm outline-none border border-slate-600 focus:border-cyan-500 max-w-xs" />
				<select bind:value={targetAgentId} class="bg-slate-700 text-slate-200 rounded px-3 py-1.5 text-sm outline-none border border-slate-600">
					<option value={null}>Select agent...</option>
					{#each $agents.filter(a => a.connected) as agent (agent.id)}
						<option value={agent.id}>{agent.name}</option>
					{/each}
				</select>
				<button class="px-3 py-1.5 rounded text-sm font-medium bg-slate-700 hover:bg-slate-600 text-slate-200 cursor-pointer border-none" onclick={save}>💾 Save</button>
				<button class="px-3 py-1.5 rounded text-sm font-medium bg-cyan-600 hover:bg-cyan-500 text-white disabled:opacity-50 cursor-pointer border-none" onclick={run} disabled={running}>{running ? '⟳ Running...' : '▶ Run'}</button>
			</div>
			<textarea bind:value={scriptContent} placeholder="# One command per line&#10;echo Hello from LAN Commander" class="flex-1 bg-slate-900 text-slate-200 p-4 font-mono text-sm outline-none resize-none border-none"></textarea>
			{#if output}
				<div class="border-t border-slate-700">
					<div class="px-4 py-1 text-xs text-slate-500 bg-slate-800/30">Output</div>
					<pre class="p-4 text-xs font-mono text-green-400 whitespace-pre-wrap max-h-40 overflow-y-auto bg-slate-900/50 m-0">{output}</pre>
				</div>
			{/if}
		</div>
	</div>
</div>
