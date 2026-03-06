package agent

import (
	"context"
	"testing"
	"time"
)

// stubAgent implements Agent only (no Watcher/SessionResolver).
type stubAgent struct{ name string }

func (s *stubAgent) Name() string { return s.name }

// stubWatcherAgent implements Agent + Watcher.
type stubWatcherAgent struct {
	name      string
	scanCount int
}

func (s *stubWatcherAgent) Name() string                                  { return s.name }
func (s *stubWatcherAgent) Run(ctx context.Context)                       {}
func (s *stubWatcherAgent) LastPollTime() time.Time                       { return time.Time{} }
func (s *stubWatcherAgent) ScanSince(ctx context.Context, since time.Time, progress ScanProgressFunc) int {
	s.scanCount++
	return 42
}

// stubResolverAgent implements Agent + SessionResolver.
type stubResolverAgent struct {
	name string
}

func (s *stubResolverAgent) Name() string { return s.name }
func (s *stubResolverAgent) ResolveSession(rating int, note string, fallbackID string) *SessionResult {
	return &SessionResult{SessionID: "resolved-" + fallbackID}
}

func TestRegistryBasics(t *testing.T) {
	r := NewRegistry()

	a := &stubAgent{name: "test"}
	r.Register(a)

	if got := r.Get("test"); got != a {
		t.Errorf("Get(test) = %v, want %v", got, a)
	}
	if got := r.Get("unknown"); got != nil {
		t.Errorf("Get(unknown) = %v, want nil", got)
	}

	names := r.Names()
	if len(names) != 1 || names[0] != "test" {
		t.Errorf("Names() = %v, want [test]", names)
	}
}

func TestRegistryWatchers(t *testing.T) {
	r := NewRegistry()

	r.Register(&stubAgent{name: "plain"})
	r.Register(&stubWatcherAgent{name: "watcher1"})
	r.Register(&stubWatcherAgent{name: "watcher2"})

	watchers := r.Watchers()
	if len(watchers) != 2 {
		t.Fatalf("Watchers() len = %d, want 2", len(watchers))
	}
	if watchers[0].Name() != "watcher1" || watchers[1].Name() != "watcher2" {
		t.Errorf("Watchers() names = [%s, %s], want [watcher1, watcher2]", watchers[0].Name(), watchers[1].Name())
	}
}

func TestRegistryResolver(t *testing.T) {
	r := NewRegistry()

	r.Register(&stubAgent{name: "plain"})
	r.Register(&stubResolverAgent{name: "resolver"})

	if got := r.Resolver("plain"); got != nil {
		t.Errorf("Resolver(plain) should be nil for non-resolver agent")
	}
	if got := r.Resolver("unknown"); got != nil {
		t.Errorf("Resolver(unknown) should be nil")
	}

	res := r.Resolver("resolver")
	if res == nil {
		t.Fatal("Resolver(resolver) should not be nil")
	}

	result := res.ResolveSession(5, "test", "fallback-id")
	if result.SessionID != "resolved-fallback-id" {
		t.Errorf("SessionID = %q, want %q", result.SessionID, "resolved-fallback-id")
	}
}

func TestRegistryReRegister(t *testing.T) {
	r := NewRegistry()

	w1 := &stubWatcherAgent{name: "a"}
	r.Register(w1)
	r.Register(&stubAgent{name: "b"})
	w2 := &stubWatcherAgent{name: "a"}
	r.Register(w2) // second agent with same name

	names := r.Names()
	if len(names) != 2 {
		t.Errorf("Names() len = %d, want 2 (deduplicated)", len(names))
	}

	// Both watchers with name "a" should be returned.
	watchers := r.Watchers()
	if len(watchers) != 2 {
		t.Errorf("Watchers() len = %d, want 2", len(watchers))
	}
}

func TestRegistryMultipleWatchersSameName(t *testing.T) {
	r := NewRegistry()

	w1 := &stubWatcherAgent{name: "claude"}
	w2 := &stubWatcherAgent{name: "claude"}
	r.Register(w1)
	r.Register(w2)

	names := r.Names()
	if len(names) != 1 {
		t.Errorf("Names() len = %d, want 1", len(names))
	}
	if names[0] != "claude" {
		t.Errorf("Names()[0] = %q, want %q", names[0], "claude")
	}

	watchers := r.Watchers()
	if len(watchers) != 2 {
		t.Fatalf("Watchers() len = %d, want 2", len(watchers))
	}

	// Get returns the first registered.
	if got := r.Get("claude"); got != w1 {
		t.Errorf("Get(claude) returned second agent, want first")
	}
}
