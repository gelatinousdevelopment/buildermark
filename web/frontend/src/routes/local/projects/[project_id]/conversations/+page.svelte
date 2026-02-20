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

	async function handlePageChange(nextPage: number) {
		if (!projectId) return;
		const params = new SvelteURLSearchParams(page.url.searchParams);
		if (nextPage <= 1) {
			params.delete('page');
		} else {
			params.set('page', String(nextPage));
		}
		const query = params.toString();
		await goto(
			resolve(
				`/local/projects/${encodeURIComponent(projectId)}/conversations${query ? `?${query}` : ''}`
			),
			{
				replaceState: true,
				noScroll: true,
				keepFocus: true
			}
		);
	}
</script>

<div class="project-section">
	<Conversations
		{projectId}
		page={currentPage}
		pageSize={40}
		compact={false}
		showPagination={true}
		onPageChange={handlePageChange}
	/>
</div>

<style>
	.project-section {
		margin: 0.5rem 0 1rem 0;
	}
</style>
