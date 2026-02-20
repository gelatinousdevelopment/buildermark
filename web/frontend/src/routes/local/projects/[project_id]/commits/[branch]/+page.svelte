<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Commits from '$lib/components/project/Commits.svelte';

	const projectId = $derived(page.params.project_id ?? '');
	const branch = $derived(page.params.branch ?? '');

	async function handleBranchChange(nextBranch: string) {
		if (!projectId || !nextBranch) return;
		const base = resolve('/local/projects/[project_id]/commits/[branch]', {
			project_id: projectId,
			branch: nextBranch
		});
		await goto(base, {
			replaceState: true,
			noScroll: true,
			keepFocus: true
		});
	}
</script>

<Commits
	{projectId}
	{branch}
	pageSize={40}
	showBranchPicker={true}
	showCoverageBar={true}
	showPagination={true}
	showLoadMore={true}
	syncPaginationWithUrl={true}
	onBranchChange={handleBranchChange}
/>
