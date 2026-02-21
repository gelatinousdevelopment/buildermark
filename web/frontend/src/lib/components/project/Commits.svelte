<script lang="ts">
	/* eslint-disable svelte/no-navigation-without-resolve */
	import { browser } from '$app/environment';
	import { resolve } from '$app/paths';
	import { listProjectCommitsPage, ingestMoreCommits, getCommitIngestionStatus } from '$lib/api';
	import { enqueueLoad } from '$lib/loadQueue';
	import type {
		ProjectCommitPageResponse,
		CommitIngestionStatusResponse,
		AgentCoverageSegment
	} from '$lib/types';
	import AgentPercentageBar from '$lib/components/AgentPercentageBar.svelte';
	import DiffCount from '$lib/components/DiffCount.svelte';
	import DailyCommitsChart from '$lib/charts/DailyCommitsChart.svelte';
	import Icon from '$lib/Icon.svelte';
	import { formatRelativeOrShortDate, formatFullDateTitle, commitUrl } from '$lib/utils';

	function toBarSegments(segs?: AgentCoverageSegment[]): { name: string; percent: number }[] {
		if (!segs || segs.length === 0) return [];
		return segs.map((s) => ({ name: s.agent, percent: s.linePercent }));
	}

	type PageChangeHandler = (page: number) => void | Promise<void>;
	type BranchChangeHandler = (branch: string) => void | Promise<void>;

	interface Props {
		projectId: string;
		page?: number;
		pageSize?: number;
		branch?: string;
		limit?: number;
		compact?: boolean;
		showHeader?: boolean;
		header?: string;
		showBranchPicker?: boolean;
		showUserPicker?: boolean;
		showCoverageBar?: boolean;
		showPagination?: boolean;
		showLoadMore?: boolean;
		showDate?: boolean;
		showBranch?: boolean;
		showDiffCount?: boolean;
		showColumnNames?: boolean;
		syncPaginationWithUrl?: boolean;
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
		pageSize = 10,
		branch = undefined,
		limit = 0,
		compact = false,
		showHeader = false,
		header = 'Git Commits',
		showBranchPicker = false,
		showUserPicker = false,
		showCoverageBar = false,
		showPagination = false,
		showLoadMore = false,
		showDate = false,
		showBranch = false,
		showDiffCount = true,
		showColumnNames = false,
		syncPaginationWithUrl = false,
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
	let internalUser = $state('');
	let initialized = $state(false);
	let requestToken = 0;
	let lastLoadKey = '';

	function parsePositivePage(value: string | null | undefined): number {
		if (!value) return 1;
		const parsed = Number.parseInt(value, 10);
		return Number.isInteger(parsed) && parsed > 0 ? parsed : 1;
	}

	$effect(() => {
		if (initialized) return;
		initialized = true;
		if (syncPaginationWithUrl && browser) {
			const params = new URLSearchParams(window.location.search);
			internalPage = page ?? parsePositivePage(params.get('page'));
			internalUser = params.get('user') ?? '';
		} else {
			internalPage = page ?? 1;
		}
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
	const selectedUser = $derived(internalUser);
	const visibleCommits = $derived.by(() => {
		const all = data?.commits ?? [];
		if (limit > 0) return all.slice(0, limit);
		return all;
	});

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
				listProjectCommitsPage(projectId, pageNum, selectedBranch, pageSize, selectedUser)
			);
			if (myToken !== requestToken) return;
			data = loaded;
			if (branch === undefined && !internalBranch && loaded.branch) {
				internalBranch = loaded.branch;
			}
			void loadIngestionStatus(branch ?? internalBranch);
		} catch (e) {
			if (myToken !== requestToken) return;
			error = e instanceof Error ? e.message : 'Failed to load commit coverage';
		} finally {
			if (myToken === requestToken) loading = false;
		}
	}

	$effect(() => {
		if (!autoload) return;
		const loadKey = `${projectId}:${currentPage}:${pageSize}:${selectedBranch}:${selectedUser}:${loadSignal}`;
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
		if (syncPaginationWithUrl && browser && !onPageChange) {
			const url = new URL(window.location.href);
			if (nextPage <= 1) {
				url.searchParams.delete('page');
			} else {
				url.searchParams.set('page', String(nextPage));
			}
			window.history.replaceState(window.history.state, '', url);
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
		// Reset author filter when branch changes (author may not exist on new branch).
		internalUser = '';
		if (syncPaginationWithUrl && browser) {
			const url = new URL(window.location.href);
			url.searchParams.delete('user');
			window.history.replaceState(window.history.state, '', url);
		}
		if (onBranchChange) {
			await onBranchChange(value);
			return;
		}
		await goToPage(1);
	}

	function handleUserChange(event: Event) {
		const value = (event.currentTarget as HTMLSelectElement).value;
		internalUser = value;
		if (syncPaginationWithUrl && browser) {
			const url = new URL(window.location.href);
			if (value) {
				url.searchParams.set('user', value);
			} else {
				url.searchParams.delete('user');
			}
			url.searchParams.delete('page');
			window.history.replaceState(window.history.state, '', url);
		}
		internalPage = 1;
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

	function commitLink(hash: string): string {
		return commitUrl(data?.project.remote ?? '', hash);
	}
</script>

{#if showHeader}
	<div class="heading">{header}{#if selectedBranch || data?.branch} <span class="heading-branch">({selectedBranch || data?.branch})</span>{/if}</div>
{/if}

{#if loading && !data}
	<p class="message loading">Loading commits...</p>
{:else if error}
	<p class="message error">{error}</p>
{:else if !data || visibleCommits.length === 0}
	<p class="message">No commits found for this project and current git user.</p>
{:else}
	{#if showBranchPicker || showCoverageBar}
		<div class="top-row">
			<div class="filters">
				{#if showBranchPicker}
					<div class="branch-picker">
						<label for="branch-{projectId}">Branch:</label>
						<select id="branch-{projectId}" value={selectedBranch} onchange={handleBranchChange}>
							{#each data.branches as b (b)}
								<option value={b}>{b}</option>
							{/each}
						</select>
					</div>
				{/if}

				{#if showUserPicker && data.users && data.users.length > 0}
					<div class="user-picker">
						<label for="user-{projectId}">User:</label>
						<select id="user-{projectId}" value={selectedUser} onchange={handleUserChange}>
							<option value="">All</option>
							<hr />
							{#each data.users as a (a.email)}
								<option value={a.email}>{a.name} ({a.email})</option>
							{/each}
						</select>
					</div>
				{/if}
			</div>

			{#if showCoverageBar}
				{#if data.dailySummary && data.dailySummary.length > 0}
					<div class="summary-chart">
						<DailyCommitsChart
							dailySummary={data.dailySummary}
							branch={selectedBranch || data.branch || ''}
						/>
					</div>
				{:else}
					<div class="summary-bar">
						<AgentPercentageBar
							agentPercent={data.summary.linePercent}
							segments={toBarSegments(data.summary.agentSegments)}
							showManual={true}
						/>
					</div>
				{/if}
			{/if}
		</div>
	{/if}

	<table class="data" class:compact>
		<colgroup>
			{#if !compact}
				<col class="timeline-col" />
			{/if}
			{#if !compact && (showDate || !showDate)}
				<col class="time-col" />
			{/if}
			<col class="title-col" />
			{#if showDiffCount}
				<col class="diff-col" />
			{/if}
			{#if showBranch}
				<col class="branch-col" />
			{/if}
			{#if !compact}
				<col class="hash-col" />
			{/if}
			<col class="bar-col" />
		</colgroup>
		{#if showColumnNames}
			<thead>
				<tr>
					{#if !compact}
						<th class="timeline-col"></th>
					{/if}
					{#if !compact && (showDate || !showDate)}
						<th class="time-col">Time</th>
					{/if}
					<th>Commit</th>
					{#if showDiffCount}
						<th class="diff-col">Diff</th>
					{/if}
					{#if showBranch}
						<th class="branch-col">Branch</th>
					{/if}
					{#if !compact}
						<th class="hash-col">Hash</th>
					{/if}
					<th class="bar-col">Agent %</th>
				</tr>
			</thead>
		{/if}
		<tbody>
			{#each visibleCommits as c (c.commitHash)}
				<tr>
					{#if !compact}
						<td class="timeline">
							<span class="timeline-line"></span>
							<span class="timeline-dot"></span>
						</td>
					{/if}
					{#if !compact && (showDate || !showDate)}
						<td class="time" title={formatFullDateTitle(c.authoredAtUnixMs)}
							>{formatRelativeOrShortDate(c.authoredAtUnixMs)}</td
						>
					{/if}
					<td class="title">
						<div class="title-content">
							<a
								href={resolve(
									selectedBranch || data?.branch || ''
										? `/local/projects/${encodeURIComponent(c.projectId)}/commits/${encodeURIComponent(selectedBranch || data?.branch || '')}/${encodeURIComponent(c.commitHash)}`
										: `/local/projects/${encodeURIComponent(c.projectId)}/commits`
								)}
								class="link-button"
							>
								{c.subject || c.commitHash.slice(0, 8)}
							</a>
						</div>
						<!-- {#if !c.workingCopy}
							<div class="commit-meta">{c.commitHash.slice(0, 12)}</div>
						{/if} -->
					</td>
					{#if showDiffCount}
						<td class="diff"
							>{#if !c.workingCopy && (c.linesAdded > 0 || c.linesRemoved > 0)}<DiffCount
									added={c.linesAdded}
									removed={c.linesRemoved}
									compact={true}
								/>{/if}</td
						>
					{/if}
					{#if showBranch}
						<td class="branch" title={selectedBranch || data?.branch || ''}>
							<div class="branch-content">
								<Icon name="branch" width="13px" />
								<span>{selectedBranch || data?.branch || ''}</span>
							</div>
						</td>
					{/if}
					{#if !compact}
						<td class="hash">
							{#if !c.workingCopy}
								<span>{c.commitHash.slice(0, 7)}</span>
								{@const url = commitLink(c.commitHash)}
								{#if url}
									<a
										href={url}
										target="_blank"
										rel="noopener noreferrer"
										class="hash-link"
										aria-label={`Open commit ${c.commitHash} on remote`}
									>
										<Icon name="externalLink" width="14px" />
									</a>
								{/if}
							{/if}
						</td>
					{/if}
					<td class="bar"
						>{#if !c.workingCopy}<AgentPercentageBar
								agentPercent={c.linePercent}
								segments={toBarSegments(c.agentSegments)}
								totalLines={c.linesTotal}
								showKey={false}
							/>{/if}</td
					>
				</tr>
			{/each}
		</tbody>
	</table>

	{#if showPagination && data.pagination.totalPages > 1}
		<div class="pager">
			<button
				class="btn-sm"
				disabled={loading || currentPage <= 1}
				onclick={() => goToPage(currentPage - 1)}
			>
				Previous
			</button>
			<span>Page {data.pagination.page} of {data.pagination.totalPages}</span>
			<button
				class="btn-sm"
				disabled={loading || currentPage >= data.pagination.totalPages}
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
		font-size: 0.9rem;
		font-weight: 600;
		margin-bottom: 0.75rem;
		opacity: 0.5;
		text-transform: uppercase;
	}

	.heading-branch {
		text-transform: none;
	}

	.top-row {
		align-items: flex-start;
		border-bottom: 0.5px solid var(--color-divider);
		display: flex;
		flex-direction: row-reverse;
		gap: 1rem;
		justify-content: space-between;
		padding: 1rem 1rem 1rem 1rem;
	}

	@media (max-width: 1100px) {
		.top-row {
			flex-direction: column-reverse;
		}
	}

	.summary-chart {
		margin-bottom: -0.5rem;
	}

	.link-button {
		background: none;
		border: 0;
		color: var(--link-color, #1f4cd1);
		cursor: pointer;
		display: block;
		font: inherit;
		max-width: 100%;
		overflow: hidden;
		padding: 0;
		text-align: left;
		text-decoration: none;
		text-overflow: ellipsis;
		white-space: nowrap;
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

	.timeline-col {
		width: 50px;
	}

	table.data.compact tr {
		border-bottom: 0px;
	}

	table.data:not(.compact) tr:nth-child(even) {
		background: var(--color-alternate-table-row-background);
	}

	table.data tr:hover,
	table.data:not(.compact) tr:nth-child(even):hover {
		background: var(--accent-color-ultralight);
	}

	table.data:not(.compact) td {
		padding-bottom: 0.7rem;
		padding-top: 0.7rem;
	}

	table.data td {
		white-space: nowrap;
	}

	table.data:not(.compact) .title a {
		-webkit-box-orient: vertical;
		-webkit-line-clamp: 3;
		display: -webkit-box;
		line-clamp: 3;
		line-height: 1.3;
		overflow: hidden;
		white-space: normal;
	}

	.time-col {
		width: 130px;
	}

	.diff-col {
		width: 140px;
	}

	.hash-col {
		width: 96px;
	}

	.branch-col {
		width: 80px;
	}

	.bar-col {
		width: 140px;
	}

	.compact .bar-col {
		width: 120px;
	}

	.branch {
		font-size: 1rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.branch-content {
		align-items: center;
		display: inline-flex;
		gap: 0.25rem;
		max-width: 100%;
		padding-right: 1rem;
	}

	.branch-content span {
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.time {
		padding-left: 1rem;
	}

	.timeline {
		padding: 0;
		position: relative;
	}

	.timeline-line {
		background: color-mix(in srgb, var(--color-border, #999) 75%, transparent);
		bottom: 0;
		left: 60%;
		position: absolute;
		top: 0;
		transform: translateX(-50%);
		width: 1px;
	}

	.timeline-dot {
		background: var(--surface-bg, #fff);
		border: 1px solid var(--color-border, #999);
		border-radius: 50%;
		height: 10px;
		left: 60%;
		position: absolute;
		top: 50%;
		transform: translate(-50%, -50%);
		width: 10px;
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

	.diff {
		padding-right: 1.5rem;
		text-align: right;
	}

	.hash {
		font-family: monospace;
		font-size: 0.9em;
		gap: 0.25rem;
		justify-content: center;
	}

	.hash-link {
		color: inherit;
		display: inline-flex;
		opacity: 0.7;
	}

	.hash-link:hover {
		opacity: 1;
	}

	.bar {
		padding-right: 1rem;
	}

	.filters {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.branch-picker,
	.user-picker {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.branch-picker select,
	.user-picker select {
		width: 200px;
	}

	.branch-picker label,
	.user-picker label {
		text-align: right;
		width: 80px;
	}
</style>
