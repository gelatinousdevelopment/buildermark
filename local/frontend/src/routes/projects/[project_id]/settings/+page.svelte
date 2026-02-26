<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import {
		deleteProject,
		getProject,
		setProjectLabel,
		setProjectPath,
		setProjectOldPaths,
		setProjectIgnoreDiffPaths,
		setProjectIgnoreDefaultDiffPaths
	} from '$lib/api';
	import { websocketStore } from '$lib/stores/websocket.svelte';
	import type { ProjectDetail } from '$lib/types';
	import Icon from '$lib/Icon.svelte';

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
	let label = $state('');
	let path = $state('');
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
		return label || path || project.path;
	}

	function getProjectID(): string {
		return project ? project.id : '';
	}

	let projectDisplayName = $derived(getProjectDisplayName());
	let projectID = $derived(getProjectID());
	let recomputeStatusMessage = $derived.by(() => {
		const job = websocketStore.getJob('diff_recompute');
		if (!job || !projectID) return null;
		if (job.message?.includes(projectID)) return job.message;
		return null;
	});

	async function load() {
		const id = page.params.project_id;
		if (!id) throw new Error('Missing project ID');
		project = await getProject(id);
		label = project.label ?? '';
		path = project.path ?? '';
		oldPaths = project.oldPaths ?? '';
		ignoreDiffPaths = project.ignoreDiffPaths ?? '';
		ignoreDefaultDiffPaths = project.ignoreDefaultDiffPaths ?? true;
	}

	async function save() {
		if (!project) return;
		saving = true;
		error = null;
		notice = null;
		websocketStore.clearJob('diff_recompute');
		try {
			if (label) await setProjectLabel(project.id, label);
			await setProjectPath(project.id, path);
			await setProjectOldPaths(project.id, oldPaths);
			await setProjectIgnoreDiffPaths(project.id, ignoreDiffPaths);
			await setProjectIgnoreDefaultDiffPaths(project.id, ignoreDefaultDiffPaths);
			notice = 'Saved. Diff recompute started.';
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
			goto(resolve('/projects'));
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
		<h1>Project Settings</h1>

		<label class="field-label" for="project-label">Label</label>
		<input
			id="project-label"
			class="project-label"
			type="text"
			bind:value={label}
			placeholder="Project label"
		/>

		<label class="field-label" for="project-path">Path</label>
		<input
			id="project-path"
			type="text"
			bind:value={path}
			placeholder="/path/to/project"
			class="mono-input"
		/>

		<br />

		<label class="field-label" for="ignore-diff-paths">Ignore Paths for Diff Matching</label>
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
				<Icon name="info" width="15px" />
			</button>
		</div>
		{#if showDefaultPaths}
			<ul class="default-paths-list">
				{#each defaultPaths as p (p)}
					<li><code>{p}</code></li>
				{/each}
			</ul>
		{/if}
		<p class="hint">One glob path per line.</p>
		<textarea
			id="ignore-diff-paths"
			bind:value={ignoreDiffPaths}
			rows="10"
			spellcheck="false"
			placeholder="Glob patterns, one per line"
		></textarea>

		<br />
		<br />

		<label class="field-label" for="old-paths">Old Filesystem Paths</label>
		<p class="hint">
			Match conversations created from previous project locations. One absolute path per line.
		</p>
		<textarea
			id="old-paths"
			bind:value={oldPaths}
			rows="4"
			spellcheck="false"
			placeholder="/old/path/to/repo"
		></textarea>

		<div class="actions">
			<button class="bordered prominent" disabled={saving} onclick={save}
				>{saving ? 'Saving...' : 'Save Settings'}</button
			>
			{#if notice}
				<span class="notice">{notice}</span>
			{/if}
			{#if recomputeStatusMessage}
				<span class="notice">{recomputeStatusMessage}</span>
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
				<button class="bordered small" onclick={() => (showDeleteModal = false)}>Cancel</button>
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
		max-width: 700px;
		padding: 1.5rem;
	}

	h1 {
		margin: 0 0 1rem;
		font-size: 1.2rem;
	}

	.defaults-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-bottom: 0.5rem;
	}

	.checkbox-label {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		font-size: 0.9rem;
		cursor: pointer;
	}

	.info-btn {
		all: unset;
		color: var(--color-text);
	}

	.info-btn:hover {
		color: var(--accent-color);
	}

	.default-paths-list {
		margin: 0 0 1rem;
		padding-left: 1.5rem;
		font-size: 0.8rem;
		color: var(--color-text-secondary);
		line-height: 1.6;
	}

	.default-paths-list code {
		font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
		font-size: 0.78rem;
		background: var(--color-button-bg);
		padding: 0.1rem 0.3rem;
		border-radius: 2px;
	}

	.field-label {
		display: block;
		margin-bottom: 0.35rem;
		font-weight: 600;
	}

	input[type='text'] {
		width: 100%;
		padding: 0.4rem 0.6rem;
		font-size: 1rem;
		border: 1px solid var(--color-border-input);
		background: var(--color-background-surface);
		color: var(--color-text);
		border-radius: 4px;
		box-sizing: border-box;
		margin-bottom: 1rem;
	}

	input[type='text'].project-label {
		font-size: 1.3rem;
	}

	.mono-input {
		font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
		font-size: 0.9rem;
	}

	textarea {
		width: 100%;
		max-width: 100%;
		font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
		font-size: 0.9rem;
		line-height: 1.45;
		padding: 0.65rem;
		border: 1px solid var(--color-border-input);
		background: var(--color-background-surface);
		color: var(--color-text);
		border-radius: 4px;
		box-sizing: border-box;
	}

	.hint {
		margin-top: 0.35rem;
		font-size: 0.8rem;
		color: var(--color-text-faded);
	}

	.actions {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		margin-top: 0.9rem;
	}

	.notice {
		font-size: 0.85rem;
		color: var(--color-notice);
	}

	.danger-zone {
		margin-top: 2.5rem;
		padding-top: 1.5rem;
		border-top: 0.5px solid var(--color-divider);
	}

	.danger-zone h2 {
		margin: 0 0 0.5rem;
		font-size: 1rem;
		color: var(--color-danger-hover);
	}

	.danger-description {
		margin: 0 0 0.75rem;
		font-size: 0.85rem;
		color: var(--color-text-faded);
	}

	.btn-danger {
		padding: 0.35rem 0.8rem;
		font-size: 0.9rem;
		font-weight: bold;
		line-height: 1.4;
		border: 1px solid var(--color-danger);
		border-radius: 3px;
		background: var(--color-danger);
		color: #fff;
		cursor: pointer;
	}

	.btn-danger:hover {
		background: var(--color-danger-hover);
		border-color: var(--color-danger-hover);
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
		background: var(--color-modal-backdrop);
	}

	.modal-dialog {
		position: relative;
		background: var(--color-modal-bg);
		border-radius: 8px;
		padding: 1.5rem;
		max-width: 440px;
		width: 90%;
		box-shadow: 0 4px 24px var(--color-popover-shadow);
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
		font-size: 1rem;
		border: 1px solid var(--color-border-input);
		background: var(--color-background-surface);
		color: var(--color-text);
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
