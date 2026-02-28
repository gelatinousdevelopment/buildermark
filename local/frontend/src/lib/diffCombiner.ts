import type { MessageRead } from '$lib/types';

/**
 * Extract the raw unified diff text from a message's content.
 * Handles ```diff wrapped content and raw git diffs.
 */
export function extractDiffText(content: string): string {
	let text = content.trim();
	if (text.startsWith('```diff')) {
		text = text.slice('```diff'.length).trimStart();
		if (text.endsWith('```')) text = text.slice(0, -3).trimEnd();
	}

	const gitIdx = text.indexOf('diff --git ');
	if (gitIdx >= 0) return text.slice(gitIdx).trim();

	const oldIdx = text.indexOf('\n--- ');
	if (oldIdx >= 0) return text.slice(oldIdx + 1).trim();
	if (text.startsWith('--- ')) return text;
	return '';
}

/**
 * Combine multiple diff messages into a single unified diff string.
 * Concatenates individual diffs separated by newlines.
 */
export function combineDiffs(diffMessages: MessageRead[]): string {
	const parts: string[] = [];
	for (const msg of diffMessages) {
		const text = extractDiffText(msg.content);
		if (text) parts.push(text);
	}
	return parts.join('\n');
}
