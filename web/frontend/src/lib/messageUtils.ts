import { marked } from 'marked';
import type { MessageRead } from '$lib/types';

export function escapeHtml(s: string): string {
	return s
		.replaceAll('&', '&amp;')
		.replaceAll('<', '&lt;')
		.replaceAll('>', '&gt;')
		.replaceAll('"', '&quot;')
		.replaceAll("'", '&#39;');
}

export function renderMarkdown(content: string): string {
	return marked.parse(escapeHtml(content), { gfm: true, breaks: true }) as string;
}

export function isUserPromptMessage(message: MessageRead): boolean {
	if (message.role !== 'user') return false;
	const trimmed = message.content.trimStart();
	if (trimmed.startsWith('/zrate') || trimmed.startsWith('$zrate')) return false;
	return true;
}

export function isDiffMessage(message: MessageRead): boolean {
	const trimmed = message.content.trimStart();
	if (trimmed.startsWith('```diff') || trimmed.startsWith('diff --git ')) return true;
	try {
		const obj = JSON.parse(message.rawJson) as Record<string, unknown>;
		return obj.source === 'derived_diff';
	} catch {
		return false;
	}
}

export function messageModel(message: MessageRead): string {
	return typeof message.model === 'string' ? message.model.trim() : '';
}

export function messageTypeLabel(message: MessageRead): string {
	if (isDiffMessage(message)) return 'diff';
	try {
		const obj = JSON.parse(message.rawJson) as Record<string, unknown>;
		const t = typeof obj.type === 'string' ? obj.type : '';
		if (t) return t;
	} catch {
		// ignore parse failures
	}
	return message.role;
}

function firstLine(s: string): string {
	return s.replace(/\s+/g, ' ').trim();
}

export function messageSummary(message: MessageRead): string {
	const line = firstLine(message.content);
	if (!line) return `[${messageTypeLabel(message)}]`;
	return line.length > 120 ? `${line.slice(0, 117)}...` : line;
}

export function groupModelLabel(messages: MessageRead[]): string {
	const models = new Set<string>();
	for (const message of messages) {
		const model = messageModel(message);
		if (model) models.add(model);
	}
	if (models.size === 1) return Array.from(models)[0] ?? 'agent';
	return 'agent';
}
