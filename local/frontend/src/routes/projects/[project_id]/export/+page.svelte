<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import {
		getProject,
		getConversationsBatchDetail,
		listProjectCommitsPage,
		getCommitConversationLinks
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

	// Shadow DOM pagination state (not reactive - managed imperatively)
	let _viewMode: 'all' | 'paged' = 'all';
	let _currentPage = 0;
	let _totalPages = 0;
	let _topLevelLabel = '';

	onMount(() => {
		layoutStore.fixedHeight = true;
		layoutStore.hideContainer = true;
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
		const gotoWrap = shadowEl('goto-wrap');
		if (btnPrev) btnPrev.disabled = !paged || _currentPage <= 0;
		if (btnNext) btnNext.disabled = !paged || _currentPage >= _totalPages - 1;
		if (btnPrev10) btnPrev10.disabled = !paged || _currentPage <= 0;
		if (btnNext10) btnNext10.disabled = !paged || _currentPage >= _totalPages - 1;
		if (pageInfo)
			pageInfo.textContent = paged
				? `${_currentPage + 1} / ${_totalPages} ${_topLevelLabel}`
				: `${_totalPages} ${_topLevelLabel}`;
		if (gotoWrap) gotoWrap.className = paged ? 'goto-wrap visible' : 'goto-wrap';
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

	function gotoShadowPage() {
		const input = shadowEl<HTMLInputElement>('goto-input');
		if (!input) return;
		let val = parseInt(input.value, 10);
		if (isNaN(val) || val < 1) val = 1;
		if (val > _totalPages) val = _totalPages;
		_currentPage = val - 1;
		input.value = '';
		showShadowSections();
		previewHost?.scrollTo(0, 0);
	}

	function renderPreview() {
		if (previewHtml && previewHost) {
			if (!shadowRoot) {
				shadowRoot = previewHost.attachShadow({ mode: 'open' });
			}
			shadowRoot.innerHTML = previewHtml;
			const sections = shadowRoot.querySelectorAll('.top-section');
			_totalPages = sections.length;
			_topLevelLabel = mode === 'commits-with-prompts' ? 'Commits' : 'Conversations';
			_viewMode = 'all';
			_currentPage = 0;

			// Wire up toolbar event listeners
			shadowEl('btn-all')?.addEventListener('click', () => setShadowViewMode('all'));
			shadowEl('btn-paged')?.addEventListener('click', () => setShadowViewMode('paged'));
			shadowEl('btn-prev')?.addEventListener('click', () => goShadowPage(-1));
			shadowEl('btn-next')?.addEventListener('click', () => goShadowPage(1));
			shadowEl('btn-prev10')?.addEventListener('click', () => goShadowPage(-10));
			shadowEl('btn-next10')?.addEventListener('click', () => goShadowPage(10));
			shadowEl('btn-goto')?.addEventListener('click', () => gotoShadowPage());
			shadowEl('goto-input')?.addEventListener('keydown', (e) => {
				if ((e as KeyboardEvent).key === 'Enter') gotoShadowPage();
			});

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
			const needCommits = mode !== 'just-prompts';

			const [allConversations, allCommits] = await Promise.all([
				fetchAllConversations(range),
				needCommits ? fetchAllCommits(range) : Promise.resolve([] as ProjectCommitCoverage[])
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
				needCommits && (allCommits.length > 0 || convIds.length > 0)
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
				<div class="radio-group">
					<label class:selected={mode === 'commits-with-prompts'}>
						<input type="radio" name="mode" value="commits-with-prompts" bind:group={mode} />
						Commits > Conversations
					</label>
					<label class:selected={mode === 'prompts-with-commits'}>
						<input type="radio" name="mode" value="prompts-with-commits" bind:group={mode} />
						Conversations > Commits
					</label>
					<label class:selected={mode === 'just-prompts'}>
						<input type="radio" name="mode" value="just-prompts" bind:group={mode} />
						Conversations
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

		{#if output}
			<div class="output-actions">
				<button class="bordered small" onclick={copyToClipboard}>
					{copied ? 'Copied!' : 'Copy to clipboard'}
				</button>
				<button class="bordered small" onclick={download}>Download</button>
			</div>
		{/if}
	</div>
	<hr class="divider" />
	<div class="column right">
		{#if output}
			{#if outputFormat === 'html'}
				<div bind:this={previewHost} class="html-preview"></div>
			{:else}
				<pre class="preview">{output}</pre>
			{/if}
		{:else}
			<div class="empty">No export generated yet</div>
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
		width: 500px;
		padding: 1.5rem;
		display: flex;
		flex-direction: column;
		gap: 1.2rem;
	}

	.column.right {
		flex: 1;
		padding: 0;
		display: flex;
		flex-direction: column;
	}

	.content .divider {
		display: block;
		background: var(--color-divider);
		width: 0.5px;
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
		border: 0.5px solid var(--color-border-input);
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
		border-top: 0.5px solid var(--color-divider);
		display: flex;
		gap: 0.5rem;
		padding-top: 1rem;
	}

	.preview {
		background: var(--color-background-surface);
		font-family: var(--font-mono, monospace);
		font-size: 0.85rem;
		line-height: 1.5;
		margin: 0;
		overflow: auto;
		padding: 1rem;
		white-space: pre-wrap;
		word-break: break-word;
		flex: 1;
	}

	.html-preview {
		flex: 1;
		overflow-y: auto;
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
			height: 0.5px;
		}
	}
</style>
