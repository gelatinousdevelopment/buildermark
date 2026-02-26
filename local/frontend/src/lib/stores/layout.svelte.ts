let _fixedHeight = $state(false);
let _hideContainer = $state(false);

export const layoutStore = {
	get fixedHeight() {
		return _fixedHeight;
	},
	set fixedHeight(v: boolean) {
		_fixedHeight = v;
	},
	get hideContainer() {
		return _hideContainer;
	},
	set hideContainer(v: boolean) {
		_hideContainer = v;
	}
};
