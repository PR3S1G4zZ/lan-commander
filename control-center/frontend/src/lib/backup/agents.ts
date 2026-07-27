import { writable, derived } from 'svelte/store';

export interface CPUInfo {
	percent: number;
	cores: number;
	model: string;
}

export interface MemoryInfo {
	total: number;
	used: number;
	free: number;
	percent: number;
}

export interface DiskInfo {
	mount: string;
	fs_type: string;
	total: number;
	used: number;
	free: number;
	percent: number;
}

export interface NetInfo {
	ip: string;
	mac: string;
	hostname: string;
}

export interface SystemInfo {
	hostname: string;
	os: string;
	platform: string;
	arch: string;
	cpu: CPUInfo;
	memory: MemoryInfo;
	disks: DiskInfo[];
	uptime: number;
	net: NetInfo;
	agent_version: string;
}

export interface CommandResult {
	stdout: string;
	stderr: string;
	exit_code: number;
	duration_ms: number;
}

export interface DirEntry {
	name: string;
	path: string;
	is_dir: boolean;
	size: number;
	mode: string;
	mod_time: string;
}

export interface DirContents {
	path: string;
	entries: DirEntry[];
	total: number;
}

export interface ScreenshotData {
	format: string;
	data: number[];
	width: number;
	height: number;
}

export interface AgentInfo {
	id: string;
	host: string;
	port: number;
	name: string;
	os: string;
	arch: string;
	connected: boolean;
	lastSeen: string;
	systemInfo: SystemInfo | null;
	cpuHistory: number[];
}

export interface Session {
	id: number;
	name: string;
	host: string;
	port: number;
	auth_token: string;
	created_at: string;
	last_connected: string;
}

export interface AuditEntry {
	id: number;
	timestamp: string;
	action: string;
	agent_id: string;
	user: string;
	details: string;
	status: string;
}

export interface Script {
	name: string;
	content: string;
	created_at: string;
}

export interface Notification {
	id: string;
	type: 'success' | 'error' | 'info' | 'warning';
	message: string;
	timestamp: number;
}

export const agents = writable<AgentInfo[]>([]);
export const selectedAgentId = writable<string | null>(null);

export const selectedAgent = derived([agents, selectedAgentId], ([$agents, $selectedAgentId]) => {
	if (!$selectedAgentId) return null;
	return $agents.find(a => a.id === $selectedAgentId) || null;
});

export const connectedAgents = derived(agents, ($agents) =>
	$agents.filter(a => a.connected)
);

export const agentCounts = derived(agents, ($agents) => ({
	total: $agents.length,
	connected: $agents.filter(a => a.connected).length,
	offline: $agents.filter(a => !a.connected).length,
}));
