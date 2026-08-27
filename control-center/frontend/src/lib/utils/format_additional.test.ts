import { afterEach, describe, expect, it, vi } from 'vitest';
import {
	formatBytes,
	formatPercent,
	formatTime,
	formatTimeAgo,
	formatUptime,
	getErrorMessage,
	getFileMeta,
	getOsMeta,
	truncate,
} from './format';

describe('format utilities', () => {
	afterEach(() => {
		vi.useRealTimers();
	});

	it('formats zero bytes and binary unit boundaries', () => {
		expect(formatBytes(0)).toBe('0 B');
		expect(formatBytes(1)).toBe('1 B');
		expect(formatBytes(1023)).toBe('1023 B');
		expect(formatBytes(1024)).toBe('1.0 KB');
		expect(formatBytes(1536)).toBe('1.5 KB');
		expect(formatBytes(1024 ** 5)).toBe('1.0 PB');
	});

	it('formats uptime at minute, hour, and day transitions', () => {
		expect(formatUptime(0)).toBe('0m');
		expect(formatUptime(59)).toBe('0m');
		expect(formatUptime(60)).toBe('1m');
		expect(formatUptime(3599)).toBe('59m');
		expect(formatUptime(3600)).toBe('1h 0m');
		expect(formatUptime(86399)).toBe('23h 59m');
		expect(formatUptime(86400)).toBe('1d 0h 0m');
		expect(formatUptime(90061)).toBe('1d 1h 1m');
	});

	it('formats percentages with one decimal place', () => {
		expect(formatPercent(0)).toBe('0.0%');
		expect(formatPercent(12.34)).toBe('12.3%');
		expect(formatPercent(100)).toBe('100.0%');
	});

	it('normalizes errors, strings, and primitive unknown values', () => {
		expect(getErrorMessage(new Error('connection refused'))).toBe('connection refused');
		expect(getErrorMessage('cancelled')).toBe('cancelled');
		expect(getErrorMessage(null)).toBe('null');
		expect(getErrorMessage(undefined)).toBe('undefined');
		expect(getErrorMessage(404)).toBe('404');
	});

	it('uses a placeholder for a missing display timestamp', () => {
		expect(formatTime('')).toBe('-');
	});

	it('formats relative times using controlled second, minute, hour, and day boundaries', () => {
		vi.useFakeTimers();
		vi.setSystemTime(new Date('2026-01-02T03:04:05.000Z'));

		expect(formatTimeAgo('')).toBe('never');
		expect(formatTimeAgo('2026-01-02T03:04:05.000Z')).toBe('0s ago');
		expect(formatTimeAgo('2026-01-02T03:03:06.000Z')).toBe('59s ago');
		expect(formatTimeAgo('2026-01-02T03:03:05.000Z')).toBe('1m ago');
		expect(formatTimeAgo('2026-01-02T02:04:06.000Z')).toBe('59m ago');
		expect(formatTimeAgo('2026-01-02T02:04:05.000Z')).toBe('1h ago');
		expect(formatTimeAgo('2026-01-01T03:04:06.000Z')).toBe('23h ago');
		expect(formatTimeAgo('2026-01-01T03:04:05.000Z')).toBe('1d ago');
	});

	it('maps supported operating systems case-insensitively', () => {
		expect(getOsMeta('Windows Server')).toEqual({ label: 'WIN', color: 'text-sky-400 bg-sky-500/10' });
		expect(getOsMeta('LINUX')).toEqual({ label: 'LNX', color: 'text-amber-400 bg-amber-500/10' });
		expect(getOsMeta('Darwin')).toEqual({ label: 'MAC', color: 'text-slate-300 bg-slate-500/10' });
		expect(getOsMeta('macOS')).toEqual({ label: 'MAC', color: 'text-slate-300 bg-slate-500/10' });
		expect(getOsMeta('FreeBSD')).toEqual({ label: 'OS', color: 'text-slate-400 bg-slate-500/10' });
	});

	it('prioritizes directory metadata over a file extension', () => {
		expect(getFileMeta({ is_dir: true, name: 'logs.zip' })).toEqual({
			icon: 'folder',
			color: 'text-amber-400',
		});
	});

	it('classifies code, image, archive, and unknown file extensions', () => {
		expect(getFileMeta({ is_dir: false, name: 'App.TSX' })).toEqual({
			icon: 'file-code',
			color: 'text-cyan-400',
		});
		expect(getFileMeta({ is_dir: false, name: 'photo.JPEG' })).toEqual({
			icon: 'image',
			color: 'text-purple-400',
		});
		expect(getFileMeta({ is_dir: false, name: 'backup.tar.gz' })).toEqual({
			icon: 'archive',
			color: 'text-orange-400',
		});
		expect(getFileMeta({ is_dir: false, name: 'README' })).toEqual({
			icon: 'file',
			color: 'text-slate-400',
		});
	});

	it('leaves strings at or below the requested truncation length unchanged', () => {
		expect(truncate('short', 10)).toBe('short');
		expect(truncate('exact', 5)).toBe('exact');
		expect(truncate('', 0)).toBe('');
	});

	it('keeps the ellipsis within a custom truncation boundary', () => {
		expect(truncate('12345678', 7)).toBe('1234...');
		expect(truncate('1234', 3)).toBe('...');
	});

	it('uses a default maximum length of fifty characters', () => {
		const longName = 'x'.repeat(51);
		expect(truncate(longName)).toBe(`${'x'.repeat(47)}...`);
	});
});
