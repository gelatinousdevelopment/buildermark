import { PUBLIC_API_URL } from '$env/static/public';

export type ConnectionState = 'connecting' | 'connected' | 'disconnected';

export type ImportStatus = {
	state: 'running' | 'complete' | 'error';
	message: string;
	projectsImported: number;
	entriesProcessed: number;
	commitsIngested: number;
};

type WSMessage = {
	type: string;
	data: unknown;
};

let _connectionState = $state<ConnectionState>('disconnected');
let _importStatus = $state<ImportStatus | null>(null);

let _ws: WebSocket | null = null;
let _reconnectTimer: ReturnType<typeof setTimeout> | null = null;
let _reconnectAttempts = 0;
const MAX_RECONNECT_DELAY = 10_000;

type ImportResolver = {
	resolve: (status: ImportStatus) => void;
	reject: (error: Error) => void;
};
let _importWaiters: ImportResolver[] = [];

export function getWsUrl(): string {
	if (PUBLIC_API_URL) {
		return PUBLIC_API_URL.replace(/^http/, 'ws') + '/api/v1/ws';
	}
	const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
	return `${proto}//${window.location.host}/api/v1/ws`;
}

function connect() {
	if (_ws && (_ws.readyState === WebSocket.CONNECTING || _ws.readyState === WebSocket.OPEN)) {
		return;
	}

	_connectionState = 'connecting';
	const ws = new WebSocket(getWsUrl());
	_ws = ws;

	ws.onopen = () => {
		_connectionState = 'connected';
		_reconnectAttempts = 0;
	};

	ws.onclose = () => {
		_connectionState = 'disconnected';
		_ws = null;

		// Reject any pending import waiters on disconnect.
		for (const waiter of _importWaiters) {
			waiter.reject(new Error('WebSocket disconnected'));
		}
		_importWaiters = [];

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
	if (msg.type === 'import_status') {
		const status = msg.data as ImportStatus;
		_importStatus = status;

		if (status.state === 'complete' || status.state === 'error') {
			for (const waiter of _importWaiters) {
				waiter.resolve(status);
			}
			_importWaiters = [];
		}
	}
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
 * Returns a promise that resolves when the current import job reaches
 * a terminal state ("complete" or "error"). If the WebSocket disconnects
 * before the import finishes, the promise rejects.
 */
function waitForImportComplete(): Promise<ImportStatus> {
	return new Promise((resolve, reject) => {
		_importWaiters.push({ resolve, reject });
	});
}

function clearImportStatus() {
	_importStatus = null;
}

export const websocketStore = {
	get connectionState() {
		return _connectionState;
	},
	get importStatus() {
		return _importStatus;
	},
	connect,
	disconnect,
	waitForImportComplete,
	clearImportStatus
};
