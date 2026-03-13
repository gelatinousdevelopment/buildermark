package cli

import "testing"

type mockOpener struct {
	url string
	err error
}

func (m *mockOpener) Open(url string) error {
	m.url = url
	return m.err
}

func TestRunOpen(t *testing.T) {
	m := &mockOpener{}
	if err := RunOpen(m, ":55022"); err != nil {
		t.Fatalf("RunOpen() error: %v", err)
	}
	want := "http://localhost:55022"
	if m.url != want {
		t.Errorf("RunOpen() opened %q, want %q", m.url, want)
	}
}

func TestXDGOpener(t *testing.T) {
	cmd := &mockCommander{}
	opener := XDGOpener{Cmd: cmd}
	if err := opener.Open("http://localhost:55022"); err != nil {
		t.Fatalf("XDGOpener.Open() error: %v", err)
	}
	want := "xdg-open http://localhost:55022"
	if len(cmd.calls) != 1 || cmd.calls[0] != want {
		t.Errorf("XDGOpener.Open() ran %v, want [%q]", cmd.calls, want)
	}
}
