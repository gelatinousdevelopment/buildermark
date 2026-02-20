export function stars(n: number, max = 5): string {
	return '★'.repeat(n) + '☆'.repeat(max - n);
}

export function shortId(id: string, len = 12): string {
	return id.length > len ? id.slice(0, len) + '…' : id;
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
