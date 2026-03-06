import type {
	ConversationWithRatings,
	ConversationBatchDetail,
	ProjectCommitCoverage,
	CommitConversationLinks
} from './types';

export type ExportMode = 'commits-with-prompts' | 'prompts-with-commits' | 'just-prompts';
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
	return new Date(unixMs).toLocaleDateString('en-US', {
		year: 'numeric',
		month: 'short',
		day: 'numeric'
	});
}

function shortHash(hash: string): string {
	return hash.slice(0, 7);
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
					part.push(`**${conv.title || 'Untitled'}** (${conv.agent})\n`);
					const messages = getUserMessages(convId, data.batchDetails);
					for (const msg of messages) {
						part.push('```\n' + escapeHtml(msg) + '\n```\n');
					}
				}
			}
			parts.push(part.join('\n'));
		}
		lines.push(parts.join(SECTION_SEPARATOR));
	} else if (mode === 'prompts-with-commits') {
		const convs = sortConversations(data.conversations, sortOrder);
		const parts: string[] = [];
		for (const conv of convs) {
			const part: string[] = [];
			part.push(
				`## ${conv.title || 'Untitled'} (${conv.agent}, ${formatDate(conv.lastMessageTimestamp)})\n`
			);
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
	} else {
		const convs = sortConversations(data.conversations, sortOrder);
		const parts: string[] = [];
		for (const conv of convs) {
			const part: string[] = [];
			part.push(
				`## ${conv.title || 'Untitled'} (${conv.agent}, ${formatDate(conv.lastMessageTimestamp)})\n`
			);
			const messages = getUserMessages(conv.id, data.batchDetails);
			for (const msg of messages) {
				part.push('```\n' + escapeHtml(msg) + '\n```\n');
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
					html += `<div class="conv-title">${escapeHtml(conv.title || 'Untitled')} <span class="agent">${escapeHtml(conv.agent)}</span></div>`;
					const messages = getUserMessages(convId, data.batchDetails);
					for (const msg of messages) {
						html += `<div class="message">${escapeHtml(msg).replace(/\n/g, '<br>')}</div>`;
					}
					html += `</div>`;
				}
			}
			sections.push({ html: `<section class="top-section">${html}</section>` });
		}
	} else if (mode === 'prompts-with-commits') {
		const convs = sortConversations(data.conversations, sortOrder);
		for (const conv of convs) {
			let html = '';
			html += `<h2>${escapeHtml(conv.title || 'Untitled')}</h2>`;
			html += `<div class="meta"><span class="agent">${escapeHtml(conv.agent)}</span> <span class="date">${formatDate(conv.lastMessageTimestamp)}</span></div>`;

			const messages = getUserMessages(conv.id, data.batchDetails);
			for (const msg of messages) {
				html += `<div class="message">${escapeHtml(msg).replace(/\n/g, '<br>')}</div>`;
			}

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
	} else {
		const convs = sortConversations(data.conversations, sortOrder);
		for (const conv of convs) {
			let html = '';
			html += `<h2>${escapeHtml(conv.title || 'Untitled')}</h2>`;
			html += `<div class="meta"><span class="agent">${escapeHtml(conv.agent)}</span> <span class="date">${formatDate(conv.lastMessageTimestamp)}</span></div>`;

			const messages = getUserMessages(conv.id, data.batchDetails);
			for (const msg of messages) {
				html += `<div class="message">${escapeHtml(msg).replace(/\n/g, '<br>')}</div>`;
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
    color: #333;
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
    color: #555;
  }
  .top-section {
    padding-bottom: 1rem;
    margin-bottom: 1rem;
    border-bottom: 1px solid #e0e0e0;
  }
  .top-section:last-child {
    border-bottom: none;
    margin-bottom: 0;
    padding-bottom: 0;
  }
  .meta {
    font-size: 0.9em;
    color: #777;
    margin-bottom: 0.6rem;
    display: flex;
    gap: 0.8rem;
    flex-wrap: wrap;
  }
  .hash {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    background: #f0f0f0;
    padding: 0.1rem 0.35rem;
    border-radius: 3px;
    font-size: 0.9em;
  }
  .diff {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.9em;
  }
  .agent {
    background: #e8eaf6;
    padding: 0.1rem 0.4rem;
    border-radius: 3px;
    font-size: 0.9em;
  }
  .conversation {
    margin: 0.6rem 0;
    padding: 0.6rem 0.8rem;
    border: 1px solid #e0e0e0;
    border-radius: 4px;
    background: #fafafa;
  }
  .conv-title {
    font-weight: 600;
    margin-bottom: 0.4rem;
  }
  .message {
    margin: 0.5rem 0;
    padding: 0.5rem 0.7rem;
    border: 1px solid #e8e8e8;
    border-radius: 4px;
    background: #fff;
    font-size: 0.95em;
    white-space: pre-wrap;
    word-break: break-word;
  }
  .commit-list {
    margin: 0.3rem 0;
    padding-left: 1.2rem;
  }
  .commit-list li {
    margin: 0.25rem 0;
  }
  .toolbar {
    background: #fff;
    border-bottom: 1px solid #ddd;
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
    border: 0.5px solid #888;
    border-radius: 3px;
    background: #ffffff;
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
  .toolbar button:disabled { background: #ffffff; opacity: 0.35; cursor: default; }
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
    min-width: 50px; text-align: center; color: #666; cursor: pointer;
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
    color: #888;
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
