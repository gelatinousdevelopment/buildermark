import { redirect, error } from '@sveltejs/kit';
import { getConversation } from '$lib/api';

export async function load({ params }) {
	const id = params.id;
	if (!id) {
		throw error(400, 'Missing conversation ID');
	}

	let conversation: Awaited<ReturnType<typeof getConversation>>;
	try {
		conversation = await getConversation(id);
	} catch {
		throw error(404, 'Conversation not found');
	}

	throw redirect(
		302,
		`/local/projects/${encodeURIComponent(conversation.projectId)}/conversations/${encodeURIComponent(id)}`
	);
}
