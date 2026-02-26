<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { SvelteURLSearchParams } from 'svelte/reactivity';
	import Conversations from '$lib/components/project/Conversations.svelte';

	const projectId = $derived(page.params.project_id ?? '');
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

	async function handlePageChange(nextPage: number) {
		updateUrl({ page: nextPage <= 1 ? null : String(nextPage) });
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
		showPagination={true}
		showHidden={currentHidden}
		agent={currentAgent}
		rating={currentRating}
		start={startMs}
		end={endMs}
		onPageChange={handlePageChange}
		onAgentChange={handleAgentChange}
		onRatingChange={handleRatingChange}
		onHiddenChange={handleHiddenChange}
		onDateChange={handleDateChange}
	/>
</div>

<style>
	.project-section {
		margin: 0.5rem 0 1rem 0;
	}
</style>
