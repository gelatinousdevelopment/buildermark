import { describe, expect, it } from 'vitest';

import { buildResumeCommand } from './agents';

describe('buildResumeCommand', () => {
	it('builds a Claude resume command', () => {
		expect(buildResumeCommand('claude', 'sess-123', '/tmp/project')).toBe(
			"cd '/tmp/project' && claude -r 'sess-123' '/rate-buildermark'"
		);
	});

	it('builds a Codex resume command with a literal $rate-buildermark prompt', () => {
		expect(buildResumeCommand('codex', 'thread-123', '/tmp/project')).toBe(
			"cd '/tmp/project' && codex resume 'thread-123' -a on-request '$rate-buildermark'"
		);
	});

	it('builds a Gemini resume command', () => {
		expect(buildResumeCommand('gemini', 'session-123', '/tmp/project')).toBe(
			"cd '/tmp/project' && gemini -r 'session-123' '/rate-buildermark'"
		);
	});

	it('returns null for unsupported agents', () => {
		expect(buildResumeCommand('claude_cloud', 'sess-123', '/tmp/project')).toBeNull();
	});

	it('quotes project paths and session ids with shell-sensitive characters', () => {
		expect(buildResumeCommand('claude', "sess'123", "/tmp/Buildermark Demo's Repo")).toBe(
			"cd '/tmp/Buildermark Demo'\"'\"'s Repo' && claude -r 'sess'\"'\"'123' '/rate-buildermark'"
		);
	});

	it('omits the cd prefix when the project path is unavailable', () => {
		expect(buildResumeCommand('codex', 'thread-123', '')).toBe(
			"codex resume 'thread-123' -a on-request '$rate-buildermark'"
		);
	});
});
