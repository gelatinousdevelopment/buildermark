import { getLocalSettings, listProjects } from '$lib/api';
import type { LocalSettings, Project } from '$lib/types';

export async function load({ fetch }) {
	let projects: Project[] = [];
	let projectError: string | null = null;
	let localSettings: LocalSettings | null = null;
	let localSettingsError: string | null = null;

	const [projectsResult, settingsResult] = await Promise.allSettled([
		listProjects(false, fetch),
		getLocalSettings(fetch)
	]);

	if (projectsResult.status === 'fulfilled') {
		projects = projectsResult.value.sort((a, b) =>
			(a.label || a.path).localeCompare(b.label || b.path)
		);
	} else {
		projectError =
			projectsResult.reason instanceof Error
				? projectsResult.reason.message
				: 'Failed to load projects';
	}

	if (settingsResult.status === 'fulfilled') {
		localSettings = settingsResult.value;
	} else {
		localSettingsError =
			settingsResult.reason instanceof Error
				? settingsResult.reason.message
				: 'Failed to load local settings';
	}

	return { projects, projectError, localSettings, localSettingsError };
}
