import { marked } from 'marked';
import { messageType as classifyMessageType } from './messageUtils';
import { combineDiffs, extractDiffText } from './diffCombiner';
import type { MessageRead } from './types';
import type {
	ConversationWithRatings,
	ConversationBatchDetail,
	ProjectCommitCoverage,
	CommitConversationLinks
} from './types';

marked.use({
	renderer: {
		html(token) {
			return escapeHtml(token.text);
		}
	}
});

export type ExportMode = 'commits-with-prompts' | 'prompts-with-commits';
export type ExportFormat = 'markdown' | 'html';
export type ExportSortOrder = 'newest' | 'oldest';

export interface ExportData {
	projectLabel: string;
	conversations: ConversationWithRatings[];
	batchDetails: ConversationBatchDetail[];
	commits: ProjectCommitCoverage[];
	links: CommitConversationLinks | null;
}

function formatDate(unixMs: number): string {
	return new Date(unixMs).toLocaleDateString(undefined, {
		year: 'numeric',
		month: 'long',
		day: 'numeric'
	});
}

function formatDateTime(unixMs: number): string {
	return new Date(unixMs).toLocaleString(undefined, {
		year: 'numeric',
		month: 'long',
		day: 'numeric',
		hour: 'numeric',
		minute: '2-digit'
	});
}

function shortHash(hash: string): string {
	return hash.slice(0, 7);
}

function convTitle(conv: ConversationWithRatings): string {
	const date = formatDateTime(conv.lastMessageTimestamp);
	const title = conv.title || 'Untitled';
	return `${date} (${conv.agent}) – ${title}`;
}

