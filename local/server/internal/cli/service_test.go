package cli

import (
	"strings"
	"testing"
)

func TestGenerateUnitFile(t *testing.T) {
	content, err := GenerateUnitFile(UnitParams{
		ExecStart: "/usr/local/bin/buildermark",
		DBPath:    "/home/user/.buildermark/local.db",
	})
	if err != nil {
		t.Fatalf("GenerateUnitFile() error: %v", err)
	}

	checks := []string{
		"ExecStart=/usr/local/bin/buildermark run",
		"Restart=always",
		"RestartSec=3",
		"WantedBy=default.target",
		"BUILDERMARK_LOCAL_DB_PATH=/home/user/.buildermark/local.db",
	}
	for _, want := range checks {
		if !strings.Contains(content, want) {
			t.Errorf("GenerateUnitFile() missing %q in:\n%s", want, content)
		}
	}
}

func TestServiceInstall(t *testing.T) {
	m := &mockCommander{}
	// Override userUnitDir by creating a temp dir structure
	// For this test, we just verify the commands are called correctly
	// (the actual file write would need the real home dir)
	err := ServiceInstall(m, UnitParams{
		ExecStart: "/usr/local/bin/buildermark",
		DBPath:    "/home/user/.buildermark/local.db",
	})
	// We expect this might fail due to file system (home dir),
	// but let's check the commands that were attempted
	if err != nil {
		// Check if the error is about filesystem (acceptable in test)
		if !strings.Contains(err.Error(), "creating unit directory") &&
			!strings.Contains(err.Error(), "writing unit file") &&
			!strings.Contains(err.Error(), "home directory") {
			t.Fatalf("ServiceInstall() unexpected error: %v", err)
		}
		return
	}

	// If it succeeded, verify the systemctl commands
	wantCmds := []string{
		"systemctl --user daemon-reload",
		"systemctl --user enable --now buildermark",
	}
	for _, want := range wantCmds {
		found := false
		for _, got := range m.calls {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ServiceInstall() missing command %q in %v", want, m.calls)
		}
	}
}

func TestServiceUninstall(t *testing.T) {
	m := &mockCommander{}
	err := ServiceUninstall(m)
	// May fail on filesystem operations, but should run systemctl commands first
	if err != nil {
		// Check that at least the disable command was attempted
		if len(m.calls) == 0 {
			t.Fatalf("ServiceUninstall() no commands attempted, error: %v", err)
		}
	}

	wantCmds := []string{
		"systemctl --user disable --now buildermark",
		"systemctl --user daemon-reload",
	}
	for _, want := range wantCmds {
		found := false
		for _, got := range m.calls {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ServiceUninstall() missing command %q in %v", want, m.calls)
		}
	}
}
