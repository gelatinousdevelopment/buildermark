import { describe, expect, it } from 'vitest';

import {
	isPlanPromptMessage,
	isStandaloneTimelineMessage,
	isUserPromptMessage,
	messageType,
	planPromptTitle
} from './messageUtils';
import type { MessageRead } from './types';

function makeMessage(overrides: Partial<MessageRead>): MessageRead {
	return {
		id: 'm-1',
		timestamp: 1,
		conversationId: 'c-1',
		role: 'user',
		content: 'hello',
		rawJson: '{}',
		...overrides
	};
}

describe('messageType', () => {
	it('downgrades agent prompt rows to log', () => {
		const message = makeMessage({
			role: 'agent',
			messageType: 'prompt',
			content: 'internal wrapper prompt'
		});

		expect(messageType(message)).toBe('log');
		expect(isUserPromptMessage(message)).toBe(false);
		expect(isStandaloneTimelineMessage(message)).toBe(false);
	});

	it('keeps real user answers standalone', () => {
		const message = makeMessage({
			messageType: 'answer',
			content: 'user answer'
		});

		expect(messageType(message)).toBe('answer');
		expect(isStandaloneTimelineMessage(message)).toBe(true);
	});

	it('treats explicit diff rows as diff messages', () => {
		const message = makeMessage({
			role: 'agent',
			messageType: 'diff',
			content: 'not fenced',
			rawJson: '{}'
		});

		expect(messageType(message)).toBe('diff');
		expect(isStandaloneTimelineMessage(message)).toBe(false);
	});
});

describe('plan prompt detection', () => {
	it('extracts a plan title from a standard plan prompt', () => {
		expect(planPromptTitle('Implement the following plan:\n\n# Plan: Fix the bug\n\nDetails')).toBe(
			'Fix the bug'
		);
	});

	it('accepts a generic markdown heading', () => {
		expect(planPromptTitle('Implement the following plan:\n\n# Some Title\n\nDetails')).toBe(
			'Some Title'
		);
	});

	it('skips blank lines before the first heading', () => {
		expect(planPromptTitle('Implement the following plan:\n\n\n# Plan: My Title')).toBe('My Title');
	});

	it('returns empty for non-plan prompts', () => {
		expect(planPromptTitle('Please fix the auth bug')).toBe('');
	});

	it('returns empty when the plan prefix has no heading', () => {
		expect(planPromptTitle('Implement the following plan:\n\nNo heading here')).toBe('');
	});

	it('flags messages with a plan title as plan prompts', () => {
		const message = makeMessage({
			content: 'Implement the following plan:\n\n# Plan: Fix the bug\n\nDetails'
		});

		expect(isPlanPromptMessage(message)).toBe(true);
	});
});
