import { browser } from '$app/environment';

const STORAGE_KEY = 'buildermark_local_settings';

export type ContentWidth = 'default' | 'wider' | 'full';
export type CommitSortOrder = 'desc' | 'asc';

interface Settings {
	commits_chart_scale_by_lines: boolean;
	commits_chart_stretch_bars: boolean;
	content_width: ContentWidth;
	commit_sort_order: CommitSortOrder;
}

const defaults: Settings = {
	commits_chart_scale_by_lines: false,
	commits_chart_stretch_bars: false,
	content_width: 'default',
	commit_sort_order: 'desc'
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

function currentSettings(): Settings {
	return {
		commits_chart_scale_by_lines: _commitsChartScaleByLines,
		commits_chart_stretch_bars: _commitsChartStretchBars,
		content_width: _contentWidth,
		commit_sort_order: _commitSortOrder
	};
}

function save() {
	if (!browser) return;
	try {
		localStorage.setItem(STORAGE_KEY, JSON.stringify(currentSettings()));
	} catch {
		// ignore quota errors
	}
}

function applyContentWidth(width: ContentWidth) {
	if (!browser) return;
	if (width === 'default') {
		delete document.documentElement.dataset.contentWidth;
	} else {
		document.documentElement.dataset.contentWidth = width;
	}
}

const initial = load();

let _commitsChartScaleByLines = $state(initial.commits_chart_scale_by_lines);
let _commitsChartStretchBars = $state(initial.commits_chart_stretch_bars);
let _contentWidth = $state(initial.content_width);
let _commitSortOrder: CommitSortOrder = $state(initial.commit_sort_order);

applyContentWidth(initial.content_width);

if (browser) {
	window.addEventListener('storage', (e) => {
		if (e.key !== STORAGE_KEY) return;
		const updated = load();
		_commitsChartScaleByLines = updated.commits_chart_scale_by_lines;
		_commitsChartStretchBars = updated.commits_chart_stretch_bars;
		_contentWidth = updated.content_width;
		_commitSortOrder = updated.commit_sort_order;
		applyContentWidth(updated.content_width);
	});
}

export const settingsStore = {
	get commitsChartScaleByLines() {
		return _commitsChartScaleByLines;
	},
	set commitsChartScaleByLines(v: boolean) {
		_commitsChartScaleByLines = v;
		save();
	},
	get commitsChartStretchBars() {
		return _commitsChartStretchBars;
	},
	set commitsChartStretchBars(v: boolean) {
		_commitsChartStretchBars = v;
		save();
	},
	get contentWidth(): ContentWidth {
		return _contentWidth;
	},
	set contentWidth(v: ContentWidth) {
		_contentWidth = v;
		applyContentWidth(v);
		save();
	},
	get commitSortOrder(): CommitSortOrder {
		return _commitSortOrder;
	},
	set commitSortOrder(v: CommitSortOrder) {
		_commitSortOrder = v;
		save();
	}
};
