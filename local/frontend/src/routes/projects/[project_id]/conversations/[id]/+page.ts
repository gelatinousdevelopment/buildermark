import { error } from '@sveltejs/kit';
import { getCommitConversationLinks, getConversation } from '$lib/api';

export async function load({ params, fetch }) {
	const id = params.id;
	if (!id) {
		throw error(400, 'Missing conversation ID');
	}

	const conversation = await getConversation(id, fetch);
	let matchedCommitHashes: string[] = [];
	let commitBranches: Record<string, string> = {};
	let commitSubjects: Record<string, string> = {};

	try {
		const links = await getCommitConversationLinks(conversation.projectId, [], [conversation.id], fetch);
		matchedCommitHashes = links.conversationToCommits[conversation.id] ?? [];
		commitBranches = links.commitBranches ?? {};
		commitSubjects = links.commitSubjects ?? {};
	} catch {
		// Commit links are supplementary; keep the conversation page usable if this fails.
	}

	return { conversation, matchedCommitHashes, commitBranches, commitSubjects };
}
