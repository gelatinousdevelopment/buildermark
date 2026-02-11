import { PUBLIC_API_URL } from '$env/static/public';
import type {
	Project,
	ProjectDetail,
	Conversation,
	ConversationDetail,
	Rating
} from './types';

interface Envelope<T> {
	ok: boolean;
	data?: T;
	error?: string;
}

async function api<T>(path: string): Promise<T> {
	const res = await fetch(`${PUBLIC_API_URL}${path}`);
	const envelope: Envelope<T> = await res.json();
	if (!envelope.ok) {
		throw new Error(envelope.error ?? `API error: ${res.status}`);
	}
	return envelope.data as T;
}

export function listProjects(): Promise<Project[]> {
	return api('/api/v1/projects');
}

export function getProject(id: string): Promise<ProjectDetail> {
	return api(`/api/v1/projects/${id}`);
}

export function listConversations(): Promise<Conversation[]> {
	return api('/api/v1/conversations');
}

export function getConversation(id: string): Promise<ConversationDetail> {
	return api(`/api/v1/conversations/${id}`);
}

export function listRatings(): Promise<Rating[]> {
	return api('/api/v1/ratings');
}
