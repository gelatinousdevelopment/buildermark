import { error } from '@sveltejs/kit';
import { getProject, listTeamServers } from '$lib/api';
import type { TeamServer } from '$lib/types';

export async function load({ params, fetch }) {
	const id = params.project_id;
	if (!id) {
		throw error(400, 'Missing project ID');
	}

	let teamServers: TeamServer[] = [];

	const [projectResult, serversResult] = await Promise.allSettled([
		getProject(id, undefined, undefined, fetch),
		listTeamServers(fetch)
	]);

	if (projectResult.status === 'rejected') {
		throw error(
			404,
			projectResult.reason instanceof Error
				? projectResult.reason.message
				: 'Failed to load project'
		);
	}
	const project = projectResult.value;

	if (serversResult.status === 'fulfilled') {
		teamServers = serversResult.value;
	}

	return { project, teamServers };
}
