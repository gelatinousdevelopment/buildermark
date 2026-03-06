import { listProjects } from '$lib/api';

export const ssr = false;
export const prerender = false;

export const load = async ({ fetch }) => {
	const projects = await listProjects(false, fetch).then((p) =>
		p.filter((project) => project.gitId)
	);
	return {
		projects
	};
};
