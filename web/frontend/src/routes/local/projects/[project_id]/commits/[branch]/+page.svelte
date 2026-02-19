<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Commits from '$lib/components/project/Commits.svelte';

	const projectId = $derived(page.params.project_id ?? '');
	const branch = $derived(page.params.branch ?? '');
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
		const base = resolve('/local/projects/[project_id]/commits/[branch]', {
			project_id: projectId,
			branch
		});
		const query = params.toString();
		await goto(query ? `${base}?${query}` : base, {
			replaceState: true,
			noScroll: true,
			keepFocus: true
		});
	}

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
	page={currentPage}
	showBranchPicker={true}
	showCoverageBar={true}
	showPagination={true}
	showLoadMore={true}
	onPageChange={handlePageChange}
	onBranchChange={handleBranchChange}
/>
