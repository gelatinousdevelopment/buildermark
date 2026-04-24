import { API_URL } from '$lib/config';
import { env } from '$env/dynamic/public';
import {
	dismissInstalledUpdateStatus,
	normalizeUpdateStatus,
	type UpdateStatus
} from '$lib/stores/updateStatus';

const READ_ONLY = (env.PUBLIC_READ_ONLY ?? 'false') === 'true';

export type ConnectionState = 'connecting' | 'connected' | 'disconnected';

export type JobStatus = {
	jobType: string;
	state: 'running' | 'complete' | 'error';
	message: string;
	projectId?: string;
	branch?: string;
};

export type WSClients = {
	frontend: number;
	notification: number;
};

export type { UpdateStatus } from '$lib/stores/updateStatus';

type WSMessage = {
	type: string;
	data: unknown;
};

let _connectionState = $state<ConnectionState>('disconnected');
let _activeJobs = $state<Record<string, JobStatus>>({});
let _wsClients = $state<WSClients>({ frontend: 0, notification: 0 });
let _updateStatus = $state<UpdateStatus>({ state: 'none' });

let _ws: WebSocket | null = null;
let _reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let _reconnectAttempts = 0;
const MAX_RECONNECT_DELAY = 10_000;

type JobResolver = {
	resolve: (status: JobStatus) => void;
	reject: (error: Error) => void;
};
// eslint-disable-next-line svelte/prefer-svelte-reactivity -- not reactive state
const _jobWaiters: Map<string, JobResolver[]> = new Map();

export function getWsUrl(): string {
	if (API_URL) {
		return API_URL.replace(/^http/, 'ws') + '/api/v1/ws';
	}
	const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
	return `${proto}//${window.location.host}/api/v1/ws`;
}

function connect() {
	if (
		READ_ONLY ||
		(_ws && (_ws.readyState === WebSocket.CONNECTING || _ws.readyState === WebSocket.OPEN))
	) {
		return;
	}

	_connectionState = 'connecting';
	const ws = new WebSocket(getWsUrl());
	_ws = ws;

	ws.onopen = () => {
		_connectionState = 'connected';
		_reconnectAttempts = 0;
		hydrateUpdateStatus();
	};

	ws.onclose = () => {
		_connectionState = 'disconnected';
		_ws = null;

		// Reject any pending job waiters on disconnect.
		for (const [, waiters] of _jobWaiters) {
			for (const waiter of waiters) {
				waiter.reject(new Error('WebSocket disconnected'));
			}
		}
		_jobWaiters.clear();

		scheduleReconnect();
	};

	ws.onerror = () => {
		// onclose fires after onerror, so reconnect is handled there.
	};

	ws.onmessage = (event) => {
		try {
			const msg = JSON.parse(event.data) as WSMessage;
			handleMessage(msg);
		} catch {
			// ignore malformed messages
		}
	};
}

function handleMessage(msg: WSMessage) {
	if (msg.type === 'job_status') {
		const status = msg.data as JobStatus;
		_activeJobs = { ..._activeJobs, [status.jobType]: status };

		if (status.state === 'complete' || status.state === 'error') {
			const waiters = _jobWaiters.get(status.jobType);
			if (waiters) {
				for (const waiter of waiters) {
					waiter.resolve(status);
				}
				_jobWaiters.delete(status.jobType);
			}
		}
	} else if (msg.type === 'ws_clients') {
		_wsClients = msg.data as WSClients;
	} else if (msg.type === 'update_status') {
		setUpdateStatus(msg.data as UpdateStatus);
	}
}

function setUpdateStatus(status: UpdateStatus) {
	_updateStatus = normalizeUpdateStatus(status);
}

function scheduleReconnect() {
	if (_reconnectTimer) return;
	const delay = Math.min(1000 * Math.pow(2, _reconnectAttempts), MAX_RECONNECT_DELAY);
	_reconnectAttempts++;
	_reconnectTimer = setTimeout(() => {
		_reconnectTimer = null;
		connect();
	}, delay);
}

function disconnect() {
	if (_reconnectTimer) {
		clearTimeout(_reconnectTimer);
		_reconnectTimer = null;
	}
	if (_ws) {
		_ws.close();
		_ws = null;
	}
	_connectionState = 'disconnected';
}

/**
 * Returns a promise that resolves when the specified job type reaches
 * a terminal state ("complete" or "error"). If the WebSocket disconnects
 * before the job finishes, the promise rejects.
 */
function waitForJob(jobType: string): Promise<JobStatus> {
	return new Promise((resolve, reject) => {
		const waiters = _jobWaiters.get(jobType) ?? [];
		waiters.push({ resolve, reject });
		_jobWaiters.set(jobType, waiters);
	});
}

function hydrateUpdateStatus() {
	const url = API_URL
		? `${API_URL}/api/v1/update-status`
		: `${window.location.origin}/api/v1/update-status`;
	fetch(url)
		.then((r) => r.json())
		.then((envelope) => {
			if (envelope.ok && envelope.data) {
				setUpdateStatus(envelope.data as UpdateStatus);
			}
		})
		.catch(() => {});
}

function clearUpdateStatus() {
	_updateStatus = { state: 'none' };
}

function dismissInstalledUpdate() {
	dismissInstalledUpdateStatus(_updateStatus);
	clearUpdateStatus();
}

function clearJob(jobType: string) {
	const { [jobType]: _removed, ...rest } = _activeJobs;
	void _removed;
	_activeJobs = rest;
}

function getJob(jobType: string): JobStatus | null {
	return _activeJobs[jobType] ?? null;
}

export const websocketStore = {
	get connectionState() {
		return _connectionState;
	},
	get activeJobs() {
		return _activeJobs;
	},
	getJob,
	get hasActiveJob() {
		return Object.values(_activeJobs).some((j) => j.state === 'running');
	},
	get wsClients() {
		return _wsClients;
	},
	get updateStatus() {
		return _updateStatus;
	},
	connect,
	disconnect,
	waitForJob,
	clearJob,
	dismissInstalledUpdate,
	clearUpdateStatus
};
