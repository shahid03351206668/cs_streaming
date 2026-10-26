package middleware

import (
	"sync"
	"time"
)

// LoginLimiter blocks a key after too many failures inside a window. It is
// in-memory, so limits are per server instance.
type LoginLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	fails  map[string][]time.Time
}

func NewLoginLimiter(max int, window time.Duration) *LoginLimiter {
	return &LoginLimiter{max: max, window: window, fails: make(map[string][]time.Time)}
}

func (l *LoginLimiter) prune(key string, now time.Time) []time.Time {
	recent := l.fails[key][:0]
	for _, t := range l.fails[key] {
		if now.Sub(t) < l.window {
			recent = append(recent, t)
		}
	}
	if len(recent) == 0 {
		delete(l.fails, key)
		return nil
	}
	l.fails[key] = recent
	return recent
}

// Blocked reports whether key is locked out and for how much longer.
func (l *LoginLimiter) Blocked(key string) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	recent := l.prune(key, now)
	if len(recent) < l.max {
		return 0, false
	}
	return l.window - now.Sub(recent[0]), true
}

func (l *LoginLimiter) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.fails[key] = append(l.fails[key], time.Now())
}

func (l *LoginLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, key)
}
