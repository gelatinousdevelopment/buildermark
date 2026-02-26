package db

import "testing"

func TestNewID(t *testing.T) {
	const n = 5000
	seen := make(map[string]struct{}, n)

	for i := 0; i < n; i++ {
		id := newID()
		if len(id) != 21 {
			t.Fatalf("len(id) = %d, want 21 (id=%q)", len(id), id)
		}
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate id generated: %q", id)
		}
		seen[id] = struct{}{}
	}
}
