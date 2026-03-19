<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { SvelteURLSearchParams } from 'svelte/reactivity';
	import Conversations from '$lib/components/project/Conversations.svelte';
	import { settingsStore } from '$lib/stores/settings.svelte';
	import type { ProjectDetail } from '$lib/types';

	const projectId = $derived(page.params.project_id ?? '');
	let conversationPagination = $state<ProjectDetail['conversationPagination'] | null>(null);

	const currentPage = $derived.by(() => {
		const raw = page.url.searchParams.get('page');
		if (!raw) return 1;
		const parsed = Number.parseInt(raw, 10);
		return Number.isInteger(parsed) && parsed > 0 ? parsed : 1;
	});
	const currentAgent = $derived(page.url.searchParams.get('agent') ?? '');
	const currentRating = $derived.by(() => {
		const raw = page.url.searchParams.get('rating');
		if (!raw) return 0;
		const parsed = Number.parseInt(raw, 10);
		return Number.isInteger(parsed) ? parsed : 0;
	});
	const currentHidden = $derived(page.url.searchParams.get('hidden') === 'true');
	const currentOrder = $derived(page.url.searchParams.get('order') ?? settingsStore.sortOrder);

	const startMs = $derived.by(() => {
		const raw = page.url.searchParams.get('start');
		if (!raw) return undefined;
		const t = new Date(raw).getTime();
		return Number.isFinite(t) ? t : undefined;
	});
	const endMs = $derived.by(() => {
		const raw = page.url.searchParams.get('end');
		if (!raw) return undefined;
		const t = new Date(raw).getTime();
		return Number.isFinite(t) ? t : undefined;
	});

	function updateUrl(updates: Record<string, string | null>) {
		if (!projectId) return;
		const params = new SvelteURLSearchParams(page.url.searchParams);
		for (const [key, value] of Object.entries(updates)) {
			if (value === null || value === '' || value === '0') {
				params.delete(key);
			} else {
				params.set(key, value);
			}
		}
		const query = params.toString();
		void goto(
			resolve(
				`/projects/${encodeURIComponent(projectId)}/conversations${query ? `?${query}` : ''}`
			),
			{
				replaceState: true,
				noScroll: true,
				keepFocus: true
			}
		);
	}

	function handleAgentChange(value: string) {
		updateUrl({ agent: value || null, page: null });
	}

	function handleRatingChange(value: string) {
		updateUrl({ rating: value === '0' ? null : value, page: null });
	}

	function handleHiddenChange(value: boolean) {
		updateUrl({ hidden: value ? 'true' : null, page: null });
	}

	function handleDateChange(startIso: string | null, endIso: string | null) {
		updateUrl({ start: startIso, end: endIso, page: null });
	}

	function handleOrderChange(value: string) {
		updateUrl({ order: value === 'asc' ? 'asc' : null, page: null });
	}

	function pageHref(nextPage: number) {
		const params = new SvelteURLSearchParams(page.url.searchParams);
		if (nextPage <= 1) {
			params.delete('page');
		} else {
			params.set('page', String(nextPage));
		}
		const query = params.toString();
		return resolve(
			`/projects/${encodeURIComponent(projectId)}/conversations${query ? `?${query}` : ''}`
		);
	}

	function handleProjectLoaded(project: ProjectDetail) {
		conversationPagination = project.conversationPagination;
	}
</script>

<div class="project-section">
	<Conversations
		{projectId}
		showFilters={true}
		showDateFilter={true}
		showFilesColumn={true}
		page={currentPage}
		pageSize={40}
		compact={false}
		showHidden={currentHidden}
		agent={currentAgent}
		rating={currentRating}
		order={currentOrder}
		start={startMs}
		end={endMs}
		onProjectLoaded={handleProjectLoaded}
		onAgentChange={handleAgentChange}
		onRatingChange={handleRatingChange}
		onHiddenChange={handleHiddenChange}
		onOrderChange={handleOrderChange}
		onDateChange={handleDateChange}
	/>

	{#if conversationPagination && conversationPagination.totalPages > 1}
		<div class="pager">
			{#if currentPage > 1}
				<a class="bordered small" href={pageHref(currentPage - 1)}>Previous</a>
			{:else}
				<span class="bordered small pager-disabled">Previous</span>
			{/if}
			<span>Page {conversationPagination.page} of {conversationPagination.totalPages}</span>
			{#if currentPage < conversationPagination.totalPages}
				<a class="bordered small" href={pageHref(currentPage + 1)}>Next</a>
			{:else}
				<span class="bordered small pager-disabled">Next</span>
			{/if}
		</div>
	{/if}
</div>

<style>
	.project-section {
		margin: 0.5rem 0 1rem 0;
	}

	.pager {
		align-items: center;
		display: flex;
		gap: 0.75rem;
		margin-top: 1rem;
		padding: 0 1rem;
	}

	.pager-disabled {
		cursor: default;
		opacity: 0.5;
		pointer-events: none;
	}
</style>
