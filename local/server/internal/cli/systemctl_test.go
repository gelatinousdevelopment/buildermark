package cli

import (
	"fmt"
	"strings"
	"testing"
)

// mockCommander records commands instead of executing them.
type mockCommander struct {
	calls []string
	err   error // if set, all calls return this error
}

func (m *mockCommander) Run(name string, args ...string) error {
	m.calls = append(m.calls, name+" "+strings.Join(args, " "))
	return m.err
}

func (m *mockCommander) RunAttached(name string, args ...string) error {
	m.calls = append(m.calls, name+" "+strings.Join(args, " "))
	return m.err
}

func TestRunSystemctl(t *testing.T) {
	tests := []struct {
		action  string
		wantCmd string
		wantErr bool
	}{
		{"start", "systemctl --user start buildermark", false},
		{"stop", "systemctl --user stop buildermark", false},
		{"restart", "systemctl --user restart buildermark", false},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			m := &mockCommander{}
			err := RunSystemctl(m, tt.action)
			if tt.wantErr {
				if err == nil {
					t.Error("RunSystemctl() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("RunSystemctl() error: %v", err)
			}
			if len(m.calls) != 1 || m.calls[0] != tt.wantCmd {
				t.Errorf("RunSystemctl() ran %v, want [%q]", m.calls, tt.wantCmd)
			}
		})
	}
}

func TestRunLogs(t *testing.T) {
	m := &mockCommander{}
	if err := RunLogs(m, 100); err != nil {
		t.Fatalf("RunLogs() error: %v", err)
	}
	want := "journalctl --user --lines 100 -u buildermark -f"
	if len(m.calls) != 1 || m.calls[0] != want {
		t.Errorf("RunLogs() ran %v, want [%q]", m.calls, want)
	}
}

func TestRunSystemctl_CommandError(t *testing.T) {
	m := &mockCommander{err: fmt.Errorf("command failed")}
	err := RunSystemctl(m, "start")
	if err == nil {
		t.Error("RunSystemctl() expected error when command fails, got nil")
	}
}
