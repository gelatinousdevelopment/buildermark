<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import {
		listProjectCommitsPage,
		ingestMoreCommits,
		getCommitIngestionStatus
	} from '$lib/api';
	import { enqueueLoad } from '$lib/loadQueue';
	import type { ProjectCommitPageResponse, CommitIngestionStatusResponse } from '$lib/types';
	import AgentPercentageBar from '$lib/components/AgentPercentageBar.svelte';

	type PageChangeHandler = (page: number) => void | Promise<void>;
	type BranchChangeHandler = (branch: string) => void | Promise<void>;

	interface Props {
		projectId: string;
		page?: number;
		branch?: string;
		limit?: number;
		compact?: boolean;
		showHeader?: boolean;
		header?: string;
		showSummary?: boolean;
		showBranchPicker?: boolean;
		showCoverageBar?: boolean;
		showPagination?: boolean;
		showLoadMore?: boolean;
		onPageChange?: PageChangeHandler;
		onBranchChange?: BranchChangeHandler;
		autoload?: boolean;
		useLoadQueue?: boolean;
		loadPriority?: number;
		loadSignal?: number;
	}

	let {
		projectId,
		page = undefined,
		branch = undefined,
		limit = 0,
		compact = false,
		showHeader = false,
		header = 'Git Commits',
		showSummary = false,
		showBranchPicker = false,
		showCoverageBar = false,
		showPagination = false,
		showLoadMore = false,
		onPageChange,
		onBranchChange,
		autoload = true,
		useLoadQueue = false,
		loadPriority = 0,
		loadSignal = 0
	}: Props = $props();

	let data: ProjectCommitPageResponse | null = $state(null);
	let ingestionStatus: CommitIngestionStatusResponse | null = $state(null);
	let loading = $state(false);
	let error: string | null = $state(null);
	let loadMoreCount = $state(20);
	let loadingMore = $state(false);
	let loadMoreError: string | null = $state(null);
	let internalPage = $state(1);
	let internalBranch = $state('');
	let initialized = $state(false);
	let requestToken = 0;
	let lastLoadKey = '';

	$effect(() => {
		if (initialized) return;
		initialized = true;
		internalPage = page ?? 1;
		internalBranch = branch ?? '';
		loading = autoload;
	});

	$effect(() => {
		if (page !== undefined) internalPage = page;
	});

	$effect(() => {
		if (branch !== undefined) internalBranch = branch;
	});

	const currentPage = $derived(page ?? internalPage);
	const selectedBranch = $derived(branch ?? internalBranch);
	const visibleCommits = $derived.by(() => {
		const all = data?.commits ?? [];
		if (limit > 0) return all.slice(0, limit);
		return all;
	});

	function percent(value: number): string {
		return `${value.toFixed(1)}%`;
	}

	function formatTime(unixMs: number): string {
		return new Date(unixMs).toLocaleString();
	}

	function withOptionalQueue<T>(task: () => Promise<T>): Promise<T> {
		if (useLoadQueue) return enqueueLoad(task, loadPriority);
		return task();
	}

	async function loadIngestionStatus(branchValue: string) {
		if (!showLoadMore || !projectId) {
			ingestionStatus = null;
			return;
		}
		try {
			ingestionStatus = await withOptionalQueue(() => getCommitIngestionStatus(projectId, branchValue));
		} catch {
			ingestionStatus = null;
		}
	}

	async function loadCommitsData() {
		if (!projectId) {
			error = 'Missing project ID';
			return;
		}
		const myToken = ++requestToken;
		loading = true;
		error = null;
		try {
			const pageNum = Math.max(1, currentPage);
			const loaded = await withOptionalQueue(() => listProjectCommitsPage(projectId, pageNum, selectedBranch));
			if (myToken !== requestToken) return;
			data = loaded;
			if (branch === undefined && !internalBranch && loaded.branch) {
				internalBranch = loaded.branch;
			}
			await loadIngestionStatus(branch ?? internalBranch);
		} catch (e) {
			if (myToken !== requestToken) return;
			error = e instanceof Error ? e.message : 'Failed to load commit coverage';
		} finally {
			if (myToken === requestToken) loading = false;
		}
	}

	$effect(() => {
		if (!autoload) return;
		const loadKey = `${projectId}:${currentPage}:${selectedBranch}:${loadSignal}`;
		if (loadKey === lastLoadKey) return;
		lastLoadKey = loadKey;
		void loadCommitsData();
	});

	async function goToPage(nextPage: number) {
		if (!data?.pagination) return;
		if (nextPage < 1 || nextPage > data.pagination.totalPages) return;
		if (page === undefined) {
			internalPage = nextPage;
		}
		if (onPageChange) {
			await onPageChange(nextPage);
		}
		if (!autoload) {
			void loadCommitsData();
		}
	}

	async function handleBranchChange(event: Event) {
		const value = (event.currentTarget as HTMLSelectElement).value;
		if (branch === undefined) {
			internalBranch = value;
		}
		if (onBranchChange) {
			await onBranchChange(value);
		}
		await goToPage(1);
	}

	async function handleLoadMore() {
		if (!projectId || loadingMore) return;
		loadingMore = true;
		loadMoreError = null;
		try {
			await withOptionalQueue(() => ingestMoreCommits(projectId, loadMoreCount, selectedBranch));
			await loadCommitsData();
		} catch (e) {
			loadMoreError = e instanceof Error ? e.message : 'Failed to load more commits';
		} finally {
			loadingMore = false;
		}
	}
