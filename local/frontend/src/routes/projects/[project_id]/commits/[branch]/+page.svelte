<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Commits from '$lib/components/project/Commits.svelte';

	const projectId = $derived(page.params.project_id ?? '');
	const branch = $derived(page.params.branch ?? '');

	async function handleBranchChange(nextBranch: string) {
		if (!projectId || !nextBranch) return;
		const base = resolve('/projects/[project_id]/commits/[branch]', {
			project_id: projectId,
			branch: encodeURIComponent(nextBranch)
		});
		await goto(base, {
			replaceState: true,
			noScroll: true,
			keepFocus: true
		});
	}
</script>

<div class="project-section">
	<Commits
		{projectId}
		{branch}
		pageSize={40}
		showBranchPicker={true}
		showUserPicker={true}
		showAgentPicker={true}
		showDateFilter={true}
		showCoverageBar={true}
		showPagination={true}
		showLoadMore={true}
		syncPaginationWithUrl={true}
		showBranch={true}
		showUser={true}
		onBranchChange={handleBranchChange}
	/>
</div>

<style>
	.project-section {
		margin: 0.5rem 0 1rem 0;
	}
</style>
