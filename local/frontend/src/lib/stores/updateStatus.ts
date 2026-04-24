import { browser } from '$app/environment';

const DISMISSED_INSTALLED_UPDATE_KEY = 'buildermark_dismissed_installed_update_version';

export type UpdateStatus = {
	state: 'available' | 'installed' | 'none';
	version?: string;
	previousVersion?: string;
	platform?: string;
	releaseNotesUrl?: string;
};

function installedUpdateVersion(status: UpdateStatus): string | null {
	return status.state === 'installed' && status.version ? status.version : null;
}

function getDismissedInstalledUpdateVersion(): string | null {
	if (!browser) return null;
	try {
		return localStorage.getItem(DISMISSED_INSTALLED_UPDATE_KEY);
	} catch {
		return null;
	}
}

function setDismissedInstalledUpdateVersion(version: string): void {
	if (!browser) return;
	try {
		localStorage.setItem(DISMISSED_INSTALLED_UPDATE_KEY, version);
	} catch {
		// ignore storage errors
	}
}

export function dismissInstalledUpdateStatus(status: UpdateStatus): void {
	const version = installedUpdateVersion(status);
	if (version) {
		setDismissedInstalledUpdateVersion(version);
	}
}

export function isDismissedInstalledUpdate(status: UpdateStatus): boolean {
	const version = installedUpdateVersion(status);
	return version !== null && getDismissedInstalledUpdateVersion() === version;
}

export function normalizeUpdateStatus(status: UpdateStatus): UpdateStatus {
	return isDismissedInstalledUpdate(status) ? { state: 'none', platform: status.platform } : status;
}
