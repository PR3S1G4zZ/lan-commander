import { writable } from 'svelte/store';
import type { Notification } from './agents';

export type ViewType = 'dashboard' | 'terminal' | 'files' | 'multi-exec' | 'scripts' | 'audit' | 'settings';

export const darkMode = writable<boolean>(true);
export const currentView = writable<ViewType>('dashboard');
export const sidebarCollapsed = writable<boolean>(false);
export const notifications = writable<Notification[]>([]);

let notifCounter = 0;

export function addNotification(type: Notification['type'], message: string): void {
	const id = `notif-${++notifCounter}`;
	const notification: Notification = { id, type, message, timestamp: Date.now() };
	notifications.update(n => [...n, notification]);

	// Auto-dismiss after 5 seconds
	setTimeout(() => {
		notifications.update(n => n.filter(item => item.id !== id));
	}, 5000);
}

export function removeNotification(id: string): void {
	notifications.update(n => n.filter(item => item.id !== id));
}
