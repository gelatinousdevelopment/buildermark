import { redirect, error } from '@sveltejs/kit';
import { getConversation } from '$lib/api';

export async function load({ params, fetch }) {
	const id = params.id;
	if (!id) {
		throw error(400, 'Missing conversation ID');
	}

	let conversation: Awaited<ReturnType<typeof getConversation>>;
	try {
		conversation = await getConversation(id, fetch);
	} catch {
		throw error(404, 'Conversation not found');
	}

	throw redirect(
		302,
		`/projects/${encodeURIComponent(conversation.projectId)}/conversations/${encodeURIComponent(conversation.id)}`
	);
}
