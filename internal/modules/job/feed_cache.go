package job

import (
	"fmt"
	"strconv"
	"sync"
	"time"
)

const (
	jobFeedCacheTTL        = 15 * time.Second
	jobFeedCacheMaxEntries = 200
)

type jobFeedCacheEntry struct {
	expiresAt time.Time
	jobs      []JobPostValue
	total     int64
}

type jobFeedCache struct {
	ttl time.Duration

	mu      sync.RWMutex
	entries map[string]jobFeedCacheEntry
}

func newJobFeedCache(ttl time.Duration) *jobFeedCache {
	if ttl <= 0 {
		return nil
	}
	return &jobFeedCache{
		ttl:     ttl,
		entries: make(map[string]jobFeedCacheEntry),
	}
}

func (c *jobFeedCache) Get(key string) (jobs []JobPostValue, total int64, ok bool) {
	if c == nil {
		return nil, 0, false
	}

	now := time.Now()

	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()

	if !exists {
		return nil, 0, false
	}
	if now.After(entry.expiresAt) {
		c.mu.Lock()
		// Re-check under write lock in case another goroutine refreshed it.
		entry2, exists2 := c.entries[key]
		if exists2 && now.After(entry2.expiresAt) {
			delete(c.entries, key)
		}
		c.mu.Unlock()
		return nil, 0, false
	}

	// Shallow copy so callers can't mutate the cached slice header.
	out := make([]JobPostValue, len(entry.jobs))
	copy(out, entry.jobs)
	return out, entry.total, true
}

func (c *jobFeedCache) Set(key string, jobs []JobPostValue, total int64) {
	if c == nil {
		return
	}

	now := time.Now()
	entry := jobFeedCacheEntry{
		expiresAt: now.Add(c.ttl),
		jobs:      make([]JobPostValue, len(jobs)),
		total:     total,
	}
	copy(entry.jobs, jobs)

	c.mu.Lock()
	c.cleanupLocked(now)
	if len(c.entries) >= jobFeedCacheMaxEntries {
		// Simple backstop to avoid unbounded growth.
		c.entries = make(map[string]jobFeedCacheEntry)
	}
	c.entries[key] = entry
	c.mu.Unlock()
}

func (c *jobFeedCache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.entries = make(map[string]jobFeedCacheEntry)
	c.mu.Unlock()
}

func (c *jobFeedCache) cleanupLocked(now time.Time) {
	for k, v := range c.entries {
		if now.After(v.expiresAt) {
			delete(c.entries, k)
		}
	}
}

func jobFeedCacheKey(params JobFeedParams) string {
	lat := ""
	lng := ""
	if params.Latitude != nil {
		lat = strconv.FormatFloat(*params.Latitude, 'f', 6, 64)
	}
	if params.Longitude != nil {
		lng = strconv.FormatFloat(*params.Longitude, 'f', 6, 64)
	}

	// Keep the key stable and explicit.
	return fmt.Sprintf(
		"v1|cat=%s|q=%s|page=%d|limit=%d|lat=%s|lng=%s|radius=%.2f",
		params.Category,
		params.SearchQuery,
		params.Page,
		params.Limit,
		lat,
		lng,
		params.RadiusKM,
	)
}
