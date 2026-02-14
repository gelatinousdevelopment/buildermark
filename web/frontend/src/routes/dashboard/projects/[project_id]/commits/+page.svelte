<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { listProjectCommitsPage, ingestMoreCommits, getCommitIngestionStatus } from '$lib/api';
	import type { ProjectCommitPageResponse, CommitIngestionStatusResponse } from '$lib/types';

	let data: ProjectCommitPageResponse | null = $state(null);
	let ingestionStatus: CommitIngestionStatusResponse | null = $state(null);
	let loading = $state(true);
	let error: string | null = $state(null);
	let loadMoreCount = $state(20);
	let loadingMore = $state(false);
	let loadMoreError: string | null = $state(null);

	function percent(value: number): string {
		return `${value.toFixed(1)}%`;
	}

	function formatTime(unixMs: number): string {
		return new Date(unixMs).toLocaleString();
	}

	async function load(pageNum: number) {
		const projectId = page.params.project_id;
		if (!projectId) throw new Error('Missing project ID');
		data = await listProjectCommitsPage(projectId, pageNum);
	}

	async function loadIngestionStatus() {
		const projectId = page.params.project_id;
		if (!projectId) return;
		try {
			ingestionStatus = await getCommitIngestionStatus(projectId);
		} catch {
			// Non-critical, just hide the load more button.
			ingestionStatus = null;
		}
	}

	async function goToPage(pageNum: number) {
		if (!data) return;
		if (pageNum < 1 || pageNum > data.pagination.totalPages) return;
		loading = true;
		error = null;
		try {
			await load(pageNum);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load commit coverage';
		} finally {
			loading = false;
		}
	}

	async function handleLoadMore() {
		const projectId = page.params.project_id;
		if (!projectId || loadingMore) return;
		loadingMore = true;
		loadMoreError = null;
		try {
			await ingestMoreCommits(projectId, loadMoreCount);
			// Reload current page and status.
			await load(data?.pagination.page ?? 1);
			await loadIngestionStatus();
		} catch (e) {
			loadMoreError = e instanceof Error ? e.message : 'Failed to load more commits';
		} finally {
			loadingMore = false;
		}
	}

	onMount(async () => {
		try {
			await load(1);
			await loadIngestionStatus();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load commit coverage';
		} finally {
			loading = false;
		}
	});
</script>

<div class="breadcrumb">
	<a href={resolve('/dashboard')}>Dashboard</a> &rsaquo;
	<a href={resolve('/dashboard/commits')}>Commits</a> &rsaquo; Project
</div>

{#if loading}
	<p class="loading">Loading commits...</p>
{:else if error}
	<p class="error">{error}</p>
{:else if !data || data.commits.length === 0}
	<p>No commits found for this project and current git user.</p>
{:else}
	<h2>{data.project.label || data.project.path}</h2>
	<p>{data.project.path}</p>

	<section class="summary-grid">
		<div class="summary-card">
			<div class="summary-label">Current User</div>
			<div class="summary-value">{data.currentUser || 'Unknown'}</div>
			{#if data.currentEmail}
				<div class="summary-subtle">{data.currentEmail}</div>
			{/if}
		</div>
		<div class="summary-card">
			<div class="summary-label">Coverage (Lines)</div>
			<div class="summary-value">{percent(data.summary.linePercent)}</div>
			<div class="summary-subtle">{data.summary.linesFromAgent} / {data.summary.linesTotal}</div>
		</div>
		<div class="summary-card">
			<div class="summary-label">Coverage (Characters)</div>
			<div class="summary-value">{percent(data.summary.characterPercent)}</div>
			<div class="summary-subtle">{data.summary.charsFromAgent} / {data.summary.charsTotal}</div>
		</div>
	</section>

	<table class="data">
		<thead>
			<tr>
				<th>Time</th>
				<th>Commit</th>
				<th>Lines</th>
				<th>Chars</th>
			</tr>
		</thead>
		<tbody>
			{#each data.commits as c (c.commitHash)}
				<tr>
					<td>{formatTime(c.authoredAtUnixMs)}</td>
					<td>
						<div>
							<a
								href={resolve('/dashboard/projects/[project_id]/commits/[commit_hash]', {
									project_id: c.projectId,
									commit_hash: c.commitHash
								})}
							>
								{c.subject || c.commitHash.slice(0, 8)}
							</a>
						</div>
						{#if !c.workingCopy}
							<div class="commit-meta">{c.commitHash.slice(0, 12)}</div>
						{/if}
					</td>
					<td>{c.linesFromAgent} / {c.linesTotal} ({percent(c.linePercent)})</td>
					<td>{c.charsFromAgent} / {c.charsTotal} ({percent(c.characterPercent)})</td>
				</tr>
			{/each}
		</tbody>
	</table>

	{#if data.pagination.totalPages > 1}
		<div class="pager">
			<button
				class="btn-sm"
				disabled={(data?.pagination.page ?? 1) <= 1}
				onclick={() => goToPage((data?.pagination.page ?? 1) - 1)}
			>
				Previous
			</button>
			<span>Page {data.pagination.page} of {data.pagination.totalPages}</span>
			<button
				class="btn-sm"
				disabled={(data?.pagination.page ?? 1) >= (data?.pagination.totalPages ?? 1)}
				onclick={() => goToPage((data?.pagination.page ?? 1) + 1)}
			>
				Next
			</button>
		</div>
	{/if}

	{#if ingestionStatus && !ingestionStatus.reachedRoot}
		<div class="load-more">
			<span class="load-more-info">
				{ingestionStatus.ingestedCount} of {ingestionStatus.totalGitCommits} commits loaded
			</span>
			<div class="load-more-controls">
				<label>
					Load
					<input
						type="number"
						min="1"
						max="500"
						bind:value={loadMoreCount}
						class="load-more-input"
						disabled={loadingMore}
					/>
					more
				</label>
				<button class="btn-sm" onclick={handleLoadMore} disabled={loadingMore}>
					{loadingMore ? 'Loading...' : 'Load'}
				</button>
			</div>
			{#if loadMoreError}
				<p class="error">{loadMoreError}</p>
			{/if}
		</div>
	{/if}
{/if}

<style>
	.summary-grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
		gap: 0.8rem;
		margin-bottom: 1rem;
	}

	.summary-card {
		border: 1px solid #e6e6e6;
		border-radius: 6px;
		padding: 0.8rem;
		background: #fbfbfb;
	}

	.summary-label {
		font-size: 0.78rem;
		text-transform: uppercase;
		letter-spacing: 0.03em;
		color: #777;
		margin-bottom: 0.35rem;
	}

	.summary-value {
		font-size: 1.3rem;
		font-weight: 600;
		color: #222;
	}

	.summary-subtle {
		margin-top: 0.2rem;
		font-size: 0.8rem;
		color: #777;
	}

	.commit-meta {
		color: #777;
		font-size: 0.78rem;
	}

	.pager {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-top: 1rem;
	}

	.load-more {
		margin-top: 1.2rem;
		padding: 0.8rem;
		border: 1px solid #e6e6e6;
		border-radius: 6px;
		background: #fbfbfb;
	}

	.load-more-info {
		font-size: 0.85rem;
		color: #666;
		display: block;
		margin-bottom: 0.5rem;
	}

	.load-more-controls {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.load-more-controls label {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		font-size: 0.9rem;
	}

	.load-more-input {
		width: 5rem;
		padding: 0.25rem 0.4rem;
		border: 1px solid #ccc;
		border-radius: 4px;
		font-size: 0.9rem;
		text-align: center;
	}
</style>
