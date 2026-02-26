let _selectedDate: string | null = $state(null);
let _projectId: string = $state('');

export const projectDateFilterStore = {
	get selectedDate() {
		return _selectedDate;
	},
	set selectedDate(value: string | null) {
		_selectedDate = value;
	},
	get projectId() {
		return _projectId;
	},
	/** Reset the filter when the project changes. */
	setProjectId(id: string) {
		if (id !== _projectId) {
			_projectId = id;
			_selectedDate = null;
		}
	}
};
