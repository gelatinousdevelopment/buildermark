import { redirect, error } from '@sveltejs/kit';
import { getProject } from '$lib/api';

export async function load({ params, fetch }) {
	const projectId = params.project_id;
	if (!projectId) {
		throw error(400, 'Missing project ID');
	}

	let project: Awaited<ReturnType<typeof getProject>>;
	try {
		project = await getProject(projectId, undefined, undefined, fetch);
	} catch {
		throw error(404, 'Project not found');
	}

	const branch = project.currentBranch || project.defaultBranch || 'main';
	throw redirect(
		302,
		`/local/projects/${encodeURIComponent(projectId)}/commits/${encodeURIComponent(branch)}`
	);
}
