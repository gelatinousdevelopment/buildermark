<script lang="ts">
	/* eslint-disable svelte/no-navigation-without-resolve */
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { SvelteURLSearchParams } from 'svelte/reactivity';
	import { listProjects, listSearchProjects } from '$lib/api';
	import Conversations from '$lib/components/project/Conversations.svelte';
	import Commits from '$lib/components/project/Commits.svelte';
	import type { Project, ProjectSearchMatch } from '$lib/types';

	const currentQuery = $derived(page.url.searchParams.get('q')?.trim() ?? '');
	const currentProjectId = $derived(page.url.searchParams.get('project') ?? '');

	let queryInput = $derived(currentQuery);
	let allProjects: Project[] = $state([]);
	let results: ProjectSearchMatch[] = $state([]);
	let loading = $state(false);
	let error: string | null = $state(null);
	let initialized = $state(false);
	let requestToken = 0;

	$effect(() => {
		if (initialized) return;
		initialized = true;
		void loadProjects();
	});

	$effect(() => {
		void loadSearchResults(currentQuery, currentProjectId);
	});

	async function loadProjects() {
		try {
			allProjects = await listProjects(false);
		} catch {
			allProjects = [];
		}
	}

	async function loadSearchResults(query: string, projectId: string) {
		if (!query) {
			results = [];
			error = null;
			loading = false;
			return;
		}
		const myToken = ++requestToken;
		loading = true;
		error = null;
		try {
			const loaded = await listSearchProjects(query, projectId);
			if (myToken !== requestToken) return;
			results = loaded;
		} catch (e) {
			if (myToken !== requestToken) return;
			error = e instanceof Error ? e.message : 'Failed to search projects';
			results = [];
		} finally {
			if (myToken === requestToken) loading = false;
		}
	}

	function updateUrl(nextQuery: string, nextProjectId: string) {
		const params = new SvelteURLSearchParams(page.url.searchParams);
		if (nextQuery.trim()) {
			params.set('q', nextQuery.trim());
		} else {
			params.delete('q');
		}
		if (nextProjectId) {
			params.set('project', nextProjectId);
		} else {
			params.delete('project');
		}
		const query = params.toString();
		const base = resolve('/search');
		void goto(query ? `${base}?${query}` : base, {
			replaceState: true,
			noScroll: true,
			keepFocus: true
		});
	}

	function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		updateUrl(queryInput, currentProjectId);
	}

	function handleProjectChange(event: Event) {
		const projectId = (event.currentTarget as HTMLSelectElement).value;
		updateUrl(queryInput, projectId);
	}

	function projectName(project: Project): string {
		return project.label || project.path;
	}
</script>

<div class="limited-content-width search-page">
	<form class="search-bar" onsubmit={handleSubmit}>
		<!-- svelte-ignore a11y_autofocus -->
		<input
			type="search"
			placeholder="Search prompts, commit subjects, diffs, and hashes"
			bind:value={queryInput}
			autofocus
		/>
		<select value={currentProjectId} onchange={handleProjectChange}>
			<option value="">All projects</option>
			{#each allProjects as project (project.id)}
				<option value={project.id}>{projectName(project)}</option>
			{/each}
		</select>
		<button class="bordered prominent" type="submit" style:min-width="90px" style:font-size="1.1rem"
			>Search</button
		>
	</form>

	{#if !currentQuery}
		<p class="message">Enter a search term to find matching conversations and commits.</p>
	{:else if loading}
		<p class="loading">Searching...</p>
	{:else if error}
		<p class="error">{error}</p>
	{:else if results.length === 0}
		<p class="message">No matching results found.</p>
	{:else}
		<div class="projects">
			{#each results as row, index (row.project.id)}
				<div class="project">
					<div class="meta">
						<div class="label">
							<a href={resolve('/projects/[project_id]', { project_id: row.project.id })}
								>{projectName(row.project)}</a
							>
						</div>
						<div class="right">
							<span>{row.conversationMatches} conversation matches</span>
							<span>&bull;</span>
							<span>{row.commitMatches} commit matches</span>
						</div>
					</div>
					<div class="content">
						<div class="column conversations">
							<div class="heading">
								<a
									href={resolve('/projects/[project_id]/conversations', {
										project_id: row.project.id
									})}>Agent Conversations</a
								>
							</div>
							<Conversations
								projectId={row.project.id}
								page={1}
								pageSize={20}
								limit={20}
								compact={true}
								showAgentColumn={true}
								showRatingsColumn={true}
								searchTerm={currentQuery}
								useLoadQueue={true}
								loadPriority={index * 2}
							/>
						</div>
						<div class="column commits">
							<Commits
								projectId={row.project.id}
								page={1}
								pageSize={20}
								limit={20}
								compact={true}
								showHeader={true}
								headerLink={resolve(`/projects/${encodeURIComponent(row.project.id)}/commits`)}
								showBranch={false}
								searchTerm={currentQuery}
								useLoadQueue={true}
								loadPriority={index * 2 + 1}
								defaultToCurrentUser={false}
							/>
						</div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

<style>
	.search-page {
		box-sizing: border-box;
		padding: 1rem;
		width: 100%;
	}

	.search-bar {
		display: flex;
		gap: 0.6rem;
		margin-bottom: 1rem;
	}

	.search-bar input {
		border: 1px solid var(--color-divider);
		border-radius: 5px;
		flex: 1;
		font-size: 1.2rem;
		min-width: 300px;
		padding: 1rem;
	}

	.search-bar select {
		border: 1px solid var(--color-divider);
		border-radius: 5px;
		font-size: 1.2rem;
		min-width: 300px;
		padding: 1rem;
	}

	@media (max-width: 900px) {
		.search-bar {
			flex-direction: column;
		}

		.search-bar select,
		.search-bar input,
		.search-bar button {
			width: 100%;
		}
	}

	.projects {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.project .content {
		background: #fff;
		border-radius: 12px;
		border: 0.5px solid var(--color-divider);
		display: flex;
		flex-direction: row;
	}

	@media (max-width: 1023px) {
		.project .content {
			flex-direction: column;
		}
	}

	.meta {
		display: flex;
		gap: 1rem;
		align-items: baseline;
		justify-content: space-between;
		padding: 0rem 1rem 0.7rem 1rem;
	}

	.meta .label a {
		color: var(--color-text);
		text-decoration: none;
		font-size: 1.6rem;
		font-weight: 300;
	}

	.meta .label a:hover {
		color: var(--accent-color);
		text-decoration: underline;
	}

	.meta .right {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		opacity: 0.75;
	}

	.project .column {
		min-height: 14rem;
		padding: 1rem 0 0.7rem 0;
		flex: 1;
		max-width: 50%;
	}

	@media (max-width: 1023px) {
		.project .column {
			max-width: 100%;
		}
	}

	.project .column .heading {
		font-weight: 600;
		text-transform: uppercase;
		font-size: 0.9rem;
		opacity: 0.5;
		margin-bottom: 0.75rem;
		margin-left: 1rem;
	}

	.project .column .heading a {
		color: inherit;
		text-decoration: none;
	}

	.project .column .heading a:hover {
		text-decoration: underline;
	}

	.commits {
		border-left: 0.5px solid var(--color-divider);
	}

	@media (max-width: 1023px) {
		.commits {
			border-left: 0;
			border-top: 0.5px solid var(--color-divider);
		}
	}

	.message,
	.loading,
	.error {
		padding: 2rem;
		text-align: center;
	}

	.message {
		opacity: 0.8;
	}
</style>
