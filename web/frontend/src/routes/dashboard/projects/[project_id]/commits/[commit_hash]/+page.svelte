<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { getProjectCommitDetail } from '$lib/api';
	import { fmtTime } from '$lib/utils';
	import type { ProjectCommitDetailResponse, ProjectCommitContributionMessage } from '$lib/types';

	let detail: ProjectCommitDetailResponse | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);
	let expandedMessageIds: string[] = $state([]);
	let breadcrumbProjectId = $derived(page.params.project_id ?? '');

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

	function extractDiffText(content: string): string {
		let text = content.trim();
		if (text.startsWith('```diff')) {
			text = text.slice('```diff'.length).trimStart();
			if (text.endsWith('```')) text = text.slice(0, -3).trimEnd();
		}

		const gitIdx = text.indexOf('diff --git ');
		if (gitIdx >= 0) return text.slice(gitIdx).trim();
		return text;
	}

	function messageSummary(message: ProjectCommitContributionMessage): string {
		const diffText = extractDiffText(message.content);
		const first = diffText.split('\n').find((line) => line.startsWith('diff --git '));
		if (first) return first;
		const oneLine = diffText.replace(/\s+/g, ' ').trim();
		if (!oneLine) return '[diff]';
		return oneLine.length > 120 ? `${oneLine.slice(0, 117)}...` : oneLine;
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
		Coverage: lines {detail.commit.linesFromAgent}/{detail.commit.linesTotal} ({detail.commit.linePercent.toFixed(
			1
		)}%) | chars {detail.commit.charsFromAgent}/{detail.commit.charsTotal} ({detail.commit.characterPercent.toFixed(
			1
		)}%)
	</p>

	{#if detail.messages.length === 0}
		<p>No tracked diff messages matched this commit.</p>
	{:else}
		{#each detail.messages as message (message.id)}
			<div class="message message-collapsed">
				<button class="message-summary-btn" onclick={() => toggleExpanded(message.id)}>
					<div class="message-header">
						<strong>diff</strong> &middot; {fmtTime(message.timestamp)}
						{#if message.model}
							<span class="message-model">{message.model}</span>
						{/if}
						<span class="message-diff-stats"
							>matched {message.linesMatched} lines, {message.charsMatched} chars</span
						>
						<span class="expansion-indicator">
							<span class="chevron">{isExpanded(message.id) ? '▾' : '▸'}</span>
						</span>
					</div>
					<div class="message-summary">{messageSummary(message)}</div>
					<div class="conversation-link">
						<a href={resolve('/dashboard/conversations/[id]', { id: message.conversationId })}
							>Conversation: {message.conversationTitle || message.conversationId}</a
						>
					</div>
				</button>
				{#if isExpanded(message.id)}
					<pre class="diff-content">{extractDiffText(message.content)}</pre>
				{/if}
			</div>
		{/each}
	{/if}
{/if}

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

	.diff-content {
		margin-top: 0.5rem;
		overflow-x: auto;
		padding: 0.5rem;
		background: #f7f7f7;
		border-radius: 4px;
		font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
		font-size: 0.85rem;
	}
</style>
