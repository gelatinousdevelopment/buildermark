import { browser } from '$app/environment';
import type { ExportMode, ExportFormat, ExportSortOrder } from '$lib/exportGenerator';

const STORAGE_KEY = 'buildermark_local_settings';

export type ContentWidth = 'default' | 'wider' | 'full';
export type CommitSortOrder = 'desc' | 'asc';

interface Settings {
	commits_chart_scale_by_lines: boolean;
	commits_chart_stretch_bars: boolean;
	activity_chart_count_answers: boolean;
	content_width: ContentWidth;
	commit_sort_order: CommitSortOrder;
	export_mode: ExportMode;
	export_format: ExportFormat;
	export_preset_days: number | null;
	export_sort_order: ExportSortOrder;
}

const defaults: Settings = {
	commits_chart_scale_by_lines: false,
	commits_chart_stretch_bars: false,
	activity_chart_count_answers: false,
	content_width: 'default',
	commit_sort_order: 'desc',
	export_mode: 'prompts-with-commits',
	export_format: 'markdown',
	export_preset_days: 30,
	export_sort_order: 'newest' as ExportSortOrder
};

function load(): Settings {
	if (!browser) return { ...defaults };
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		if (raw) {
			const parsed = { ...defaults, ...JSON.parse(raw) };
			// Migrate removed export mode
			if (parsed.export_mode === 'just-prompts') {
				parsed.export_mode = 'prompts-with-commits';
			}
			return parsed;
		}
	} catch {
		// ignore corrupt data
	}
	return { ...defaults };
}

function currentSettings(): Settings {
	return {
		commits_chart_scale_by_lines: _commitsChartScaleByLines,
		commits_chart_stretch_bars: _commitsChartStretchBars,
		activity_chart_count_answers: _activityChartCountAnswers,
		content_width: _contentWidth,
		commit_sort_order: _commitSortOrder,
		export_mode: _exportMode,
		export_format: _exportFormat,
		export_preset_days: _exportPresetDays,
		export_sort_order: _exportSortOrder
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
let _activityChartCountAnswers = $state(initial.activity_chart_count_answers);
let _contentWidth = $state(initial.content_width);
let _commitSortOrder: CommitSortOrder = $state(initial.commit_sort_order);
let _exportMode: ExportMode = $state(initial.export_mode);
let _exportFormat: ExportFormat = $state(initial.export_format);
let _exportPresetDays: number | null = $state(initial.export_preset_days);
let _exportSortOrder: ExportSortOrder = $state(initial.export_sort_order);

applyContentWidth(initial.content_width);

if (browser) {
	window.addEventListener('storage', (e) => {
		if (e.key !== STORAGE_KEY) return;
		const updated = load();
		_commitsChartScaleByLines = updated.commits_chart_scale_by_lines;
		_commitsChartStretchBars = updated.commits_chart_stretch_bars;
		_activityChartCountAnswers = updated.activity_chart_count_answers;
		_contentWidth = updated.content_width;
		_commitSortOrder = updated.commit_sort_order;
		_exportMode = updated.export_mode;
		_exportFormat = updated.export_format;
		_exportPresetDays = updated.export_preset_days;
		_exportSortOrder = updated.export_sort_order;
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
	get activityChartCountAnswers() {
		return _activityChartCountAnswers;
	},
	set activityChartCountAnswers(v: boolean) {
		_activityChartCountAnswers = v;
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
	},
	get exportMode(): ExportMode {
		return _exportMode;
	},
	set exportMode(v: ExportMode) {
		_exportMode = v;
		save();
	},
	get exportFormat(): ExportFormat {
		return _exportFormat;
	},
	set exportFormat(v: ExportFormat) {
		_exportFormat = v;
		save();
	},
	get exportPresetDays(): number | null {
		return _exportPresetDays;
	},
	set exportPresetDays(v: number | null) {
		_exportPresetDays = v;
		save();
	},
	get exportSortOrder(): ExportSortOrder {
		return _exportSortOrder;
	},
	set exportSortOrder(v: ExportSortOrder) {
		_exportSortOrder = v;
		save();
	}
};
