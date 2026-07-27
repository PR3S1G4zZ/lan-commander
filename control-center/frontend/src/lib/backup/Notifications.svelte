<script lang="ts">
	import { notifications, removeNotification } from '../stores/ui';

	const iconMap = { success: '✅', error: '❌', info: 'ℹ️', warning: '⚠️' };
	const colorMap = {
		success: 'border-green-500/50 bg-green-500/10',
		error: 'border-red-500/50 bg-red-500/10',
		info: 'border-blue-500/50 bg-blue-500/10',
		warning: 'border-yellow-500/50 bg-yellow-500/10',
	};
</script>

{#if $notifications.length > 0}
	<div class="fixed top-4 right-4 z-[100] flex flex-col gap-2 max-w-sm">
		{#each $notifications as notif (notif.id)}
			<div class="flex items-center gap-2 px-4 py-3 rounded-lg border shadow-lg text-sm backdrop-blur-sm bg-slate-900/80 {colorMap[notif.type]}">
				<span class="flex-shrink-0">{iconMap[notif.type]}</span>
				<span class="flex-1 text-slate-200">{notif.message}</span>
				<button class="text-slate-500 hover:text-slate-300 text-xs flex-shrink-0 bg-transparent border-none cursor-pointer" onclick={() => removeNotification(notif.id)}>✕</button>
			</div>
		{/each}
	</div>
{/if}
