<script lang="ts">
  import { GetStatus, Quit } from '../wailsjs/go/main/App';

  type AgentStatus = {
    active: boolean;
    port: number;
    secure: boolean;
    checked_at: string;
    message: string;
    managed_by: string;
    agent_version: string;
  };

  let status = $state<AgentStatus>({
    active: false,
    port: 9474,
    secure: false,
    checked_at: '',
    message: 'Comprobando el servicio...',
    managed_by: '',
    agent_version: '1.0.0'
  });
  let loading = $state(true);

  async function refresh() {
    loading = true;
    try {
      status = (await GetStatus()) as AgentStatus;
    } catch {
      status = { ...status, active: false, message: 'No se pudo consultar el servicio' };
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    refresh();
    const timer = setInterval(refresh, 4000);
    return () => clearInterval(timer);
  });

  function formatTime(value: string) {
    return value ? new Date(value).toLocaleTimeString() : '--';
  }
</script>

<main class="shell">
  <header class="header">
    <div class="brand-mark">LC</div>
    <div>
      <h1>LAN Commander</h1>
      <p>Aplicación del equipo gestionado</p>
    </div>
    <button class="close" title="Cerrar aplicación" aria-label="Cerrar aplicación" onclick={Quit}>×</button>
  </header>

  <section class="hero">
    <div class:online={status.active} class="status-dot"></div>
    <div>
      <strong>{status.active ? 'Equipo protegido y disponible' : 'Servicio sin conexión'}</strong>
      <span>{status.message}</span>
    </div>
  </section>

  <section class="cards">
    <div class="card"><span>Estado</span><strong>{status.active ? 'Activo' : 'Inactivo'}</strong></div>
    <div class="card"><span>Puerto</span><strong>{status.port}</strong></div>
    <div class="card"><span>Conexión</span><strong>{status.secure ? 'TLS' : 'LAN'}</strong></div>
    <div class="card"><span>Versión</span><strong>{status.agent_version}</strong></div>
  </section>

  {#if status.managed_by}
    <section class="notice"><strong>Equipo gestionado por</strong><span>{status.managed_by}</span></section>
  {/if}

  <footer>
    <span>Última comprobación: {formatTime(status.checked_at)}</span>
    <button class="refresh" onclick={refresh} disabled={loading}>{loading ? 'Actualizando…' : 'Actualizar'}</button>
  </footer>
</main>
