<script lang="ts">
	import { notifications, removeNotification } from '../stores/ui';
	import Icon from './Icon.svelte';

	const iconMap = { success: 'check-circle', error: 'x-circle', info: 'info', warning: 'alert-triangle' };
	const iconColorMap = {
		success: 'text-emerald-400',
		error: 'text-red-400',
		info: 'text-blue-400',
		warning: 'text-amber-400',
	};
	const colorMap = {
		success: 'border-emerald-500/50 bg-emerald-500/10',
		error: 'border-red-500/50 bg-red-500/10',
		info: 'border-blue-500/50 bg-blue-500/10',
		warning: 'border-amber-500/50 bg-amber-500/10',
	};
</script>

{#if $notifications.length > 0}
	<div class="fixed top-4 right-4 z-[100] flex flex-col gap-2 max-w-sm">
		{#each $notifications as notif (notif.id)}
			<div class="flex items-center gap-2 px-4 py-3 rounded-lg border shadow-lg text-sm backdrop-blur-sm bg-slate-900/80 {colorMap[notif.type]}">
				<Icon name={iconMap[notif.type]} size={17} class="flex-shrink-0 {iconColorMap[notif.type]}" />
				<span class="flex-1 text-slate-200">{notif.message}</span>
				<button class="text-slate-500 hover:text-slate-300 flex-shrink-0 bg-transparent border-none cursor-pointer p-0.5 rounded hover:bg-slate-800 transition-colors" onclick={() => removeNotification(notif.id)}>
					<Icon name="x" size={13} />
				</button>
			</div>
		{/each}
	</div>
{/if}
