import type {
	ConversationWithRatings,
	ConversationBatchDetail,
	ProjectCommitCoverage,
	CommitConversationLinks
} from './types';

export type ExportMode = 'commits-with-prompts' | 'prompts-with-commits' | 'just-prompts';
export type ExportFormat = 'markdown' | 'html';

export interface ExportData {
	projectLabel: string;
	conversations: ConversationWithRatings[];
	batchDetails: ConversationBatchDetail[];
	commits: ProjectCommitCoverage[];
	links: CommitConversationLinks | null;
}

function formatDate(unixMs: number): string {
	return new Date(unixMs).toLocaleDateString('en-US', {
		year: 'numeric',
		month: 'short',
		day: 'numeric'
	});
}

function shortHash(hash: string): string {
	return hash.slice(0, 7);
}

function escapeMarkdown(text: string): string {
	return text.replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

function getUserMessages(
	conversationId: string,
	batchDetails: ConversationBatchDetail[]
): string[] {
	const detail = batchDetails.find((d) => d.conversationId === conversationId);
	if (!detail) return [];
	return detail.userMessages
		.filter((m) => m.role === 'user')
		.map((m) => m.content)
		.filter((c) => c.trim().length > 0);
}

export function generateMarkdown(data: ExportData, mode: ExportMode): string {
	const lines: string[] = [];
	lines.push(`# Project Timeline: ${data.projectLabel}\n`);

	if (mode === 'commits-with-prompts') {
		const commits = [...data.commits].sort((a, b) => b.authoredAtUnixMs - a.authoredAtUnixMs);
		for (const commit of commits) {
			lines.push(
				`## ${commit.subject} (${shortHash(commit.commitHash)}, ${formatDate(commit.authoredAtUnixMs)})\n`
			);
			lines.push(`+${commit.linesAdded} -${commit.linesRemoved} lines\n`);

			const linkedConvIds = data.links?.commitToConversations[commit.commitHash] ?? [];
			if (linkedConvIds.length > 0) {
				lines.push(`### Related Prompts\n`);
				for (const convId of linkedConvIds) {
					const conv = data.conversations.find((c) => c.id === convId);
					if (!conv) continue;
					lines.push(`**${conv.title || 'Untitled'}** (${conv.agent})\n`);
					const messages = getUserMessages(convId, data.batchDetails);
					for (const msg of messages) {
						const escaped = escapeMarkdown(msg);
						lines.push(
							escaped
								.split('\n')
								.map((l) => `> ${l}`)
								.join('\n') + '\n'
						);
					}
				}
			}
		}
	} else if (mode === 'prompts-with-commits') {
		const convs = [...data.conversations].sort(
			(a, b) => b.lastMessageTimestamp - a.lastMessageTimestamp
		);
		for (const conv of convs) {
			lines.push(
				`## ${conv.title || 'Untitled'} (${conv.agent}, ${formatDate(conv.lastMessageTimestamp)})\n`
			);
			const messages = getUserMessages(conv.id, data.batchDetails);
			for (const msg of messages) {
				const escaped = escapeMarkdown(msg);
				lines.push(
					escaped
						.split('\n')
						.map((l) => `> ${l}`)
						.join('\n') + '\n'
				);
			}

			const linkedCommitHashes = data.links?.conversationToCommits[conv.id] ?? [];
			if (linkedCommitHashes.length > 0) {
				lines.push(`### Related Commits\n`);
				for (const hash of linkedCommitHashes) {
					const commit = data.commits.find((c) => c.commitHash === hash);
					if (!commit) continue;
					lines.push(
						`- ${shortHash(hash)}: ${commit.subject} (+${commit.linesAdded} -${commit.linesRemoved})`
					);
				}
				lines.push('');
			}
		}
	} else {
		// just-prompts
		const convs = [...data.conversations].sort(
			(a, b) => b.lastMessageTimestamp - a.lastMessageTimestamp
		);
		for (const conv of convs) {
			lines.push(
				`## ${conv.title || 'Untitled'} (${conv.agent}, ${formatDate(conv.lastMessageTimestamp)})\n`
			);
			const messages = getUserMessages(conv.id, data.batchDetails);
			for (const msg of messages) {
				const escaped = escapeMarkdown(msg);
				lines.push(
					escaped
						.split('\n')
						.map((l) => `> ${l}`)
						.join('\n') + '\n'
				);
			}
		}
	}

	return lines.join('\n');
}

export function generateHTML(data: ExportData, mode: ExportMode): string {
	const md = generateMarkdown(data, mode);
	// Simple markdown-to-HTML conversion
	const bodyHtml = md
		.split('\n')
		.map((line) => {
			if (line.startsWith('# ')) return `<h1>${line.slice(2)}</h1>`;
			if (line.startsWith('## ')) return `<h2>${line.slice(3)}</h2>`;
			if (line.startsWith('### ')) return `<h3>${line.slice(4)}</h3>`;
			if (line.startsWith('> ')) return `<blockquote>${line.slice(2)}</blockquote>`;
			if (line.startsWith('- ')) return `<li>${line.slice(2)}</li>`;
			if (line.startsWith('**') && line.endsWith('**'))
				return `<p><strong>${line.slice(2, -2)}</strong></p>`;
			if (line.startsWith('**')) return `<p><strong>${line.replace(/\*\*/g, '')}</strong></p>`;
			if (line.trim() === '') return '';
			return `<p>${line}</p>`;
		})
		.filter((l) => l.length > 0)
		.join('\n');

	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Export: ${data.projectLabel}</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 800px; margin: 2rem auto; padding: 0 1rem; line-height: 1.6; color: #333; }
  h1 { border-bottom: 2px solid #eee; padding-bottom: 0.5rem; }
  h2 { margin-top: 2rem; border-bottom: 1px solid #eee; padding-bottom: 0.3rem; }
  h3 { margin-top: 1.5rem; color: #555; }
  blockquote { border-left: 3px solid #ddd; margin: 0.5rem 0; padding: 0.3rem 1rem; color: #555; background: #f9f9f9; }
  li { margin: 0.3rem 0; }
  code { background: #f4f4f4; padding: 0.15rem 0.3rem; border-radius: 3px; font-size: 0.9em; }
</style>
</head>
<body>
${bodyHtml}
</body>
</html>`;
}
