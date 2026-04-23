import { settingsStore } from '$lib/stores/settings.svelte';
import type { ProjectDetail, DailyCommitSummary } from '$lib/types';

function trimToActiveDays(
	summary: DailyCommitSummary[],
	targetActive: number
): DailyCommitSummary[] {
	if (targetActive <= 0) return summary;
	let activeCount = 0;
	for (let i = summary.length - 1; i >= 0; i--) {
		if (summary[i].linesTotal > 0) {
			activeCount++;
			if (activeCount >= targetActive) return summary.slice(i);
		}
	}
	return summary;
}

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
		if (settingsStore.commitsChartCollapseEmptyDays) {
			return trimToActiveDays(_dailySummary, _dailyWindowDays);
		}
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
	get effectiveFetchDays() {
		if (settingsStore.commitsChartCollapseEmptyDays) {
			return Math.min(_dailyWindowDays * 3, 365);
		}
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
