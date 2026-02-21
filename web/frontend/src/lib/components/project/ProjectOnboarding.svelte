<script lang="ts">
	import type { Project } from '$lib/types';

	type Props = {
		detectedProjects: Project[];
		detectedLoading: boolean;
		detectedError: string | null;
		selectedProjectIds: string[];
		savingSelection: boolean;
		saveSelectionError: string | null;
		onToggleSelection: (projectId: string, checked: boolean) => void;
		onStartTrackingSelected: () => void;
	};

	let {
		detectedProjects,
		detectedLoading,
		detectedError,
		selectedProjectIds,
		savingSelection,
		saveSelectionError,
		onToggleSelection,
		onStartTrackingSelected
	}: Props = $props();

	const selectedCount = $derived(selectedProjectIds.length);

	function projectName(project: Project): string {
		return project.label || project.path;
	}

	function pathTail(path: string): string {
		const normalized = path.replace(/[\\/]+$/, '');
		const parts = normalized.split(/[\\/]/);
		return parts[parts.length - 1] || path;
	}

	function previousLocationSuggestions(currentProject: Project): Project[] {
		const currentTail = pathTail(currentProject.path).toLowerCase();
		return detectedProjects.filter(
			(project) =>
				project.id !== currentProject.id && pathTail(project.path).toLowerCase() === currentTail
		);
	}
</script>

<div class="onboarding inset-when-limited-content-width">
	<div class="column left">
		<h2>Welcome to BuilderBit Local</h2>
		<p>
			Track projects to see agent conversations and commit attribution side-by-side in one
			dashboard.
		</p>
		<p class="muted">
			We found projects from your agent conversation folders. Choose what to track now—you can
			always change this later in settings.
		</p>
	</div>
	<div class="column right">
		<h3>Select projects to track</h3>
		{#if detectedLoading}
			<p class="loading">Finding projects from agent conversations…</p>
		{:else if detectedError}
			<p class="error">{detectedError}</p>
		{:else if detectedProjects.length === 0}
			<p class="muted">No detected projects found yet.</p>
		{:else}
			<ul class="project-options">
				{#each detectedProjects as project (project.id)}
					<li>
						<label>
							<input
								type="checkbox"
								checked={selectedProjectIds.includes(project.id)}
								onchange={(event) =>
									onToggleSelection(project.id, (event.currentTarget as HTMLInputElement).checked)}
							/>
							<span class="text">
								<span class="title">{projectName(project)}</span>
								<span class="subtitle">{project.path}</span>
							</span>
						</label>
						{#if previousLocationSuggestions(project).length > 0}
							<ul class="suggestions">
								{#each previousLocationSuggestions(project) as suggestion (suggestion.id)}
									<li>
										<label>
											<input
												type="checkbox"
												checked={selectedProjectIds.includes(suggestion.id)}
												onchange={(event) =>
													onToggleSelection(
														suggestion.id,
														(event.currentTarget as HTMLInputElement).checked
													)}
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
			{#if saveSelectionError}
				<p class="error">{saveSelectionError}</p>
			{/if}
			<button
				class="btn-sm"
				disabled={selectedCount === 0 || savingSelection}
				onclick={onStartTrackingSelected}
			>
				{savingSelection
					? 'Saving…'
					: `Track ${selectedCount || ''} selected project${selectedCount === 1 ? '' : 's'}`}
			</button>
		{/if}
	</div>
</div>

<style>
	.onboarding {
		display: grid;
		grid-template-columns: 40% 60%;
		padding: 1.2rem;
		gap: 1.2rem;
	}

	.onboarding h2,
	.onboarding h3 {
		margin: 0;
	}

	.onboarding .left {
		display: flex;
		flex-direction: column;
		gap: 0.7rem;
	}

	.onboarding .left p {
		margin: 0;
		font-size: 1rem;
		line-height: 1.45;
	}

	.onboarding .muted {
		opacity: 0.75;
	}

	.onboarding .right {
		display: flex;
		flex-direction: column;
		gap: 0.8rem;
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

	@media (max-width: 900px) {
		.onboarding {
			grid-template-columns: 1fr;
		}
	}
</style>
