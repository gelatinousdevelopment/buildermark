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
