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
		type ExportMode,
		type ExportFormat,
		type ExportData
	} from '$lib/exportGenerator';
	import type {
		ConversationWithRatings,
		ConversationBatchDetail,
		ProjectCommitCoverage,
		CommitConversationLinks
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
	let presetDays: number | null = $state(settingsStore.exportPresetDays);
	let customStart = $state('');
	let customEnd = $state('');
	let loading = $state(false);
	let error: string | null = $state(null);
	let output = $state('');
	let outputFormat: ExportFormat = $state('markdown');
	let copied = $state(false);
	let previewIframe: HTMLIFrameElement | undefined = $state();

	onMount(() => {
		layoutStore.fixedHeight = true;
		layoutStore.hideContainer = true;
	});

	onDestroy(() => {
		layoutStore.fixedHeight = false;
		layoutStore.hideContainer = false;
	});

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
		if (outputFormat === 'html' && output && previewIframe) {
			previewIframe.srcdoc = output;
		}
	});

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

	async function generate() {
		if (!projectId) return;
		loading = true;
		error = null;
		output = '';
		copied = false;

		try {
			const range = getDateRange();

			const allConversations: ConversationWithRatings[] = [];
			let pg = 1;
			const pageSize = 200;
			while (true) {
				const resp = await getProject(projectId, pg, pageSize, undefined, {
					start: range?.start,
					end: range?.end,
					order: 'desc'
				});
				allConversations.push(...resp.conversations);
				if (pg >= resp.conversationPagination.totalPages) break;
				pg++;
			}

			const allBatchDetails: ConversationBatchDetail[] = [];
			const convIds = allConversations.map((c) => c.id);
			for (let i = 0; i < convIds.length; i += 200) {
				const batch = convIds.slice(i, i + 200);
				const details = await getConversationsBatchDetail(batch);
				allBatchDetails.push(...details);
			}

			let allCommits: ProjectCommitCoverage[] = [];
			let links: CommitConversationLinks | null = null;

			if (mode !== 'just-prompts') {
				pg = 1;
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
					allCommits.push(...resp.commits);
					if (pg >= resp.pagination.totalPages) break;
					pg++;
				}

				if (allCommits.length > 0 || convIds.length > 0) {
					const commitHashes = allCommits.map((c) => c.commitHash);
					links = await getCommitConversationLinks(projectId, commitHashes, convIds);
				}
			}

			const exportData: ExportData = {
				projectLabel: navStore.projectName || projectId,
				conversations: allConversations,
				batchDetails: allBatchDetails,
				commits: allCommits,
				links
			};

			outputFormat = format;
			if (format === 'markdown') {
				output = generateMarkdown(exportData, mode);
			} else {
				output = generateHTML(exportData, mode);
			}
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
						Commits ❯ Conversations
					</label>
					<label class:selected={mode === 'prompts-with-commits'}>
						<input type="radio" name="mode" value="prompts-with-commits" bind:group={mode} />
						Conversations ❯ Commits
					</label>
					<label class:selected={mode === 'just-prompts'}>
						<input type="radio" name="mode" value="just-prompts" bind:group={mode} />
						Conversations
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
				<iframe
					bind:this={previewIframe}
					class="html-preview"
					title="Export preview"
					sandbox="allow-same-origin"
				></iframe>
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
		border: none;
		flex: 1;
		width: 100%;
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
