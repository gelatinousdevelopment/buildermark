<script lang="ts">
	/* eslint-disable svelte/no-navigation-without-resolve */
	import { resolve } from '$app/paths';
	import { listProjectCommitsPage, ingestMoreCommits, getCommitIngestionStatus } from '$lib/api';
	import { withOptionalQueue } from '$lib/loadQueue';
	import { websocketStore } from '$lib/stores/websocket.svelte';
	import type {
		ProjectCommitPageResponse,
		CommitIngestionStatusResponse,
		AgentCoverageSegment
	} from '$lib/types';
	import AgentPercentageBar from '$lib/components/AgentPercentageBar.svelte';
	import DiffCount from '$lib/components/DiffCount.svelte';
	import Popover from '$lib/components/Popover.svelte';
	import Icon from '$lib/Icon.svelte';
	import DateFilterPicker from '$lib/components/DateFilterPicker.svelte';
	import { relationshipCache } from '$lib/stores/relationshipCache.svelte';
	import { settingsStore, type CommitSortOrder } from '$lib/stores/settings.svelte';
	import { formatRelativeOrShortDate, formatFullDateTitle, commitUrl } from '$lib/utils';

	const USER_AND_AGENTS = '@me+agents';

	function toBarSegments(segs?: AgentCoverageSegment[]): { name: string; percent: number }[] {
		if (!segs || segs.length === 0) return [];
		return segs.map((s) => ({ name: s.agent, percent: s.linePercent }));
	}

	type BranchChangeHandler = (branch: string) => void | Promise<void>;
	type FilterChangeHandler = (value: string) => void | Promise<void>;

	interface Props {
		projectId: string;
		page?: number;
		pageSize?: number;
		branch?: string;
		user?: string;
		agent?: string;
		limit?: number;
		compact?: boolean;
		showHeader?: boolean;
		header?: string;
		headerLink?: string;
		showBranchPicker?: boolean;
		showUserPicker?: boolean;
		showAgentPicker?: boolean;
		showLoadMore?: boolean;
		showBranch?: boolean;
		showDiffCount?: boolean;
		showUser?: boolean;
		showCoverageBar?: boolean;
		showColumnNames?: boolean;
		onBranchChange?: BranchChangeHandler;
		onUserChange?: FilterChangeHandler;
		onAgentChange?: FilterChangeHandler;
		onOrderChange?: FilterChangeHandler;
		order?: string;
		autoload?: boolean;
		useLoadQueue?: boolean;
		loadPriority?: number;
		loadSignal?: number;
		enableRelationshipHover?: boolean;
		onCommitsLoaded?: (commitHashes: string[]) => void;
		onCommitsDataLoaded?: (data: {
			dailySummary: import('$lib/types').DailyCommitSummary[];
			branch: string;
			pagination: import('$lib/types').ProjectCommitPagination;
		}) => void;
		searchTerm?: string;
		defaultToCurrentUser?: boolean;
		start?: number;
		end?: number;
		showDateFilter?: boolean;
		onDateChange?: (start: string | null, end: string | null) => void;
	}

	let {
		projectId,
		page = undefined,
		pageSize = 10,
		branch = undefined,
		user = undefined,
		agent = undefined,
		limit = 0,
		compact = false,
		showHeader = false,
		header = 'Git Commits',
		headerLink = undefined,
		showBranchPicker = false,
		showUserPicker = false,
		showAgentPicker = false,
		showLoadMore = false,
		showBranch = false,
		showDiffCount = true,
		showUser = false,
		showCoverageBar = true,
		showColumnNames = false,
		onBranchChange,
		onUserChange,
		onAgentChange,
		onOrderChange,
		order = undefined,
		autoload = true,
		useLoadQueue = false,
		loadPriority = 0,
		loadSignal = 0,
		enableRelationshipHover = false,
		onCommitsLoaded = undefined,
		onCommitsDataLoaded = undefined,
		searchTerm = '',
		defaultToCurrentUser = true,
		start = undefined,
		end = undefined,
		showDateFilter = false,
		onDateChange = undefined
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
	let internalAgent = $state('');
	let internalOrder = $state(settingsStore.commitSortOrder);
	let internalStart: number | undefined = $state(undefined);
	let internalEnd: number | undefined = $state(undefined);
	let userDefaultApplied = false;
	let initialized = $state(false);
	let requestToken = 0;
	let lastLoadKey = '';

	$effect(() => {
		if (initialized) return;
		initialized = true;
		internalPage = page ?? 1;
		if (user !== undefined) {
			internalUser = user;
			userDefaultApplied = true;
		} else if (defaultToCurrentUser && !userDefaultApplied) {
			internalUser = USER_AND_AGENTS;
			userDefaultApplied = true;
		}
		if (agent !== undefined) {
			internalAgent = agent;
		}
		internalBranch = branch ?? '';
		if (start !== undefined) internalStart = start;
		if (end !== undefined) internalEnd = end;
		if (order !== undefined) internalOrder = order as CommitSortOrder;
		loading = autoload;
	});

	$effect(() => {
		if (page !== undefined) internalPage = page;
	});

	$effect(() => {
		if (branch !== undefined) internalBranch = branch;
	});

	$effect(() => {
		if (user !== undefined) {
			internalUser = user;
			userDefaultApplied = true;
		}
	});

	$effect(() => {
		if (agent !== undefined) internalAgent = agent;
	});

	$effect(() => {
		if (order !== undefined) internalOrder = order as CommitSortOrder;
	});

	$effect(() => {
		if (start !== undefined) internalStart = start;
	});

	$effect(() => {
		if (end !== undefined) internalEnd = end;
	});

	// Reset to page 1 when date filter changes.
	let lastDateKey = '';
	$effect(() => {
		const key = `${effectiveStart}:${effectiveEnd}`;
		if (lastDateKey && key !== lastDateKey) {
			internalPage = 1;
		}
		lastDateKey = key;
	});

	const currentPage = $derived(page ?? internalPage);
	const selectedBranch = $derived(branch ?? internalBranch);
	const selectedUser = $derived(user ?? internalUser);
	const selectedAgent = $derived(agent ?? internalAgent);
	const selectedOrder = $derived(order ?? internalOrder);
	const effectiveStart = $derived(start ?? internalStart);
	const effectiveEnd = $derived(end ?? internalEnd);

	const selectedUserDisplay = $derived.by(() => {
		if (!selectedUser) return '';
		if (selectedUser === USER_AND_AGENTS) return 'Local User';
		const match = data?.users?.find((u) => u.email === selectedUser);
		return match?.name || selectedUser;
	});
	const visibleCommits = $derived.by(() => {
		let all = data?.commits ?? [];
		if (limit > 0) all = all.slice(0, limit);
		if (searchTerm.trim()) all = all.filter((c) => !c.workingCopy);

		if (selectedOrder === 'asc') {
			const hasMore =
				data?.pagination &&
				data.pagination.totalPages > 1 &&
				currentPage < data.pagination.totalPages;
			const wc = all.filter((c) => c.workingCopy);
			const rest = all.filter((c) => !c.workingCopy);
			if (hasMore) return rest;
			return [...rest, ...wc];
		}

		return all;
	});

	async function loadIngestionStatus(branchValue: string) {
		if (!showLoadMore || !projectId) {
			ingestionStatus = null;
			return;
		}
		try {
			ingestionStatus = await withOptionalQueue(
				() => getCommitIngestionStatus(projectId, branchValue),
				useLoadQueue,
				loadPriority
			);
		} catch {
			ingestionStatus = null;
		}
	}

	function resolveUserFilter(raw: string): string {
		return raw;
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
			const resolvedUser = resolveUserFilter(selectedUser);
			const loaded = await withOptionalQueue(
				() =>
					listProjectCommitsPage(
						projectId,
						pageNum,
						selectedBranch,
						pageSize,
						resolvedUser,
						selectedAgent,
						searchTerm.trim(),
						effectiveStart,
						effectiveEnd,
						undefined,
						undefined,
						selectedOrder
					),
				useLoadQueue,
				loadPriority
			);
			if (myToken !== requestToken) return;
			data = loaded;
			if (branch === undefined && !internalBranch && loaded.branch) {
				internalBranch = loaded.branch;
			}
			void loadIngestionStatus(branch ?? internalBranch);
			if (onCommitsLoaded) {
				onCommitsLoaded(loaded.commits.filter((c) => !c.workingCopy).map((c) => c.commitHash));
			}
			if (onCommitsDataLoaded) {
				onCommitsDataLoaded({
					dailySummary: loaded.dailySummary ?? [],
					branch: loaded.branch,
					pagination: loaded.pagination
				});
			}
		} catch (e) {
			if (myToken !== requestToken) return;
			error = e instanceof Error ? e.message : 'Failed to load commit coverage';
		} finally {
			if (myToken === requestToken) loading = false;
		}
	}

	$effect(() => {
		if (!autoload) return;
		const resolved = resolveUserFilter(selectedUser);
		const loadKey = `${projectId}:${currentPage}:${pageSize}:${selectedBranch}:${selectedUser}:${resolved}:${selectedAgent}:${selectedOrder}:${searchTerm}:${effectiveStart}:${effectiveEnd}:${loadSignal}`;
		if (loadKey === lastLoadKey) return;
		lastLoadKey = loadKey;
		void loadCommitsData();
	});

	// Auto-reload when async commit ingestion completes.
	$effect(() => {
		const job = websocketStore.getJob('commit_ingest');
		if (!job || job.state !== 'complete') return;
		if (job.projectId && job.projectId !== projectId) return;
		websocketStore.clearJob('commit_ingest');
		void loadCommitsData();
	});

	async function handleBranchChange(event: Event) {
		const value = (event.currentTarget as HTMLSelectElement).value;
		if (branch === undefined) {
			internalBranch = value;
		}
		if (onBranchChange) {
			await onBranchChange(value);
			return;
		}
		internalPage = 1;
	}

	function handleUserChange(event: Event) {
		const value = (event.currentTarget as HTMLSelectElement).value;
		userDefaultApplied = true;
		if (user === undefined) internalUser = value;
		internalPage = 1;
		if (onUserChange) {
			onUserChange(value);
		}
	}

	function handleAgentChange(event: Event) {
		const value = (event.currentTarget as HTMLSelectElement).value;
		if (agent === undefined) internalAgent = value;
		internalPage = 1;
		if (onAgentChange) {
			onAgentChange(value);
		}
	}

	function handleOrderChange(event: Event) {
		const value = (event.currentTarget as HTMLSelectElement).value as CommitSortOrder;
		internalOrder = value;
		settingsStore.commitSortOrder = value;
		internalPage = 1;
		if (onOrderChange) {
			onOrderChange(value);
		}
	}

	function handleDateFilterChange(range: { from: number; to: number } | null) {
		if (range) {
			internalStart = range.from;
			internalEnd = range.to;
			if (onDateChange) {
				onDateChange(new Date(range.from).toISOString(), new Date(range.to).toISOString());
			}
		} else {
			internalStart = undefined;
			internalEnd = undefined;
			if (onDateChange) {
				onDateChange(null, null);
			}
		}
		internalPage = 1;
	}

	async function handleLoadMore() {
		if (!projectId || loadingMore) return;
		loadingMore = true;
		loadMoreError = null;
		try {
			await withOptionalQueue(
				() => ingestMoreCommits(projectId, loadMoreCount, selectedBranch),
				useLoadQueue,
				loadPriority
			);
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
	<div class="heading">
		{#if headerLink}<a href={headerLink}>{header}</a>{:else}{header}{/if}
		<div style:flex="1"></div>
		{#if selectedUserDisplay}<span class="heading-filter" title={selectedUserDisplay}
				><Icon name="user" width="12px" />{selectedUserDisplay}</span
			>{/if}{#if selectedBranch || data?.branch}<span
				class="heading-filter"
				title={selectedBranch || data?.branch}
				><Icon name="branch" width="13px" />{selectedBranch || data?.branch}</span
			>{/if}
	</div>
{/if}

{#if loading && !data}
	<p class="message loading">Loading commits...</p>
{:else if error}
	<p class="message error">{error}</p>
{:else if !data || visibleCommits.length === 0}
	<p class="message">No Results</p>
{:else}
	{#if showBranchPicker || showUserPicker || showAgentPicker || showDateFilter}
		<div class="top-row">
			<div class="filters">
				{#if showDateFilter}
					<div class="filter-picker">
						<select id="order-{projectId}" value={selectedOrder} onchange={handleOrderChange}>
							<option value="desc">Newest First</option>
							<option value="asc">Oldest First</option>
						</select>
					</div>

					<DateFilterPicker start={effectiveStart} onchange={handleDateFilterChange} />
				{/if}

				{#if showBranchPicker}
					<div class="filter-picker branch-picker">
						<label for="branch-{projectId}">Branch:</label>
						<select id="branch-{projectId}" value={selectedBranch} onchange={handleBranchChange}>
							{#each data.branches as b (b)}
								<option value={b}>{b}</option>
							{/each}
						</select>
					</div>
				{/if}

				{#if showUserPicker && data.users && data.users.length > 0}
					<div class="filter-picker user-picker">
						<label for="user-{projectId}">User:</label>
						<select id="user-{projectId}" value={selectedUser} onchange={handleUserChange}>
							<option value="">All</option>
							<hr />
							<option value={USER_AND_AGENTS}>Local User</option>
							<hr />
							{#each data.users as a (a.email)}
								<option value={a.email}>{a.name} ({a.email})</option>
							{/each}
						</select>
					</div>
				{/if}

				{#if showAgentPicker && data.agents && data.agents.length > 0}
					<div class="filter-picker agent-picker">
						<label for="agent-{projectId}">Agent:</label>
						<select id="agent-{projectId}" value={selectedAgent} onchange={handleAgentChange}>
							<option value="">All Agents and Manual</option>
							<option value="manual">Manual</option>
							<hr />
							{#each data.agents as a (a)}
								<option value={a}>{a}</option>
							{/each}
						</select>
					</div>
				{/if}
			</div>
		</div>
	{/if}

	<table class="data" class:compact>
		<colgroup>
			{#if !compact}
				<col class="timeline-col" />
			{/if}
			<col class="date-col" />
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
			{#if showUser}
				<col class="user-col" />
			{/if}
			{#if showCoverageBar}
				<col class="bar-col" />
			{/if}
		</colgroup>
		{#if showColumnNames}
			<thead>
				<tr>
					{#if !compact}
						<th class="timeline-col"></th>
					{/if}
					<th class="date-col">Time</th>
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
					{#if showUser}
						<th class="user-col">User</th>
					{/if}
					{#if showCoverageBar}
						<th class="bar-col">Agent %</th>
					{/if}
				</tr>
			</thead>
		{/if}
		<tbody
			onmouseleave={() => {
				if (enableRelationshipHover) relationshipCache.clearHover();
			}}
		>
			{#each visibleCommits as c (c.commitHash)}
				<tr
					class:relationship-highlight={enableRelationshipHover &&
						!c.workingCopy &&
						relationshipCache.highlightedCommitHashes.has(c.commitHash)}
					class:relationship-source={enableRelationshipHover &&
						!c.workingCopy &&
						relationshipCache.hoveredCommitHash === c.commitHash}
					onmouseenter={() => {
						if (enableRelationshipHover) {
							if (c.workingCopy) relationshipCache.clearHover();
							else relationshipCache.hoverCommit(projectId, c.commitHash);
						}
					}}
				>
					{#if !compact}
						<td class="timeline">
							<span class="timeline-line"></span>
							<span class="timeline-dot"></span>
						</td>
					{/if}
					<td class="date" title={formatFullDateTitle(c.authoredAtUnixMs)}
						>{c.workingCopy ? '' : formatRelativeOrShortDate(c.authoredAtUnixMs, compact)}</td
					>
					<td class="title">
						<div class="title-content">
							<a
								href={resolve(
									selectedBranch || data?.branch || ''
										? `/projects/${encodeURIComponent(c.projectId)}/commits/${encodeURIComponent(selectedBranch || data?.branch || '')}/${encodeURIComponent(c.commitHash)}`
										: `/projects/${encodeURIComponent(c.projectId)}/commits`
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
					{#if showUser}
						<td class="user">
							{#if c.userName}
								<Popover position="leading" width="auto" padding="0.5rem 0.75rem">
									<span class="user-tag">{c.userName}</span>
									{#snippet popover()}
										<div class="user-popover">
											<div class="user-popover-name">{c.userName}</div>
											{#if c.userEmail}
												<div class="user-popover-email">{c.userEmail}</div>
											{/if}
										</div>
									{/snippet}
								</Popover>
							{/if}
						</td>
					{/if}
					{#if showCoverageBar}
						<td class="bar"
							>{#if !c.workingCopy}<AgentPercentageBar
									agentPercent={c.linePercent}
									segments={c.overrideLinePercent != null ? [] : toBarSegments(c.agentSegments)}
									totalLines={c.linesTotal}
									showKey={false}
									needsParent={c.needsParent}
									commitHref={selectedBranch || data?.branch
										? resolve(
												`/projects/${encodeURIComponent(c.projectId)}/commits/${encodeURIComponent(selectedBranch || data?.branch || '')}/${encodeURIComponent(c.commitHash)}`
											)
										: undefined}
								/>{/if}</td
						>
					{/if}
				</tr>
			{/each}
		</tbody>
	</table>

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
				<button class="bordered small" onclick={handleLoadMore} disabled={loadingMore}>
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
		align-items: center;
		display: flex;
		font-size: 0.9rem;
		font-weight: 600;
		gap: 1rem;
		justify-content: space-between;
		margin-bottom: 0.75rem;
		opacity: 0.5;
		padding: 0 1rem;
		text-transform: uppercase;
	}

	.heading a {
		color: inherit;
		text-decoration: none;
	}

	.heading a:hover {
		text-decoration: underline;
	}

	.heading-filter {
		align-items: center;
		display: inline-flex;
		font-weight: 400;
		gap: 0.2rem;
		text-transform: none;
	}

	.top-row {
		align-items: flex-start;
		border-bottom: 0.5px solid var(--color-divider);
		display: flex;
		flex-direction: row;
		gap: 1rem;
		justify-content: space-between;
		padding: 0.5rem 1rem 1rem 1rem;
	}

	.link-button {
		background: none;
		border: 0;
		color: var(--color-link-body);
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

	.load-more {
		margin-top: 1.2rem;
		padding: 0.8rem;
		border: 1px solid var(--color-border-light);
		border-radius: 6px;
		background: var(--color-background-elevated);
	}

	.load-more-info {
		font-size: 0.85rem;
		color: var(--color-text-secondary);
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
		border: 1px solid var(--color-border-input);
		background: var(--color-background-surface);
		color: var(--color-text);
		border-radius: 4px;
		font-size: 0.9rem;
		text-align: center;
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
		padding-bottom: 0.5rem;
		padding-top: 0.5rem;
	}

	table.data td {
		white-space: nowrap;
	}

	table.data td a {
		color: var(--accent-color-darkest);
		width: fit-content;
	}

	table.data:not(.compact) .title a {
		-webkit-box-orient: vertical;
		-webkit-line-clamp: 3;
		display: -webkit-box;
		line-clamp: 3;
		line-height: 1.3;
		overflow: hidden;
	}

	.date-col {
		width: 130px;
	}

	.compact .date-col {
		width: 70px;
	}

	.diff-col {
		width: 140px;
	}

	.hash-col {
		width: 80px;
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

	.date {
		padding-left: 1rem;
	}

	.compact .date {
		color: var(--color-text-secondary);
		font-size: 0.9rem;
		padding-left: 1rem;
		padding-right: 0;
		text-align: right;
		white-space: nowrap;
		width: fit-content;
	}

	.timeline {
		padding: 0;
		position: relative;
	}

	.timeline-line {
		background: color-mix(in srgb, var(--color-text-faded) 75%, transparent);
		bottom: 0;
		left: 60%;
		position: absolute;
		top: 0;
		transform: translateX(-50%);
		width: 1px;
	}

	.timeline-dot {
		background: var(--color-background-content);
		border: 1px solid var(--color-text-faded);
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

	.title .title-content {
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
		padding-right: 0rem;
	}

	.hash-link {
		color: inherit;
		display: inline-flex;
		opacity: 0.7;
	}

	.hash-link:hover {
		opacity: 1;
	}

	.user {
		text-align: center;
	}

	.bar {
		padding-right: 1rem;
	}

	.filters {
		align-items: center;
		display: flex;
		flex-direction: row;
		flex-wrap: wrap;
		gap: 1.5rem;
	}

	.branch-picker select {
		width: 200px;
	}

	.user-picker select {
		width: 200px;
	}

	.agent-picker select {
		width: 160px;
	}

	.branch-picker label {
		text-align: right;
		width: 54px;
	}

	tr.relationship-highlight {
		background: var(--color-relationship-highlight, #fff8e1) !important;
	}

	tr.relationship-highlight .title a {
		color: var(--color-relationship-foreground);
	}

	tr.relationship-source {
		background: var(--color-relationship-source, #e3f2fd) !important;
	}

	.user-col {
		width: 120px;
	}

	.user-tag {
		display: inline-flex;
		align-items: center;
		padding: calc(0.2rem - 0.5px) calc(0.6rem - 0.5px);
		border-radius: 999px;
		background: var(--color-tag-background, #e8e8e8);
		color: var(--color-tag-text, #555);
		font-size: 0.82rem;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		max-width: 100%;
		cursor: default;
		border: 0.5px solid var(--color-tag-border, #888);
		box-sizing: border-box;
	}

	.user-tag:hover {
		background: var(--color-tag-background-hover, #d8d8d8);
		color: var(--color-tag-text-hover, #333);
	}

	.user-popover {
		white-space: nowrap;
	}

	.user-popover-name {
		font-weight: 600;
		font-size: 0.85rem;
	}

	.user-popover-email {
		font-size: 0.82rem;
		color: var(--color-text-secondary);
		margin-top: 0.15rem;
	}
</style>
