/**
 * Formatting utilities for LAN Commander UI
 */

export function formatBytes(bytes: number): string {
	if (bytes === 0) return '0 B';
	const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
	const k = 1024;
	const i = Math.floor(Math.log(bytes) / Math.log(k));
	const val = bytes / Math.pow(k, i);
	return `${val.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

export function formatUptime(seconds: number): string {
	const days = Math.floor(seconds / 86400);
	const hours = Math.floor((seconds % 86400) / 3600);
	const mins = Math.floor((seconds % 3600) / 60);

	if (days > 0) return `${days}d ${hours}h ${mins}m`;
	if (hours > 0) return `${hours}h ${mins}m`;
	return `${mins}m`;
}

export function formatPercent(pct: number): string {
	return `${pct.toFixed(1)}%`;
}

export function formatTime(isoStr: string): string {
	if (!isoStr) return '-';
	const d = new Date(isoStr);
	return d.toLocaleTimeString();
}

export function formatTimeAgo(isoStr: string): string {
	if (!isoStr) return 'never';
	const diff = Date.now() - new Date(isoStr).getTime();
	const secs = Math.floor(diff / 1000);
	if (secs < 60) return `${secs}s ago`;
	const mins = Math.floor(secs / 60);
	if (mins < 60) return `${mins}m ago`;
	const hours = Math.floor(mins / 60);
	if (hours < 24) return `${hours}h ago`;
	const days = Math.floor(hours / 24);
	return `${days}d ago`;
}

export function getOsIcon(os: string): string {
	const lower = os.toLowerCase();
	if (lower.includes('windows')) return '🪟';
	if (lower.includes('linux')) return '🐧';
	if (lower.includes('darwin') || lower.includes('mac')) return '🍎';
	return '💻';
}

export function getFileIcon(entry: { is_dir: boolean; name: string }): string {
	if (entry.is_dir) return '📁';
	const ext = entry.name.split('.').pop()?.toLowerCase() || '';
	const icons: Record<string, string> = {
		txt: '📄', md: '📝', json: '📋', xml: '📋',
		yml: '📋', yaml: '📋', toml: '📋',
		js: '📜', ts: '📜', jsx: '📜', tsx: '📜',
		py: '🐍', go: '🔷', rs: '🦀',
		html: '🌐', css: '🎨', scss: '🎨',
		png: '🖼️', jpg: '🖼️', jpeg: '🖼️', gif: '🖼️', svg: '🖼️',
		zip: '📦', rar: '📦', '7z': '📦', tar: '📦', gz: '📦',
		exe: '⚙️', msi: '⚙️', deb: '⚙️', rpm: '⚙️',
		pdf: '📕', doc: '📘', docx: '📘', xls: '📊', xlsx: '📊',
		mp3: '🎵', wav: '🎵', mp4: '🎬', avi: '🎬', mkv: '🎬',
	};
	return icons[ext] || '📄';
}

export function truncate(str: string, maxLen: number = 50): string {
	if (str.length <= maxLen) return str;
	return str.substring(0, maxLen - 3) + '...';
}
