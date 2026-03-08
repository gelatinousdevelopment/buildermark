import { browser } from '$app/environment';
import { API_URL } from '$lib/config';
import type {
	Project,
	LocalSettings,
	ProjectDetail,
	TeamServer,
	Conversation,
	ConversationDetail,
	ConversationBatchDetail,
	Rating,
	PluginInventoryResponse,
	ProjectCommitCoverageResponse,
	ProjectCommitDetailResponse,
	ProjectCommitPageResponse,
	IngestCommitsResponse,
	CommitIngestionStatusResponse,
	DiscoverImportableProjectsResponse,
	ImportProjectsResponse,
	CommitConversationLinks,
	ProjectSearchMatch
} from './types';

interface Envelope<T> {
	ok: boolean;
	data?: T;
	error?: string;
}

type APIFetch = typeof fetch;

const isReadOnlyMode = import.meta.env.PUBLIC_READ_ONLY === 'true';

async function api<T>(path: string, init?: RequestInit, fetchFn: APIFetch = fetch): Promise<T> {
	const res = await fetchFn(`${API_URL}${path}`, init);
	const raw = await res.text();
	let envelope: Envelope<T> | null = null;
	try {
		envelope = JSON.parse(raw) as Envelope<T>;
	} catch {
		const snippet = raw.trim().slice(0, 200) || res.statusText || `HTTP ${res.status}`;
		throw new Error(`API returned non-JSON response (${res.status}): ${snippet}`);
	}
	if (!envelope.ok) {
		if (
			isReadOnlyMode &&
			browser &&
			(init?.method ?? 'GET').toUpperCase() !== 'GET' &&
			(init?.method ?? 'GET').toUpperCase() !== 'HEAD'
		) {
			window.alert('This server is running in read-only demo mode.');
		}
		throw new Error(envelope.error ?? `API error: ${res.status}`);
	}
	return envelope.data as T;
}

export function listProjects(ignored = false, fetchFn?: APIFetch): Promise<Project[]> {
	return api(`/api/v1/projects?ignored=${ignored}`, undefined, fetchFn);
}

export function getLocalSettings(fetchFn?: APIFetch): Promise<LocalSettings> {
	return api('/api/v1/settings', undefined, fetchFn);
}

export function listPlugins(fetchFn?: APIFetch): Promise<PluginInventoryResponse> {
	return api('/api/v1/plugins', undefined, fetchFn);
}

export function setPluginInstalled(
	homePath: string,
	agent: string,
	install: boolean
): Promise<PluginInventoryResponse> {
	return api('/api/v1/plugins', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ homePath, agent, install })
	});
}

export function updateLocalSettings(
	extraAgentHomes: string[],
	extraLocalUserEmails?: string[]
): Promise<LocalSettings> {
	return api('/api/v1/settings', {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ extraAgentHomes, extraLocalUserEmails })
	});
}
export function deleteProject(id: string): Promise<void> {
	return api(`/api/v1/projects/${id}`, { method: 'DELETE' });
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

export function setProjectPath(id: string, path: string): Promise<void> {
	return api(`/api/v1/projects/${id}/path`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ path })
	});
}

export function setProjectOldPaths(id: string, oldPaths: string): Promise<void> {
	return api(`/api/v1/projects/${id}/old-paths`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ oldPaths })
	});
}

export function getProject(
	id: string,
	page?: number,
	pageSize?: number,
	fetchFn?: APIFetch,
	filters?: {
		agent?: string;
		rating?: number;
		hiddenOnly?: boolean;
		search?: string;
		start?: number;
		end?: number;
		order?: string;
	}
): Promise<ProjectDetail> {
	const params = new URLSearchParams();
	if (page !== undefined) params.set('page', String(page));
	if (pageSize !== undefined) params.set('pageSize', String(pageSize));
	if (filters?.agent) params.set('agent', filters.agent);
	if (filters?.rating !== undefined && filters.rating !== 0)
		params.set('rating', String(filters.rating));
	if (filters?.hiddenOnly) params.set('hidden', 'true');
	if (filters?.search) params.set('search', filters.search);
	if (filters?.start) params.set('start', String(filters.start));
	if (filters?.end) params.set('end', String(filters.end));
	if (filters?.order === 'asc') params.set('order', 'asc');
	const q = params.size > 0 ? `?${params.toString()}` : '';
	return api(`/api/v1/projects/${id}${q}`, undefined, fetchFn);
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

export function listConversations(hiddenOnly = false): Promise<Conversation[]> {
	return api(`/api/v1/conversations${hiddenOnly ? '?hidden=true' : ''}`);
}

export function getConversation(id: string, fetchFn?: APIFetch): Promise<ConversationDetail> {
	return api(`/api/v1/conversations/${id}`, undefined, fetchFn);
}

export function setConversationHidden(
	id: string,
	hidden: boolean
): Promise<{ conversationId: string; hidden: boolean; queued: boolean }> {
	return api(`/api/v1/conversations/${id}/hidden`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ hidden })
	});
}

