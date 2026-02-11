import { PUBLIC_API_URL } from '$env/static/public';
import type { Project, ProjectDetail, Conversation, ConversationDetail, Rating } from './types';

interface Envelope<T> {
	ok: boolean;
	data?: T;
	error?: string;
}

async function api<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await fetch(`${PUBLIC_API_URL}${path}`, init);
	const envelope: Envelope<T> = await res.json();
	if (!envelope.ok) {
		throw new Error(envelope.error ?? `API error: ${res.status}`);
	}
	return envelope.data as T;
}

export function listProjects(ignored = false): Promise<Project[]> {
	return api(`/api/v1/projects?ignored=${ignored}`);
}

export function setProjectIgnored(id: string, ignored: boolean): Promise<void> {
	return api(`/api/v1/projects/${id}/ignored`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ ignored })
	});
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
