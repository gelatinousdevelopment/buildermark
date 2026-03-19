import { beforeEach, describe, expect, it, vi } from 'vitest';
import { load } from './+page';
import { getProject, listProjects } from '$lib/api';
import type { Project, ProjectDetail } from '$lib/types';

vi.mock('$lib/api', () => ({
	listProjects: vi.fn(),
	getProject: vi.fn()
}));

describe('routes/projects/+page load', () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it('sorts projects by max conversation timestamp in preview rows', async () => {
		vi.mocked(listProjects).mockResolvedValue([
			{ id: 'p-1', path: '/one', label: 'one', gitId: 'git-1' } as Project,
			{ id: 'p-2', path: '/two', label: 'two', gitId: 'git-2' } as Project
		]);
		vi.mocked(getProject).mockImplementation(async (projectId: string) => {
			if (projectId === 'p-1') {
				return {
					conversations: [
						{ id: 'parent', lastMessageTimestamp: 1000 },
						{ id: 'child', lastMessageTimestamp: 9000 }
					]
				} as unknown as ProjectDetail;
			}
			return {
				conversations: [{ id: 'solo', lastMessageTimestamp: 5000 }]
			} as unknown as ProjectDetail;
		});

		const result = await load({ fetch: vi.fn() as typeof fetch } as Parameters<typeof load>[0]);

		expect(result.shouldRedirectToImport).toBe(false);
		expect(result.error).toBeNull();
		expect(result.rows[0].project.id).toBe('p-1');
		expect(result.rows[0].lastMessageTimestamp).toBe(9000);
		expect(result.rows[1].project.id).toBe('p-2');
	});
});
