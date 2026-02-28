package updater

import "testing"

func TestGetUpdater_ReturnsNonNil(t *testing.T) {
	u := GetUpdater("1.0.0")
	if u == nil {
		t.Error("GetUpdater() returned nil")
	}
}
