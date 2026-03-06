import type { ProjectDetail, DailyCommitSummary } from '$lib/types';

let _project: ProjectDetail | null = $state(null);
let _dailySummary: DailyCommitSummary[] = $state([]);
let _branch: string = $state('');
let _projectId: string = $state('');

export const projectLayoutData = {
	get project() {
		return _project;
	},
	get dailySummary() {
		return _dailySummary;
	},
	get branch() {
		return _branch;
	},
	get projectId() {
		return _projectId;
	},
	setProject(projectId: string, project: ProjectDetail) {
		if (projectId !== _projectId) return;
		_project = project;
	},
	setCommitsData(projectId: string, dailySummary: DailyCommitSummary[], branch: string) {
		if (projectId !== _projectId) return;
		_dailySummary = dailySummary;
		_branch = branch;
	},
	reset(projectId: string) {
		if (projectId === _projectId) return;
		_projectId = projectId;
		_project = null;
		_dailySummary = [];
		_branch = '';
	}
};
