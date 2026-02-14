<script lang="ts">
	import { html as diffToHtml } from 'diff2html';
	import 'diff2html/bundles/css/diff2html.min.css';
	import { fmtTime } from '$lib/utils';

	export let label = 'diff';
	export let timestamp: number | string;
	export let model = '';
	export let content: string;
	export let expanded = false;
	export let statsLabel: string | null = null;
	export let linkHref: string | null = null;
	export let linkLabel: string | null = null;
	export let onToggle: (() => void) | null = null;

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

	function diffStatsLabel(textInput: string): string {
		const diffText = extractDiffText(textInput);
		if (!diffText) return '0 files, +0 -0';

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
		const fileLabel = files === 1 ? 'file' : 'files';
		return `${files} ${fileLabel}, +${added} -${removed}`;
	}

	function summary(textInput: string): string {
		const diffText = extractDiffText(textInput);
		const first = diffText.split('\n').find((line) => line.startsWith('diff --git '));
		if (first) return first;
		const oneLine = diffText.replace(/\s+/g, ' ').trim();
		if (!oneLine) return '[diff]';
		return oneLine.length > 120 ? `${oneLine.slice(0, 117)}...` : oneLine;
	}

	function escapeHtml(s: string): string {
		return s
			.replaceAll('&', '&amp;')
			.replaceAll('<', '&lt;')
			.replaceAll('>', '&gt;')
			.replaceAll('"', '&quot;')
			.replaceAll("'", '&#39;');
	}

	function renderDiff(textInput: string): string {
		const diffText = extractDiffText(textInput);
		if (!diffText) return `<pre>${escapeHtml(textInput)}</pre>`;
		try {
			return diffToHtml(diffText, {
				drawFileList: true,
				matching: 'lines',
				outputFormat: 'line-by-line'
			});
		} catch {
			return `<pre>${escapeHtml(diffText)}</pre>`;
		}
	}

	function handleToggle() {
		onToggle?.();
	}
</script>

<div class="message message-collapsed">
	<button class="message-summary-btn" onclick={handleToggle}>
		<div class="message-header">
			<strong>{label}</strong> &middot; {fmtTime(timestamp)}
			{#if model}
				<span class="message-model">{model}</span>
			{/if}
			<span class="message-diff-stats">{statsLabel ?? diffStatsLabel(content)}</span>
			<span class="expansion-indicator">
				<span class="chevron">{expanded ? '▾' : '▸'}</span>
			</span>
		</div>
		<div class="message-summary">{summary(content)}</div>
		{#if linkHref && linkLabel}
			<div class="conversation-link"><a href={linkHref}>{linkLabel}</a></div>
		{/if}
	</button>
	{#if expanded}
		<div class="message-content diff-body">
			<!-- eslint-disable-next-line svelte/no-at-html-tags -->
			{@html renderDiff(content)}
		</div>
	{/if}
</div>

<style>
	.message {
		margin-bottom: 1rem;
		padding: 0.75rem;
		border: 1px solid #eee;
		border-radius: 4px;
	}

	.message-collapsed {
		padding: 0.5rem 0.75rem;
		background: #fafafa;
	}

	.message-header {
		font-size: 0.85rem;
		color: #666;
		margin-bottom: 0.25rem;
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.message-summary-btn {
		display: block;
		width: 100%;
		text-align: left;
		background: transparent;
		border: 0;
		padding: 0;
		cursor: pointer;
	}

	.message-summary {
		font-size: 0.9rem;
		color: #333;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.message-model {
		color: #9a9a9a;
	}

	.message-diff-stats {
		color: #5f6b7a;
		font-variant-numeric: tabular-nums;
	}

	.expansion-indicator {
		margin-left: auto;
		color: #888;
	}

	.chevron {
		display: inline-block;
		width: 0.8rem;
	}

	.conversation-link {
		margin-top: 0.3rem;
		font-size: 0.85rem;
	}

	.message-content {
		font-size: 0.9rem;
		margin-top: 0.35rem;
	}

	.diff-body :global(.d2h-wrapper) {
		overflow-x: auto;
	}
</style>
