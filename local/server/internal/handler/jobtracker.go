package handler

import "sync"

// jobTracker manages a set of in-progress background jobs keyed by string.
// It provides a thread-safe tryStart/finish pattern to prevent duplicate
// concurrent execution of the same job.
type jobTracker struct {
	mu      sync.Mutex
	running map[string]bool
}

func newJobTracker() *jobTracker {
	return &jobTracker{running: make(map[string]bool)}
}

// tryStart attempts to mark a job as running. Returns true if the job was
// not already running and has been started. Returns false if a job with the
// same key is already in progress.
func (t *jobTracker) tryStart(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.running[key] {
		return false
	}
	t.running[key] = true
	return true
}

// finish marks a job as no longer running.
func (t *jobTracker) finish(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.running, key)
}

// isIdle returns true if no jobs are currently running.
func (t *jobTracker) isIdle() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.running) == 0
}
