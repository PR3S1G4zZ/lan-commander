<script lang="ts">
	let { name, size = 16, class: className = '' }: { name: string; size?: number; class?: string } = $props();
</script>

<svg
	width={size}
	height={size}
	viewBox="0 0 24 24"
	fill="none"
	stroke="currentColor"
	stroke-width="1.75"
	stroke-linecap="round"
	stroke-linejoin="round"
	class="shrink-0 {className}"
	aria-hidden="true"
>
	{#if name === 'logo'}
		<rect x="7" y="7" width="10" height="10" rx="1" />
		<rect x="10" y="10" width="4" height="4" rx="0.5" />
		<line x1="7" y1="10" x2="4" y2="10" /><line x1="7" y1="14" x2="4" y2="14" />
		<line x1="17" y1="10" x2="20" y2="10" /><line x1="17" y1="14" x2="20" y2="14" />
		<line x1="10" y1="7" x2="10" y2="4" /><line x1="14" y1="7" x2="14" y2="4" />
		<line x1="10" y1="17" x2="10" y2="20" /><line x1="14" y1="17" x2="14" y2="20" />
	{:else if name === 'chevron-left'}
		<polyline points="15,5 8,12 15,19" />
	{:else if name === 'chevron-right'}
		<polyline points="9,5 16,12 9,19" />
	{:else if name === 'chevron-up'}
		<polyline points="5,15 12,8 19,15" />
	{:else if name === 'arrow-left'}
		<line x1="19" y1="12" x2="5" y2="12" /><polyline points="11,6 5,12 11,18" />
	{:else if name === 'plus'}
		<line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" />
	{:else if name === 'x'}
		<line x1="6" y1="6" x2="18" y2="18" /><line x1="18" y1="6" x2="6" y2="18" />
	{:else if name === 'search'}
		<circle cx="10" cy="10" r="6" /><line x1="20" y1="20" x2="14.8" y2="14.8" />
	{:else if name === 'menu'}
		<line x1="4" y1="6" x2="20" y2="6" /><line x1="4" y1="12" x2="20" y2="12" /><line x1="4" y1="18" x2="20" y2="18" />
	{:else if name === 'grid'}
		<rect x="3" y="3" width="8" height="8" rx="1.5" /><rect x="13" y="3" width="8" height="8" rx="1.5" />
		<rect x="3" y="13" width="8" height="8" rx="1.5" /><rect x="13" y="13" width="8" height="8" rx="1.5" />
	{:else if name === 'terminal'}
		<rect x="3" y="4" width="18" height="16" rx="2" />
		<polyline points="7,9 11,12 7,15" /><line x1="13" y1="15" x2="17" y2="15" />
	{:else if name === 'folder'}
		<path d="M3 7 L3 18 L21 18 L21 9 L11 9 L9 7 Z" />
	{:else if name === 'folder-open'}
		<path d="M3 7 L3 18 L21 18 L19 11 L7 11 L5.5 15 M3 7 L9 7 L11 9 L20 9" />
	{:else if name === 'file'}
		<path d="M6 3 L14 3 L18 7 L18 21 L6 21 Z" /><polyline points="14,3 14,7 18,7" />
	{:else if name === 'file-code'}
		<path d="M6 3 L14 3 L18 7 L18 21 L6 21 Z" /><polyline points="14,3 14,7 18,7" />
		<polyline points="9,13 7,15.5 9,18" /><polyline points="15,13 17,15.5 15,18" />
	{:else if name === 'image'}
		<path d="M6 3 L14 3 L18 7 L18 21 L6 21 Z" /><polyline points="14,3 14,7 18,7" />
		<circle cx="10.5" cy="12.5" r="1.25" /><polyline points="8,19 11,15.5 13.5,18 15,16.5 16,17.5" />
	{:else if name === 'archive'}
		<path d="M6 3 L14 3 L18 7 L18 21 L6 21 Z" /><polyline points="14,3 14,7 18,7" />
		<line x1="10.5" y1="12" x2="10.5" y2="13.5" /><line x1="10.5" y1="15" x2="10.5" y2="16.5" /><line x1="10.5" y1="18" x2="10.5" y2="19" />
	{:else if name === 'zap'}
		<polygon points="13,2 5,14 11,14 9,22 19,10 13,10" />
	{:else if name === 'clipboard-list'}
		<rect x="6" y="4" width="12" height="17" rx="1.5" /><rect x="9" y="2" width="6" height="3" rx="1" />
		<line x1="9" y1="11" x2="15" y2="11" /><line x1="9" y1="14.5" x2="15" y2="14.5" /><line x1="9" y1="18" x2="13" y2="18" />
	{:else if name === 'sliders'}
		<line x1="5" y1="4" x2="5" y2="20" /><circle cx="5" cy="9" r="2" />
		<line x1="12" y1="4" x2="12" y2="20" /><circle cx="12" cy="15" r="2" />
		<line x1="19" y1="4" x2="19" y2="20" /><circle cx="19" cy="7" r="2" />
	{:else if name === 'plug'}
		<rect x="9" y="2" width="2" height="6" /><rect x="13" y="2" width="2" height="6" />
		<rect x="6" y="8" width="12" height="6" rx="2" /><line x1="12" y1="14" x2="12" y2="21" />
	{:else if name === 'save'}
		<rect x="4" y="4" width="16" height="16" rx="1.5" /><rect x="7.5" y="4" width="9" height="5" />
		<rect x="8.5" y="13.5" width="7" height="6.5" />
	{:else if name === 'trash'}
		<line x1="4" y1="7" x2="20" y2="7" /><rect x="9" y="3" width="6" height="2.2" rx="0.5" />
		<path d="M6 7 L6.8 20 a1,1 0 0 0 1,1 H16.2 a1,1 0 0 0 1,-1 L18 7" />
		<line x1="10" y1="10.5" x2="10" y2="17" /><line x1="14" y1="10.5" x2="14" y2="17" />
	{:else if name === 'copy'}
		<rect x="4" y="4" width="10.5" height="10.5" rx="1.5" /><rect x="9.5" y="9.5" width="10.5" height="10.5" rx="1.5" />
	{:else if name === 'play'}
		<polygon points="6,4 20,12 6,20" />
	{:else if name === 'check-circle'}
		<circle cx="12" cy="12" r="9" /><polyline points="8,12.5 10.8,15.5 16,9.5" />
	{:else if name === 'x-circle'}
		<circle cx="12" cy="12" r="9" /><line x1="9" y1="9" x2="15" y2="15" /><line x1="15" y1="9" x2="9" y2="15" />
	{:else if name === 'info'}
		<circle cx="12" cy="12" r="9" /><line x1="12" y1="11" x2="12" y2="16.5" />
		<circle cx="12" cy="7.7" r="1" fill="currentColor" stroke="none" />
	{:else if name === 'alert-triangle'}
		<path d="M12 3 L22 20 H2 Z" />
		<line x1="12" y1="9.5" x2="12" y2="14" /><circle cx="12" cy="17" r="1" fill="currentColor" stroke="none" />
	{:else if name === 'monitor'}
		<rect x="3" y="4" width="18" height="12" rx="1.5" /><line x1="8" y1="20" x2="16" y2="20" /><line x1="12" y1="16" x2="12" y2="20" />
	{:else if name === 'refresh'}
		<path d="M20 11 A8 8 0 1 0 19.5 15.5" /><polyline points="20,5 20,11 14,11" />
	{/if}
</svg>
