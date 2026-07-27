import { writable } from 'svelte/store';
import type { Session } from './agents';

// These will use Wails runtime bindings
// We load dynamically to avoid import errors before bindings exist
export const sessions = writable<Session[]>([]);
export const sessionLoading = writable<boolean>(false);
