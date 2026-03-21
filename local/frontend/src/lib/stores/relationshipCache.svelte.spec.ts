import { page } from 'vitest/browser';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { relationshipCache } from './relationshipCache.svelte';
import RelationshipCacheReactivityTest from './RelationshipCacheReactivityTest.svelte';
import { getCommitConversationLinks } from '$lib/api';

vi.mock('$lib/api', () => ({
	getCommitConversationLinks: vi.fn()
}));

describe('relationshipCache reactivity', () => {
	const projectId = 'project-1';

	beforeEach(() => {
		vi.clearAllMocks();
		relationshipCache.clearProject(projectId);
		relationshipCache.clearHover();
	});

	it('updates component classes when hover relationships change', async () => {
		vi.mocked(getCommitConversationLinks).mockResolvedValue({
			commitToConversations: { 'commit-1': ['conv-1'] },
			conversationToCommits: { 'conv-1': ['commit-1'] },
			commitBranches: {},
			commitSubjects: {}
		});

		render(RelationshipCacheReactivityTest);

		await relationshipCache.loadRelationships(projectId, ['commit-1'], ['conv-1']);
		relationshipCache.hoverCommit(projectId, 'commit-1');

		await expect
			.element(page.getByTestId('source-commit'))
			.toHaveClass(/source/);
		await expect
			.element(page.getByTestId('highlighted-conversation'))
			.toHaveClass(/highlighted/);

		relationshipCache.hoverConversation(projectId, 'conv-1');

		await expect
			.element(page.getByTestId('source-conversation'))
			.toHaveClass(/source/);
		await expect
			.element(page.getByTestId('highlighted-commit'))
			.toHaveClass(/highlighted/);

		relationshipCache.clearHover();

		await expect
			.element(page.getByTestId('source-conversation'))
			.not.toHaveClass(/source/);
		await expect
			.element(page.getByTestId('source-commit'))
			.not.toHaveClass(/source/);
		await expect
			.element(page.getByTestId('highlighted-conversation'))
			.not.toHaveClass(/highlighted/);
		await expect
			.element(page.getByTestId('highlighted-commit'))
			.not.toHaveClass(/highlighted/);
	});
});
