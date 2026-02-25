import { browser } from '$app/environment';

const STORAGE_KEY = 'buildermark_local_settings';

interface Settings {
	commits_chart_scale_by_lines: boolean;
}

const defaults: Settings = {
	commits_chart_scale_by_lines: false
};

function load(): Settings {
	if (!browser) return { ...defaults };
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		if (raw) return { ...defaults, ...JSON.parse(raw) };
	} catch {
		// ignore corrupt data
	}
	return { ...defaults };
}

function save(s: Settings) {
	if (!browser) return;
	try {
		localStorage.setItem(STORAGE_KEY, JSON.stringify(s));
	} catch {
		// ignore quota errors
	}
}

const initial = load();

let _commitsChartScaleByLines = $state(initial.commits_chart_scale_by_lines);

export const settingsStore = {
	get commitsChartScaleByLines() {
		return _commitsChartScaleByLines;
	},
	set commitsChartScaleByLines(v: boolean) {
		_commitsChartScaleByLines = v;
		save({ commits_chart_scale_by_lines: v });
	}
};
