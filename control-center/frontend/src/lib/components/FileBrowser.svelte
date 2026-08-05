<script lang="ts">
  import { selectedAgent } from '../stores/agents';
  import { addNotification } from '../stores/ui';
  import { listDir, chooseSavePath, transferFile } from '../utils/api';
  import { formatBytes, formatTime, getFileMeta } from '../utils/format';
  import type { DirEntry, DirContents } from '../stores/agents';
  import Icon from './Icon.svelte';

  let currentPath = $state('/');
  let entries = $state<DirEntry[]>([]);
  let loading = $state(false);
  let downloading = $state<string | null>(null);
  let breadcrumbs = $state<string[]>([]);

  $effect(() => { if ($selectedAgent) navigateTo('/'); });

  function updateBreadcrumbs(path: string) {
    const parts = path.replace(/\\/g, '/').split('/').filter(Boolean);
    breadcrumbs = path === '/' ? ['Root'] : parts;
  }

  async function navigateTo(path: string) {
    if (!$selectedAgent) return;
    loading = true;
    currentPath = path;
    updateBreadcrumbs(path);
    try {
      const result = (await listDir($selectedAgent.id, path)) as DirContents;
      entries = result.entries || [];
    } catch (err: any) {
      addNotification('error', `No se pudo listar la carpeta: ${err.message || err}`);
      entries = [];
    } finally { loading = false; }
  }

  function navigateToCrumb(index: number) {
    const parts = currentPath.replace(/\\/g, '/').split('/').filter(Boolean);
    navigateTo('/' + parts.slice(0, index + 1).join('/'));
  }

  function openEntry(entry: DirEntry) { if (entry.is_dir) navigateTo(entry.path); }
  function goBack() { navigateTo(currentPath.substring(0, currentPath.lastIndexOf('/')) || '/'); }

  async function downloadEntry(entry: DirEntry) {
    if (!$selectedAgent || entry.is_dir || downloading) return;
    const localPath = (await chooseSavePath(entry.name)) as string;
    if (!localPath) return;
    downloading = entry.path;
    try {
      await transferFile($selectedAgent.id, entry.path, localPath);
      addNotification('success', `Archivo descargado en ${localPath}`);
    } catch (err: any) {
      addNotification('error', `No se pudo descargar el archivo: ${err.message || err}`);
    } finally { downloading = null; }
  }
</script>

<div class="h-full flex flex-col bg-slate-900">
  <div class="flex items-center justify-between px-4 py-2 bg-slate-800 border-b border-slate-700">
    <div class="flex items-center gap-1 text-sm font-mono">
      <button class="flex items-center text-amber-400 hover:text-amber-300 px-1 py-0.5 rounded bg-transparent border-none cursor-pointer" onclick={() => navigateTo('/')} title="RaÃ­z">
        <Icon name="folder" size={15} />
      </button>
      {#each breadcrumbs as crumb, i}
        <span class="text-slate-600">/</span>
        <button class="text-slate-400 hover:text-slate-200 px-1 py-0.5 rounded bg-transparent border-none cursor-pointer {i === breadcrumbs.length - 1 ? 'text-cyan-400' : ''}" onclick={() => navigateToCrumb(i)}>{crumb}</button>
      {/each}
    </div>
    <button class="flex items-center gap-1 px-3 py-1 text-xs rounded bg-slate-700 hover:bg-slate-600 text-slate-300 disabled:opacity-50 cursor-pointer border-none" onclick={goBack} disabled={currentPath === '/'}>
      <Icon name="arrow-left" size={12} /> AtrÃ¡s
    </button>
  </div>

  <div class="flex-1 overflow-y-auto">
    <div class="flex items-center px-4 py-2 text-xs text-slate-500 font-medium border-b border-slate-800 bg-slate-800/30">
      <span class="flex-1">Nombre</span><span class="w-24 text-right">TamaÃ±o</span><span class="w-32 text-right">Modificado</span><span class="w-24 text-right">AcciÃ³n</span>
    </div>
    {#if loading}
      <div class="flex items-center justify-center gap-2 py-12 text-sm text-slate-500"><div class="w-4 h-4 border-2 border-slate-600 border-t-cyan-500 rounded-full animate-spin"></div>Cargandoâ€¦</div>
    {:else if entries.length === 0}
      <div class="flex items-center justify-center py-12 text-sm text-slate-500">Carpeta vacÃ­a</div>
    {:else}
      {#each entries as entry (entry.path)}
        {@const meta = getFileMeta(entry)}
        <div class="w-full flex items-center px-4 py-2 text-sm text-slate-300 hover:bg-slate-800/50 border-b border-slate-800/30">
          <button class="flex-1 flex items-center gap-2 min-w-0 text-left bg-transparent border-none cursor-pointer text-slate-300" onclick={() => openEntry(entry)}>
            <Icon name={meta.icon} size={15} class={meta.color} /><span class="truncate">{entry.name}</span>
          </button>
          <span class="w-24 text-right text-slate-500">{entry.is_dir ? '--' : formatBytes(entry.size)}</span>
          <span class="w-32 text-right text-slate-500">{formatTime(entry.mod_time)}</span>
          <span class="w-24 flex justify-end">
            {#if !entry.is_dir}
              <button class="p-1.5 rounded bg-slate-700 hover:bg-cyan-700 text-slate-300 disabled:opacity-50 cursor-pointer border-none" onclick={() => downloadEntry(entry)} disabled={downloading !== null} title="Descargar">
                {#if downloading === entry.path}<span class="text-xs">â€¦</span>{:else}<Icon name="download" size={14} />{/if}
              </button>
            {/if}
          </span>
        </div>
      {/each}
    {/if}
  </div>
</div>
