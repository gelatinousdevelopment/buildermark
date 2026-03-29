<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import {
		getProject,
		getConversationsBatchDetail,
		listProjectCommitsPage,
		getCommitConversationLinks,
		getLocalSettings
	} from '$lib/api';
	import {
		generateMarkdown,
		generateHTML,
		generateHTMLPreview,
		type ExportMode,
		type ExportFormat,
		type ExportSortOrder,
		type ExportData
	} from '$lib/exportGenerator';
	import type {
		ConversationWithRatings,
		ConversationBatchDetail,
		ProjectCommitCoverage
	} from '$lib/types';
	import { navStore } from '$lib/stores/nav.svelte';
	import { layoutStore } from '$lib/stores/layout.svelte';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import { referenceNowDate } from '$lib/utils';

	const PRESET_DAYS = [7, 14, 30, 90] as const;
	const DAY_IN_MS = 24 * 60 * 60 * 1000;

	const projectId = $derived(page.params.project_id ?? '');

	let mode: ExportMode = $state(settingsStore.exportMode);
	let format: ExportFormat = $state(settingsStore.exportFormat);
	let sortOrder: ExportSortOrder = $state(settingsStore.exportSortOrder);
	let presetDays: number | null = $state(settingsStore.exportPresetDays);
	let customStart = $state('');
	let customEnd = $state('');
	let loading = $state(false);
	let error: string | null = $state(null);
	let output = $state('');
	let outputFormat: ExportFormat = $state('markdown');
	let copied = $state(false);
	let previewHost: HTMLDivElement | undefined = $state();
	let shadowRoot: ShadowRoot | null = null;
	let previewHtml = '';
	let dbPath = $state('');
	let dbCopied = $state(false);

	// Shadow DOM pagination state (not reactive - managed imperatively)
	let _viewMode: 'all' | 'paged' = 'all';
	let _currentPage = 0;
	let _totalPages = 0;
	let _editing = false;

	onMount(async () => {
		layoutStore.fixedHeight = true;
		layoutStore.hideContainer = true;
		try {
			const settings = await getLocalSettings();
			dbPath = settings.dbPath;
		} catch {
			// settings fetch is best-effort
		}
	});

	onDestroy(() => {
		layoutStore.fixedHeight = false;
		layoutStore.hideContainer = false;
	});

	// Persist settings without triggering preview updates
	$effect(() => {
		settingsStore.exportMode = mode;
	});
	$effect(() => {
		settingsStore.exportFormat = format;
	});
	$effect(() => {
		settingsStore.exportPresetDays = presetDays;
	});
	$effect(() => {
		settingsStore.exportSortOrder = sortOrder;
	});

	function shadowEl<T extends HTMLElement>(id: string): T | null {
		return shadowRoot?.getElementById(id) as T | null;
	}

	function updateShadowNav() {
		if (!shadowRoot) return;
		const paged = _viewMode === 'paged';
		const btnPrev = shadowEl<HTMLButtonElement>('btn-prev');
		const btnNext = shadowEl<HTMLButtonElement>('btn-next');
		const btnPrev10 = shadowEl<HTMLButtonElement>('btn-prev10');
		const btnNext10 = shadowEl<HTMLButtonElement>('btn-next10');
		const pageInfo = shadowEl('page-info');
		if (btnPrev) btnPrev.disabled = !paged || _currentPage <= 0;
		if (btnNext) btnNext.disabled = !paged || _currentPage >= _totalPages - 1;
		if (btnPrev10) btnPrev10.disabled = !paged || _currentPage <= 0;
		if (btnNext10) btnNext10.disabled = !paged || _currentPage >= _totalPages - 1;
		if (pageInfo && !_editing) {
			pageInfo.textContent = paged ? `${_currentPage + 1} / ${_totalPages}` : `${_totalPages}`;
		}
	}

	function showShadowSections() {
		if (!shadowRoot) return;
		const sections = shadowRoot.querySelectorAll<HTMLElement>('.top-section');
		for (let i = 0; i < sections.length; i++) {
			sections[i].style.display = _viewMode === 'paged' && i !== _currentPage ? 'none' : '';
		}
		updateShadowNav();
	}

	function setShadowViewMode(m: 'all' | 'paged') {
		_viewMode = m;
		const btnAll = shadowEl('btn-all');
		const btnPaged = shadowEl('btn-paged');
		if (btnAll) btnAll.className = m === 'all' ? 'active' : '';
		if (btnPaged) btnPaged.className = m === 'paged' ? 'active' : '';
		if (m === 'paged') _currentPage = 0;
		showShadowSections();
	}

	function goShadowPage(delta: number) {
		if (_viewMode !== 'paged') return;
		let p = _currentPage + delta;
		if (p < 0) p = 0;
		if (p >= _totalPages) p = _totalPages - 1;
		_currentPage = p;
		showShadowSections();
		previewHost?.scrollTo(0, 0);
	}

	function startShadowEdit() {
		if (_viewMode !== 'paged' || _editing) return;
		_editing = true;
		const pageInfo = shadowEl('page-info');
		if (!pageInfo) return;
		const input = document.createElement('input');
		input.type = 'text';
		input.className = 'goto-input';
		input.value = String(_currentPage + 1);
		pageInfo.textContent = '';
		pageInfo.appendChild(input);
		input.focus();
		input.select();
		function commit() {
			if (!_editing) return;
			_editing = false;
			let val = parseInt(input.value, 10);
			if (!isNaN(val) && val >= 1 && val <= _totalPages) {
				_currentPage = val - 1;
				showShadowSections();
				previewHost?.scrollTo(0, 0);
			} else {
				updateShadowNav();
			}
			if (pageInfo && input.parentNode === pageInfo) pageInfo.removeChild(input);
		}
		input.addEventListener('keydown', (e) => {
			if (e.key === 'Enter') commit();
			if (e.key === 'Escape') {
				_editing = false;
				if (pageInfo && input.parentNode === pageInfo) pageInfo.removeChild(input);
				updateShadowNav();
			}
		});
		input.addEventListener('blur', commit);
	}

	function renderPreview() {
		if (previewHtml && previewHost) {
			if (!shadowRoot || shadowRoot.host !== previewHost) {
				shadowRoot = previewHost.attachShadow({ mode: 'open' });
			}
			shadowRoot.innerHTML = previewHtml;
			const sections = shadowRoot.querySelectorAll('.top-section');
			_totalPages = sections.length;
			_viewMode = 'all';
			_currentPage = 0;

			// Wire up toolbar event listeners
			shadowEl('btn-all')?.addEventListener('click', () => setShadowViewMode('all'));
			shadowEl('btn-paged')?.addEventListener('click', () => setShadowViewMode('paged'));
			shadowEl('btn-prev')?.addEventListener('click', () => goShadowPage(-1));
			shadowEl('btn-next')?.addEventListener('click', () => goShadowPage(1));
			shadowEl('btn-prev10')?.addEventListener('click', () => goShadowPage(-10));
			shadowEl('btn-next10')?.addEventListener('click', () => goShadowPage(10));
			shadowEl('page-info')?.addEventListener('click', () => startShadowEdit());

			setShadowViewMode('paged');
			updateShadowNav();
		}
	}

	function getDateRange(): { start: number; end: number } | null {
		if (presetDays !== null) {
			const now = referenceNowDate();
			const endDate = new Date(now.getFullYear(), now.getMonth(), now.getDate());
			const end = endDate.getTime() + DAY_IN_MS;
			const start = end - presetDays * DAY_IN_MS;
			return { start, end };
		}
		if (customStart && customEnd) {
			const s = new Date(customStart).getTime();
			const e = new Date(customEnd).getTime() + DAY_IN_MS;
			if (Number.isFinite(s) && Number.isFinite(e) && s < e) {
				return { start: s, end: e };
			}
		}
		return null;
	}

	function selectPreset(days: number) {
		presetDays = days;
		customStart = '';
		customEnd = '';
	}

	function selectAll() {
		presetDays = null;
		customStart = '';
		customEnd = '';
	}

	function onCustomStartChange(value: string) {
		customStart = value;
		presetDays = null;
	}

	function onCustomEndChange(value: string) {
		customEnd = value;
		presetDays = null;
	}

	async function fetchAllConversations(range: { start: number; end: number } | null) {
		const all: ConversationWithRatings[] = [];
		let pg = 1;
		const pageSize = 200;
		while (true) {
			const resp = await getProject(projectId, pg, pageSize, undefined, {
				start: range?.start,
				end: range?.end,
				order: 'desc'
			});
			all.push(...resp.conversations);
			if (pg >= resp.conversationPagination.totalPages) break;
			pg++;
		}
		return all;
	}

	async function fetchAllCommits(range: { start: number; end: number } | null) {
		const all: ProjectCommitCoverage[] = [];
		let pg = 1;
		while (true) {
			const resp = await listProjectCommitsPage(
				projectId,
				pg,
				'',
				200,
				'',
				'',
				'',
				range?.start,
				range?.end
			);
			all.push(...resp.commits);
			if (pg >= resp.pagination.totalPages) break;
			pg++;
		}
		return all;
	}

	async function generate() {
		if (!projectId) return;
		loading = true;
		error = null;
		output = '';
		previewHtml = '';
		copied = false;

		try {
			const range = getDateRange();
			const [allConversations, allCommits] = await Promise.all([
				fetchAllConversations(range),
				fetchAllCommits(range)
			]);

			const convIds = allConversations.map((c) => c.id);

			const batchDetailPromise = (async () => {
				const all: ConversationBatchDetail[] = [];
				for (let i = 0; i < convIds.length; i += 200) {
					const batch = convIds.slice(i, i + 200);
					const details = await getConversationsBatchDetail(batch);
					all.push(...details);
				}
				return all;
			})();

			const linksPromise =
				allCommits.length > 0 || convIds.length > 0
					? getCommitConversationLinks(
							projectId,
							allCommits.map((c) => c.commitHash),
							convIds
						)
					: Promise.resolve(null);

			const [allBatchDetails, links] = await Promise.all([batchDetailPromise, linksPromise]);

			const exportData: ExportData = {
				projectLabel: navStore.projectName || projectId,
				conversations: allConversations,
				batchDetails: allBatchDetails,
				commits: allCommits,
				links
			};

			outputFormat = format;
			if (format === 'markdown') {
				output = generateMarkdown(exportData, mode, sortOrder);
				previewHtml = '';
			} else {
				output = generateHTML(exportData, mode, sortOrder);
				previewHtml = generateHTMLPreview(exportData, mode, sortOrder);
			}

			// Render shadow DOM preview after state settles
			requestAnimationFrame(() => renderPreview());
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to generate export';
		} finally {
			loading = false;
		}
	}

	async function copyToClipboard() {
		await navigator.clipboard.writeText(output);
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}

	function download() {
		const ext = format === 'markdown' ? 'md' : 'html';
		const mimeType = format === 'markdown' ? 'text/markdown' : 'text/html';
		const blob = new Blob([output], { type: mimeType });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = `export-${navStore.projectName || 'project'}.${ext}`;
		a.click();
		URL.revokeObjectURL(url);
	}
