<script lang="ts">
  import { addNotification } from '../stores/ui';
  import { connectAgent } from '../utils/api';
  import Icon from './Icon.svelte';

  let { onclose = () => {} }: { onclose: () => void } = $props();
  let host = $state('');
  let port = $state(9474);
  let authToken = $state('');
  let secure = $state(false);
  let connecting = $state(false);

  async function connect() {
    if (!host.trim()) { addNotification('warning', 'Escribe el host o la IP del equipo'); return; }
    if (port < 1 || port > 65535) { addNotification('warning', 'El puerto debe estar entre 1 y 65535'); return; }
    connecting = true;
    try { await connectAgent(host.trim(), port, authToken, secure); addNotification('success', `Conectado a ${host}:${port}`); onclose(); }
    catch (err: any) { addNotification('error', `No se pudo conectar: ${err.message || err}`); }
    finally { connecting = false; }
  }
</script>

<div class="fixed inset-0 flex items-center justify-center z-50">
  <button type="button" aria-label="Cerrar diálogo" class="absolute inset-0 bg-black/60 border-none cursor-default" onclick={onclose}></button>
  <div class="relative bg-slate-800 rounded-xl border border-slate-700 w-full max-w-md shadow-2xl" role="dialog" aria-modal="true" aria-labelledby="connect-agent-title">
    <div class="flex justify-between items-center px-6 py-4 border-b border-slate-700"><h3 id="connect-agent-title" class="text-lg font-semibold text-slate-100 flex items-center gap-2"><Icon name="plug" size={17} class="text-cyan-400" /> Conectar agente</h3><button type="button" class="text-slate-400 hover:text-slate-200 bg-transparent border-none cursor-pointer p-1 rounded hover:bg-slate-700" onclick={onclose} aria-label="Cerrar"><Icon name="x" size={16} /></button></div>
    <div class="px-6 py-4 space-y-4">
      <label class="flex flex-col gap-1"><span class="text-sm text-slate-400">Host / IP</span><input type="text" bind:value={host} placeholder="192.168.1.10" class="bg-slate-700 text-slate-200 rounded-lg px-3 py-2 text-sm outline-none border border-slate-600 focus:border-cyan-500" /></label>
      <label class="flex flex-col gap-1"><span class="text-sm text-slate-400">Puerto</span><input type="number" bind:value={port} min="1" max="65535" class="bg-slate-700 text-slate-200 rounded-lg px-3 py-2 text-sm outline-none border border-slate-600 focus:border-cyan-500" /></label>
      <label class="flex flex-col gap-1"><span class="text-sm text-slate-400">Token de autenticación</span><input type="password" bind:value={authToken} placeholder="Token generado por el instalador" class="bg-slate-700 text-slate-200 rounded-lg px-3 py-2 text-sm outline-none border border-slate-600 focus:border-cyan-500" /></label>
      <label class="flex items-center gap-2 text-sm text-slate-300"><input type="checkbox" bind:checked={secure} /> Usar TLS (wss://)</label>
      {#if secure}<p class="text-xs text-amber-300">El agente debe estar configurado con un certificado confiable para TLS.</p>{/if}
    </div>
    <div class="flex justify-end gap-2 px-6 py-4 border-t border-slate-700"><button type="button" class="px-4 py-2 rounded-lg text-sm font-medium text-slate-400 hover:text-slate-200 bg-transparent border-none cursor-pointer" onclick={onclose}>Cancelar</button><button type="button" class="px-4 py-2 rounded-lg text-sm font-medium bg-gradient-to-r from-cyan-600 to-blue-600 hover:from-cyan-500 hover:to-blue-500 text-white disabled:opacity-50 cursor-pointer border-none" onclick={connect} disabled={connecting}>{connecting ? 'Conectando…' : 'Conectar'}</button></div>
  </div>
</div>
