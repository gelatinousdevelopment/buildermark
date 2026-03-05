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
	lines.push(`# Project Timeline: ${data.projectLabel}\n`);

	if (mode === 'commits-with-prompts') {
		const commits = sortCommits(data.commits, sortOrder);
		const parts: string[] = [];
		for (const commit of commits) {
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
): { headerHtml: string; sections: HTMLSection[] } {
	const headerHtml = `<h1>${escapeHtml(data.projectLabel)}</h1>`;

	const sections: HTMLSection[] = [];

	if (mode === 'commits-with-prompts') {
		const commits = sortCommits(data.commits, sortOrder);
		for (const commit of commits) {
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

	return { headerHtml, sections };
}

const htmlStyles = `
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
    border-bottom: 2px solid #e0e0e0;
    padding-bottom: 0.4rem;
    margin: 0 0 1.2rem 0;
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
    padding-bottom: 1.5rem;
    margin-bottom: 1.5rem;
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
    position: sticky; top: 0; background: #fff;
    border-bottom: 1px solid #ddd;
    padding: 0.4rem 0.8rem;
    margin: -2rem -2rem 1rem -2rem;
    display: flex; align-items: center; gap: 0.8rem;
    z-index: 10; font-size: 12px;
  }
  .toolbar button {
    font-size: 12px; padding: 0.2rem 0.5rem; cursor: pointer;
    border: 1px solid #ccc; border-radius: 3px; background: #f8f8f8;
  }
  .toolbar button:hover { background: #eee; }
  .toolbar button.active { background: #e0e7ff; border-color: #6366f1; color: #4338ca; }
  .toolbar button:disabled { opacity: 0.35; cursor: default; }
  .toolbar .sep { width: 1px; height: 18px; background: #ddd; }
  .toolbar .page-info { min-width: 80px; text-align: center; color: #666; }
  .goto-wrap { display: none; align-items: center; gap: 0.3rem; }
  .goto-wrap.visible { display: flex; }
  .goto-wrap input {
    width: 3.5em; font-size: 12px; padding: 0.15rem 0.3rem;
    border: 1px solid #ccc; border-radius: 3px;
  }
`;

const toolbarHtml = `<div class="toolbar">
  <button id="btn-all" class="active">All on one page</button>
  <button id="btn-paged">One per page</button>
  <div class="sep"></div>
  <button id="btn-prev10" disabled title="Back 10">&laquo;</button>
  <button id="btn-prev" disabled title="Previous">&lsaquo;</button>
  <span id="page-info" class="page-info"></span>
  <button id="btn-next" disabled title="Next">&rsaquo;</button>
  <button id="btn-next10" disabled title="Forward 10">&raquo;</button>
  <div class="sep"></div>
  <div id="goto-wrap" class="goto-wrap">
    <button id="btn-goto">Go to</button>
    <input id="goto-input" type="text" placeholder="#">
  </div>
</div>`;

/** Returns HTML + CSS suitable for injecting into a shadow root (toolbar without inline handlers). */
export function generateHTMLPreview(
	data: ExportData,
	mode: ExportMode,
	sortOrder: ExportSortOrder = 'newest'
): string {
	const { headerHtml, sections } = buildHTMLSections(data, mode, sortOrder);
	const bodyHtml = headerHtml + '\n' + sections.map((s) => s.html).join('\n');
	return `<style>${htmlStyles}</style>\n${toolbarHtml}\n${bodyHtml}`;
}

/** Returns a full standalone HTML document with embedded toolbar and pagination JS. */
export function generateHTML(
	data: ExportData,
	mode: ExportMode,
	sortOrder: ExportSortOrder = 'newest'
): string {
	const { headerHtml, sections } = buildHTMLSections(data, mode, sortOrder);
	const bodyHtml = headerHtml + '\n' + sections.map((s) => s.html).join('\n');
	const totalSections = sections.length;
	const topLevelLabel = mode === 'commits-with-prompts' ? 'Commits' : 'Conversations';

	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Export: ${escapeHtml(data.projectLabel)}</title>
<style>
  ${htmlStyles.replace(':host, body', 'body')}
</style>
</head>
<body>
${toolbarHtml}
${bodyHtml}
<script>
(function(){
  var sections = document.querySelectorAll('.top-section');
  var total = ${totalSections};
  var label = '${topLevelLabel}';
  var currentPage = 0;
  var vm = 'all';
  var btnAll = document.getElementById('btn-all');
  var btnPaged = document.getElementById('btn-paged');
  var btnPrev = document.getElementById('btn-prev');
  var btnNext = document.getElementById('btn-next');
  var btnPrev10 = document.getElementById('btn-prev10');
  var btnNext10 = document.getElementById('btn-next10');
  var pageInfo = document.getElementById('page-info');
  var gotoWrap = document.getElementById('goto-wrap');
  var btnGoto = document.getElementById('btn-goto');
  var gotoInput = document.getElementById('goto-input');

  function setMode(m) {
    vm = m;
    btnAll.className = m === 'all' ? 'active' : '';
    btnPaged.className = m === 'paged' ? 'active' : '';
    gotoWrap.className = m === 'paged' ? 'goto-wrap visible' : 'goto-wrap';
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
    pageInfo.textContent = paged ? (currentPage + 1) + ' / ' + total + ' ' + label : total + ' ' + label;
  }

  function go(delta) {
    if (vm !== 'paged') return;
    var p = currentPage + delta;
    if (p < 0) p = 0;
    if (p >= total) p = total - 1;
    currentPage = p;
    showPage();
  }

  function doGoto() {
    var val = parseInt(gotoInput.value, 10);
    if (isNaN(val) || val < 1) val = 1;
    if (val > total) val = total;
    currentPage = val - 1;
    gotoInput.value = '';
    showPage();
  }

  btnAll.addEventListener('click', function(){ setMode('all'); });
  btnPaged.addEventListener('click', function(){ setMode('paged'); });
  btnPrev.addEventListener('click', function(){ go(-1); });
  btnNext.addEventListener('click', function(){ go(1); });
  btnPrev10.addEventListener('click', function(){ go(-10); });
  btnNext10.addEventListener('click', function(){ go(10); });
  btnGoto.addEventListener('click', doGoto);
  gotoInput.addEventListener('keydown', function(e){ if(e.key==='Enter') doGoto(); });

  updateNav();
})();
</script>
</body>
</html>`;
}
