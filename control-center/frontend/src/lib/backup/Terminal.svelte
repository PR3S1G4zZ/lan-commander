<script lang="ts">
	import { selectedAgent } from '../stores/agents';
	import { addNotification } from '../stores/ui';
	import { execCommand } from '../utils/api';

	let command = $state('');
	let output = $state<string[]>([]);
	let history = $state<string[]>([]);
	let historyIndex = $state(-1);
	let running = $state(false);
	let autoScroll = $state(true);
	let inputRef: HTMLInputElement | undefined = $state();

	$effect(() => {
		if ($selectedAgent && inputRef) {
			inputRef.focus();
		}
	});

	async function runCommand() {
		if (!command.trim() || !$selectedAgent || running) return;
		const cmd = command.trim();
		history.push(cmd);
		historyIndex = history.length;
		output.push(`> ${cmd}`);
		command = '';
		running = true;
		try {
			const result: any = await execCommand($selectedAgent, cmd, 30);
			if (result.stdout) output.push(result.stdout);
			if (result.stderr) output.push(`[stderr] ${result.stderr}`);
			if (result.exit_code !== 0) {
				output.push(`[exit: ${result.exit_code} | ${result.duration_ms}ms]`);
			} else {
				output.push(`[done: ${result.duration_ms}ms]`);
			}
		} catch (err: any) {
			output.push(`[error: ${err.message || err}]`);
			addNotification('error', `Command failed: ${err.message || err}`);
		} finally {
			running = false;
			if (autoScroll) {
				setTimeout(() => {
					const container = document.querySelector('.terminal-output');
					if (container) container.scrollTop = container.scrollHeight;
				}, 50);
			}
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') { runCommand(); }
		else if (e.key === 'ArrowUp') {
			e.preventDefault();
			if (historyIndex > 0) { historyIndex--; command = history[historyIndex]; }
		} else if (e.key === 'ArrowDown') {
			e.preventDefault();
			if (historyIndex < history.length - 1) { historyIndex++; command = history[historyIndex]; }
			else { historyIndex = history.length; command = ''; }
		} else if (e.key === 'l' && e.ctrlKey) { e.preventDefault(); output = []; }
	}

	function clearOutput() { output = []; }
	async function copyOutput() {
		await navigator.clipboard.writeText(output.join('\n'));
		addNotification('success', 'Output copied to clipboard');
	}
</script>

<div class="h-full flex flex-col bg-gray-950">
	<div class="flex items-center justify-between px-4 py-2 bg-gray-900 border-b border-gray-800">
		<span class="text-sm font-mono text-slate-400">&gt;_ Terminal{$selectedAgent ? ' - ' + $selectedAgent : ''}</span>
		<div class="flex items-center gap-2">
			<button class="px-2 py-1 text-xs rounded bg-gray-800 hover:bg-gray-700 text-slate-400 cursor-pointer border-none" onclick={copyOutput} title="Copy">📋</button>
			<button class="px-2 py-1 text-xs rounded bg-gray-800 hover:bg-gray-700 text-slate-400 cursor-pointer border-none" onclick={clearOutput} title="Clear">🗑️</button>
			<label class="flex items-center gap-1 text-xs text-slate-600"><input type="checkbox" bind:checked={autoScroll} /> Auto-scroll</label>
		</div>
	</div>

	<div class="flex-1 overflow-y-auto p-4 font-mono text-sm leading-relaxed terminal-output">
		{#each output as line}
			<div class="whitespace-pre-wrap break-all text-slate-300">{line}</div>
		{/each}
		{#if running}
			<div class="text-yellow-400">Running...</div>
		{/if}
		<div class="text-slate-700 text-xs mt-2">Type command + Enter. Ctrl+L=clear. ↑↓=history.</div>
	</div>

	<div class="flex items-center gap-2 px-4 py-2 bg-gray-900 border-t border-gray-800">
		<span class="text-cyan-400 font-bold">❯</span>
		<input
			bind:this={inputRef}
			type="text"
			bind:value={command}
			onkeydown={handleKeydown}
			disabled={running || !$selectedAgent}
			placeholder={$selectedAgent ? 'Enter command...' : 'Select an agent first'}
			class="flex-1 bg-transparent outline-none text-slate-200 font-mono text-sm disabled:opacity-50"
		/>
	</div>
</div>
