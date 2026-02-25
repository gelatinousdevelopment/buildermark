<script lang="ts">
	import { getProject, getConversationsBatchDetail } from '$lib/api';
	import { enqueueLoad } from '$lib/loadQueue';
	import {
		shortId,
		formatRelativeOrShortDate,
		formatFullDateTitle,
		singleLineTitle
	} from '$lib/utils';
	import AgentTag from '$lib/components/AgentTag.svelte';
	import RatingStars from '$lib/components/RatingStars.svelte';
	import Popover from '$lib/components/Popover.svelte';
	import UserPromptMessageCard from '$lib/components/UserPromptMessageCard.svelte';
	import RatingMessageCard from '$lib/components/RatingMessageCard.svelte';
	import { resolve } from '$app/paths';
	import { SvelteMap } from 'svelte/reactivity';
	import type { ProjectDetail, ConversationBatchDetail } from '$lib/types';
	import { relationshipCache } from '$lib/stores/relationshipCache.svelte';
	import Icon from '$lib/Icon.svelte';

	type PageChangeHandler = (page: number) => void | Promise<void>;
	type FilterChangeHandler = (value: string) => void | Promise<void>;

	interface Props {
		projectId: string;
		page?: number;
		pageSize?: number;
		limit?: number;
		compact?: boolean;
		showHeader?: boolean;
		showFilters?: boolean;
		header?: string;
		showAgentColumn?: boolean;
		showFilesColumn?: boolean;
		showRatingsColumn?: boolean;
		showPagination?: boolean;
		showColumnNames?: boolean;
		onPageChange?: PageChangeHandler;
		onAgentChange?: FilterChangeHandler;
		onRatingChange?: FilterChangeHandler;
		agent?: string;
		rating?: number;
		autoload?: boolean;
		useLoadQueue?: boolean;
		loadPriority?: number;
		loadSignal?: number;
		initialData?: ProjectDetail | null;
		initialError?: string | null;
		enableRelationshipHover?: boolean;
		onConversationsLoaded?: (conversationIds: string[]) => void;
	}

	let {
		projectId,
		page = undefined,
		pageSize = 0,
		limit = 0,
		compact = false,
		showHeader = false,
		showFilters = false,
		header = 'Agent Conversations',
		showAgentColumn = true,
		showFilesColumn = false,
		showRatingsColumn = true,
		showPagination = false,
		showColumnNames = false,
		onPageChange,
		onAgentChange,
		onRatingChange,
		agent = undefined,
		rating = undefined,
		autoload = true,
		useLoadQueue = false,
		loadPriority = 0,
		loadSignal = 0,
		initialData = null,
		initialError = null,
		enableRelationshipHover = false,
		onConversationsLoaded = undefined
	}: Props = $props();

	let project: ProjectDetail | null = $state(null);
	let loading = $state(false);
	let error: string | null = $state(null);
	let internalPage = $state(1);
	let internalAgent = $state('');
	let internalRating = $state(0);
	let detailed = $state(false);
	let initialized = $state(false);
	let requestToken = 0;
	let lastLoadKey = '';

	// Detailed mode data: keyed by conversation ID
	let detailData = new SvelteMap<string, ConversationBatchDetail>();
	let lastDetailIds = '';

	$effect(() => {
		if (initialized) return;
		initialized = true;
		project = initialData;
		error = initialError;
		internalPage = page ?? 1;
		internalAgent = agent ?? '';
		internalRating = rating ?? 0;
		loading = autoload && !initialData;
	});

	$effect(() => {
		if (page !== undefined) internalPage = page;
	});

	$effect(() => {
		if (agent !== undefined) internalAgent = agent;
	});

	$effect(() => {
		if (rating !== undefined) internalRating = rating;
	});

	const currentPage = $derived(page ?? internalPage);
	const selectedAgent = $derived(agent ?? internalAgent);
	const selectedRating = $derived(rating ?? internalRating);
	const visibleConversations = $derived.by(() => {
		const all = project?.conversations ?? [];
		if (limit > 0) return all.slice(0, limit);
		return all;
	});

	function withOptionalQueue<T>(task: () => Promise<T>): Promise<T> {
		if (useLoadQueue) return enqueueLoad(task, loadPriority);
		return task();
	}

	async function loadProjectData() {
		if (!projectId) {
			error = 'Missing project ID';
			return;
		}
		const myToken = ++requestToken;
		loading = true;
		error = null;
		try {
			const requestedPage = Math.max(1, currentPage);
			const requestedPageSize = pageSize > 0 ? pageSize : undefined;
			const filters: { agent?: string; rating?: number } = {};
			if (selectedAgent) filters.agent = selectedAgent;
			if (selectedRating !== 0) filters.rating = selectedRating;
			const detail = await withOptionalQueue(() =>
				getProject(projectId, requestedPage, requestedPageSize, undefined, filters)
			);
			if (myToken !== requestToken) return;
			project = detail;
			if (enableRelationshipHover) {
				relationshipCache.loadConversationParentLinks(projectId, detail.conversations);
			}
			if (onConversationsLoaded) {
				onConversationsLoaded(detail.conversations.map((c) => c.id));
			}
		} catch (e) {
			if (myToken !== requestToken) return;
			error = e instanceof Error ? e.message : 'Failed to load project conversations';
		} finally {
			if (myToken === requestToken) loading = false;
		}
	}

	$effect(() => {
		if (!autoload) return;
		const loadKey = `${projectId}:${currentPage}:${pageSize}:${selectedAgent}:${selectedRating}:${loadSignal}`;
		if (loadKey === lastLoadKey) return;
		lastLoadKey = loadKey;
		void loadProjectData();
	});

	// Load batch detail data when detailed mode is on and conversations change.
	$effect(() => {
		if (!detailed) return;
		const ids = visibleConversations.map((c) => c.id);
		const idsKey = ids.join(',');
		if (idsKey === lastDetailIds || ids.length === 0) return;
		lastDetailIds = idsKey;
		void loadDetailData(ids);
	});

	async function loadDetailData(ids: string[]) {
		try {
			const details = await getConversationsBatchDetail(ids);
			detailData.clear();
			for (const d of details) {
				detailData.set(d.conversationId, d);
			}
		} catch {
			// Silently fail — detailed data is supplementary
		}
	}

	async function goToPage(nextPage: number) {
		if (!project?.conversationPagination) return;
		if (nextPage < 1 || nextPage > project.conversationPagination.totalPages) return;
		if (page === undefined) {
			internalPage = nextPage;
		}
		if (onPageChange) {
			await onPageChange(nextPage);
		}
		if (!autoload) {
			void loadProjectData();
		}
	}

	function handleAgentChange(event: Event) {
		const value = (event.currentTarget as HTMLSelectElement).value;
		if (agent === undefined) {
			internalAgent = value;
		}
		internalPage = 1;
		if (onAgentChange) {
			onAgentChange(value);
		}
	}

	function handleRatingChange(event: Event) {
		const value = Number((event.currentTarget as HTMLSelectElement).value);
		if (rating === undefined) {
			internalRating = value;
		}
		internalPage = 1;
		if (onRatingChange) {
			onRatingChange(String(value));
		}
	}
