<script lang="ts">
  import { agents, selectedAgentId, type AgentInfo } from '../stores/agents';
  import { currentView, addNotification } from '../stores/ui';
  import { requestScreenshot } from '../utils/api';
  import { formatBytes, formatPercent, formatUptime, getOsMeta } from '../utils/format';
  import Icon from './Icon.svelte';

  let screenshotUrl = $state('');
  let screenshotAgent = $state('');
  let capturing = $state<string | null>(null);

  function getCpuColor(pct: number): string { return pct > 90 ? 'from-red-500 to-red-600' : pct > 70 ? 'from-yellow-500 to-orange-500' : 'from-cyan-500 to-blue-500'; }
  function getMemColor(pct: number): string { return pct > 90 ? 'from-red-500 to-red-600' : pct > 70 ? 'from-yellow-500 to-orange-500' : 'from-emerald-500 to-teal-500'; }
  function viewTerminal(agentId: string) { $selectedAgentId = agentId; $currentView = 'terminal'; }
  function viewFiles(agentId: string) { $selectedAgentId = agentId; $currentView = 'files'; }

  async function capture(agent: AgentInfo) {
    if (capturing) return;
    capturing = agent.id;
    try {
      const result = await requestScreenshot(agent.id) as { format: string; data: number[] };
      const bytes = Uint8Array.from(result.data || []);
      if (screenshotUrl) URL.revokeObjectURL(screenshotUrl);
      screenshotUrl = URL.createObjectURL(new Blob([bytes], { type: `image/${result.format || 'png'}` }));
      screenshotAgent = agent.name;
    } catch (err: any) { addNotification('error', `No se pudo capturar la pantalla: ${err.message || err}`); }
    finally { capturing = null; }
  }

  function closeScreenshot() {
    if (screenshotUrl) URL.revokeObjectURL(screenshotUrl);
    screenshotUrl = '';
  }
</script>