export function setCommitOverrideLinePercent(
	projectId: string,
	commitHash: string,
	value: number | null
): Promise<void> {
	return api(
		`/api/v1/projects/${encodeURIComponent(projectId)}/commits/${encodeURIComponent(commitHash)}/override-line-percent`,
		{
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ overrideLinePercent: value })
		}
	);
}

export function deepenCommit(
	projectId: string,
	commitHash: string,
	branch: string
): Promise<{ needsParent: boolean; success: boolean }> {
	return api(
		`/api/v1/projects/${encodeURIComponent(projectId)}/commits/${encodeURIComponent(commitHash)}/deepen?branch=${encodeURIComponent(branch)}`,
		{ method: 'POST' }
	);
}

export function recalculateCommitDiffMatch(
	projectId: string,
	commitHash: string
): Promise<{ recomputed: number }> {
	return api(
		`/api/v1/projects/${encodeURIComponent(projectId)}/commits/${encodeURIComponent(commitHash)}/recalculate`,
		{ method: 'POST' }
	);
}

export function getConversationsBatchDetail(ids: string[]): Promise<ConversationBatchDetail[]> {
	return api(`/api/v1/conversations/batch-detail?ids=${ids.map(encodeURIComponent).join(',')}`);
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
	branch = '',
	pageSize = 10,
	user = '',
	agent = '',
	search = '',
	start?: number,
	end?: number,
	dailyWindowDays?: number,
	dailyWindowEnd?: number,
	order?: string
): Promise<ProjectCommitPageResponse> {
	const params = new URLSearchParams({ page: String(page), pageSize: String(pageSize) });
	if (branch) params.set('branch', branch);
	if (user) params.set('user', user);
	if (agent) params.set('agent', agent);
	if (search) params.set('search', search);
	if (start) params.set('start', String(start));
	if (end) params.set('end', String(end));
	if (dailyWindowDays) params.set('dailyWindowDays', String(dailyWindowDays));
	if (dailyWindowEnd) params.set('dailyWindowEnd', String(dailyWindowEnd));
	if (order === 'asc') params.set('order', 'asc');
	params.set('tzOffset', String(new Date().getTimezoneOffset()));
	return api(`/api/v1/projects/${projectId}/commits?${params.toString()}`);
}

export function listSearchProjects(
	query: string,
	projectId = '',
	fetchFn?: APIFetch
): Promise<ProjectSearchMatch[]> {
	const params = new URLSearchParams();
	if (query) params.set('q', query);
	if (projectId) params.set('projectId', projectId);
	return api(`/api/v1/search/projects?${params.toString()}`, undefined, fetchFn);
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
	branch = '',
	days?: number
): Promise<{ queued: boolean }> {
	const params = new URLSearchParams();
	if (branch) params.set('branch', branch);
	if (days && days > 0) params.set('days', String(days));
	const q = params.size > 0 ? `?${params.toString()}` : '';
	return api(`/api/v1/projects/${projectId}/refresh-commits${q}`, {
		method: 'POST'
	});
}

export function scanHistory(timeframe: string, agent = ''): Promise<ImportProjectsResponse> {
	return api('/api/v1/history/scan', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ timeframe, agent })
	});
}

export function discoverImportableProjects(days = 30): Promise<DiscoverImportableProjectsResponse> {
	return api(`/api/v1/projects/discover-importable?days=${days}`);
}

export function importProjects(
	paths: string[],
	historyDays: string
): Promise<ImportProjectsResponse> {
	return api('/api/v1/projects/import', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ paths, historyDays })
	});
}

export function listTeamServers(fetchFn?: APIFetch): Promise<TeamServer[]> {
	return api('/api/v1/team-servers', undefined, fetchFn);
}

export function createTeamServer(label: string, url: string, apiKey: string): Promise<TeamServer> {
	return api('/api/v1/team-servers', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ label, url, apiKey })
	});
}

export function updateTeamServer(
	id: string,
	label: string,
	url: string,
	apiKey: string
): Promise<void> {
	return api(`/api/v1/team-servers/${id}`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ label, url, apiKey })
	});
}

export function deleteTeamServer(id: string): Promise<void> {
	return api(`/api/v1/team-servers/${id}`, { method: 'DELETE' });
}

export function setProjectTeamServer(id: string, teamServerId: string): Promise<void> {
	return api(`/api/v1/projects/${id}/team-server`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ teamServerId })
	});
}

export function getCommitConversationLinks(
	projectId: string,
	commitHashes: string[],
	conversationIds: string[]
): Promise<CommitConversationLinks> {
	// Use POST for large payloads to avoid URL length limits.
	if (commitHashes.length > 50 || conversationIds.length > 50) {
		return api(`/api/v1/projects/${projectId}/commit-conversation-links`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({
				commitHashes,
				conversationIds: conversationIds.length > 0 ? conversationIds : undefined
			})
		});
	}
	const params = new URLSearchParams();
	params.set('commitHashes', commitHashes.join(','));
	if (conversationIds.length > 0) {
		params.set('conversationIds', conversationIds.join(','));
	}
	return api(`/api/v1/projects/${projectId}/commit-conversation-links?${params.toString()}`);
}