function escapeHtml(text: string): string {
	return text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function getUserMessages(
	conversationId: string,
	batchDetails: ConversationBatchDetail[]
): string[] {
	const detail = batchDetails.find((d) => d.conversationId === conversationId);
	if (!detail) return [];
	return detail.userMessages
		.filter((m) => m.role === 'user')
		.map((m) => m.content)
		.filter((c) => c.trim().length > 0);
}

type ChatMessageType = 'prompt' | 'question' | 'answer' | 'final_answer' | 'diff';

interface ChatMessage {
	role: 'user' | 'agent';
	type: ChatMessageType;
	content: string;
}

const CHAT_MESSAGE_TYPES = new Set<string>([
	'prompt',
	'question',
	'answer',
	'final_answer',
	'diff'
]);

function getChatMessages(
	conversationId: string,
	batchDetails: ConversationBatchDetail[]
): ChatMessage[] {
	const detail = batchDetails.find((d) => d.conversationId === conversationId);
	if (!detail) return [];
	return detail.userMessages
		.map((m) => ({
			role: m.role as 'user' | 'agent',
			type: classifyMessageType(m),
			content: m.content
		}))
		.filter((m) => CHAT_MESSAGE_TYPES.has(m.type) && m.content.trim().length > 0) as ChatMessage[];
}

function renderMarkdownToHtml(content: string): string {
	const normalized = content.replaceAll('\\r\\n', '\n').replaceAll('\\n', '\n');
	return marked.parse(normalized, { gfm: true, breaks: true }) as string;
}

interface DiffFileStat {
	path: string;
	added: number;
	removed: number;
}

function normalizeGitPath(pathText: string): string {
	let path = pathText.trim();
	if (path.startsWith('"') && path.endsWith('"')) {
		path = path.slice(1, -1).replaceAll('\\"', '"').replaceAll('\\\\', '\\');
	}
	if (path.startsWith('a/') || path.startsWith('b/')) {
		return path.slice(2);
	}
	return path;
}

/** Same logic as DiffMessageCard.perFileDiffStats — parses diff --git headers. */
function perFileDiffStats(textInput: string): DiffFileStat[] {
	const diffText = extractDiffText(textInput);
	if (!diffText) return [];

	const lines = diffText.split('\n');
	const files: DiffFileStat[] = [];
	let current: DiffFileStat | null = null;

	for (const line of lines) {
		if (line.startsWith('diff --git ')) {
			if (current) files.push(current);
			const match = line.match(/^diff --git (.+) (.+)$/);
			let path = 'unknown';
			if (match) {
				const newPath = normalizeGitPath(match[2]);
				path = newPath === '/dev/null' ? normalizeGitPath(match[1]) : newPath;
			}
			current = { path, added: 0, removed: 0 };
			continue;
		}
		if (!current) continue;
		if (line.startsWith('+++ ') || line.startsWith('--- ')) continue;
		if (line.startsWith('+')) current.added++;
		else if (line.startsWith('-')) current.removed++;
	}
	if (current) files.push(current);

	return files;
}

function renderDiffSummaryHtml(combinedContent: string): string {
	const files = perFileDiffStats(combinedContent);
	if (files.length === 0) {
		// Fallback: show a trimmed preview if we can't parse structured diff
		const preview = combinedContent
			.trim()
			.split('\n')
			.slice(0, 3)
			.map((l) => escapeHtml(l))
			.join('\n');
		return `<div class="diff-block"><pre class="diff-content">${preview}</pre></div>`;
	}

	const totalAdded = files.reduce((s, f) => s + f.added, 0);
	const totalRemoved = files.reduce((s, f) => s + f.removed, 0);

	let html = '<div class="diff-summary">';
	html += `<div class="diff-summary-header">${files.length} ${files.length === 1 ? 'file' : 'files'} <span class="diff-stat-add">+${totalAdded}</span> <span class="diff-stat-del">-${totalRemoved}</span></div>`;
	for (const file of files) {
		const counts: string[] = [];
		if (file.added > 0) counts.push(`<span class="diff-stat-add">+${file.added}</span>`);
		if (file.removed > 0) counts.push(`<span class="diff-stat-del">-${file.removed}</span>`);
		const countStr = counts.length > 0 ? counts.join(' ') : '';
		html += `<div class="diff-file-row"><span class="diff-file-path">${escapeHtml(file.path)}</span><span class="diff-file-stats">${countStr}</span></div>`;
	}
	html += '</div>';
	return html;
}

function bubbleLabel(msg: ChatMessage): string {
	if (msg.type === 'diff') return 'Diff';
	if (msg.type === 'question') return 'Agent Question';
	if (msg.type === 'answer') return 'Your Answer';
	return msg.role === 'user' ? 'User' : 'Agent';
}

/** Group consecutive diff messages together, leave other messages as-is. */
interface ChatGroup {
	kind: 'message' | 'diff-group';
	messages: ChatMessage[];
}

function groupChatMessages(messages: ChatMessage[]): ChatGroup[] {
	const groups: ChatGroup[] = [];
	let currentDiffs: ChatMessage[] = [];

	function flushDiffs() {
		if (currentDiffs.length > 0) {
			groups.push({ kind: 'diff-group', messages: currentDiffs });
			currentDiffs = [];
		}
	}

	for (const msg of messages) {
		if (msg.type === 'diff') {
			currentDiffs.push(msg);
		} else {
			flushDiffs();
			groups.push({ kind: 'message', messages: [msg] });
		}
	}
	flushDiffs();
	return groups;
}

function renderChatHtml(messages: ChatMessage[]): string {
	if (messages.length === 0) return '';
	const groups = groupChatMessages(messages);
	let html = '<div class="chat">';
	for (const group of groups) {
		if (group.kind === 'diff-group') {
			// Combine all consecutive diffs into one bubble using combineDiffs
			const fakeMessages: MessageRead[] = group.messages.map((m, i) => ({
				id: `diff-${i}`,
				timestamp: 0,
				conversationId: '',
				role: m.role,
				messageType: 'diff',
				model: '',
				content: m.content,
				rawJson: ''
			}));
			const combined = combineDiffs(fakeMessages);
			html += `<div class="bubble bubble-diff">`;
			html += `<div class="bubble-label">Diff</div>`;
			html += renderDiffSummaryHtml(combined);
			html += `</div>`;
		} else {
			const msg = group.messages[0];
			const baseCls = msg.role === 'user' ? 'bubble bubble-user' : 'bubble bubble-agent';
			html += `<div class="${baseCls}">`;
			html += `<div class="bubble-label">${bubbleLabel(msg)}</div>`;
			html += `<div class="bubble-content markdown-body">${renderMarkdownToHtml(msg.content)}</div>`;
			html += `</div>`;
		}
	}
	html += '</div>';
	return html;
}

function sortCommits(
	commits: ProjectCommitCoverage[],
	order: ExportSortOrder
): ProjectCommitCoverage[] {
	return [...commits].sort((a, b) =>
		order === 'newest'
			? b.authoredAtUnixMs - a.authoredAtUnixMs
			: a.authoredAtUnixMs - b.authoredAtUnixMs
	);
}

function sortConversations(
	convs: ConversationWithRatings[],
	order: ExportSortOrder
): ConversationWithRatings[] {
	return [...convs].sort((a, b) =>
		order === 'newest'
			? b.lastMessageTimestamp - a.lastMessageTimestamp
			: a.lastMessageTimestamp - b.lastMessageTimestamp
	);
}

const SECTION_SEPARATOR = '\n---\n\n';

// ── Markdown ──

export function generateMarkdown(
	data: ExportData,
	mode: ExportMode,
	sortOrder: ExportSortOrder = 'newest'
): string {
	const lines: string[] = [];
	lines.push(`# ${data.projectLabel}\n`);
	lines.push(`Coding sessions log${mode == 'commits-with-prompts' ? ', by commit' : ''}\n\n---\n`);

	if (mode === 'commits-with-prompts') {
		const commits = sortCommits(data.commits, sortOrder);
		const parts: string[] = [];
		for (const commit of commits) {
			if (commit.workingCopy) continue;
			const part: string[] = [];
			part.push(
				`## ${commit.subject} (${shortHash(commit.commitHash)}, ${formatDate(commit.authoredAtUnixMs)})\n`
			);
			part.push(`+${commit.linesAdded} -${commit.linesRemoved} lines\n`);

			const linkedConvIds = data.links?.commitToConversations[commit.commitHash] ?? [];
			if (linkedConvIds.length > 0) {
				part.push(`### Related Conversations\n`);
				for (const convId of linkedConvIds) {
					const conv = data.conversations.find((c) => c.id === convId);
					if (!conv) continue;
					part.push(`**${convTitle(conv)}**\n`);
					const messages = getUserMessages(convId, data.batchDetails);
					for (const msg of messages) {
						part.push('```\n' + escapeHtml(msg) + '\n```\n');
					}
				}
			}
			parts.push(part.join('\n'));
		}
		lines.push(parts.join(SECTION_SEPARATOR));
	} else {
		const convs = sortConversations(data.conversations, sortOrder);
		const parts: string[] = [];
		for (const conv of convs) {
			const part: string[] = [];
			part.push(`## ${convTitle(conv)}\n`);
			const messages = getUserMessages(conv.id, data.batchDetails);
			for (const msg of messages) {
				part.push('```\n' + escapeHtml(msg) + '\n```\n');
			}

			const linkedCommitHashes = data.links?.conversationToCommits[conv.id] ?? [];
			if (linkedCommitHashes.length > 0) {
				part.push(`### Related Commits\n`);
				for (const hash of linkedCommitHashes) {
					const commit = data.commits.find((c) => c.commitHash === hash);
					if (!commit) continue;
					part.push(
						`- ${shortHash(hash)}: ${commit.subject} (+${commit.linesAdded} -${commit.linesRemoved})`
					);
				}
				part.push('');
			}
			parts.push(part.join('\n'));
		}
		lines.push(parts.join(SECTION_SEPARATOR));
	}

	return lines.join('\n');
}

// ── HTML ──

interface HTMLSection {
	html: string;
}

function buildHTMLSections(
	data: ExportData,
	mode: ExportMode,
	sortOrder: ExportSortOrder
): { headerHtml: string; sections: HTMLSection[]; footerHtml: string } {
	const headerHtml = `<div class="title"><h1>${escapeHtml(data.projectLabel)}</h1><div class="subtitle">Coding sessions log${mode == 'commits-with-prompts' ? ', by commit' : ''}</div></div>`;

	const footerHtml = `<footer>Page generated by <a href="https://buildermark.dev" target="_blank">Buildermark</a></footer>`;

	const sections: HTMLSection[] = [];

	if (mode === 'commits-with-prompts') {
		const commits = sortCommits(data.commits, sortOrder);
		for (const commit of commits) {
			if (commit.workingCopy) continue;
			let html = '';
			html += `<h2>${escapeHtml(commit.subject)}</h2>`;
			html += `<div class="meta"><span class="hash">${shortHash(commit.commitHash)}</span> <span class="date">${formatDate(commit.authoredAtUnixMs)}</span> <span class="diff">+${commit.linesAdded} -${commit.linesRemoved} lines</span></div>`;

			const linkedConvIds = data.links?.commitToConversations[commit.commitHash] ?? [];
			if (linkedConvIds.length > 0) {
				html += `<h3>Related Conversations</h3>`;
				for (const convId of linkedConvIds) {
					const conv = data.conversations.find((c) => c.id === convId);
					if (!conv) continue;
					html += `<div class="conversation">`;
					html += `<div class="conv-title">${escapeHtml(convTitle(conv))}</div>`;
					html += renderChatHtml(getChatMessages(convId, data.batchDetails));
					html += `</div>`;
				}
			}
			sections.push({ html: `<section class="top-section">${html}</section>` });
		}
	} else {
		const convs = sortConversations(data.conversations, sortOrder);
		for (const conv of convs) {
			let html = '';
			html += `<h2>${escapeHtml(convTitle(conv))}</h2>`;

			html += renderChatHtml(getChatMessages(conv.id, data.batchDetails));

			const linkedCommitHashes = data.links?.conversationToCommits[conv.id] ?? [];
			if (linkedCommitHashes.length > 0) {
				html += `<h3>Related Commits</h3>`;
				html += `<ul class="commit-list">`;
				for (const hash of linkedCommitHashes) {
					const commit = data.commits.find((c) => c.commitHash === hash);
					if (!commit) continue;
					html += `<li><span class="hash">${shortHash(hash)}</span> ${escapeHtml(commit.subject)} <span class="diff">+${commit.linesAdded} -${commit.linesRemoved}</span></li>`;
				}
				html += `</ul>`;
			}
			sections.push({ html: `<section class="top-section">${html}</section>` });
		}
	}

	return { headerHtml, sections, footerHtml };
}

const htmlStyles = `
  :root {
    --accent-color: light-dark(#0066cc, #4d9aff);
    --accent-color-darker: light-dark(#0055aa, #3d88ee);
    --accent-color-darkest: light-dark(#003377, #9ecdff);
    --accent-color-ultralight: light-dark(#eff7ff, #002b5e);
    --accent-color-divider: light-dark(#9fbbd6, #3a5a7a);
    --color-text: light-dark(#444, #d4d4d4);
    --color-text-faded: light-dark(#888, #777);
    --color-text-secondary: light-dark(#666, #999);
    --color-text-tertiary: light-dark(#888, #777);
    --color-text-strong: light-dark(#333, #e0e0e0);
    --color-divider: light-dark(#bbb, #3a3a3e);
    --color-border-light: light-dark(#eee, #333);
    --color-border-medium: light-dark(#ccc, #444);
    --color-border-input: light-dark(#aaa, #666);
  }
  html {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    font-size: 13px;
  }
  :host, body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    font-size: 13px;
    line-height: 1.6;
    color: var(--color-text);
    margin: 0;
    padding: 2rem;
  }
  h1 {
    font-size: 1.3em;
    margin: 0;
  }
  h2 {
    font-size: 1.1em;
    margin: 0 0 0.3rem 0;
  }
  h3 {
    font-size: 1em;
    margin: 1rem 0 0.4rem 0;
    color: var(--color-text-secondary);
  }
  .top-section {
    padding-bottom: 1rem;
    margin-bottom: 1rem;
    border-bottom: 1px solid var(--color-border-light);
  }
  .top-section:last-child {
    border-bottom: none;
    margin-bottom: 0;
    padding-bottom: 0;
  }
  .meta {
    font-size: 0.9em;
    color: var(--color-text-tertiary);
    margin-bottom: 0.6rem;
    display: flex;
    gap: 0.8rem;
    flex-wrap: wrap;
  }
  .hash {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    background: light-dark(#f0f0f0, #2a2a2e);
    padding: 0.1rem 0.35rem;
    border-radius: 3px;
    font-size: 0.9em;
  }
  .diff {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.9em;
  }
  .agent {
    background: light-dark(#e8eaf6, #2a2e4a);
    padding: 0.1rem 0.4rem;
    border-radius: 3px;
    font-size: 0.9em;
  }
  .conversation {
    margin: 0.6rem 0;
    padding: 0.6rem 0.8rem;
    border: 1px solid var(--color-border-light);
    border-radius: 6px;
    background: light-dark(#fafafa, #1e1e22);
  }
  .conv-title {
    font-weight: 600;
    margin-bottom: 0.4rem;
  }
  .chat {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    margin: 0.6rem 0;
  }
  .bubble {
    max-width: 90%;
    padding: 0.55rem 0.75rem;
    border-radius: 10px;
    font-size: 0.95em;
    word-break: break-word;
    line-height: 1.5;
  }
  .bubble-user {
    align-self: flex-end;
    margin-left: 10%;
    background: light-dark(#e8f0fe, #1a3a5c);
    border: 1px solid light-dark(#c5d8f0, #2a5580);
    border-bottom-right-radius: 3px;
  }
  .bubble-agent {
    align-self: flex-start;
    margin-right: 10%;
    background: light-dark(#f5f5f5, #2a2a2e);
    border: 1px solid light-dark(#e0e0e0, #3a3a3e);
    border-bottom-left-radius: 3px;
  }
  .bubble-diff {
    align-self: stretch;
    background: light-dark(#fafafa, #1e1e22);
    border: 1px solid var(--color-border-medium);
    border-bottom-left-radius: 3px;
  }
  .bubble-label {
    font-size: 0.8em;
    font-weight: 600;
    color: var(--color-text-tertiary);
    margin-bottom: 0.15rem;
  }
  .bubble-user .bubble-label {
    color: light-dark(#0055aa, #6db3f8);
    text-align: right;
  }
  .bubble-diff .bubble-label {
    color: var(--color-text-secondary);
  }
  .diff-block {
    overflow-x: auto;
  }
  .diff-content {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.85em;
    line-height: 1.4;
    margin: 0;
    white-space: pre;
    background: none;
    border: none;
    padding: 0;
  }
  .diff-add {
    color: light-dark(#1a7f37, #3fb950);
    background: light-dark(#dafbe1, #0d2117);
    display: inline-block;
    width: 100%;
  }
  .diff-del {
    color: light-dark(#cf222e, #f85149);
    background: light-dark(#fce4e4, #2d1114);
    display: inline-block;
    width: 100%;
  }
  .diff-hunk {
    color: light-dark(#6e40c9, #d2a8ff);
    font-style: italic;
  }
  .diff-summary {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    font-size: 0.85em;
  }
  .diff-summary-header {
    font-weight: 600;
    color: var(--color-text-secondary);
    margin-bottom: 0.15rem;
    display: flex;
    gap: 0.4rem;
    align-items: center;
  }
  .diff-file-row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 1rem;
  }
  .diff-file-path {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    color: var(--color-text-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
  }
  .diff-file-stats {
    flex-shrink: 0;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    display: flex;
    gap: 0.4rem;
  }
  .diff-stat-add {
    color: light-dark(#1a7f37, #3fb950);
  }
  .diff-stat-del {
    color: light-dark(#cf222e, #f85149);
  }
  .diff-stat-unchanged {
    color: var(--color-text-tertiary);
  }
  /* Markdown body styles for bubble content */
  .markdown-body {
    line-height: 1.4em;
  }
  .markdown-body h1 { font-size: 1.1em; font-weight: bold; }
  .markdown-body h2 { font-size: 1em; font-weight: 500; }
  .markdown-body h3, .markdown-body h4, .markdown-body h5, .markdown-body h6 {
    font-size: 0.9em; font-weight: 500;
  }
  .markdown-body p {
    margin: 0.25rem 0;
    word-wrap: break-word;
  }
  .markdown-body pre {
    background: light-dark(#f6f8fa, #161b22);
    border-radius: 4px;
    border: 1px solid var(--color-border-light);
    overflow-x: auto;
    padding: 0.5rem;
  }
  .markdown-body code {
    background: light-dark(#eff1f3, #262a30);
    border-radius: 5px;
    border: 1px solid var(--color-border-light);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.8em;
    padding: 0.2rem 0.4rem;
  }
  .markdown-body pre code {
    border: none;
    background: none;
    padding: 0.2rem 0.4rem;
    border-radius: 5px;
  }
  .markdown-body ul, .markdown-body ol {
    padding-left: 1.2rem;
  }
  .markdown-body blockquote {
    margin: 0.25rem 0;
    padding: 0.1rem 0.6rem;
    border-left: 3px solid var(--color-border-medium);
    color: var(--color-text-secondary);
  }
  .markdown-body table {
    border-collapse: collapse;
    margin: 0.25rem 0;
    font-size: 0.9em;
  }
  .markdown-body th, .markdown-body td {
    border: 1px solid var(--color-border-medium);
    padding: 0.3rem 0.5rem;
  }
  .markdown-body th {
    background: light-dark(#f6f8fa, #262a30);
  }
  .markdown-body img { max-width: 100%; }
  .commit-list {
    margin: 0.3rem 0;
    padding-left: 1.2rem;
  }
  .commit-list li {
    margin: 0.25rem 0;
  }
  .toolbar {
    background: light-dark(#fff, #1e1e22);
    border-bottom: 1px solid var(--color-divider);
    padding: 1rem 2rem;
    margin: -2rem -2rem 1rem -2rem;
    display: flex;
    align-items: center;
    gap: 0.4rem;
    font-size: 12px;
  }
  .toolbar h1 {
    font-size: 1.7em;
    margin: 0;
    padding: 0;
    border: none;
    font-weight: 400;
  }
  .toolbar .spacer { flex: 1; }
  .toolbar button {
    font-size: 12px;
    padding: 0.2rem 0.35rem;
    cursor: pointer;
    border: 0.5px solid var(--color-border-input);
    border-radius: 3px;
    background: light-dark(#ffffff, #2a2a2e);
    display: flex;
    align-items: center;
    justify-content: center;
    min-width: 30px;
    height: 26px;
  }
  .toolbar button:hover {
    background: var(--accent-color-ultralight);
    border-color: var(--accent-color);
    color: var(--accent-color);
  }
  .toolbar button:active,
  .toolbar button.active {
    background: var(--accent-color);
    border-color: var(--accent-color);
    color: var(--accent-color-ultralight);
  }
  .toolbar button:disabled { background: light-dark(#ffffff, #2a2a2e); opacity: 0.35; cursor: default; }
  .toolbar button svg { width: 14px; height: 14px; }
  .toolbar .sep { width: 10px; height: 18px; background: transparent; }
  .seg-control {
    display: flex;
    gap: 0;
    border: 0.5px solid var(--accent-color);
    border-radius: 3px;
    overflow: hidden;
  }
  .seg-control button {
    border: none;
    border-radius: 0;
    border-right: 0.5px solid var(--accent-color);
    color: var(--color-text-secondary);
    padding: 0.2rem 0.8rem;
  }
  .seg-control button:last-child { border-right: none; }
  .seg-control button.active {
    background: var(--accent-color-ultralight);
    border-color: var(--accent-color);
    color: var(--accent-color);
  }
  .page-info {
    min-width: 50px; text-align: center; color: var(--color-text-secondary); cursor: pointer;
    padding: 0.15rem 0.3rem; border-radius: 3px; user-select: none;
  }
  .page-info:hover { background: var(--accent-color-ultralight); }
  .goto-input {
    width: 4em; font-size: 12px; padding: 0.15rem 0.3rem;
    border: 1px solid #6366f1; border-radius: 3px; text-align: center;
    outline: none;
  }
  footer,
  footer a {
    color: var(--color-text-faded);
    font-size: 0.9rem;
    padding: 0;
  }
  footer a:hover {
    color: #4346ff;
  }
`;

function buildToolbarHtml(titleHtml: string): string {
	return `<div class="toolbar">
  ${titleHtml}
  <div class="spacer"></div>
  <div class="seg-control">
    <button id="btn-all" class="active" title="All on one page">One Page</button>
    <button id="btn-paged" title="One per page">Paged</button>
  </div>
  <div class="sep"></div>
  <button id="btn-prev10" disabled title="Back 10">❮❮</button>
  <button id="btn-prev" disabled title="Previous">❮</button>
  <span id="page-info" class="page-info"></span>
  <button id="btn-next" disabled title="Next">❯</button>
  <button id="btn-next10" disabled title="Forward 10">❯❯</button>
</div>`;
}

/** Returns HTML + CSS suitable for injecting into a shadow root (toolbar without inline handlers). */
export function generateHTMLPreview(
	data: ExportData,
	mode: ExportMode,
	sortOrder: ExportSortOrder = 'newest'
): string {
	const { headerHtml, sections, footerHtml } = buildHTMLSections(data, mode, sortOrder);
	const bodyHtml = sections.map((s) => s.html).join('\n');
	return `<style>${htmlStyles}</style>\n${buildToolbarHtml(headerHtml)}\n${bodyHtml}\n${footerHtml}`;
}

/** Returns a full standalone HTML document with embedded toolbar and pagination JS. */
export function generateHTML(
	data: ExportData,
	mode: ExportMode,
	sortOrder: ExportSortOrder = 'newest'
): string {
	const { headerHtml, sections, footerHtml } = buildHTMLSections(data, mode, sortOrder);
	const bodyHtml = sections.map((s) => s.html).join('\n');
	const totalSections = sections.length;

	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>${escapeHtml(data.projectLabel)} | Export from Buildermark</title>
<style>
  ${htmlStyles.replace(':host, body', 'body')}
</style>
</head>
<body>
${buildToolbarHtml(headerHtml)}
${bodyHtml}
${footerHtml}
<script>
(function(){
  var sections = document.querySelectorAll('.top-section');
  var total = ${totalSections};
  var currentPage = 0;
  var vm = 'all';
  var btnAll = document.getElementById('btn-all');
  var btnPaged = document.getElementById('btn-paged');
  var btnPrev = document.getElementById('btn-prev');
  var btnNext = document.getElementById('btn-next');
  var btnPrev10 = document.getElementById('btn-prev10');
  var btnNext10 = document.getElementById('btn-next10');
  var pageInfo = document.getElementById('page-info');
  var editing = false;

  function setMode(m) {
    vm = m;
    btnAll.className = m === 'all' ? 'active' : '';
    btnPaged.className = m === 'paged' ? 'active' : '';
    if (m === 'all') {
      for (var i = 0; i < sections.length; i++) sections[i].style.display = '';
      updateNav();
    } else {
      currentPage = 0;
      showPage();
    }
  }

  function showPage() {
    for (var i = 0; i < sections.length; i++) {
      sections[i].style.display = i === currentPage ? '' : 'none';
    }
    updateNav();
    window.scrollTo(0, 0);
  }

  function updateNav() {
    var paged = vm === 'paged';
    btnPrev.disabled = !paged || currentPage <= 0;
    btnNext.disabled = !paged || currentPage >= total - 1;
    btnPrev10.disabled = !paged || currentPage <= 0;
    btnNext10.disabled = !paged || currentPage >= total - 1;
    if (!editing) {
      pageInfo.textContent = paged ? (currentPage + 1) + ' / ' + total : String(total);
    }
  }

  function go(delta) {
    if (vm !== 'paged') return;
    var p = currentPage + delta;
    if (p < 0) p = 0;
    if (p >= total) p = total - 1;
    currentPage = p;
    showPage();
  }

  function startEdit() {
    if (vm !== 'paged' || editing) return;
    editing = true;
    var input = document.createElement('input');
    input.type = 'text';
    input.className = 'goto-input';
    input.value = String(currentPage + 1);
    pageInfo.textContent = '';
    pageInfo.appendChild(input);
    input.focus();
    input.select();
    function commit() {
      if (!editing) return;
      editing = false;
      var val = parseInt(input.value, 10);
      if (!isNaN(val) && val >= 1 && val <= total) {
        currentPage = val - 1;
        showPage();
      }
      pageInfo.removeChild(input);
      updateNav();
    }
    input.addEventListener('keydown', function(e) {
      if (e.key === 'Enter') commit();
      if (e.key === 'Escape') { editing = false; pageInfo.removeChild(input); updateNav(); }
    });
    input.addEventListener('blur', commit);
  }

  btnAll.addEventListener('click', function(){ setMode('all'); });
  btnPaged.addEventListener('click', function(){ setMode('paged'); });
  btnPrev.addEventListener('click', function(){ go(-1); });
  btnNext.addEventListener('click', function(){ go(1); });
  btnPrev10.addEventListener('click', function(){ go(-10); });
  btnNext10.addEventListener('click', function(){ go(10); });
  pageInfo.addEventListener('click', startEdit);

  updateNav();

  setMode('paged');
})();
</script>
</body>
</html>`;
}
