import { listProjects, listSearchProjects } from '$lib/api';
import type { Project, ProjectSearchMatch } from '$lib/types';

export async function load({ url, fetch }) {
	const query = url.searchParams.get('q')?.trim() ?? '';
	const projectId = url.searchParams.get('project') ?? '';

	let allProjects: Project[] = [];
	try {
		allProjects = await listProjects(false, fetch);
	} catch {
		allProjects = [];
	}

	let results: ProjectSearchMatch[] = [];
	let searchError: string | null = null;
	if (query) {
		try {
			results = await listSearchProjects(query, projectId, fetch);
		} catch (e) {
			searchError = e instanceof Error ? e.message : 'Failed to search projects';
		}
	}

	return { allProjects, results, searchError, query, projectId };
}
