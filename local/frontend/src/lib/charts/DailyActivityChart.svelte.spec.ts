import { page } from 'vitest/browser';
import { beforeEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import DailyActivityChart from './DailyActivityChart.svelte';
import { settingsStore } from '$lib/stores/settings.svelte';

const sampleActivity = [
	{
		date: '2026-03-11',
		conversations: 2,
		userPrompts: 3,
		userAnswers: 1
	}
];

describe('DailyActivityChart', () => {
	beforeEach(() => {
		settingsStore.activityChartCountAnswers = false;
	});

	it('renders a Details popover explaining chart semantics', async () => {
		render(DailyActivityChart, {
			dailyActivity: sampleActivity
		});

		const details = page.getByRole('button', { name: 'Activity chart details' });
		await expect.element(details).toBeInTheDocument();

		await details.hover();

		await expect
			.element(
				page.getByText('Conversations assigns each conversation to one day only: the day of its latest user message.')
			)
			.toBeInTheDocument();
		await expect
			.element(page.getByText('First prompts in child conversations are excluded from this chart.'))
			.toBeInTheDocument();
		await expect
			.element(
				page.getByText(
					'The Conversations page date filter is broader and shows any conversation with any message on that day, so those counts can differ.'
				)
			)
			.toBeInTheDocument();
	});

	it('updates the Details copy when answers are counted as prompts', async () => {
		settingsStore.activityChartCountAnswers = true;

		render(DailyActivityChart, {
			dailyActivity: sampleActivity
		});

		const details = page.getByRole('button', { name: 'Activity chart details' });
		await details.hover();

		await expect
			.element(
				page.getByText(
					'Answers are also included in prompt totals, once each on the day they were sent.'
				)
			)
			.toBeInTheDocument();
	});
});
