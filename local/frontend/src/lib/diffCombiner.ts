import type { MessageRead } from '$lib/types';
import { mergeSequentialDiffs } from '$lib/diffmerge';

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
 * Extract the file path from a unified diff text by looking at +++ header.
 * Falls back to --- header if +++ is /dev/null (deleted file).
 */
function extractFilePath(diffText: string): string {
	for (const line of diffText.split('\n')) {
		if (line.startsWith('+++ ')) {
			let path = line.slice(4).trim();
			if (path.startsWith('b/')) path = path.slice(2);
			if (path !== '/dev/null') return path;
		}
		if (line.startsWith('--- ')) {
			let path = line.slice(4).trim();
			if (path.startsWith('a/')) path = path.slice(2);
			if (path !== '/dev/null') return path;
		}
	}
	return '';
}

/**
 * Combine multiple diff messages into a single unified diff string.
 * Groups diffs by file path and merges sequential edits to the same file
 * using mergeSequentialDiffs, so multiple edits to one file become a single diff.
 */
export function combineDiffs(diffMessages: MessageRead[], merge = true): string {
	const parts: { text: string; filePath: string }[] = [];
	for (const msg of diffMessages) {
		const text = extractDiffText(msg.content);
		if (text) parts.push({ text, filePath: extractFilePath(text) });
	}

	// Group by file path, maintaining order of first appearance
	const fileGroups = new Map<string, string[]>();
	const fileOrder: string[] = [];
	let unknownCounter = 0;
	for (const part of parts) {
		const key = part.filePath || `__unknown_${unknownCounter++}`;
		if (!fileGroups.has(key)) {
			fileGroups.set(key, []);
			fileOrder.push(key);
		}
		fileGroups.get(key)!.push(part.text);
	}

	// Merge sequential diffs for each file, or keep single diffs as-is
	const results: string[] = [];
	for (const key of fileOrder) {
		const diffs = fileGroups.get(key)!;
		if (merge && diffs.length > 1) {
			const merged = mergeSequentialDiffs(diffs);
			if (merged) {
				// mergeSequentialDiffs outputs --- /+++ without a diff --git header.
				// DiffMessageCard relies on diff --git headers to count files and
				// extract per-file stats, so prepend one.
				results.push(`diff --git a/${key} b/${key}\n${merged}`);
			}
		} else {
			results.push(...diffs);
		}
	}

	return results.join('\n');
}
