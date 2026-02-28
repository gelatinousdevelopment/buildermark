import { getProject, listProjects } from '$lib/api';
import type { Project, ProjectDetail } from '$lib/types';

type ProjectRow = {
	project: Project;
	conversationData: ProjectDetail | null;
	conversationError: string | null;
	lastMessageTimestamp: number;
};

export async function load({ fetch }) {
	let rows: ProjectRow[] = [];
	let error: string | null = null;
	let shouldRedirectToImport = false;

	try {
		const projects = (await listProjects(false, fetch)).filter((project) => project.gitId);
		if (projects.length === 0) {
			shouldRedirectToImport = true;
			return { rows, error, shouldRedirectToImport };
		}
		rows = await Promise.all(
			projects.map(async (project): Promise<ProjectRow> => {
				try {
					const conversationData = await getProject(project.id, 1, 10, fetch);
					const latestConversationTs =
						conversationData.conversations[0]?.lastMessageTimestamp ?? 0;
					return {
						project,
						conversationData,
						conversationError: null,
						lastMessageTimestamp: latestConversationTs
					};
				} catch (e) {
					return {
						project,
						conversationData: null,
						conversationError:
							e instanceof Error ? e.message : 'Failed to load project conversations',
						lastMessageTimestamp: 0
					};
				}
			})
		);
		rows.sort((a, b) => {
			if (a.lastMessageTimestamp !== b.lastMessageTimestamp) {
				return b.lastMessageTimestamp - a.lastMessageTimestamp;
			}
			return (a.project.label || a.project.path).localeCompare(
				b.project.label || b.project.path
			);
		});
	} catch (e) {
		error = e instanceof Error ? e.message : 'Failed to load projects';
	}

	return { rows, error, shouldRedirectToImport };
}
