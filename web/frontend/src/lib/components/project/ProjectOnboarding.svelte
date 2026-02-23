<script lang="ts">
	import ProjectTrackingForm from '$lib/components/project/ProjectTrackingForm.svelte';
	import type { ImportableProject, ProjectTrackingOption } from '$lib/types';

	type Props = {
		detectedProjects: ImportableProject[];
		detectedLoading: boolean;
		detectedError: string | null;
		selectedProjectPaths: string[];
		selectedHistoryDays: string;
		historyDayOptions: string[];
		savingSelection: boolean;
		saveSelectionError: string | null;
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
		onToggleSelection,
		onHistoryDaysChange,
		onStartTrackingSelected
	}: Props = $props();

	const trackingOptions = $derived(
		detectedProjects.map(
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
		<ProjectTrackingForm
			heading="Select projects to track"
			projects={trackingOptions}
			loading={detectedLoading}
			error={detectedError}
			emptyMessage="No detected projects found yet."
			checkedPaths={selectedProjectPaths}
			{selectedHistoryDays}
			{historyDayOptions}
			saving={savingSelection}
			saveError={saveSelectionError}
			submitLabel="Import Projects"
			submitDisabled={selectedProjectPaths.length === 0}
			onToggle={onToggleSelection}
			{onHistoryDaysChange}
			onSubmit={onStartTrackingSelected}
		/>
	</div>
</div>

<style>
	.onboarding {
		display: grid;
		grid-template-columns: 40% 60%;
		padding: 1.2rem;
		gap: 1.2rem;
	}

	.onboarding h2 {
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

	@media (max-width: 900px) {
		.onboarding {
			grid-template-columns: 1fr;
		}
	}
</style>
