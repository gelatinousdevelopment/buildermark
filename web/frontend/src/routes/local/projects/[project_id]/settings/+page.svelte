<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import {
		getProject,
		setProjectIgnoreDiffPaths,
		setProjectIgnoreDefaultDiffPaths
	} from '$lib/api';
	import type { ProjectDetail } from '$lib/types';

	const defaultPaths = [
		'**/.git/**',
		'**/.next/**',
		'**/.nuxt/**',
		'**/__pycache__/**',
		'**/node_modules/**',
		'*.map',
		'*.min.css',
		'*.min.js',
		'bun.lockb',
		'Cargo.lock',
		'composer.lock',
		'Gemfile.lock',
		'go.sum',
		'npm-shrinkwrap.json',
		'package-lock.json',
		'packages.lock.json',
		'paket.lock',
		'pdm.lock',
		'Pipfile.lock',
		'pnpm-lock.yaml',
		'poetry.lock',
		'yarn.lock'
	];

	let project: ProjectDetail | null = $state(null);
	let ignoreDiffPaths = $state('');
	let ignoreDefaultDiffPaths = $state(true);
	let showDefaultPaths = $state(false);
	let loading = $state(true);
	let saving = $state(false);
	let error: string | null = $state(null);
	let notice: string | null = $state(null);

	async function load() {
		const id = page.params.project_id;
		if (!id) throw new Error('Missing project ID');
		project = await getProject(id);
		ignoreDiffPaths = project.ignoreDiffPaths ?? '';
		ignoreDefaultDiffPaths = project.ignoreDefaultDiffPaths ?? true;
	}

	async function save() {
		if (!project) return;
		saving = true;
		error = null;
		notice = null;
		try {
			await setProjectIgnoreDiffPaths(project.id, ignoreDiffPaths);
			await setProjectIgnoreDefaultDiffPaths(project.id, ignoreDefaultDiffPaths);
			notice = 'Saved';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to save settings';
		} finally {
			saving = false;
		}
	}

	onMount(async () => {
		try {
			await load();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load project settings';
		} finally {
			loading = false;
		}
	});
</script>

<div class="page">
	{#if loading}
		<p class="loading">Loading settings...</p>
	{:else if error && !project}
		<p class="error">{error}</p>
	{:else if project}
		<h1>{project.label || project.path}</h1>
		{#if project.label}
			<p class="project-path">{project.path}</p>
		{/if}

		<div class="defaults-row">
			<label class="checkbox-label">
				<input type="checkbox" bind:checked={ignoreDefaultDiffPaths} />
				Ignore default paths
			</label>
			<button
				class="info-btn"
				title="Show default paths"
				onclick={() => (showDefaultPaths = !showDefaultPaths)}
			>
				{showDefaultPaths ? 'hide' : 'info'}
			</button>
		</div>
		{#if showDefaultPaths}
			<ul class="default-paths-list">
				{#each defaultPaths as p (p)}
					<li><code>{p}</code></li>
				{/each}
			</ul>
		{/if}

		<label class="field-label" for="ignore-diff-paths">Ignore Diff Paths</label>
		<textarea
			id="ignore-diff-paths"
			bind:value={ignoreDiffPaths}
			rows="14"
			spellcheck="false"
			placeholder="Glob patterns, one per line"
		></textarea>
		<p class="hint">One glob path per line.</p>

		<div class="actions">
			<button class="btn-sm" disabled={saving} onclick={save}
				>{saving ? 'Saving...' : 'Save'}</button
			>
			{#if notice}
				<span class="notice">{notice}</span>
			{/if}
		</div>
		{#if error}
			<p class="error">{error}</p>
		{/if}
	{/if}
</div>

<style>
	.page {
		max-width: 920px;
		padding: 1.5rem;
	}

	h1 {
		margin: 0;
		font-size: 1.2rem;
	}

	.project-path {
		margin: 0.35rem 0 1rem;
		color: #888;
		font-size: 0.85rem;
	}

	.defaults-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-bottom: 1rem;
	}

	.checkbox-label {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		font-weight: 600;
		font-size: 0.9rem;
		cursor: pointer;
	}

	.info-btn {
		padding: 0.1rem 0.4rem;
		font-size: 0.7rem;
		line-height: 1.4;
		border: 1px solid #ccc;
		border-radius: 3px;
		background: #fafafa;
		color: #777;
		cursor: pointer;
	}

	.info-btn:hover {
		background: #eee;
		border-color: #bbb;
		color: #333;
	}

	.default-paths-list {
		margin: 0 0 1rem;
		padding-left: 1.5rem;
		font-size: 0.8rem;
		color: #666;
		line-height: 1.6;
	}

	.default-paths-list code {
		font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
		font-size: 0.78rem;
		background: #f5f5f5;
		padding: 0.1rem 0.3rem;
		border-radius: 2px;
	}

	.field-label {
		display: block;
		margin-bottom: 0.35rem;
		font-weight: 600;
	}

	textarea {
		width: 100%;
		max-width: 100%;
		font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
		font-size: 0.85rem;
		line-height: 1.45;
		padding: 0.65rem;
		border: 1px solid #ccc;
		border-radius: 4px;
		box-sizing: border-box;
	}

	.hint {
		margin-top: 0.35rem;
		font-size: 0.8rem;
		color: #777;
	}

	.actions {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		margin-top: 0.9rem;
	}

	.btn-sm {
		padding: 0.3rem 0.7rem;
		font-size: 0.8rem;
		line-height: 1.4;
		border: 1px solid #ccc;
		border-radius: 3px;
		background: #fafafa;
		color: #555;
		cursor: pointer;
	}

	.btn-sm:hover {
		background: #eee;
		border-color: #bbb;
		color: #333;
	}

	.btn-sm:disabled {
		opacity: 0.6;
		cursor: default;
	}

	.notice {
		font-size: 0.85rem;
		color: #1d6d1d;
	}
</style>