</script>

{#if showHeader}
	<div class="heading">{header}</div>
{/if}

{#if loading}
	<p class="loading">Loading commits...</p>
{:else if error}
	<p class="error">{error}</p>
{:else if !data || visibleCommits.length === 0}
	<p>No commits found for this project and current git user.</p>
{:else}
	{#if showSummary}
		<section class="summary-grid">
			<div class="summary-card">
				<div class="summary-label">Current User</div>
				<div class="summary-value">{data.currentUser || 'Unknown'}</div>
				{#if data.currentEmail}
					<div class="summary-subtle">{data.currentEmail}</div>
				{/if}
			</div>
			<div class="summary-card">
				<div class="summary-label">Coverage (Lines)</div>
				<div class="summary-value">{percent(data.summary.linePercent)}</div>
				<div class="summary-subtle">{data.summary.linesFromAgent} / {data.summary.linesTotal}</div>
			</div>
			<div class="summary-card">
				<div class="summary-label">Coverage (Characters)</div>
				<div class="summary-value">{percent(data.summary.characterPercent)}</div>
				<div class="summary-subtle">{data.summary.charsFromAgent} / {data.summary.charsTotal}</div>
			</div>
		</section>
	{/if}

	{#if showBranchPicker}
		<div class="branch-picker">
			<label for="branch-{projectId}">Branch</label>
			<select id="branch-{projectId}" value={selectedBranch} onchange={handleBranchChange}>
				{#each data.branches as b (b)}
					<option value={b}>{b}</option>
				{/each}
			</select>
		</div>
	{/if}

	{#if showCoverageBar}
		<div class="summary-bar">
			<AgentPercentageBar agentPercent={data.summary.linePercent} showManual={true} />
		</div>
	{/if}

	<table class="data" class:compact>
		<thead>
			<tr>
				<th>Time</th>
				<th>Commit</th>
				<th>Lines</th>
				{#if !compact}
					<th>Chars</th>
				{/if}
				<th class="bar-col">Agent %</th>
			</tr>
		</thead>
		<tbody>
			{#each visibleCommits as c (c.commitHash)}
				<tr>
					<td>{formatTime(c.authoredAtUnixMs)}</td>
					<td>
						<div>
							<button
								type="button"
								class="link-button"
								onclick={() =>
									goto(
										resolve('/local/projects/[project_id]/commits/[commit_hash]', {
											project_id: c.projectId,
											commit_hash: c.commitHash
										})
									)
								}
							>
								{c.subject || c.commitHash.slice(0, 8)}
							</button>
						</div>
						{#if !c.workingCopy}
							<div class="commit-meta">{c.commitHash.slice(0, 12)}</div>
						{/if}
					</td>
					<td>{c.linesFromAgent} / {c.linesTotal} ({percent(c.linePercent)})</td>
					{#if !compact}
						<td>{c.charsFromAgent} / {c.charsTotal} ({percent(c.characterPercent)})</td>
					{/if}
					<td class="bar-col"
						><AgentPercentageBar agentPercent={c.linePercent} showKey={false} /></td
					>
				</tr>
			{/each}
		</tbody>
	</table>

	{#if showPagination && data.pagination.totalPages > 1}
		<div class="pager">
			<button class="btn-sm" disabled={currentPage <= 1} onclick={() => goToPage(currentPage - 1)}>
				Previous
			</button>
			<span>Page {data.pagination.page} of {data.pagination.totalPages}</span>
			<button
				class="btn-sm"
				disabled={currentPage >= data.pagination.totalPages}
				onclick={() => goToPage(currentPage + 1)}
			>
				Next
			</button>
		</div>
	{/if}

	{#if showLoadMore && ingestionStatus && !ingestionStatus.reachedRoot}
		<div class="load-more">
			<span class="load-more-info">
				{ingestionStatus.ingestedCount} of {ingestionStatus.totalGitCommits} commits loaded
			</span>
			<div class="load-more-controls">
				<label>
					Load
					<input
						type="number"
						min="1"
						max="500"
						bind:value={loadMoreCount}
						class="load-more-input"
						disabled={loadingMore}
					/>
					more
				</label>
				<button class="btn-sm" onclick={handleLoadMore} disabled={loadingMore}>
					{loadingMore ? 'Loading...' : 'Load'}
				</button>
			</div>
			{#if loadMoreError}
				<p class="error">{loadMoreError}</p>
			{/if}
		</div>
	{/if}
{/if}

<style>
	.heading {
		font-weight: 600;
		text-transform: uppercase;
		font-size: 0.9rem;
		opacity: 0.5;
		margin-bottom: 0.75rem;
	}

	.summary-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
		gap: 0.8rem;
		margin-bottom: 1rem;
	}

	.summary-card {
		border: 1px solid #e6e6e6;
		border-radius: 6px;
		padding: 0.8rem;
		background: #fbfbfb;
	}

	.summary-label {
		font-size: 0.78rem;
		text-transform: uppercase;
		letter-spacing: 0.03em;
		color: #777;
		margin-bottom: 0.35rem;
	}

	.summary-value {
		font-size: 1.3rem;
		font-weight: 600;
		color: #222;
	}

	.summary-subtle {
		margin-top: 0.2rem;
		font-size: 0.8rem;
		color: #777;
	}

	.link-button {
		background: none;
		border: 0;
		padding: 0;
		color: var(--link-color, #1f4cd1);
		cursor: pointer;
		font: inherit;
		text-decoration: underline;
	}

	.commit-meta {
		color: #777;
		font-size: 0.78rem;
	}

	.pager {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-top: 1rem;
	}

	.load-more {
		margin-top: 1.2rem;
		padding: 0.8rem;
		border: 1px solid #e6e6e6;
		border-radius: 6px;
		background: #fbfbfb;
	}

	.load-more-info {
		font-size: 0.85rem;
		color: #666;
		display: block;
		margin-bottom: 0.5rem;
	}

	.load-more-controls {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.load-more-controls label {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		font-size: 0.9rem;
	}

	.load-more-input {
		width: 5rem;
		padding: 0.25rem 0.4rem;
		border: 1px solid #ccc;
		border-radius: 4px;
		font-size: 0.9rem;
		text-align: center;
	}

	.summary-bar {
		margin-bottom: 1rem;
		max-width: 600px;
	}

	.bar-col {
		width: 120px;
		min-width: 80px;
	}

	.compact .bar-col {
		width: 90px;
	}

	.branch-picker {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-bottom: 0.75rem;
	}
</style>
