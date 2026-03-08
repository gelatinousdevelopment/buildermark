import { page } from 'vitest/browser';
import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import UserPromptMessageCard from './UserPromptMessageCard.svelte';

describe('UserPromptMessageCard', () => {
	it('renders the plan icon and extracted title for plan prompts', async () => {
		render(UserPromptMessageCard, {
			message: {
				id: 'msg-1',
				timestamp: 1,
				conversationId: 'conv-1',
				role: 'user',
				messageType: 'prompt',
				content: 'Implement the following plan:\n\n# Plan: Fix the bug\n\nDetails go here.',
				rawJson: '{}'
			}
		});

		await expect.element(page.getByText('user', { exact: true })).toBeInTheDocument();
		expect(document.querySelector('.plan-icon')).not.toBeNull();
	});

	it('does not render the plan banner for regular prompts', () => {
		render(UserPromptMessageCard, {
			message: {
				id: 'msg-2',
				timestamp: 1,
				conversationId: 'conv-1',
				role: 'user',
				messageType: 'prompt',
				content: 'Regular prompt',
				rawJson: '{}'
			}
		});

		expect(document.querySelector('.plan-icon')).toBeNull();
	});
});
