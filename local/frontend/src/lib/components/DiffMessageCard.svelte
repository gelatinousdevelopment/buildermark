<script lang="ts">
	import { html as diffToHtml } from 'diff2html';
	import type { ColorSchemeType } from 'diff2html';
	import 'diff2html/bundles/css/diff2html.min.css';
	import { fmtTime } from '$lib/utils';
	import { escapeHtml } from '$lib/messageUtils';
	import DiffCount from './DiffCount.svelte';
	import AgentTag from './AgentTag.svelte';

	interface Props {
		label?: string;
		/** Timestamp to display. Omit to hide. */
		timestamp?: number | string;
		/** Message role, e.g. "agent" or "user". */
		role?: string;
		model?: string;
		content: string;
		expanded?: boolean;
		statsLabel?: string | null;
		linkHref?: string | null;
		linkLabel?: string | null;
		/** When provided, shows agent percentage in the header. */
		agentPercent?: number;
		/** When provided, the header becomes clickable to collapse. */
		onToggle?: () => void;
		/** Render only the diff body and omit metadata/header elements. */
		contentOnly?: boolean;
		/** Render agent role tag in subtle mode. */
		subtleAgentTag?: boolean;
	}

	let {
		label = 'diff',
		timestamp,
		role = '',
		model = '',
		content,
		expanded = false,
		statsLabel = null,
		linkHref = null,
		linkLabel = null,
		agentPercent,
		onToggle,
		contentOnly = false,
		subtleAgentTag = false
	}: Props = $props();

	interface FileDiffStat {
		path: string;
		added: number;
		removed: number;
	}

	function extractDiffText(textInput: string): string {
		let text = textInput.trim();
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

	function perFileDiffStats(textInput: string): FileDiffStat[] {
		const diffText = extractDiffText(textInput);
		if (!diffText) return [];

		const lines = diffText.split('\n');
		const files: FileDiffStat[] = [];
		let current: FileDiffStat | null = null;

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

	function diffStats(textInput: string): { files: number; added: number; removed: number } {
		const diffText = extractDiffText(textInput);
		if (!diffText) return { files: 0, added: 0, removed: 0 };

		const lines = diffText.split('\n');
		let files = 0;
		let added = 0;
		let removed = 0;

		for (const line of lines) {
			if (line.startsWith('diff --git ')) {
				files++;
				continue;
			}
			if (line.startsWith('+++ ') || line.startsWith('--- ')) continue;
			if (line.startsWith('+')) {
				added++;
				continue;
			}
			if (line.startsWith('-')) removed++;
		}

		if (files === 0 && (added > 0 || removed > 0)) files = 1;
		return { files, added, removed };
	}

	function renderDiff(textInput: string): string {
		const diffText = extractDiffText(textInput);
		if (!diffText) return `<pre>${escapeHtml(textInput)}</pre>`;
		try {
			return diffToHtml(diffText, {
				drawFileList: false,
				matching: 'lines',
				outputFormat: 'line-by-line',
				colorScheme: 'auto' as ColorSchemeType
			});
		} catch {
			return `<pre>${escapeHtml(diffText)}</pre>`;
		}
	}

	let fileStats = $derived(perFileDiffStats(content));
	const nonAgentRoles = new Set(['user', 'system', 'tool', 'assistant']);
	let isAgentRole = $derived(role ? !nonAgentRoles.has(role.toLowerCase()) : false);

	/** Show file list when it adds info the header doesn't already convey. */
	let showFileList = $derived(
		fileStats.length > 1 || (fileStats.length === 1 && label !== fileStats[0].path)
	);
</script>

{#if !contentOnly}
	<!-- svelte-ignore a11y_no_noninteractive_tabindex -->
	<div
		class="message-header"
		class:message-header-clickable={onToggle}
		role={onToggle ? 'button' : undefined}
		tabindex={onToggle ? 0 : undefined}
		onclick={(e: MouseEvent) => {
			if (onToggle) {
				e.stopPropagation();
				onToggle();
			}
		}}
		onkeydown={(e: KeyboardEvent) => {
			if (onToggle && (e.key === 'Enter' || e.key === ' ')) {
				e.preventDefault();
				e.stopPropagation();
				onToggle();
			}
		}}
	>
		<strong>{label}</strong>
		{#if timestamp !== undefined}
			<span>&middot; {fmtTime(timestamp)}</span>
		{/if}
		{#if role || model}
			<span class="message-model">
				{#if role}
					{#if isAgentRole}
						<AgentTag agent={role} subtle={subtleAgentTag} />
					{:else}
						<span>{role}</span>
					{/if}
				{/if}
				{#if role && model}
					<span>&middot;</span>
				{/if}
				{#if model}
					<span>{model}</span>
				{/if}
			</span>
		{/if}
		{#if statsLabel}
			<span class="message-diff-stats">{statsLabel}</span>
		{:else}
			{@const stats = diffStats(content)}
			{#if stats.files > 1}
				<DiffCount
					added={stats.added}
					removed={stats.removed}
					files={stats.files}
					showFiles={true}
				/>
			{/if}
		{/if}
		{#if agentPercent !== undefined}
			<span class="agent-pct">{agentPercent.toFixed(1)}% agent</span>
		{/if}
	</div>
	{#if showFileList}
		<div class="file-stats-list">
			{#each fileStats as f, i (`${f.path}:${i}`)}
				<div class="file-stats-item">
					<span class="file-stats-path">{f.path}</span>
					<DiffCount added={f.added} removed={f.removed} />
				</div>
			{/each}
		</div>
	{/if}
	{#if linkHref && linkLabel}
		<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
		<div class="conversation-link"><a href={linkHref}>{linkLabel}</a></div>
	{/if}
{/if}
{#if contentOnly || expanded}
	<div class="message-content diff-body content-only">
		<!-- eslint-disable-next-line svelte/no-at-html-tags -->
		{@html renderDiff(content)}
	</div>
{/if}

<style>
	.message-header {
		font-size: 0.85rem;
		color: var(--color-text-secondary);
		margin-bottom: 0.25rem;
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.message-header-clickable {
		cursor: pointer;
		border-radius: 3px;
		padding: 0.15rem 0.3rem;
		border: 1px solid transparent;
		margin: calc(-0.1rem - 1px) calc(-0.3rem - 1px);
	}

	.message-header-clickable:hover {
		background: var(--accent-color-ultralight);
		color: var(--accent-color);
		border-color: var(--accent-color);
	}

	.message-model {
		color: var(--color-text-faded);
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
	}

	.message-diff-stats {
		color: var(--color-text-secondary);
		font-variant-numeric: tabular-nums;
	}

	.agent-pct {
		color: var(--color-text-secondary);
		font-variant-numeric: tabular-nums;
	}

	.file-stats-list {
		margin-top: 0.15rem;
	}

	.file-stats-item {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.82rem;
		padding: 0.05rem 0;
		justify-content: space-between;
		min-width: 0;
	}

	.file-stats-path {
		color: var(--color-text-strong);
		font-family: var(--font-family-monospace);
		flex: 1 1 auto;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.conversation-link {
		margin-top: 0.3rem;
		font-size: 0.85rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.message-content {
		background: var(--color-background-content);
		font-size: 0.85rem;
		margin-top: 0.35rem;
	}

	.message-content.content-only {
		margin-top: 0px;
	}

	.diff-body :global(*) {
		font-size: 1em;
		line-height: 1em;
	}

	.diff-body :global(.d2h-file-name) {
		font-family: var(--font-family-monospace);
	}

	.diff-body :global(.d2h-wrapper) {
		overflow-x: auto;
	}

	.diff-body :global(.d2h-code-linenumber) {
		border-left: 0;
	}
</style>
