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

marked.use({
	renderer: {
		html(token) {
			return escapeHtml(token.text);
		}
	}
});

export function renderMarkdown(content: string): string {
	return marked.parse(content, { gfm: true, breaks: true }) as string;
}

export function normalizeEscapedNewlines(content: string): string {
	return content.replaceAll('\\r\\n', '\n').replaceAll('\\n', '\n');
}

export function isUserPromptMessage(message: MessageRead): boolean {
	if (messageType(message) === 'prompt') return true;
	if (message.role !== 'user') return false;
	const trimmed = message.content.trimStart();
	if (trimmed.startsWith('/') || trimmed.startsWith('$bb')) return false;
	return true;
}

export function messageType(
	message: MessageRead
): 'prompt' | 'question' | 'answer' | 'final_answer' | 'log' {
	const t = typeof message.messageType === 'string' ? message.messageType.trim().toLowerCase() : '';
	if (t === 'prompt') return message.role === 'user' ? 'prompt' : 'log';
	if (t === 'question') return message.role === 'agent' ? 'question' : 'log';
	if (t === 'answer') return message.role === 'user' ? 'answer' : 'log';
	if (t === 'final_answer') return message.role === 'agent' ? 'final_answer' : 'log';
	if (t === 'log') return 'log';
	if (isUserPromptMessageLegacy(message)) return 'prompt';
	return 'log';
}

function isUserPromptMessageLegacy(message: MessageRead): boolean {
	if (message.role !== 'user') return false;
	const trimmed = message.content.trimStart();
	if (trimmed.startsWith('/') || trimmed.startsWith('$bb')) return false;
	return true;
}

export function isQuestionMessage(message: MessageRead): boolean {
	return messageType(message) === 'question';
}

export function isAnswerMessage(message: MessageRead): boolean {
	return messageType(message) === 'answer';
}

export function isFinalAnswerMessage(message: MessageRead): boolean {
	return messageType(message) === 'final_answer';
}

export function isStandaloneTimelineMessage(message: MessageRead): boolean {
	const t = messageType(message);
	return t === 'prompt' || t === 'question' || t === 'answer' || t === 'final_answer';
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
	const model = typeof message.model === 'string' ? message.model.trim() : '';
	if (model) return model;
	return detectModelFromRawJson(message.rawJson);
}

export function messageTypeLabel(message: MessageRead): string {
	const type = messageType(message);
	if (type === 'final_answer') return 'final answer';
	if (type !== 'log') return type;
	if (isDiffMessage(message)) return 'diff';
	try {
		const obj = JSON.parse(message.rawJson) as Record<string, unknown>;
		const t = typeof obj.type === 'string' ? obj.type : '';
		if (t && t !== 'user') return t;
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

export function formatDuration(ms: number): string {
	const seconds = Math.round(ms / 1000);
	if (seconds < 60) return `${seconds}s`;
	const minutes = Math.floor(seconds / 60);
	const remainingSeconds = seconds % 60;
	if (minutes < 60) {
		return remainingSeconds > 0 ? `${minutes}m ${remainingSeconds}s` : `${minutes}m`;
	}
	const hours = Math.floor(minutes / 60);
	const remainingMinutes = minutes % 60;
	return remainingMinutes > 0 ? `${hours}h ${remainingMinutes}m` : `${hours}h`;
}

export function groupTimeSpan(messages: MessageRead[]): number {
	if (messages.length < 2) return 0;
	let min = messages[0].timestamp;
	let max = messages[0].timestamp;
	for (const m of messages) {
		if (m.timestamp < min) min = m.timestamp;
		if (m.timestamp > max) max = m.timestamp;
	}
	return max - min;
}

export function groupModelLabel(messages: MessageRead[], fallbackAgent = 'agent'): string {
	const models = new Set<string>();
	for (const message of messages) {
		const model = messageModel(message);
		if (model) models.add(model);
	}
	if (models.size === 1) return Array.from(models)[0] ?? fallbackAgent;
	return fallbackAgent;
}

function detectModelFromRawJson(rawJson: string): string {
	try {
		const parsed = JSON.parse(rawJson) as unknown;
		return findModel(parsed);
	} catch {
		return '';
	}
}

function findModel(value: unknown): string {
	if (!value || typeof value !== 'object') return '';
	if (Array.isArray(value)) {
		for (const item of value) {
			const model = findModel(item);
			if (model) return model;
		}
		return '';
	}
	const map = value as Record<string, unknown>;
	for (const key of ['model', 'modelName', 'model_name', 'model_slug', 'modelSlug']) {
		const model = typeof map[key] === 'string' ? map[key].trim() : '';
		if (model) return model;
	}
	for (const nested of Object.values(map)) {
		const model = findModel(nested);
		if (model) return model;
	}
	return '';
}
