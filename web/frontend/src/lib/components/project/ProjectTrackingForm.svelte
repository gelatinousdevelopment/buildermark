<script lang="ts">
	import type { ProjectTrackingOption } from '$lib/types';

	type Props = {
		heading: string;
		projects: ProjectTrackingOption[];
		loading: boolean;
		error: string | null;
		emptyMessage?: string;
		checkedPaths: string[];
		selectedHistoryDays: string;
		historyDayOptions: string[];
		saving: boolean;
		saveError: string | null;
		submitLabel?: string;
		submitDisabled?: boolean;
		onToggle: (projectPath: string, checked: boolean) => void;
		onHistoryDaysChange: (days: string) => void;
		onSubmit: () => void;
	};

	let {
		heading,
		projects,
		loading,
		error,
		emptyMessage = 'No projects found yet.',
		checkedPaths,
		selectedHistoryDays,
		historyDayOptions,
		saving,
		saveError,
		submitLabel = 'Import Projects',
		submitDisabled = false,
		onToggle,
		onHistoryDaysChange,
		onSubmit
	}: Props = $props();

	const selectedCount = $derived(checkedPaths.length);

	function projectName(project: ProjectTrackingOption): string {
		return project.label || project.path;
	}

	function pathTail(path: string): string {
		const normalized = path.replace(/[\\/]+$/, '');
		const parts = normalized.split(/[\\/]/);
		return parts[parts.length - 1] || path;
	}

	function previousLocationSuggestions(
		currentProject: ProjectTrackingOption
	): ProjectTrackingOption[] {
		const currentTail = pathTail(currentProject.path).toLowerCase();
		return projects.filter(
			(project) =>
				project.path !== currentProject.path && pathTail(project.path).toLowerCase() === currentTail
		);
	}

	function historyLabel(days: string): string {
		return days === 'all' ? 'All' : `${days} days`;
	}
</script>

<h3>{heading}</h3>
{#if loading}
	<p class="loading">
		<span class="spinner" aria-hidden="true"></span>
		Finding projects from agent conversations...
	</p>
{:else if error}
	<p class="error">{error}</p>
{:else if projects.length === 0}
	<p class="muted">{emptyMessage}</p>
{:else}
	<ul class="project-options">
		{#each projects as project (project.path)}
			<li>
				<label>
					<input
						type="checkbox"
						checked={checkedPaths.includes(project.path)}
						onchange={(event) =>
							onToggle(project.path, (event.currentTarget as HTMLInputElement).checked)}
					/>
					<span class="text">
						<span class="title">{projectName(project)}</span>
						<span class="subtitle">{project.path}</span>
						{#if project.missingOnDisk}
							<span class="status warning">Not found on disk</span>
						{:else if project.tracked}
							<span class="status">Tracked</span>
						{/if}
					</span>
				</label>
				{#if previousLocationSuggestions(project).length > 0}
					<ul class="suggestions">
						{#each previousLocationSuggestions(project) as suggestion (suggestion.path)}
							<li>
								<label>
									<input
										type="checkbox"
										checked={checkedPaths.includes(suggestion.path)}
										onchange={(event) =>
											onToggle(suggestion.path, (event.currentTarget as HTMLInputElement).checked)}
									/>
									<span>{suggestion.path}</span>
								</label>
							</li>
						{/each}
					</ul>
				{/if}
			</li>
		{/each}
	</ul>
	{#if saveError}
		<p class="error">{saveError}</p>
	{/if}
	<div class="import-settings">
		<label for="import-days-select">History to import</label>
		<select
			id="import-days-select"
			value={selectedHistoryDays}
			disabled={saving}
			onchange={(event) => onHistoryDaysChange((event.currentTarget as HTMLSelectElement).value)}
		>
			{#each historyDayOptions as option (option)}
				<option value={option}>{historyLabel(option)}</option>
			{/each}
		</select>
	</div>
	<button class="btn-sm" disabled={submitDisabled || saving} onclick={onSubmit}>
		{#if saving}
			<span class="spinner" aria-hidden="true"></span>
			Importing...
		{:else}
			{`${submitLabel} (${selectedCount})`}
		{/if}
	</button>
{/if}

<style>
	.loading {
		display: inline-flex;
		align-items: center;
		gap: 0.45rem;
	}

	.project-options {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		max-height: 30rem;
		overflow: auto;
	}

	.project-options > li {
		border: 0.5px solid var(--color-divider);
		border-radius: 8px;
		padding: 0.45rem 0.55rem;
	}

	.project-options label {
		display: flex;
		align-items: flex-start;
		gap: 0.45rem;
	}

	.project-options .text {
		display: flex;
		flex-direction: column;
		gap: 0.1rem;
	}

	.project-options .title {
		font-weight: 600;
	}

	.project-options .subtitle {
		opacity: 0.7;
		font-size: 0.85rem;
		font-family: var(--font-family-monospace);
	}

	.status {
		font-size: 0.75rem;
		opacity: 0.75;
	}

	.status.warning {
		color: #b45309;
		opacity: 1;
	}

	.suggestions {
		list-style: none;
		margin: 0.45rem 0 0 1.6rem;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}

	.suggestions span {
		font-size: 0.8rem;
		opacity: 0.75;
		font-family: var(--font-family-monospace);
	}

	.import-settings {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.import-settings select {
		padding: 0.25rem 0.5rem;
		border: 1px solid #ccc;
		border-radius: 4px;
		background: #fff;
		color: #444;
	}

	.spinner {
		width: 0.8rem;
		height: 0.8rem;
		border: 2px solid #bbb;
		border-top-color: #333;
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
</style>
