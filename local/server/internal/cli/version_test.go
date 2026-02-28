package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	var buf bytes.Buffer
	RunVersion(&buf)
	got := buf.String()
	if !strings.Contains(got, "buildermark") {
		t.Errorf("RunVersion() = %q, want to contain %q", got, "buildermark")
	}
	if !strings.Contains(got, Version) {
		t.Errorf("RunVersion() = %q, want to contain version %q", got, Version)
	}
}

func TestRunVersion_Dev(t *testing.T) {
	var buf bytes.Buffer
	RunVersion(&buf)
	want := "buildermark dev\n"
	if got := buf.String(); got != want {
		t.Errorf("RunVersion() = %q, want %q", got, want)
	}
}
