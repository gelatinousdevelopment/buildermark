import { PUBLIC_API_URL } from '$env/static/public';
import type {
	Project,
	ProjectDetail,
	Conversation,
	ConversationDetail,
	Rating,
	ProjectCommitCoverageResponse,
	ProjectCommitDetailResponse,
	ProjectCommitPageResponse,
	IngestCommitsResponse,
	CommitIngestionStatusResponse
} from './types';

interface Envelope<T> {
	ok: boolean;
	data?: T;
	error?: string;
}

async function api<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await fetch(`${PUBLIC_API_URL}${path}`, init);
	const raw = await res.text();
	let envelope: Envelope<T> | null = null;
	try {
		envelope = JSON.parse(raw) as Envelope<T>;
	} catch {
		const snippet = raw.trim().slice(0, 200) || res.statusText || `HTTP ${res.status}`;
		throw new Error(`API returned non-JSON response (${res.status}): ${snippet}`);
	}
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

export function setProjectLabel(id: string, label: string): Promise<void> {
	return api(`/api/v1/projects/${id}/label`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ label })
	});
}

export function getProject(id: string, page?: number, pageSize?: number): Promise<ProjectDetail> {
	const params = new URLSearchParams();
	if (page !== undefined) params.set('page', String(page));
	if (pageSize !== undefined) params.set('pageSize', String(pageSize));
	const q = params.size > 0 ? `?${params.toString()}` : '';
	return api(`/api/v1/projects/${id}${q}`);
}

export function setProjectIgnoreDiffPaths(id: string, ignoreDiffPaths: string): Promise<void> {
	return api(`/api/v1/projects/${id}/ignore-diff-paths`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ ignoreDiffPaths })
	});
}

export function setProjectIgnoreDefaultDiffPaths(
	id: string,
	ignoreDefaultDiffPaths: boolean
): Promise<void> {
	return api(`/api/v1/projects/${id}/ignore-default-diff-paths`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ ignoreDefaultDiffPaths })
	});
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

export function createRating(
	conversationId: string,
	rating: number,
	note: string
): Promise<Rating> {
	return api('/api/v1/rating', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ conversationId, rating, note })
	});
}

export function listProjectCommits(branch = ''): Promise<ProjectCommitCoverageResponse> {
	const q = branch ? `?branch=${encodeURIComponent(branch)}` : '';
	return api(`/api/v1/projects/commits${q}`);
}

export function listProjectCommitsPage(
	projectId: string,
	page = 1,
	branch = ''
): Promise<ProjectCommitPageResponse> {
	const params = new URLSearchParams({ page: String(page) });
	if (branch) params.set('branch', branch);
	return api(`/api/v1/projects/${projectId}/commits?${params.toString()}`);
}

export function getProjectCommitDetail(
	projectId: string,
	commitHash: string,
	branch = ''
): Promise<ProjectCommitDetailResponse> {
	const q = branch ? `?branch=${encodeURIComponent(branch)}` : '';
	return api(`/api/v1/projects/${projectId}/commits/${commitHash}${q}`);
}

export function ingestMoreCommits(
	projectId: string,
	count: number,
	branch = ''
): Promise<IngestCommitsResponse> {
	const q = branch ? `?branch=${encodeURIComponent(branch)}` : '';
	return api(`/api/v1/projects/${projectId}/ingest-commits${q}`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ count })
	});
}

export function getCommitIngestionStatus(
	projectId: string,
	branch = ''
): Promise<CommitIngestionStatusResponse> {
	const q = branch ? `?branch=${encodeURIComponent(branch)}` : '';
	return api(`/api/v1/projects/${projectId}/commit-ingestion-status${q}`);
}

export function refreshProjectCommits(
	projectId: string,
	branch = ''
): Promise<{ queued: boolean }> {
	const q = branch ? `?branch=${encodeURIComponent(branch)}` : '';
	return api(`/api/v1/projects/${projectId}/refresh-commits${q}`, {
		method: 'POST'
	});
}
