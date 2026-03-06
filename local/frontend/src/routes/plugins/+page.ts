import { listPlugins } from '$lib/api';
import type { PluginInventoryResponse } from '$lib/types';

export async function load({ fetch }) {
	let inventory: PluginInventoryResponse | null = null;
	let error: string | null = null;

	try {
		inventory = await listPlugins(fetch);
	} catch (e) {
		error = e instanceof Error ? e.message : 'Failed to load plugins';
	}

	return { inventory, error };
}
