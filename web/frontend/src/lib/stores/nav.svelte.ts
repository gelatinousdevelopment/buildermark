import { SvelteMap } from 'svelte/reactivity';

let _projectName = $state<string | null>(null);
const _labelCache = new SvelteMap<string, string>();

export const navStore = {
	get projectName() {
		return _projectName;
	},
	set projectName(v: string | null) {
		_projectName = v;
	},
	getCachedLabel(projectId: string): string | undefined {
		return _labelCache.get(projectId);
	},
	setCachedLabel(projectId: string, label: string) {
		_labelCache.set(projectId, label);
	}
};
