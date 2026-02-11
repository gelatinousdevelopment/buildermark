<script lang="ts">
	import { onMount } from 'svelte';
	import { resolve } from '$app/paths';
	import { listProjects, getProject, setProjectIgnored } from '$lib/api';
	import { stars, shortId } from '$lib/utils';
	import type { ProjectDetail } from '$lib/types';

	let projects: ProjectDetail[] = $state([]);
	let ignoredProjects: ProjectDetail[] = $state([]);
	let loading = $state(true);
	let error: string | null = $state(null);

	function latestRatingTimestamp(project: ProjectDetail): string {
		let latest = '';
		for (const conv of project.conversations) {
			for (const r of conv.ratings) {
				if (r.createdAt > latest) latest = r.createdAt;
			}
		}
		return latest;
	}

	function sortByRecentRating(list: ProjectDetail[]): ProjectDetail[] {
		return list.slice().sort((a, b) => {
			const ta = latestRatingTimestamp(a);
			const tb = latestRatingTimestamp(b);
			if (ta && !tb) return -1;
			if (!ta && tb) return 1;
			if (ta !== tb) return tb.localeCompare(ta);
			return a.path.localeCompare(b.path);
		});
	}

	async function load() {
		const [active, ignored] = await Promise.all([listProjects(false), listProjects(true)]);
		const [activeDetails, ignoredDetails] = await Promise.all([
			Promise.all(active.map((p) => getProject(p.id))),
			Promise.all(ignored.map((p) => getProject(p.id)))
		]);
		projects = sortByRecentRating(activeDetails);
		ignoredProjects = sortByRecentRating(ignoredDetails);
	}

	onMount(async () => {
		try {
			await load();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load projects';
		} finally {
			loading = false;
		}
	});

	async function ignoreProject(id: string) {
		await setProjectIgnored(id, true);
		const project = projects.find((p) => p.id === id);
		if (project) {
			projects = projects.filter((p) => p.id !== id);
			ignoredProjects = [...ignoredProjects, project];
		}
	}

	async function trackProject(id: string) {
		await setProjectIgnored(id, false);
		const project = ignoredProjects.find((p) => p.id === id);
		if (project) {
			ignoredProjects = ignoredProjects.filter((p) => p.id !== id);
			projects = [...projects, project];
		}
	}
</script>

{#if loading}
	<p class="loading">Loading projects...</p>
{:else if error}
	<p class="error">{error}</p>
{:else if projects.length === 0 && ignoredProjects.length === 0}
	<p>No projects found.</p>
{:else}
	{#each projects as project (project.id)}
		<div class="project-section">
			<div class="project-heading">
				<h2>{project.path}</h2>
				<button class="btn-sm" onclick={() => ignoreProject(project.id)}>Ignore</button>
			</div>
			{#if project.conversations.length === 0}
				<p>No conversations.</p>
			{:else}
				<table class="data">
					<thead>
						<tr>
							<th>Conversation</th>
							<th>Agent</th>
							<th>Ratings</th>
						</tr>
					</thead>
					<tbody>
						{#each project.conversations as conv (conv.id)}
							<tr>
								<td>
									<a href={resolve('/dashboard/conversations/[id]', { id: conv.id })}>
										{conv.title || shortId(conv.id)}
									</a>
								</td>
								<td>{conv.agent}</td>
								<td class="ratings">
									{#if conv.ratings.length > 0}
										{#each conv.ratings as r (r.id)}
											<span title={r.analysis ? `${r.note}\n\nAnalysis: ${r.analysis}` : r.note}
												>{stars(r.rating)}</span
											>
										{/each}
									{:else}
										—
									{/if}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			{/if}
		</div>
	{/each}

	{#if ignoredProjects.length > 0}
		<div class="ignored-section">
			<h3>Ignored Projects</h3>
			{#each ignoredProjects as project (project.id)}
				<div class="ignored-row">
					<span>{project.path}</span>
					<button class="btn-sm" onclick={() => trackProject(project.id)}>Track</button>
				</div>
			{/each}
		</div>
	{/if}
{/if}

<style>
	.project-section {
		margin-bottom: 2rem;
	}

	.project-heading {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-bottom: 0.5rem;
	}

	.project-heading h2 {
		font-size: 1.1rem;
		margin: 0;
		color: #333;
	}

	.ignored-section {
		margin-top: 2.5rem;
		padding-top: 1rem;
		border-top: 1px solid #e0e0e0;
	}

	.ignored-section h3 {
		font-size: 0.9rem;
		color: #999;
		text-transform: uppercase;
		letter-spacing: 0.03em;
		margin-bottom: 0.5rem;
	}

	.ignored-row {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.3rem 0;
		font-size: 0.85rem;
		color: #888;
	}

	.ratings {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
</style>
