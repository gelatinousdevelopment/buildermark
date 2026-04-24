import { beforeEach, describe, expect, it } from 'vitest';
import { dismissInstalledUpdateStatus, normalizeUpdateStatus } from './updateStatus';

const DISMISSED_INSTALLED_UPDATE_KEY = 'buildermark_dismissed_installed_update_version';

describe('update status dismissal', () => {
	beforeEach(() => {
		localStorage.removeItem(DISMISSED_INSTALLED_UPDATE_KEY);
	});

	it('hides a dismissed installed update when status is rehydrated', () => {
		dismissInstalledUpdateStatus({
			state: 'installed',
			version: 'v1.2.3',
			previousVersion: 'v1.2.2',
			platform: 'darwin'
		});

		const status = normalizeUpdateStatus({
			state: 'installed',
			version: 'v1.2.3',
			previousVersion: 'v1.2.2',
			platform: 'darwin'
		});

		expect(status).toEqual({ state: 'none', platform: 'darwin' });
	});

	it('still shows the next installed version after an older dismissal', () => {
		dismissInstalledUpdateStatus({
			state: 'installed',
			version: 'v1.2.3',
			previousVersion: 'v1.2.2',
			platform: 'darwin'
		});

		const status = normalizeUpdateStatus({
			state: 'installed',
			version: 'v1.2.4',
			previousVersion: 'v1.2.3',
			platform: 'darwin'
		});

		expect(status).toEqual({
			state: 'installed',
			version: 'v1.2.4',
			previousVersion: 'v1.2.3',
			platform: 'darwin'
		});
	});

	it('does not hide available updates for a dismissed installed version', () => {
		dismissInstalledUpdateStatus({
			state: 'installed',
			version: 'v1.2.3',
			previousVersion: 'v1.2.2',
			platform: 'darwin'
		});

		const status = normalizeUpdateStatus({
			state: 'available',
			version: 'v1.2.3',
			platform: 'darwin'
		});

		expect(status).toEqual({
			state: 'available',
			version: 'v1.2.3',
			platform: 'darwin'
		});
	});
});
