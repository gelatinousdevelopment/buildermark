<script lang="ts">
	import { onMount } from 'svelte';
	import { goto, invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { discoverImportableProjects, importProjects } from '$lib/api';
	import ProjectOnboarding from '$lib/components/project/ProjectOnboarding.svelte';
	import { websocketStore } from '$lib/stores/websocket.svelte';
	import type { ImportableProject } from '$lib/types';

	let detectedProjects: ImportableProject[] = $state([]);
	let detectedLoading = $state(false);
	let detectedError: string | null = $state(null);
	let selectedProjectPaths: string[] = $state([]);
	let historyImportDays = $state('30');
	let savingSelection = $state(false);
	let saveSelectionError: string | null = $state(null);
	const historyDayOptions = ['7', '14', '30', '60', '90', '180', '365', 'all'];

	let importStatusMessage = $derived(
		websocketStore.getJob('import')?.state === 'running'
			? (websocketStore.getJob('import')?.message ?? null)
			: null
	);

	function projectName(project: { label: string; path: string }): string {
		return project.label || project.path;
	}

	function toggleSelection(projectPath: string, checked: boolean) {
		if (checked) {
			selectedProjectPaths = selectedProjectPaths.includes(projectPath)
				? selectedProjectPaths
				: [...selectedProjectPaths, projectPath];
		} else {
			selectedProjectPaths = selectedProjectPaths.filter((path) => path !== projectPath);
		}
	}

	function setHistoryImportDays(days: string) {
		historyImportDays = days;
	}

	async function loadDetectedProjects() {
		detectedLoading = true;
		detectedError = null;
		try {
			const response = await discoverImportableProjects(30);
			detectedProjects = response.projects.sort((a, b) =>
				projectName(a).localeCompare(projectName(b))
			);
		} catch (e) {
			detectedError = e instanceof Error ? e.message : 'Failed to load detected projects';
		} finally {
			detectedLoading = false;
		}
	}

	async function startTrackingSelected() {
		if (selectedProjectPaths.length === 0) return;
		savingSelection = true;
		saveSelectionError = null;
		websocketStore.clearJob('import');
		try {
			await importProjects(selectedProjectPaths, historyImportDays);
			const result = await websocketStore.waitForJob('import');
			if (result.state === 'error') {
				saveSelectionError = result.message;
			} else {
				await invalidateAll();
				goto(resolve('/projects'));
			}
		} catch (e) {
			saveSelectionError = e instanceof Error ? e.message : 'Failed to import selected projects';
		} finally {
			savingSelection = false;
			websocketStore.clearJob('import');
		}
	}

	onMount(() => {
		loadDetectedProjects();
	});
</script>

<div class="outer">
	<div class="onboarding">
		<ProjectOnboarding
			{detectedProjects}
			{detectedLoading}
			{detectedError}
			{selectedProjectPaths}
			selectedHistoryDays={historyImportDays}
			{historyDayOptions}
			{savingSelection}
			{saveSelectionError}
			{importStatusMessage}
			onToggleSelection={toggleSelection}
			onHistoryDaysChange={setHistoryImportDays}
			onStartTrackingSelected={startTrackingSelected}
		/>
	</div>
</div>

<style>
	.outer {
		flex: 1;
		/*background: white;*/
	}

	.onboarding {
		background: var(--color-background-content);
		padding: 1rem;
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 1rem;

		max-width: 900px;
		margin: 0 auto;

		background: var(--color-background-content);
		border-radius: var(--content-section-border-radius);
		border: 0.5px solid var(--color-divider);
		box-sizing: border-box;
		margin: 1.5rem auto;
		width: 100%;

		box-shadow: 0 3px 10px 2px rgba(0, 0, 0, 0.07);
	}

	@media (max-width: 900px) {
		.onboarding {
			border-width: 0 0 0.5px 0;
			margin: 0 auto;
			border-radius: 0;
		}
	}
</style>
