<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
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
		const params = new URLSearchParams(page.url.searchParams);
		if (nextPage <= 1) {
			params.delete('page');
		} else {
			params.set('page', String(nextPage));
		}
		const base = resolve('/local/projects/[project_id]/conversations', { project_id: projectId });
		const query = params.toString();
		await goto(query ? `${base}?${query}` : base, {
			replaceState: true,
			noScroll: true,
			keepFocus: true
		});
	}
</script>

<div class="project-section">
	<Conversations
		{projectId}
		page={currentPage}
		pageSize={20}
		showPagination={true}
		onPageChange={handlePageChange}
	/>
</div>

<style>
	.project-section {
		margin-bottom: 2rem;
	}
</style>
