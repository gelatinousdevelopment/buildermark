<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import {
		deleteProject,
		getProject,
		setProjectOldPaths,
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
	let oldPaths = $state('');
	let ignoreDiffPaths = $state('');
	let ignoreDefaultDiffPaths = $state(true);
	let showDefaultPaths = $state(false);
	let loading = $state(true);
	let saving = $state(false);
	let error: string | null = $state(null);
	let notice: string | null = $state(null);

	let showDeleteModal = $state(false);
	let deleteConfirmName = $state('');
	let deleting = $state(false);
	let deleteError: string | null = $state(null);

	function getProjectDisplayName(): string {
		if (!project) return '';
		return project.label || project.path;
	}

	let projectDisplayName = $derived(getProjectDisplayName());

	async function load() {
		const id = page.params.project_id;
		if (!id) throw new Error('Missing project ID');
		project = await getProject(id);
		oldPaths = project.oldPaths ?? '';
		ignoreDiffPaths = project.ignoreDiffPaths ?? '';
		ignoreDefaultDiffPaths = project.ignoreDefaultDiffPaths ?? true;
	}

	async function save() {
		if (!project) return;
		saving = true;
		error = null;
		notice = null;
		try {
			await setProjectOldPaths(project.id, oldPaths);
			await setProjectIgnoreDiffPaths(project.id, ignoreDiffPaths);
			await setProjectIgnoreDefaultDiffPaths(project.id, ignoreDefaultDiffPaths);
			notice = 'Saved';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to save settings';
		} finally {
			saving = false;
		}
	}

	async function confirmDeleteProject() {
		if (!project) return;
		deleting = true;
		deleteError = null;
		try {
			await deleteProject(project.id);
			goto(resolve('/local/projects'));
		} catch (e) {
			deleteError = e instanceof Error ? e.message : 'Failed to delete project';
		} finally {
			deleting = false;
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

		<label class="field-label" for="old-paths">Old Paths</label>
		<textarea
			id="old-paths"
			bind:value={oldPaths}
			rows="6"
			spellcheck="false"
			placeholder="/old/path/to/repo"
		></textarea>
		<p class="hint">One absolute path per line for previous project locations.</p>

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

		<div class="danger-zone">
			<h2>Danger Zone</h2>
			<p class="danger-description">
				Permanently delete this project and all its data, including conversations, messages,
				ratings, and commits.
			</p>
			<button class="btn-danger" onclick={() => (showDeleteModal = true)}>Delete Project</button>
		</div>
	{/if}
</div>

{#if showDeleteModal}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="modal-overlay" onkeydown={(e) => e.key === 'Escape' && (showDeleteModal = false)}>
		<!-- svelte-ignore a11y_click_events_have_key_events -->
		<div class="modal-backdrop" onclick={() => (showDeleteModal = false)}></div>
		<div class="modal-dialog">
			<h3>Delete Project</h3>
			<p>
				This will permanently delete <strong>{projectDisplayName}</strong> and all associated data. This
				action cannot be undone.
			</p>
			<label class="field-label" for="delete-confirm">
				Type <strong>{projectDisplayName}</strong> to confirm:
			</label>
			<input
				id="delete-confirm"
				type="text"
				bind:value={deleteConfirmName}
				placeholder={projectDisplayName}
				autocomplete="off"
			/>
			{#if deleteError}
				<p class="error">{deleteError}</p>
			{/if}
			<div class="modal-actions">
				<button class="btn-sm" onclick={() => (showDeleteModal = false)}>Cancel</button>
				<button
					class="btn-danger"
					disabled={deleteConfirmName !== projectDisplayName || deleting}
					onclick={confirmDeleteProject}
				>
					{deleting ? 'Deleting...' : 'Delete Project'}
				</button>
			</div>
		</div>
	</div>
{/if}

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

	.danger-zone {
		margin-top: 2.5rem;
		padding-top: 1.5rem;
		border-top: 1px solid #e5c0c0;
	}

	.danger-zone h2 {
		margin: 0 0 0.5rem;
		font-size: 1rem;
		color: #b91c1c;
	}

	.danger-description {
		margin: 0 0 0.75rem;
		font-size: 0.85rem;
		color: #777;
	}

	.btn-danger {
		padding: 0.35rem 0.8rem;
		font-size: 0.8rem;
		line-height: 1.4;
		border: 1px solid #dc2626;
		border-radius: 3px;
		background: #dc2626;
		color: #fff;
		cursor: pointer;
	}

	.btn-danger:hover {
		background: #b91c1c;
		border-color: #b91c1c;
	}

	.btn-danger:disabled {
		opacity: 0.5;
		cursor: default;
	}

	.modal-overlay {
		position: fixed;
		inset: 0;
		z-index: 1000;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.modal-backdrop {
		position: absolute;
		inset: 0;
		background: rgba(0, 0, 0, 0.4);
	}

	.modal-dialog {
		position: relative;
		background: #fff;
		border-radius: 8px;
		padding: 1.5rem;
		max-width: 440px;
		width: 90%;
		box-shadow: 0 4px 24px rgba(0, 0, 0, 0.15);
	}

	.modal-dialog h3 {
		margin: 0 0 0.75rem;
		font-size: 1.1rem;
	}

	.modal-dialog p {
		margin: 0 0 0.75rem;
		font-size: 0.9rem;
		line-height: 1.45;
	}

	.modal-dialog input {
		width: 100%;
		padding: 0.4rem 0.6rem;
		font-size: 0.85rem;
		border: 1px solid #ccc;
		border-radius: 4px;
		box-sizing: border-box;
	}

	.modal-actions {
		display: flex;
		justify-content: flex-end;
		gap: 0.5rem;
		margin-top: 1rem;
	}
</style>
