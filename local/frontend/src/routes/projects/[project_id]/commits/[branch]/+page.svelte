<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { SvelteURLSearchParams } from 'svelte/reactivity';
	import Commits from '$lib/components/project/Commits.svelte';
	import type { ProjectCommitPagination } from '$lib/types';

	const projectId = $derived(page.params.project_id ?? '');
	const branch = $derived(page.params.branch ?? '');
	let commitPagination = $state<ProjectCommitPagination | null>(null);

	const currentPage = $derived.by(() => {
		const raw = page.url.searchParams.get('page');
		if (!raw) return 1;
		const parsed = Number.parseInt(raw, 10);
		return Number.isInteger(parsed) && parsed > 0 ? parsed : 1;
	});
	const currentUser = $derived(page.url.searchParams.get('user') ?? undefined);
	const currentAgent = $derived(page.url.searchParams.get('agent') ?? undefined);
	const currentOrder = $derived(page.url.searchParams.get('order') === 'asc' ? 'asc' : undefined);
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
		if (!projectId || !branch) return;
		const params = new SvelteURLSearchParams(page.url.searchParams);
		for (const [key, value] of Object.entries(updates)) {
			if (value === null || value === '') {
				params.delete(key);
			} else {
				params.set(key, value);
			}
		}
		const query = params.toString();
		void goto(
			`${resolve('/projects/[project_id]/commits/[branch]', {
				project_id: projectId,
				branch: encodeURIComponent(branch)
			})}${query ? `?${query}` : ''}`,
			{
				replaceState: true,
				noScroll: true,
				keepFocus: true
			}
		);
	}

	function pageHref(nextPage: number) {
		const params = new SvelteURLSearchParams(page.url.searchParams);
		if (nextPage <= 1) {
			params.delete('page');
		} else {
			params.set('page', String(nextPage));
		}
		const query = params.toString();
		return `${resolve('/projects/[project_id]/commits/[branch]', {
			project_id: projectId,
			branch: encodeURIComponent(branch)
		})}${query ? `?${query}` : ''}`;
	}

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

	function handleUserChange(nextUser: string) {
		updateUrl({ user: nextUser || null, page: null });
	}

	function handleAgentChange(nextAgent: string) {
		updateUrl({ agent: nextAgent || null, page: null });
	}

	function handleOrderChange(nextOrder: string) {
		updateUrl({ order: nextOrder === 'asc' ? 'asc' : null, page: null });
	}

	function handleDateChange(startIso: string | null, endIso: string | null) {
		updateUrl({ start: startIso, end: endIso, page: null });
	}

	function handleCommitsDataLoaded(data: { pagination: ProjectCommitPagination }) {
		commitPagination = data.pagination;
	}
</script>

<div class="project-section">
	<Commits
		{projectId}
		{branch}
		page={currentPage}
		pageSize={40}
		user={currentUser}
		agent={currentAgent}
		order={currentOrder}
		start={startMs}
		end={endMs}
		showBranchPicker={true}
		showUserPicker={true}
		showAgentPicker={true}
		showDateFilter={true}
		showCoverageBar={true}
		showLoadMore={true}
		showBranch={true}
		showUser={true}
		onBranchChange={handleBranchChange}
		onUserChange={handleUserChange}
		onAgentChange={handleAgentChange}
		onOrderChange={handleOrderChange}
		onDateChange={handleDateChange}
		onCommitsDataLoaded={handleCommitsDataLoaded}
	/>

	{#if commitPagination && commitPagination.totalPages > 1}
		<div class="pager">
			{#if currentPage > 1}
				<a class="bordered small" href={pageHref(currentPage - 1)}>Previous</a>
			{:else}
				<span class="bordered small pager-disabled">Previous</span>
			{/if}
			<span>Page {commitPagination.page} of {commitPagination.totalPages}</span>
			{#if currentPage < commitPagination.totalPages}
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
