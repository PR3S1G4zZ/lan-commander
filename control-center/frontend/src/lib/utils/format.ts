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

export function getErrorMessage(error: unknown): string {
	if (error instanceof Error) return error.message;
	if (typeof error === 'string') return error;
	return String(error);
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

export interface OsMeta {
	label: string;
	color: string;
}

export function getOsMeta(os: string): OsMeta {
	const lower = os.toLowerCase();
	if (lower.includes('windows')) return { label: 'WIN', color: 'text-sky-400 bg-sky-500/10' };
	if (lower.includes('linux')) return { label: 'LNX', color: 'text-amber-400 bg-amber-500/10' };
	if (lower.includes('darwin') || lower.includes('mac')) return { label: 'MAC', color: 'text-slate-300 bg-slate-500/10' };
	return { label: 'OS', color: 'text-slate-400 bg-slate-500/10' };
}

export interface FileMeta {
	icon: string;
	color: string;
}

export function getFileMeta(entry: { is_dir: boolean; name: string }): FileMeta {
	if (entry.is_dir) return { icon: 'folder', color: 'text-amber-400' };
	const ext = entry.name.split('.').pop()?.toLowerCase() || '';
	const groups: Record<string, [string, string]> = {
		code: ['file-code', 'text-cyan-400'],
		image: ['image', 'text-purple-400'],
		archive: ['archive', 'text-orange-400'],
	};
	const codeExt = ['js', 'ts', 'jsx', 'tsx', 'py', 'go', 'rs', 'html', 'css', 'scss', 'json', 'xml', 'yml', 'yaml', 'toml', 'sh', 'ps1'];
	const imageExt = ['png', 'jpg', 'jpeg', 'gif', 'svg', 'webp', 'bmp'];
	const archiveExt = ['zip', 'rar', '7z', 'tar', 'gz'];
	if (codeExt.includes(ext)) return { icon: groups.code[0], color: groups.code[1] };
	if (imageExt.includes(ext)) return { icon: groups.image[0], color: groups.image[1] };
	if (archiveExt.includes(ext)) return { icon: groups.archive[0], color: groups.archive[1] };
	return { icon: 'file', color: 'text-slate-400' };
}

export function truncate(str: string, maxLen: number = 50): string {
	if (str.length <= maxLen) return str;
	return str.substring(0, maxLen - 3) + '...';
}
