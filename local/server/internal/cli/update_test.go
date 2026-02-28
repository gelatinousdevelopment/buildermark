package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/updater"
)

type mockUpdater struct {
	result   *updater.UpdateResult
	checkErr error
	applyErr error
	applied  bool
}

func (m *mockUpdater) Check() (*updater.UpdateResult, error) {
	return m.result, m.checkErr
}

func (m *mockUpdater) Apply(r *updater.UpdateResult) error {
	m.applied = true
	return m.applyErr
}

func TestRunUpdateCheck_NoUpdate(t *testing.T) {
	var buf bytes.Buffer
	u := &mockUpdater{
		result: &updater.UpdateResult{
			CurrentVersion: "1.0.0",
			HasUpdate:      false,
		},
	}

	if err := RunUpdateCheck(&buf, u); err != nil {
		t.Fatalf("RunUpdateCheck() error: %v", err)
	}
	if !strings.Contains(buf.String(), "up to date") {
		t.Errorf("RunUpdateCheck() = %q, want to contain 'up to date'", buf.String())
	}
}

func TestRunUpdateCheck_HasUpdate(t *testing.T) {
	var buf bytes.Buffer
	u := &mockUpdater{
		result: &updater.UpdateResult{
			CurrentVersion: "1.0.0",
			LatestVersion:  "2.0.0",
			HasUpdate:      true,
		},
	}

	if err := RunUpdateCheck(&buf, u); err != nil {
		t.Fatalf("RunUpdateCheck() error: %v", err)
	}
	if !strings.Contains(buf.String(), "2.0.0") {
		t.Errorf("RunUpdateCheck() = %q, want to contain '2.0.0'", buf.String())
	}
}

func TestRunUpdateCheck_Error(t *testing.T) {
	var buf bytes.Buffer
	u := &mockUpdater{checkErr: fmt.Errorf("network error")}

	err := RunUpdateCheck(&buf, u)
	if err == nil {
		t.Error("RunUpdateCheck() expected error, got nil")
	}
}

func TestRunUpdateApply_Success(t *testing.T) {
	var buf bytes.Buffer
	u := &mockUpdater{
		result: &updater.UpdateResult{
			CurrentVersion: "1.0.0",
			LatestVersion:  "2.0.0",
			HasUpdate:      true,
		},
	}

	if err := RunUpdateApply(&buf, u); err != nil {
		t.Fatalf("RunUpdateApply() error: %v", err)
	}
	if !u.applied {
		t.Error("RunUpdateApply() did not call Apply()")
	}
	if !strings.Contains(buf.String(), "Updated to 2.0.0") {
		t.Errorf("RunUpdateApply() = %q, want to contain 'Updated to 2.0.0'", buf.String())
	}
}

func TestRunUpdateApply_NoUpdate(t *testing.T) {
	var buf bytes.Buffer
	u := &mockUpdater{
		result: &updater.UpdateResult{
			CurrentVersion: "1.0.0",
			HasUpdate:      false,
		},
	}

	if err := RunUpdateApply(&buf, u); err != nil {
		t.Fatalf("RunUpdateApply() error: %v", err)
	}
	if u.applied {
		t.Error("RunUpdateApply() called Apply() when no update available")
	}
}

func TestRunUpdateSetMode(t *testing.T) {
	tests := []struct {
		mode    string
		wantErr bool
	}{
		{"auto", false},
		{"check", false},
		{"off", false},
		{"invalid", true},
		{"", true},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			dir := t.TempDir()
			err := RunUpdateSetMode(dir, tt.mode)
			if (err != nil) != tt.wantErr {
				t.Errorf("RunUpdateSetMode(%q) error = %v, wantErr = %v", tt.mode, err, tt.wantErr)
			}
			if !tt.wantErr {
				cfg, err := LoadConfig(dir)
				if err != nil {
					t.Fatalf("LoadConfig() error: %v", err)
				}
				if cfg.UpdateMode != tt.mode {
					t.Errorf("UpdateMode = %q, want %q", cfg.UpdateMode, tt.mode)
				}
			}
		})
	}
}
