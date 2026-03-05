import { getLocalSettings, listProjects } from '$lib/api';
import type { LocalSettings } from '$lib/types';

export const ssr = false;
export const prerender = false;

export const load = async ({ fetch }) => {
	const [projects, localSettings] = await Promise.all([
		listProjects(false, fetch).then((p) => p.filter((project) => project.gitId)),
		getLocalSettings(fetch).catch((): LocalSettings | null => null)
	]);
	return {
		projects,
		localSettings
	};
};