</script>

<div class="content">
	<div class="column left">
		<div class="controls">
			<div class="control-group">
				<span class="control-label">Date Range</span>
				<div class="date-controls">
					<div class="preset-range">
						{#each PRESET_DAYS as days (days)}
							<button
								type="button"
								class:active={presetDays === days}
								onclick={() => selectPreset(days)}>{days}d</button
							>
						{/each}
						<button type="button" class:active={presetDays === null} onclick={selectAll}>All</button
						>
					</div>
					<div class="date-range">
						<input
							type="date"
							value={customStart}
							onchange={(e) => onCustomStartChange((e.currentTarget as HTMLInputElement).value)}
						/>
						<span>–</span>
						<input
							type="date"
							value={customEnd}
							onchange={(e) => onCustomEndChange((e.currentTarget as HTMLInputElement).value)}
						/>
					</div>
				</div>
			</div>

			<div class="control-group">
				<span class="control-label">Organization</span>
				<div class="radio-list">
					<label class:selected={mode === 'prompts-with-commits'}>
						<input type="radio" name="mode" value="prompts-with-commits" bind:group={mode} />
						Conversations
					</label>
					<label class:selected={mode === 'commits-with-prompts'}>
						<input type="radio" name="mode" value="commits-with-prompts" bind:group={mode} />
						Commits ❯ Conversations
					</label>
				</div>
			</div>

			<div class="control-group">
				<span class="control-label">Sort Order</span>
				<div class="radio-group">
					<label class:selected={sortOrder === 'newest'}>
						<input type="radio" name="sortOrder" value="newest" bind:group={sortOrder} />
						Newest first
					</label>
					<label class:selected={sortOrder === 'oldest'}>
						<input type="radio" name="sortOrder" value="oldest" bind:group={sortOrder} />
						Oldest first
					</label>
				</div>
			</div>

			<div class="control-group">
				<span class="control-label">Format</span>
				<div class="radio-group">
					<label class:selected={format === 'markdown'}>
						<input type="radio" name="format" value="markdown" bind:group={format} />
						Markdown
					</label>
					<label class:selected={format === 'html'}>
						<input type="radio" name="format" value="html" bind:group={format} />
						HTML
					</label>
				</div>
			</div>

			<div class="control-group">
				<button class="generate-btn" onclick={generate} disabled={loading}>
					{loading ? 'Generating...' : 'Generate'}
				</button>
			</div>
		</div>

		{#if error}
			<div class="error">{error}</div>
		{/if}

		{#if dbPath}
			<div class="db-note">
				<span class="control-label">Direct Database Access</span>
				<p>The raw data is stored in a SQLite database at:</p>
				<code class="db-path">{dbPath}</code>
				<p>
					Query the <code>conversations</code> and <code>messages</code> tables. For this project:
				</p>
				<div class="query-block">
					<code
						>{`sqlite3 "${dbPath}" "SELECT * FROM conversations WHERE project_id = '${projectId}' ORDER BY started_at DESC;"`}</code
					>
					<button
						class="bordered tiny copy-query"
						onclick={async () => {
							await navigator.clipboard.writeText(
								`sqlite3 "${dbPath}" "SELECT * FROM conversations WHERE project_id = '${projectId}' ORDER BY started_at DESC;"`
							);
							dbCopied = true;
							setTimeout(() => (dbCopied = false), 2000);
						}}
					>
						{dbCopied ? 'Copied!' : 'Copy Command'}
					</button>
				</div>
			</div>
		{/if}
	</div>
	<hr class="divider" />
	<div class="column right">
		{#if output}
			<div class="output-actions">
				<div class="inner">
					<button class="bordered small" onclick={copyToClipboard}>
						{copied ? 'Copied!' : 'Copy to Clipboard'}
					</button>
					<button class="bordered small" onclick={download}>Download File</button>
				</div>
			</div>
			<div class="preview-container">
				{#if outputFormat === 'html'}
					<div bind:this={previewHost} class="html-preview"></div>
				{:else}
					<pre class="markdown-preview">{output}</pre>
				{/if}
			</div>
		{:else}
			<div class="empty">Export Preview</div>
		{/if}
	</div>
</div>

<style>
	.content {
		display: flex;
		flex-direction: row;
		align-items: stretch;
		flex: 1;
		min-height: 100%;
		background: var(--color-background-content);
	}

	.column {
		box-sizing: border-box;
		overflow-y: auto;
		position: relative;
	}

	.column.left {
		width: 400px;
		padding: 1.5rem;
		display: flex;
		flex-direction: column;
		gap: 1.2rem;
	}

	.column.right {
		background: var(--color-background-empty);
		flex: 1;
		padding: 0;
		display: flex;
		flex-direction: column;
	}

	.content .divider {
		display: block;
		background: var(--color-divider);
		width: var(--divider-width);
		margin: 0;
		padding: 0;
		border: 0;
	}

	.column.right .empty {
		font-size: 1.3rem;
		justify-self: center;
		margin-top: 40vh;
		opacity: 0.4;
		text-align: center;
	}

	.controls {
		display: flex;
		flex-direction: column;
		gap: 1.2rem;
	}

	.control-group {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.control-label {
		font-size: 0.85rem;
		font-weight: 600;
		color: var(--color-text-secondary);
		text-transform: uppercase;
		letter-spacing: 0.03em;
	}

	.date-controls {
		display: flex;
		align-items: center;
		gap: 1rem;
		flex-wrap: wrap;
	}

	.preset-range {
		display: flex;
		gap: 0;
	}

	.preset-range button {
		font-size: 1rem;
		font-weight: 600;
		padding: 0.35rem 0.6rem;
		border: none;
		border-radius: 0;
		background: var(--color-background-surface);
		color: var(--color-text-secondary);
		cursor: pointer;
		line-height: 1.2;
	}

	.preset-range button:first-of-type {
		border-radius: 4px 0 0 4px;
	}

	.preset-range button:last-of-type {
		border-radius: 0 4px 4px 0;
	}

	.preset-range button:hover {
		background: var(--color-background-elevated);
		color: var(--color-text);
	}

	.preset-range button.active {
		background: var(--accent-color-ultralight);
		color: var(--accent-color);
	}

	.date-range {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.date-range input[type='date'] {
		font-size: 0.95rem;
		padding: 0.35rem 0.5rem;
	}

	.radio-group {
		display: flex;
		gap: 0;
		flex-wrap: wrap;
	}

	.radio-group label {
		cursor: pointer;
		font-size: 0.95rem;
		padding: 0.35rem 0.7rem;
		border: var(--divider-width) solid var(--color-border-input);
		background: var(--color-background-elevated);
	}

	.radio-group label:first-child {
		border-radius: 4px 0 0 4px;
	}

	.radio-group label:last-child {
		border-radius: 0 4px 4px 0;
		border-left: 0;
	}

	.radio-group label:not(:first-child):not(:last-child) {
		border-left: 0;
	}

	.radio-group label.selected {
		background: var(--accent-color-ultralight);
		color: var(--accent-color);
		border-color: var(--accent-color);
	}

	.radio-group label.selected + label {
		border-left: 0;
	}

	.radio-group input {
		display: none;
	}

	.radio-list {
		display: flex;
		flex-direction: column;
		gap: 0;
	}

	.radio-list label {
		cursor: pointer;
		font-size: 0.95rem;
		padding: 0.35rem 0.7rem;
		border: var(--divider-width) solid var(--color-border-input);
		background: var(--color-background-elevated);
	}

	.radio-list label:first-child {
		border-radius: 4px 4px 0 0;
	}

	.radio-list label:last-child {
		border-radius: 0 0 4px 4px;
		border-top: 0;
	}

	.radio-list label:not(:first-child):not(:last-child) {
		border-top: 0;
	}

	.radio-list label.selected {
		background: var(--accent-color-ultralight);
		color: var(--accent-color);
		border-color: var(--accent-color);
	}

	.radio-list label.selected + label {
		border-top: 0;
	}

	.radio-list input {
		display: none;
	}

	.generate-btn {
		align-self: flex-start;
		background: var(--accent-color);
		border: none;
		border-radius: 4px;
		color: white;
		cursor: pointer;
		font-size: 1rem;
		font-weight: 600;
		padding: 0.5rem 1.5rem;
	}

	.generate-btn:hover:not(:disabled) {
		opacity: 0.9;
	}

	.generate-btn:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.error {
		color: var(--color-danger, #b00020);
		font-size: 0.95rem;
	}

	.output-actions {
		display: flex;
		gap: 0.5rem;
		justify-content: center;
		width: 100%;
		position: sticky;
		top: 0.7rem;
		margin: 0.7rem 0;
	}

	.output-actions .inner {
		background: var(--color-background-empty);
		padding: 0.5rem;
		margin: -0.5rem;
		border-radius: 6px;
		min-width: fit-content;
		display: flex;
		gap: 0.5rem;
	}

	.preview-container {
		box-sizing: border-box;
		padding: 0 1rem 1rem 1rem;
		min-height: 100%;
	}

	.html-preview {
		background: var(--color-background-content);
		border: var(--divider-width) solid var(--color-divider);
		border-radius: 5px;
		box-shadow: 0 2px 8px rgba(0, 0, 0, 0.2);
		flex: 1;
		overflow-y: auto;
	}

	.markdown-preview {
		background: var(--color-background-content);
		border: var(--divider-width) solid var(--color-divider);
		border-radius: 5px;
		box-shadow: 0 2px 8px rgba(0, 0, 0, 0.2);
		font-family: var(--font-mono, monospace);
		font-size: 0.85rem;
		line-height: 1.5;
		margin: 0;
		overflow: auto;
		padding: 2rem;
		white-space: pre-wrap;
		word-break: break-word;
		flex: 1;
	}

	.db-note {
		border-top: var(--divider-width) solid var(--color-divider);
		padding-top: 1rem;
		margin-top: auto;
	}

	.db-note p {
		font-size: 0.85rem;
		color: var(--color-text-secondary);
		margin: 0.4rem 0;
	}

	.db-note p code {
		font-family: var(--font-mono, monospace);
		font-size: 0.8rem;
		background: var(--color-background-surface);
		padding: 0.1rem 0.3rem;
		border-radius: 3px;
	}

	.query-block {
		position: relative;
	}

	.db-note code.db-path,
	.query-block code {
		display: block;
		background: var(--color-background-empty);
		padding: 0.5rem;
		font-size: 0.8rem;
		border-radius: 4px;
		font-family: var(--font-mono, monospace);
		word-break: break-all;
		line-height: 1.4;
	}

	.query-block .copy-query {
		margin-top: 0.5rem;
		/*position: absolute;*/
		/*top: 0.3rem;*/
		/*right: 0.3rem;*/
	}

	@media (max-width: 768px) {
		.content {
			flex-direction: column;
		}

		.column.left {
			flex: none;
			max-width: 100%;
		}

		.column.right {
			min-height: 400px;
		}

		.content .divider {
			width: 100%;
			height: var(--divider-width);
		}
	}
</style>
