let _projectName = $state<string | null>(null);

export const navStore = {
	get projectName() {
		return _projectName;
	},
	set projectName(v: string | null) {
		_projectName = v;
	}
};
