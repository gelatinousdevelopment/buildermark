package handler

import (
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
	"time"
)

// commitDetailCacheStore is a thread-safe cache for commit detail page results.
type commitDetailCacheStore struct {
	mu    sync.RWMutex
	items map[string]*commitDetailCacheEntry
	ttl   time.Duration
}

func newCommitDetailCacheStore() *commitDetailCacheStore {
	return &commitDetailCacheStore{
		items: make(map[string]*commitDetailCacheEntry),
		ttl:   5 * time.Minute,
	}
}

func (c *commitDetailCacheStore) get(key string) (*commitDetailCacheEntry, bool) {
	c.mu.RLock()
	entry, ok := c.items[key]
	if ok && time.Since(entry.fetchedAt) >= c.ttl {
		ok = false
	}
	c.mu.RUnlock()
	return entry, ok
}

func (c *commitDetailCacheStore) set(key string, entry *commitDetailCacheEntry) {
	c.mu.Lock()
	c.items[key] = entry
	c.mu.Unlock()
}

func (c *commitDetailCacheStore) clearProject(projectID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	prefix := projectID + ":"
	for key := range c.items {
		if strings.HasPrefix(key, prefix) {
			delete(c.items, key)
		}
	}
}

func commitDetailCacheKey(projectID, commitHash string, ignorePatterns []string) string {
	h := fnv.New64a()
	for _, p := range ignorePatterns {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return fmt.Sprintf("%s:%s:%d:%x", projectID, commitHash, currentCommitCoverageVersion, h.Sum64())
}

// branchCacheStore is a thread-safe cache for branch list results.
type branchCacheStore struct {
	mu    sync.RWMutex
	items map[string]branchCacheEntry
	ttl   time.Duration
}

func newBranchCacheStore() *branchCacheStore {
	return &branchCacheStore{
		items: make(map[string]branchCacheEntry),
		ttl:   30 * time.Second,
	}
}

func (c *branchCacheStore) get(key string) ([]string, bool) {
	c.mu.RLock()
	entry, ok := c.items[key]
	if ok && time.Since(entry.fetchedAt) >= c.ttl {
		ok = false
	}
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return entry.branches, true
}

func (c *branchCacheStore) set(key string, branches []string) {
	c.mu.Lock()
	c.items[key] = branchCacheEntry{branches: branches, fetchedAt: time.Now()}
	c.mu.Unlock()
}
