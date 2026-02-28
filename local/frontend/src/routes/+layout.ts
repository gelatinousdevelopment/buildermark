import { listProjects } from '$lib/api';

export const ssr = false;
export const prerender = false;

export const load = async () => {
	const projects = (await listProjects(false)).filter((project) => project.gitId);
	return {
		projects
	};
};
