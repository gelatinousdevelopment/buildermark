import { describe, expect, it } from 'vitest';

import { isStandaloneTimelineMessage, isUserPromptMessage, messageType } from './messageUtils';
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
});
