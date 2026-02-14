import { expect, test } from '@playwright/test';

test('commit detail renders contributing message and conversation link', async ({ page }) => {
	await page.route('**/api/v1/projects/proj-1/commits/hash-1', async (route) => {
		await route.fulfill({
			status: 200,
			contentType: 'application/json',
			body: JSON.stringify({
				ok: true,
				data: {
					branch: 'main',
					commit: {
						projectId: 'proj-1',
						projectLabel: 'demo',
						projectPath: '/tmp/demo',
						projectGitId: 'root',
						commitHash: 'hash-1',
						subject: 'agent change',
						authoredAtUnixMs: 1760000000000,
						linesTotal: 2,
						linesFromAgent: 1,
						linePercent: 50,
						charsTotal: 20,
						charsFromAgent: 10,
						characterPercent: 50
					},
					diff: 'diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-old\n+new\n\ndiff --git a/ignored.txt b/ignored.txt\n--- a/ignored.txt\n+++ b/ignored.txt\n@@ -1 +1 @@\n-secret-old\n+do-not-show\n',
					files: [
						{
							path: 'a.txt',
							added: 1,
							removed: 1,
							ignored: false,
							moved: false,
							movedFrom: '',
							copiedFromAgent: false,
							linesTotal: 2,
							linesFromAgent: 1,
							linePercent: 50
						},
						{
							path: 'ignored.txt',
							added: 1,
							removed: 1,
							ignored: true,
							moved: false,
							movedFrom: '',
							copiedFromAgent: false,
							linesTotal: 2,
							linesFromAgent: 0,
							linePercent: 0
						}
					],
					messages: [
						{
							id: 'msg-1',
							timestamp: 1760000000000,
							conversationId: 'conv-1',
							conversationTitle: 'Fix auth',
							model: 'gpt-5',
							content:
								'```diff\ndiff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-old\n+new\n```',
							linesMatched: 1,
							charsMatched: 3
						}
					]
				}
			})
		});
	});

	await page.goto('/dashboard/projects/proj-1/commits/hash-1');
	await expect(page.getByText('agent change')).toBeVisible();
	await expect(page.getByText('Conversation: Fix auth')).toBeVisible();
	await expect(page.getByRole('link', { name: 'a.txt' })).toHaveAttribute('href', '#diff-a.txt');
	await expect(page.getByRole('link', { name: 'ignored.txt' })).toHaveCount(0);
	await expect(page.locator('tr', { hasText: 'ignored.txt' }).locator('.changes-col')).toHaveText(
		''
	);
	await page.getByRole('button').filter({ hasText: 'matched 1 lines, 3 chars' }).click();
	await expect(page.locator('.commit-diff').first()).toContainText('new');
	await expect(page.getByText('do-not-show')).toHaveCount(0);
});

test('commit detail surfaces non-json API error cleanly', async ({ page }) => {
	await page.route('**/api/v1/projects/proj-2/commits/hash-2', async (route) => {
		await route.fulfill({
			status: 404,
			contentType: 'text/plain',
			body: '404 page not found'
		});
	});

	await page.goto('/dashboard/projects/proj-2/commits/hash-2');
	await expect(
		page.getByText('API returned non-JSON response (404): 404 page not found')
	).toBeVisible();
});
