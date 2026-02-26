export function stars(n: number, max = 5): string {
	return '★'.repeat(n) + '☆'.repeat(max - n);
}

export function shortId(id: string, len = 12): string {
	return id.length > len ? id.slice(0, len) + '…' : id;
}

export function singleLineTitle(title: string): string {
	return title.replace(/\r?\n/g, ' ').trim();
}

export function fmtTime(t: string | number): string {
	const d = new Date(t);
	return d.toLocaleString();
}

export function fmtTimeWithSeconds(t: string | number): string {
	const d = new Date(t);
	return d.toLocaleString(undefined, {
		year: 'numeric',
		month: '2-digit',
		day: '2-digit',
		hour: '2-digit',
		minute: '2-digit',
		second: '2-digit',
		hour12: false
	});
}

const DAY_IN_MS = 24 * 60 * 60 * 1000;
const YEAR_IN_MS = DAY_IN_MS * 365;

// eslint-disable-next-line @typescript-eslint/no-explicit-any
function getTemporal(): any | null {
	const temporal = (globalThis as { Temporal?: unknown }).Temporal;
	if (!temporal || typeof temporal !== 'object') return null;
	return temporal;
}

export function formatRelativeOrShortDate(unixMs: number): string {
	if (!Number.isFinite(unixMs) || unixMs <= 0) return 'Unknown';

	const temporal = getTemporal();
	if (!temporal?.Instant?.fromEpochMilliseconds || !temporal?.Now?.instant) {
		return new Date(unixMs).toLocaleString(undefined, {
			month: 'short',
			day: 'numeric',
			hour: 'numeric',
			minute: '2-digit'
		});
	}

	const instant = temporal.Instant.fromEpochMilliseconds(unixMs);
	const now = temporal.Now.instant();
	const deltaMs = now.epochMilliseconds - instant.epochMilliseconds;
	const absDeltaMs = Math.abs(deltaMs);

	if (deltaMs < 1000) {
		return '–';
	}

	if (absDeltaMs < DAY_IN_MS) {
		const rtf = new Intl.RelativeTimeFormat(undefined, { style: 'short', numeric: 'auto' });
		const absSeconds = Math.floor(absDeltaMs / 1000);
		if (absSeconds < 60)
			return rtf.format(Math.round(-deltaMs / 1000), 'second').replace(/\./g, '');

		const absMinutes = Math.floor(absDeltaMs / 60000);
		if (absMinutes < 60)
			return rtf.format(Math.round(-deltaMs / 60000), 'minute').replace(/\./g, '');

		return rtf.format(Math.round(-deltaMs / 3600000), 'hour').replace(/\./g, '');
	}

	const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
	const zoned = instant.toZonedDateTimeISO(zone);
	return zoned.toLocaleString(undefined, {
		month: 'short',
		day: 'numeric',
		hour: 'numeric',
		minute: '2-digit',
		year: absDeltaMs > YEAR_IN_MS ? 'numeric' : undefined
	});
}

export function formatFullDateTitle(unixMs: number): string {
	if (!Number.isFinite(unixMs) || unixMs <= 0) return '';

	const temporal = getTemporal();
	if (!temporal?.Instant?.fromEpochMilliseconds) {
		return new Date(unixMs).toLocaleString(undefined, {
			dateStyle: 'full',
			timeStyle: 'long'
		});
	}

	const instant = temporal.Instant.fromEpochMilliseconds(unixMs);
	const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
	return instant.toZonedDateTimeISO(zone).toLocaleString(undefined, {
		dateStyle: 'full',
		timeStyle: 'long'
	});
}

export function dateStringToUnixMsRange(dateStr: string): { from: number; to: number } {
	const [y, m, d] = dateStr.split('-').map(Number);
	const start = new Date(y, m - 1, d);
	const next = new Date(y, m - 1, d + 1);
	return { from: start.getTime(), to: next.getTime() };
}

type ParsedRemote = {
	domain: string;
	owner: string;
	repo: string;
};

function parseRemoteUrl(raw: string): ParsedRemote | null {
	const remote = raw.trim();
	if (!remote) return null;

	let domain = '';
	let path = '';

	if (remote.startsWith('ssh://')) {
		try {
			const parsed = new URL(remote);
			domain = parsed.hostname;
			path = parsed.pathname.replace(/^\/+/, '');
		} catch {
			return null;
		}
	} else if (remote.includes('://')) {
		try {
			const parsed = new URL(remote);
			domain = parsed.hostname;
			path = parsed.pathname.replace(/^\/+/, '');
		} catch {
			return null;
		}
	} else {
		const at = remote.indexOf('@');
		const colon = remote.indexOf(':');
		if (at < 0 || colon < 0 || colon <= at) return null;
		domain = remote.slice(at + 1, colon);
		path = remote.slice(colon + 1);
	}

	path = path.replace(/\.git$/, '').replace(/\/+$/, '');
	const parts = path.split('/');
	if (parts.length < 2) return null;

	const repo = parts[parts.length - 1];
	const owner = parts.slice(0, -1).join('/');
	if (!owner || !repo) return null;

	return {
		domain: domain.toLowerCase(),
		owner,
		repo
	};
}

export function commitUrl(remoteRaw: string, hash: string): string {
	const parsed = parseRemoteUrl(remoteRaw);
	if (!parsed || !hash) return '';

	switch (parsed.domain) {
		case 'github.com':
			return `https://github.com/${parsed.owner}/${parsed.repo}/commit/${hash}`;
		case 'gitlab.com':
			return `https://gitlab.com/${parsed.owner}/${parsed.repo}/-/commit/${hash}`;
		case 'codeberg.org':
			return `https://codeberg.org/${parsed.owner}/${parsed.repo}/commit/${hash}`;
		case 'bitbucket.org':
			return `https://bitbucket.org/${parsed.owner}/${parsed.repo}/commits/${hash}`;
		default:
			return `https://${parsed.domain}/${parsed.owner}/${parsed.repo}/commit/${hash}`;
	}
}
