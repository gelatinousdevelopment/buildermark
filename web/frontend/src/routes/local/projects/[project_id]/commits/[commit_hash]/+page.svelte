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
	let totalAdded = $derived.by(() => detail?.files.reduce((sum, f) => sum + f.added, 0) ?? 0);
	let totalRemoved = $derived.by(() => detail?.files.reduce((sum, f) => sum + f.removed, 0) ?? 0);
	type DiffSection = {
		path: string;
		diffText: string;
	};
	let diffSections = $derived.by(() => parseDiffSections(detail?.diff ?? ''));
	let diffSectionByPath = $derived.by(
		() => new Map(diffSections.map((section) => [section.path, section.diffText]))
	);
	let renderableDiffFiles = $derived.by(
		() => detail?.files.filter((file) => !file.ignored && diffSectionByPath.has(file.path)) ?? []
	);
	let agentLinesTotal = $derived.by(
		() =>
			detail?.files
				.filter((f) => !f.ignored && !f.moved)
				.reduce((sum, f) => sum + f.added + f.removed, 0) ?? 0
	);
	let agentLinesFromAgent = $derived.by(
		() =>
			detail?.files
				.filter((f) => !f.ignored && !f.moved)
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

	function parseDiffHeader(line: string): { oldPath: string; newPath: string } | null {
		const match = line.match(/^diff --git (.+) (.+)$/);
		if (!match) return null;
		return {
			oldPath: normalizeGitPath(match[1]),
			newPath: normalizeGitPath(match[2])
		};
	}

	function parseDiffSections(diffText: string): DiffSection[] {
		if (!diffText.trim()) return [];
		const lines = diffText.split('\n');
		const sections: DiffSection[] = [];
		let currentPath = '';
		let currentLines: string[] = [];

		const pushCurrent = () => {
			if (!currentPath || currentLines.length === 0) return;
			sections.push({
				path: currentPath,
				diffText: currentLines.join('\n')
			});
		};

		for (const line of lines) {
			if (line.startsWith('diff --git ')) {
				pushCurrent();
				currentLines = [line];
				const parsed = parseDiffHeader(line);
				if (!parsed) {
					currentPath = '';
					continue;
				}
				currentPath = parsed.newPath === '/dev/null' ? parsed.oldPath : parsed.newPath;
				continue;
			}
			if (currentLines.length > 0) currentLines.push(line);
		}
		pushCurrent();
		return sections;
	}

	function diffAnchor(path: string): string {
		return `diff-${encodeURIComponent(path)}`;
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

{#if loading}
	<p class="loading">Loading commit...</p>
{:else if error}
	<p class="error">{error}</p>
{:else if detail}
	<h2>{detail.commit.subject || detail.commit.commitHash.slice(0, 8)}</h2>
	<p>{fmtTime(detail.commit.authoredAtUnixMs)} | {detail.commit.commitHash.slice(0, 12)}</p>
	<p>
		Agent attribution: {Math.round(agentLinesFromAgent)}/{agentLinesTotal} changed lines ({percent(
			agentLinesFromAgent,
			agentLinesTotal
		).toFixed(1)}%) in non-ignored, non-moved files
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
						<tr class:ignored-row={file.ignored || file.moved}>
							<td>
								{#if !file.ignored && diffSectionByPath.has(file.path)}
									<a href={`#${diffAnchor(file.path)}`}>{file.path}</a>
								{:else}
									{file.path}
								{/if}
								{#if file.moved}
									<span class="file-tag">[moved]</span>
								{:else if file.copiedFromAgent}
									<span class="file-tag">[copied-from-agent]</span>
								{/if}
							</td>
							<td class="changes-col">
								{#if !file.ignored}
									<span class="plus">+{file.added}</span>
									<span class="minus">-{file.removed}</span>
								{/if}
							</td>
							<td class="pct-col"
								>{file.ignored || file.moved ? '' : `${file.linePercent.toFixed(1)}%`}</td
							>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
	{#if renderableDiffFiles.length === 0}
		<p>No non-ignored file diffs to display.</p>
	{:else}
		{#each renderableDiffFiles as file (file.path)}
			<div class="commit-diff-section" id={diffAnchor(file.path)}>
				<h4>{file.path}</h4>
				<div class="commit-diff diff-body markdown-body">
					<!-- eslint-disable-next-line svelte/no-at-html-tags -->
					{@html renderCommitDiff(diffSectionByPath.get(file.path) ?? '')}
				</div>
			</div>
		{/each}
	{/if}

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
				linkHref={resolve('/local/projects/[project_id]/conversations/[id]', {
					project_id: detail.commit.projectId,
					id: message.conversationId
				})}
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

	.file-tag {
		margin-left: 0.4rem;
		font-size: 0.8rem;
		color: #8a8a8a;
	}

	.commit-diff {
		margin-bottom: 1rem;
	}

	.commit-diff-section h4 {
		margin: 0.75rem 0 0.5rem;
		font-size: 0.95rem;
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
