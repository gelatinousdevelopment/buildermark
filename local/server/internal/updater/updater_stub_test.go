//go:build !cli

package updater

import (
	"errors"
	"testing"
)

func TestStubUpdater_Check(t *testing.T) {
	u := GetUpdater("1.0.0")
	_, err := u.Check()
	if err == nil {
		t.Error("stub Check() expected error, got nil")
	}
	if !errors.Is(err, errUnsupported) {
		t.Errorf("stub Check() error = %v, want errUnsupported", err)
	}
}

func TestStubUpdater_Apply(t *testing.T) {
	u := GetUpdater("1.0.0")
	err := u.Apply(&UpdateResult{})
	if err == nil {
		t.Error("stub Apply() expected error, got nil")
	}
	if !errors.Is(err, errUnsupported) {
		t.Errorf("stub Apply() error = %v, want errUnsupported", err)
	}
}
