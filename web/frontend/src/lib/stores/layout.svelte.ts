let _fixedHeight = $state(false);

export const layoutStore = {
	get fixedHeight() {
		return _fixedHeight;
	},
	set fixedHeight(v: boolean) {
		_fixedHeight = v;
	}
};
