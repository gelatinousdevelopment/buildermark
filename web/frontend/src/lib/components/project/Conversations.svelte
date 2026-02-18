<script lang="ts">
	import { getProject } from '$lib/api';
	import { enqueueLoad } from '$lib/loadQueue';
	import { stars, shortId } from '$lib/utils';
	import { resolve } from '$app/paths';
	import type { ProjectDetail } from '$lib/types';

	type PageChangeHandler = (page: number) => void | Promise<void>;

	interface Props {
		projectId: string;
		page?: number;
		pageSize?: number;
		limit?: number;
		showHeader?: boolean;
		header?: string;
		showAgentColumn?: boolean;
		showRatingsColumn?: boolean;
		showPagination?: boolean;
		onPageChange?: PageChangeHandler;
		autoload?: boolean;
		useLoadQueue?: boolean;
		loadPriority?: number;
		loadSignal?: number;
		initialData?: ProjectDetail | null;
		initialError?: string | null;
	}

	let {
		projectId,
		page = undefined,
		pageSize = 0,
		limit = 0,
		showHeader = false,
		header = 'Agent Conversations',
		showAgentColumn = true,
		showRatingsColumn = true,
		showPagination = false,
		onPageChange,
		autoload = true,
		useLoadQueue = false,
		loadPriority = 0,
		loadSignal = 0,
		initialData = null,
		initialError = null
	}: Props = $props();

	let project: ProjectDetail | null = $state(null);
	let loading = $state(false);
	let error: string | null = $state(null);
	let internalPage = $state(1);
	let initialized = $state(false);
	let requestToken = 0;
	let lastLoadKey = '';

	$effect(() => {
		if (initialized) return;
		initialized = true;
		project = initialData;
		error = initialError;
		internalPage = page ?? 1;
		loading = autoload && !initialData;
	});

	$effect(() => {
		if (page !== undefined) internalPage = page;
	});

	const currentPage = $derived(page ?? internalPage);
	const visibleConversations = $derived.by(() => {
		const all = project?.conversations ?? [];
		if (limit > 0) return all.slice(0, limit);
		return all;
	});

	function formatRatingTitle(note: string, analysis: string): string {
		if (!analysis) return note;
		return `${note}\n\nAnalysis: ${analysis}`;
	}

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
			const detail = await withOptionalQueue(() => getProject(projectId, requestedPage, requestedPageSize));
			if (myToken !== requestToken) return;
			project = detail;
		} catch (e) {
			if (myToken !== requestToken) return;
			error = e instanceof Error ? e.message : 'Failed to load project conversations';
		} finally {
			if (myToken === requestToken) loading = false;
		}
	}

	$effect(() => {
		if (!autoload) return;
		const loadKey = `${projectId}:${currentPage}:${pageSize}:${loadSignal}`;
		if (loadKey === lastLoadKey) return;
		lastLoadKey = loadKey;
		void loadProjectData();
	});

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
</script>

{#if showHeader}
	<div class="heading">{header}</div>
{/if}

{#if loading}
	<p class="loading">Loading conversations...</p>
{:else if error}
	<p class="error">{error}</p>
{:else if !project || visibleConversations.length === 0}
	<p>No conversations.</p>
{:else}
	<table class="data">
		<thead>
			<tr>
				<th>Conversation</th>
				{#if showAgentColumn}
					<th>Agent</th>
				{/if}
				{#if showRatingsColumn}
					<th>Ratings</th>
				{/if}
			</tr>
		</thead>
		<tbody>
			{#each visibleConversations as conv (conv.id)}
				<tr>
					<td>
						<a
							href={resolve('/local/projects/[project_id]/conversations/[id]', {
								project_id: project.id,
								id: conv.id
							})}
						>
							{conv.title || shortId(conv.id)}
						</a>
					</td>
					{#if showAgentColumn}
						<td>{conv.agent}</td>
					{/if}
					{#if showRatingsColumn}
						<td class="ratings">
							{#if conv.ratings.length > 0}
								{#each conv.ratings as r (r.id)}
									<span title={formatRatingTitle(r.note, r.analysis)}>{stars(r.rating)}</span>
								{/each}
							{:else}
								—
							{/if}
						</td>
					{/if}
				</tr>
			{/each}
		</tbody>
	</table>

	{#if showPagination && (project.conversationPagination.totalPages ?? 0) > 1}
		<div class="pager">
			<button
				class="btn-sm"
				disabled={currentPage <= 1}
				onclick={() => goToPage(currentPage - 1)}
			>
				Previous
			</button>
			<span>Page {project.conversationPagination.page} of {project.conversationPagination.totalPages}</span>
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

	.ratings {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.pager {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-top: 1rem;
	}
</style>
