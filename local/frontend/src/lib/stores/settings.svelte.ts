import { browser } from '$app/environment';
import type { ExportMode, ExportFormat, ExportSortOrder } from '$lib/exportGenerator';

const STORAGE_KEY = 'buildermark_local_settings';

export type ContentWidth = 'default' | 'wider' | 'full';
export type SortOrder = 'desc' | 'asc';
export type Theme = 'system' | 'light' | 'dark';

interface Settings {
	commits_chart_scale_by_lines: boolean;
	commits_chart_stretch_bars: boolean;
	commits_chart_window_days: number;
	activity_chart_count_answers: boolean;
	activity_chart_count_child_conversations_separately: boolean;
	content_width: ContentWidth;
	sort_order: SortOrder;
	theme: Theme;
	export_mode: ExportMode;
	export_format: ExportFormat;
	export_preset_days: number | null;
	export_sort_order: ExportSortOrder;
	file_type_coverage_show_all: boolean;
}

const defaults: Settings = {
	commits_chart_scale_by_lines: false,
	commits_chart_stretch_bars: false,
	commits_chart_window_days: 45,
	activity_chart_count_answers: false,
	activity_chart_count_child_conversations_separately: true,
	content_width: 'default',
	sort_order: 'desc',
	theme: 'system' as Theme,
	export_mode: 'prompts-with-commits',
	export_format: 'markdown',
	export_preset_days: 30,
	export_sort_order: 'newest' as ExportSortOrder,
	file_type_coverage_show_all: false
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
			// Migrate renamed sort order key
			if (parsed.commit_sort_order && !JSON.parse(raw).sort_order) {
				parsed.sort_order = parsed.commit_sort_order;
			}
			delete parsed.commit_sort_order;
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
		commits_chart_window_days: _commitsChartWindowDays,
		activity_chart_count_answers: _activityChartCountAnswers,
		activity_chart_count_child_conversations_separately:
			_activityChartCountChildConversationsSeparately,
		content_width: _contentWidth,
		sort_order: _sortOrder,
		theme: _theme,
		export_mode: _exportMode,
		export_format: _exportFormat,
		export_preset_days: _exportPresetDays,
		export_sort_order: _exportSortOrder,
		file_type_coverage_show_all: _fileTypeCoverageShowAll
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

function applyTheme(theme: Theme) {
	if (!browser) return;
	if (theme === 'light' || theme === 'dark') {
		document.documentElement.setAttribute('data-theme', theme);
		localStorage.setItem('theme', theme);
	} else {
		document.documentElement.removeAttribute('data-theme');
		localStorage.removeItem('theme');
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
let _commitsChartWindowDays = $state(initial.commits_chart_window_days);
let _activityChartCountAnswers = $state(initial.activity_chart_count_answers);
let _activityChartCountChildConversationsSeparately = $state(
	initial.activity_chart_count_child_conversations_separately
);
let _contentWidth = $state(initial.content_width);
let _sortOrder: SortOrder = $state(initial.sort_order);
let _theme: Theme = $state(initial.theme);
let _exportMode: ExportMode = $state(initial.export_mode);
let _exportFormat: ExportFormat = $state(initial.export_format);
let _exportPresetDays: number | null = $state(initial.export_preset_days);
let _exportSortOrder: ExportSortOrder = $state(initial.export_sort_order);
let _fileTypeCoverageShowAll = $state(initial.file_type_coverage_show_all);

applyContentWidth(initial.content_width);
applyTheme(initial.theme);

if (browser) {
	window.addEventListener('storage', (e) => {
		if (e.key !== STORAGE_KEY) return;
		const updated = load();
		_commitsChartScaleByLines = updated.commits_chart_scale_by_lines;
		_commitsChartStretchBars = updated.commits_chart_stretch_bars;
		_commitsChartWindowDays = updated.commits_chart_window_days;
		_activityChartCountAnswers = updated.activity_chart_count_answers;
		_activityChartCountChildConversationsSeparately =
			updated.activity_chart_count_child_conversations_separately;
		_contentWidth = updated.content_width;
		_sortOrder = updated.sort_order;
		_theme = updated.theme;
		_exportMode = updated.export_mode;
		_exportFormat = updated.export_format;
		_exportPresetDays = updated.export_preset_days;
		_exportSortOrder = updated.export_sort_order;
		_fileTypeCoverageShowAll = updated.file_type_coverage_show_all;
		applyContentWidth(updated.content_width);
		applyTheme(updated.theme);
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
	get commitsChartWindowDays() {
		return _commitsChartWindowDays;
	},
	set commitsChartWindowDays(v: number) {
		_commitsChartWindowDays = v;
		save();
	},
	get activityChartCountAnswers() {
		return _activityChartCountAnswers;
	},
	set activityChartCountAnswers(v: boolean) {
		_activityChartCountAnswers = v;
		save();
	},
	get activityChartCountChildConversationsSeparately() {
		return _activityChartCountChildConversationsSeparately;
	},
	set activityChartCountChildConversationsSeparately(v: boolean) {
		_activityChartCountChildConversationsSeparately = v;
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
	get sortOrder(): SortOrder {
		return _sortOrder;
	},
	set sortOrder(v: SortOrder) {
		_sortOrder = v;
		save();
	},
	get theme(): Theme {
		return _theme;
	},
	set theme(v: Theme) {
		_theme = v;
		applyTheme(v);
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
	},
	get fileTypeCoverageShowAll() {
		return _fileTypeCoverageShowAll;
	},
	set fileTypeCoverageShowAll(v: boolean) {
		_fileTypeCoverageShowAll = v;
		save();
	}
};
