<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { getProjectCommitDetail } from '$lib/api';
	import { html as diffToHtml } from 'diff2html';
	import 'diff2html/bundles/css/diff2html.min.css';
	import { fmtTime } from '$lib/utils';
	import type { ProjectCommitDetailResponse } from '$lib/types';
	import DiffMessageCard from '$lib/components/DiffMessageCard.svelte';

	let detail: ProjectCommitDetailResponse | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);
	let expandedMessageIds: string[] = $state([]);
	let breadcrumbProjectId = $derived(page.params.project_id ?? '');
	let totalAdded = $derived.by(() => detail?.files.reduce((sum, f) => sum + f.added, 0) ?? 0);
	let totalRemoved = $derived.by(() => detail?.files.reduce((sum, f) => sum + f.removed, 0) ?? 0);
	let agentLinesTotal = $derived.by(
		() =>
			detail?.files
				.filter((f) => !f.ignored)
				.reduce((sum, f) => sum + f.added + f.removed, 0) ?? 0
	);
	let agentLinesFromAgent = $derived.by(
		() =>
			detail?.files
				.filter((f) => !f.ignored)
				.reduce((sum, f) => sum + (f.added + f.removed) * (f.linePercent / 100), 0) ?? 0
	);

	function isExpanded(id: string): boolean {
		return expandedMessageIds.includes(id);
	}

	function toggleExpanded(id: string) {
		if (expandedMessageIds.includes(id)) {
			expandedMessageIds = expandedMessageIds.filter((v) => v !== id);
			return;
		}
		expandedMessageIds = [...expandedMessageIds, id];
	}

	function escapeHtml(s: string): string {
		return s
			.replaceAll('&', '&amp;')
			.replaceAll('<', '&lt;')
			.replaceAll('>', '&gt;')
			.replaceAll('"', '&quot;')
			.replaceAll("'", '&#39;');
	}

	function renderCommitDiff(diffText: string): string {
		if (!diffText.trim()) return '<p>No diff content found.</p>';
		try {
			return diffToHtml(diffText, {
				drawFileList: false,
				matching: 'lines',
				outputFormat: 'line-by-line'
			});
		} catch {
			return `<pre>${escapeHtml(diffText)}</pre>`;
		}
	}

	function percent(part: number, total: number): number {
		if (total <= 0) return 0;
		return (part * 100) / total;
	}

	onMount(async () => {
		try {
			const projectId = page.params.project_id;
			const commitHash = page.params.commit_hash;
			if (!projectId || !commitHash) {
				throw new Error('Missing project or commit ID');
			}
			detail = await getProjectCommitDetail(projectId, commitHash);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load commit detail';
		} finally {
			loading = false;
		}
	});
</script>

<div class="breadcrumb">
	<a href={resolve('/dashboard')}>Dashboard</a> &rsaquo;
	<a href={resolve('/dashboard/commits')}>Commits</a> &rsaquo;
	<a href={resolve('/dashboard/projects/[project_id]/commits', { project_id: breadcrumbProjectId })}
		>Project</a
	>
	&rsaquo; Commit
</div>

{#if loading}
	<p class="loading">Loading commit...</p>
{:else if error}
	<p class="error">{error}</p>
{:else if detail}
	<h2>{detail.commit.subject || detail.commit.commitHash.slice(0, 8)}</h2>
	<p>
		{fmtTime(detail.commit.authoredAtUnixMs)} | {detail.commit.projectLabel ||
			detail.commit.projectPath} |
		{detail.commit.commitHash.slice(0, 12)}
	</p>
	<p>
		Agent attribution: {Math.round(agentLinesFromAgent)}/{agentLinesTotal} changed lines
		({percent(agentLinesFromAgent, agentLinesTotal).toFixed(1)}%) in non-ignored files
	</p>
	<p>Changes: <span class="plus">+{totalAdded}</span><span class="minus">-{totalRemoved}</span></p>

	<h3>{detail.commit.workingCopy ? 'Working Copy Diff' : 'Commit Diff'}</h3>
	{#if detail.files.length === 0}
		<p>No changed files in this diff.</p>
	{:else}
		<div class="file-table-wrap">
			<table class="file-table">
				<thead>
					<tr>
						<th>File</th>
						<th>Changes</th>
						<th class="pct-col">Agent</th>
					</tr>
				</thead>
				<tbody>
					{#each detail.files as file (file.path)}
						<tr class:ignored-row={file.ignored}>
							<td>{file.path}</td>
							<td class="changes-col">
								<span class="plus">+{file.added}</span>
								<span class="minus">-{file.removed}</span>
							</td>
							<td class="pct-col">{file.ignored ? '' : `${file.linePercent.toFixed(1)}%`}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
	<div class="commit-diff diff-body markdown-body">
		<!-- eslint-disable-next-line svelte/no-at-html-tags -->
		{@html renderCommitDiff(detail.diff)}
	</div>

	<h3>Matched Messages</h3>
	{#if detail.messages.length === 0}
		<p>No tracked diff messages matched this commit.</p>
	{:else}
		{#each detail.messages as message (message.id)}
			<DiffMessageCard
				timestamp={message.timestamp}
				model={message.model ?? ''}
				content={message.content}
				expanded={isExpanded(message.id)}
				onToggle={() => toggleExpanded(message.id)}
				statsLabel={`matched ${message.linesMatched} lines, ${message.charsMatched} chars`}
				linkHref={resolve('/dashboard/conversations/[id]', { id: message.conversationId })}
				linkLabel={`Conversation: ${message.conversationTitle || message.conversationId}`}
			/>
		{/each}
	{/if}
{/if}

<style>
	.file-table-wrap {
		overflow-x: auto;
		margin-bottom: 0.75rem;
	}

	.file-table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.9rem;
	}

	.file-table th,
	.file-table td {
		padding: 0.4rem 0.5rem;
		border-bottom: 1px solid #ececec;
		text-align: left;
	}

	.file-table th {
		color: #666;
		font-size: 0.82rem;
		font-weight: 600;
	}

	.pct-col {
		text-align: right;
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	.changes-col {
		font-variant-numeric: tabular-nums;
		white-space: nowrap;
	}

	.plus {
		color: #1a7f37;
		margin-right: 0.65rem;
	}

	.minus {
		color: #cf222e;
	}

	.ignored-row {
		color: #9a9a9a;
		background: #f9f9f9;
	}

	.commit-diff {
		margin-bottom: 1rem;
	}

	.diff-body :global(.d2h-wrapper) {
		overflow-x: auto;
	}

	.markdown-body :global(pre) {
		overflow-x: auto;
		padding: 0.5rem;
		background: #f7f7f7;
		border-radius: 4px;
	}

	.markdown-body :global(code) {
		font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
	}
</style>
