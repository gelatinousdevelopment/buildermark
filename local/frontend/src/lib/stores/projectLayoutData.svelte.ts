import { settingsStore } from '$lib/stores/settings.svelte';
import type { ProjectDetail, DailyCommitSummary } from '$lib/types';

let _project: ProjectDetail | null = $state(null);
let _dailySummary: DailyCommitSummary[] = $state([]);
let _branch: string = $state('');
let _projectId: string = $state('');
let _dailyWindowDays: number = $state(settingsStore.commitsChartWindowDays);

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
	get dailyWindowDays() {
		return _dailyWindowDays;
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
	setDailyWindowDays(days: number) {
		_dailyWindowDays = days;
		settingsStore.commitsChartWindowDays = days;
	},
	reset(projectId: string) {
		if (projectId === _projectId) return;
		_projectId = projectId;
		_project = null;
		_dailySummary = [];
		_branch = '';
		_dailyWindowDays = settingsStore.commitsChartWindowDays;
	}
};