</script>

{#if showHeader}
	<div class="heading">{header}</div>
{/if}

{#if showFilters}
	<div class="top-row">
		<div class="filters">
			{#if project?.agents && project.agents.length > 1}
				<div class="filter-picker">
					<!-- <label for="agent-{projectId}">Agent:</label> -->
					<select id="agent-{projectId}" value={selectedAgent} onchange={handleAgentChange}>
						<option value="">All Agents</option>
						<hr />
						{#each project.agents as a (a)}
							<option value={a}>{a}</option>
						{/each}
					</select>
				</div>
			{/if}
			<div class="filter-picker">
				<!-- <label for="rating-{projectId}">Rating:</label> -->
				<select id="rating-{projectId}" value={selectedRating} onchange={handleRatingChange}>
					<option value={0}>All Ratings</option>
					<hr />
					<option value={-1}>&lt; 5 Stars</option>
					<hr />
					<option value={5}>5</option>
					<option value={4}>4</option>
					<option value={3}>3</option>
					<option value={2}>2</option>
					<option value={1}>1</option>
				</select>
			</div>
			<label class="toggle-label">
				<input type="checkbox" bind:checked={detailed} />
				Show prompts and ratings
			</label>
		</div>
	</div>
{/if}

{#if loading && !project}
	<p class="message loading">Loading conversations...</p>
{:else if error}
	<p class="message error">{error}</p>
{:else if !project || visibleConversations.length === 0}
	<p class="message">No conversations.</p>
{:else}
	<table class="data" class:compact class:detailed>
		<colgroup>
			{#if !compact}
				<col class="date-col" />
			{/if}
			<col />
			{#if showRatingsColumn}
				<col class={detailed ? 'ratings-col-detailed' : 'ratings-col'} />
			{/if}
			{#if showFilesColumn}
				<col class="files-col" />
			{/if}
			{#if showAgentColumn}
				<col class="agent-col" />
			{/if}
		</colgroup>
		{#if showColumnNames}
			<thead>
				<tr>
					{#if !compact}
						<th class="date-col">Date</th>
					{/if}
					<th>Conversation</th>
					{#if showRatingsColumn}
						<th>Ratings</th>
					{/if}
					{#if showFilesColumn}
						<th>Files</th>
					{/if}
					{#if showAgentColumn}
						<th>Agent</th>
					{/if}
				</tr>
			</thead>
		{/if}
		<tbody
			onmouseleave={() => {
				if (enableRelationshipHover) relationshipCache.clearHover();
			}}
		>
			{#each visibleConversations as conv (conv.id)}
				{@const detail = detailData.get(conv.id)}
				<tr
					class:relationship-highlight={enableRelationshipHover &&
						relationshipCache.highlightedConversationIds.has(conv.id)}
					class:relationship-source={enableRelationshipHover &&
						relationshipCache.hoveredConversationId === conv.id}
					onmouseenter={() => {
						if (enableRelationshipHover) relationshipCache.hoverConversation(projectId, conv.id);
					}}
				>
					{#if !compact}
						<td class="date" title={formatFullDateTitle(conv.lastMessageTimestamp)}
							>{formatRelativeOrShortDate(conv.lastMessageTimestamp)}</td
						>
					{/if}
					<td class="title">
						<a
							href={resolve('/local/projects/[project_id]/conversations/[id]', {
								project_id: project.id,
								id: conv.id
							})}
						>
							{#if conv.parentConversationId}<div class="document-icon">
									<Icon name="document" width="13px" />
								</div>{/if}{(conv.title && singleLineTitle(conv.title)) || shortId(conv.id)}
						</a>
						{#if detailed && detail}
							<div class="detail-messages">
								{#each detail.userMessages as msg (msg.id)}
									<div class="detail-user-message">
										<UserPromptMessageCard message={msg} />
									</div>
								{/each}
							</div>
						{/if}
					</td>
					{#if showRatingsColumn}
						<td class="ratings">
							{#if detailed && detail}
								<div class="ratings-detail">
									{#each detail.ratings as r (r.id)}
										<div class="detail-rating-card">
											<RatingMessageCard rating={r} />
										</div>
									{/each}
								</div>
							{:else}
								<RatingStars ratings={conv.ratings} />
							{/if}
						</td>
					{/if}
					{#if showFilesColumn}
						<td class="files">
							{#if conv.filesEdited.length > 0}
								<Popover position="leading" width="500px" padding="0.75rem">
									<span class="files-tag"
										>{conv.filesEdited.length}
										{conv.filesEdited.length === 1 ? 'File' : 'Files'}</span
									>
									{#snippet popover()}
										<div class="files-popover">
											{#each conv.filesEdited as fp (fp)}
												<div class="files-path" title={fp}>{fp}</div>
											{/each}
										</div>
									{/snippet}
								</Popover>
							{/if}
						</td>
					{/if}
					{#if showAgentColumn}
						<td class="agent"><AgentTag agent={conv.agent} /></td>
					{/if}
				</tr>
			{/each}
		</tbody>
	</table>

	{#if showPagination && (project.conversationPagination.totalPages ?? 0) > 1}
		<div class="pager">
			<button class="btn-sm" disabled={currentPage <= 1} onclick={() => goToPage(currentPage - 1)}>
				Previous
			</button>
			<span
				>Page {project.conversationPagination.page} of {project.conversationPagination
					.totalPages}</span
			>
			<button
				class="btn-sm"
				disabled={currentPage >= project.conversationPagination.totalPages}
				onclick={() => goToPage(currentPage + 1)}
			>
				Next
			</button>
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

	.top-row {
		align-items: flex-start;
		border-bottom: 0.5px solid var(--color-divider);
		display: flex;
		flex-direction: row;
		gap: 1rem;
		justify-content: space-between;
		padding: 0.5rem 1rem 1rem 1rem;
	}

	@media (max-width: 1100px) {
		.top-row {
			flex-direction: column-reverse;
		}
	}

	.filters {
		align-items: center;
		display: flex;
		flex-direction: row;
		gap: 1.5rem;
	}

	.filter-picker {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.filter-picker select {
		min-width: 100px;
	}

	/*.filter-picker label {
		text-align: right;
		width: auto;
	}*/

	.toggle-label {
		align-items: center;
		display: flex;
		align-items: center;
		gap: 0.4rem;
		cursor: pointer;
		user-select: none;
	}

	.message {
		padding-left: 1rem;
		padding-right: 1rem;
	}

	table.data {
		table-layout: fixed;
	}

	table.data.compact tr {
		border-bottom: 0px;
	}

	table.data.detailed td {
		vertical-align: top;
	}

	table.data:not(.compact) tr:nth-child(even) {
		background: var(--color-alternate-table-row-background);
	}

	table.data:not(.detailed) tr:hover,
	table.data:not(.compact):not(.detailed) tr:nth-child(even):hover {
		background: var(--accent-color-ultralight);
	}

	table.data:not(.compact) td {
		padding-bottom: 0.6rem;
		padding-top: 0.6rem;
	}

	.files-col {
		width: 70px;
	}

	.files {
		text-align: center;
	}

	table.data.detailed td.files {
		vertical-align: top;
	}

	.files-tag {
		display: inline-flex;
		align-items: center;
		padding: calc(0.2rem - 0.5px) calc(0.6rem - 0.5px);
		border-radius: 999px;
		background: var(--color-tag-background, #e8e8e8);
		color: var(--color-tag-text, #555);
		font-size: 0.75rem;
		font-weight: 600;
		line-height: 1.2;
		margin: -0.1rem 0;
		white-space: nowrap;
		cursor: default;
		border: 0.5px solid var(--color-tag-border, #888);
		box-sizing: border-box;
	}

	.files-tag:hover {
		background: var(--color-tag-background-hover, #d8d8d8);
		color: var(--color-tag-text-hover, #333);
	}

	.files-popover {
		display: flex;
		align-items: flex-start;
		flex-direction: column;
		gap: 0.25rem;
		font-size: 0.82rem;
		white-space: normal;
	}

	.files-path {
		font-family: var(--font-mono, monospace);
		font-size: 0.78rem;
		color: #444;
		white-space: nowrap;
		line-height: 1.4;
		overflow: hidden;
		text-overflow: ellipsis;
		max-width: 100%;
	}

	.agent-col {
		width: 80px;
	}

	.date-col {
		width: 140px;
	}

	.ratings-col {
		width: 78px;
	}

	.ratings-col-detailed {
		width: 30%;
	}

	.date {
		padding-left: 1rem;
		white-space: nowrap;
	}

	table.data.detailed td.date {
		vertical-align: top;
	}

	.title {
		overflow: hidden;
		padding-left: 1rem;
	}

	.title a {
		color: var(--color-text);
		display: block;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.document-icon {
		color: var(--color-relationship-foreground);
		display: inline-block;
		flex-shrink: 0;
		height: 12px;
		margin-right: 5px;
		vertical-align: -2px;
		width: 12px;
	}

	.title a:hover {
		color: var(--accent-color);
	}

	.detail-messages {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		margin-top: 0.5rem;
	}

	.detail-user-message {
		background: var(--color-prompt-background);
		border: 1px solid var(--color-prompt-border);
		border-radius: 8px;
		color: #444;
		padding: 0.5rem 0.75rem;
	}

	.ratings-detail {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.detail-rating-card {
		background: var(--color-rating-background);
		border: 1px solid var(--color-rating-border);
		border-radius: 8px;
		padding: 0.5rem 0.75rem;
		color: #444;
	}

	table.data.detailed td.ratings {
		vertical-align: top;
	}

	.agent {
		text-align: center;
	}

	table.data.detailed td.agent {
		vertical-align: top;
	}

	.ratings {
		padding-right: 1rem;
	}

	table.data:not(.detailed) .ratings {
		align-items: center;
		display: flex;
		gap: 0.5rem;
	}

	.pager {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-top: 1rem;
		padding: 0 1rem;
	}

	tr.relationship-highlight {
		background: var(--color-relationship-highlight, #fff8e1) !important;
	}

	tr.relationship-source {
		background: var(--color-relationship-source, #e3f2fd) !important;
	}
</style>
