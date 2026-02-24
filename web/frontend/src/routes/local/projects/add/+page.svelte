<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { discoverImportableProjects, importProjects } from '$lib/api';
	import ProjectOnboarding from '$lib/components/project/ProjectOnboarding.svelte';
	import { websocketStore } from '$lib/stores/websocket.svelte';
	import type { ImportableProject } from '$lib/types';

	let detectedProjects: ImportableProject[] = $state([]);
	let detectedLoading = $state(false);
	let detectedError: string | null = $state(null);
	let selectedProjectPaths: string[] = $state([]);
	let historyImportDays = $state('90');
	let savingSelection = $state(false);
	let saveSelectionError: string | null = $state(null);
	const historyDayOptions = ['7', '14', '30', '60', '90', '180', '365', 'all'];

	let importStatusMessage = $derived(
		websocketStore.importStatus?.state === 'running' ? websocketStore.importStatus.message : null
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
		websocketStore.clearImportStatus();
		try {
			await importProjects(selectedProjectPaths, historyImportDays);
			const result = await websocketStore.waitForImportComplete();
			if (result.state === 'error') {
				saveSelectionError = result.message;
			} else {
				goto(resolve('/local/projects'));
			}
		} catch (e) {
			saveSelectionError = e instanceof Error ? e.message : 'Failed to import selected projects';
		} finally {
			savingSelection = false;
			websocketStore.clearImportStatus();
		}
	}

	onMount(() => {
		loadDetectedProjects();
	});
</script>

<div class="limited-content-width">
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
