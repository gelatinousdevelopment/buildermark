<script lang="ts">
	import ProjectTrackingForm from '$lib/components/project/ProjectTrackingForm.svelte';
	import type { ImportableProject, ProjectTrackingOption } from '$lib/types';
	import { resolve } from '$app/paths';

	type Props = {
		detectedProjects: ImportableProject[];
		detectedLoading: boolean;
		detectedError: string | null;
		selectedProjectPaths: string[];
		selectedHistoryDays: string;
		historyDayOptions: string[];
		savingSelection: boolean;
		saveSelectionError: string | null;
		importStatusMessage?: string | null;
		onToggleSelection: (projectPath: string, checked: boolean) => void;
		onHistoryDaysChange: (days: string) => void;
		onStartTrackingSelected: () => void;
	};

	let {
		detectedProjects,
		detectedLoading,
		detectedError,
		selectedProjectPaths,
		selectedHistoryDays,
		historyDayOptions,
		savingSelection,
		saveSelectionError,
		importStatusMessage = null,
		onToggleSelection,
		onHistoryDaysChange,
		onStartTrackingSelected
	}: Props = $props();

	const trackingOptions = $derived(
		detectedProjects
			.filter((project) => !project.tracked)
			.map(
				(project): ProjectTrackingOption => ({
					path: project.path,
					label: project.label,
					projectId: project.projectId,
					tracked: project.tracked,
					importable: true,
					missingOnDisk: false
				})
			)
	);

	const emptyMessage = $derived(
		detectedProjects.length > 0 && trackingOptions.length === 0
			? 'All detected projects are already tracked.'
			: 'No detected projects found yet.'
	);
</script>

<div class="onboarding">
	<div class="column left">
		<h2>Welcome to Buildermark Local</h2>
		<p>
			Track projects to import your conversation history with agents like Claude and Codex, along
			with git commits.
		</p>
		<p>
			<a href={resolve('/plugins')} target="_blank">Install the plugins</a>
			for your agents, so you can rate and log analysis about each AI coding session by running the
			<code>brate</code> skill.
		</p>
		<h3>Rate a conversation in Claude Code:</h3>
		<code class="bigger">› /brate</code>
		<h3>Rate a conversation in Codex:</h3>
		<code class="bigger">› $brate</code>
		<p>
			If you have questions or encounter a bug, start a discussion or file a bug report on github: <a
				href="https://github.com/gelatinousdevelopment/buildermark"
				target="_blank">github.com/gelatinousdevelopment/buildermark</a
			>
		</p>
	</div>
	<hr />
	<div class="column right">
		<ProjectTrackingForm
			heading="Select projects to track"
			projects={trackingOptions}
			loading={detectedLoading}
			error={detectedError}
			{emptyMessage}
			checkedPaths={selectedProjectPaths}
			{selectedHistoryDays}
			{historyDayOptions}
			saving={savingSelection}
			saveError={saveSelectionError}
			{importStatusMessage}
			submitLabel={selectedProjectPaths.length == 1 ? 'Import Project' : 'Import Projects'}
			submitDisabled={selectedProjectPaths.length === 0}
			onToggle={onToggleSelection}
			{onHistoryDaysChange}
			onSubmit={onStartTrackingSelected}
		/>
	</div>
</div>

<style>
	.onboarding {
		display: flex;
		flex-direction: row;
		padding: 1.2rem;
		gap: 2.5rem;
	}

	@media (max-width: 768px) {
		.onboarding {
			flex-direction: column;
			gap: 1rem;
		}
	}

	.onboarding h2 {
		margin: 0;
	}

	.onboarding h3 {
		color: var(--accent-color-darkest);
		font-size: 1em;
		font-weight: 600;
		margin: 0;
	}

	.onboarding code {
		background: var(--color-background-empty);
		font-size: 0.9em;
		padding: 0.3em 0.5em;
		border-radius: 3px;
	}
	.onboarding code.bigger {
		font-size: 1em;
		padding: 0.5em 0.8em;
	}

	.onboarding .left {
		display: flex;
		flex: 1;
		flex-direction: column;
		gap: 0.7rem;
	}

	.onboarding .left p {
		margin: 0;
		font-size: 1rem;
		line-height: 1.45;
	}

	.onboarding hr {
		background: var(--color-divider);
		border: 0;
		margin: 0;
		min-width: 0.5px;
		width: 0.5px;
	}

	.onboarding .right {
		display: flex;
		flex: 1;
		flex-direction: column;
		gap: 0.8rem;
	}

	@media (max-width: 900px) {
		.onboarding {
			grid-template-columns: 1fr;
		}
	}
</style>
