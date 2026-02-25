import { error } from '@sveltejs/kit';
import { getConversation } from '$lib/api';

export async function load({ params, fetch }) {
	const id = params.id;
	if (!id) {
		throw error(400, 'Missing conversation ID');
	}

	const conversation = await getConversation(id, fetch);
	return { conversation };
}