<div class="p-6 h-full overflow-y-auto box-border">
  {#if $agents.filter(a => a.connected).length > 0}
    <div class="grid gap-4" style="grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));">
      {#each $agents.filter(a => a.connected) as agent (agent.id)}
        {@const osMeta = getOsMeta(agent.os)}
        <div class="bg-slate-800/50 rounded-xl border border-slate-700 overflow-hidden hover:border-slate-600 transition-all duration-200">
          <div class="flex items-center gap-2 px-4 py-3 border-b border-slate-700/50">
            <div class="flex items-center gap-2 flex-1"><span class="w-2 h-2 rounded-full bg-emerald-500 shadow-[0_0_6px_theme(colors.emerald.500)] flex-shrink-0"></span><h3 class="text-sm font-semibold text-slate-100">{agent.name}</h3></div>
            <span class="text-[10px] font-bold tracking-wide px-1.5 py-0.5 rounded {osMeta.color}">{osMeta.label}</span><span class="text-xs text-slate-500 font-mono">{agent.host}:{agent.port}</span>
          </div>
          {#if agent.systemInfo}
            {@const sys = agent.systemInfo}
            <div class="p-4 space-y-3">
              <div><div class="flex justify-between text-xs text-slate-300 mb-1"><span>CPU</span><span class="font-bold font-mono">{formatPercent(sys.cpu.percent)}</span></div><div class="w-full h-2 bg-slate-700 rounded-full overflow-hidden"><div class="h-full rounded-full bg-gradient-to-r {getCpuColor(sys.cpu.percent)}" style="width: {sys.cpu.percent}%; transition: width 0.5s ease-out;"></div></div><div class="text-xs text-slate-600 mt-0.5">{sys.cpu.cores} cores</div></div>
              <div><div class="flex justify-between text-xs text-slate-300 mb-1"><span>RAM</span><span class="font-bold font-mono">{formatPercent(sys.memory.percent)}</span></div><div class="w-full h-2 bg-slate-700 rounded-full overflow-hidden"><div class="h-full rounded-full bg-gradient-to-r {getMemColor(sys.memory.percent)}" style="width: {sys.memory.percent}%; transition: width 0.5s ease-out;"></div></div><div class="text-xs text-slate-600 mt-0.5">{formatBytes(sys.memory.used)} / {formatBytes(sys.memory.total)}</div></div>
              {#if sys.disks && sys.disks.length > 0}{@const disk = sys.disks[0]}<div><div class="flex justify-between text-xs text-slate-300 mb-1"><span>Disco ({disk.mount})</span><span class="font-bold font-mono">{formatPercent(disk.percent)}</span></div><div class="w-full h-2 bg-slate-700 rounded-full overflow-hidden"><div class="h-full rounded-full bg-gradient-to-r from-violet-500 to-purple-500" style="width: {disk.percent}%; transition: width 0.5s ease-out;"></div></div><div class="text-xs text-slate-600 mt-0.5">{formatBytes(disk.used)} / {formatBytes(disk.total)}</div></div>{/if}
            </div>
            <div class="flex items-center justify-between px-4 py-2 bg-slate-800/30 border-t border-slate-700/50"><span class="flex items-center gap-1 text-xs text-slate-500"><Icon name="chevron-up" size={11} /> {formatUptime(sys.uptime)}</span><div class="flex gap-1">
              <button class="p-1.5 rounded bg-slate-700 hover:bg-slate-600 text-slate-300 cursor-pointer border-none" onclick={() => viewTerminal(agent.id)} title="Terminal"><Icon name="terminal" size={13} /></button>
              <button class="p-1.5 rounded bg-slate-700 hover:bg-slate-600 text-slate-300 cursor-pointer border-none" onclick={() => viewFiles(agent.id)} title="Archivos"><Icon name="folder" size={13} /></button>
              <button class="p-1.5 rounded bg-slate-700 hover:bg-cyan-700 text-slate-300 disabled:opacity-50 cursor-pointer border-none" onclick={() => capture(agent)} disabled={capturing !== null} title="Capturar pantalla"><Icon name="image" size={13} /></button>
            </div></div>
          {:else}<div class="flex flex-col items-center justify-center py-8 text-sm text-slate-500"><div class="w-6 h-6 border-2 border-slate-600 border-t-cyan-500 rounded-full animate-spin mb-2"></div><span>Conectando…</span></div>{/if}
        </div>
      {/each}
    </div>
  {:else}
    <div class="flex flex-col items-center justify-center h-full text-center"><div class="flex items-center justify-center w-16 h-16 rounded-2xl bg-gradient-to-br from-cyan-500 to-blue-600 mb-4 shadow-lg shadow-cyan-500/25"><Icon name="logo" size={32} class="text-white" /></div><h2 class="text-2xl font-bold text-slate-200 mb-2">LAN Commander</h2><p class="text-slate-400">No hay agentes conectados</p><p class="text-sm text-slate-600 mt-2">Instala <code class="bg-slate-800 px-1 rounded">lan-agent</code> o agrega un equipo manualmente</p></div>
  {/if}
</div>

{#if screenshotUrl}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/75 p-6" role="dialog" aria-modal="true">
    <div class="max-w-5xl max-h-full rounded-xl bg-slate-800 border border-slate-700 shadow-2xl overflow-hidden">
      <div class="flex items-center justify-between px-4 py-3 border-b border-slate-700"><span class="text-sm text-slate-200">Captura de {screenshotAgent}</span><div class="flex gap-2"><a class="px-3 py-1 rounded bg-cyan-600 text-white text-xs no-underline" href={screenshotUrl} download="lan-commander-screenshot.png">Guardar</a><button class="text-slate-400 hover:text-white bg-transparent border-none cursor-pointer text-xl" onclick={closeScreenshot} aria-label="Cerrar">×</button></div></div>
      <div class="p-3 max-h-[75vh] overflow-auto"><img src={screenshotUrl} alt={`Captura de ${screenshotAgent}`} class="max-w-full h-auto" /></div>
    </div>
  </div>
{/if}
