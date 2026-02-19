<script lang="ts">
	import { resolve } from '$app/paths';
	import { listProjectCommitsPage, ingestMoreCommits, getCommitIngestionStatus } from '$lib/api';
	import { enqueueLoad } from '$lib/loadQueue';
	import type {
		ProjectCommitPageResponse,
		CommitIngestionStatusResponse,
		AgentCoverageSegment
	} from '$lib/types';
	import AgentPercentageBar from '$lib/components/AgentPercentageBar.svelte';

	function toBarSegments(segs?: AgentCoverageSegment[]): { name: string; percent: number }[] {
		if (!segs || segs.length === 0) return [];
		return segs.map((s) => ({ name: s.agent, percent: s.linePercent }));
	}

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
		showBranchPicker?: boolean;
		showCoverageBar?: boolean;
		showPagination?: boolean;
		showLoadMore?: boolean;
		showColumnNames?: boolean;
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
		showBranchPicker = false,
		showCoverageBar = false,
		showPagination = false,
		showLoadMore = false,
		showColumnNames = false,
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

	function commitDetailsHref(projectId: string, commitHash: string): string {
		const branchForLink = selectedBranch || data?.branch || '';
		if (branchForLink) {
			return resolve('/local/projects/[project_id]/commits/[branch]/[commit_hash]', {
				project_id: projectId,
				branch: branchForLink,
				commit_hash: commitHash
			});
		}
		return resolve('/local/projects/[project_id]/commits', { project_id: projectId });
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
			ingestionStatus = await withOptionalQueue(() =>
				getCommitIngestionStatus(projectId, branchValue)
			);
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
			const loaded = await withOptionalQueue(() =>
				listProjectCommitsPage(projectId, pageNum, selectedBranch)
			);
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
			return;
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
	<p class="message loading">Loading commits...</p>
{:else if error}
	<p class="message error">{error}</p>
{:else if !data || visibleCommits.length === 0}
	<p class="message">No commits found for this project and current git user.</p>
{:else}
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
			<AgentPercentageBar
				agentPercent={data.summary.linePercent}
				segments={toBarSegments(data.summary.agentSegments)}
				showManual={true}
			/>
		</div>
	{/if}

	<table class="data" class:compact>
		<colgroup>
			{#if !compact}
				<col class="time-col" />
			{/if}
			<col class="title-col" />
			{#if !compact}
				<col class="stats-col" />
				<col class="stats-col" />
			{/if}
			<col class="bar-col" />
		</colgroup>
		{#if showColumnNames}
			<thead>
				<tr>
					{#if !compact}
						<th class="time-col">Time</th>
					{/if}
					<th>Commit</th>
					{#if !compact}
						<th class="stats-col">Lines</th>
						<th class="stats-col">Chars</th>
					{/if}
					<th class="bar-col">Agent %</th>
				</tr>
			</thead>
		{/if}
		<tbody>
			{#each visibleCommits as c (c.commitHash)}
				<tr>
					{#if !compact}
						<td class="time">{formatTime(c.authoredAtUnixMs)}</td>
					{/if}
						<td class="title">
							<div>
								<a
									href={commitDetailsHref(c.projectId, c.commitHash)}
									class="link-button"
								>
									{c.subject || c.commitHash.slice(0, 8)}
							</a>
						</div>
						<!-- {#if !c.workingCopy}
							<div class="commit-meta">{c.commitHash.slice(0, 12)}</div>
						{/if} -->
					</td>
					{#if !compact}
						<td class="stats">{c.linesFromAgent} / {c.linesTotal} ({percent(c.linePercent)})</td>
						<td class="stats"
							>{c.charsFromAgent} / {c.charsTotal} ({percent(c.characterPercent)})</td
						>
					{/if}
					<td class="bar"
						><AgentPercentageBar
							agentPercent={c.linePercent}
							segments={toBarSegments(c.agentSegments)}
							showKey={false}
						/></td
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

	.link-button {
		display: block;
		max-width: 100%;
		background: none;
		border: 0;
		padding: 0;
		color: var(--link-color, #1f4cd1);
		cursor: pointer;
		font: inherit;
		text-decoration: none;
		text-align: left;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.link-button:hover {
		text-decoration: underline;
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
		margin: 0 1rem 1rem 1rem;
		max-width: 600px;
	}

	.message {
		padding-left: 1rem;
		padding-right: 1rem;
	}

	table.data {
		table-layout: fixed;
	}

	table.data tr {
		border-bottom: 0px;
	}

	table.data tr:hover {
		background: var(--accent-color-ultralight);
	}

	table.data td {
		white-space: nowrap;
	}

	.time-col {
		width: 180px;
	}

	.stats-col {
		width: 170px;
	}

	.bar-col {
		width: 140px;
	}

	.compact .bar-col {
		width: 120px;
	}

	.compact .stats-col {
		width: 150px;
	}

	.time {
		padding-left: 1rem;
	}

	.title {
		overflow: hidden;
	}

	.compact .title {
		padding-left: 1rem;
	}

	.title > div {
		overflow: hidden;
	}

	.title a {
		color: var(--color-text);
		display: block;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.title a:hover {
		color: var(--accent-color);
	}

	.bar {
		padding-right: 1rem;
	}

	.branch-picker {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin: 1rem;
	}
</style>
