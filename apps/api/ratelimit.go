package main

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	apiRateLimitMaxEntries = 2048
	apiMutationLimit       = 60
	apiAuthLimit           = 20
	apiRateLimitWindow     = time.Minute
)

type rateLimitBucket struct {
	started time.Time
	count   int
}

// boundedRateLimiter is intentionally process-local. Its entry cap prevents an
// attacker from turning a high-cardinality client key into unbounded memory use.
type boundedRateLimiter struct {
	mu      sync.Mutex
	entries map[string]rateLimitBucket
	maxKeys int
	limit   int
	window  time.Duration
	now     func() time.Time
}

func newBoundedRateLimiter(maxKeys, limit int, window time.Duration) *boundedRateLimiter {
	return &boundedRateLimiter{
		entries: make(map[string]rateLimitBucket),
		maxKeys: maxKeys,
		limit:   limit,
		window:  window,
		now:     time.Now,
	}
}

func (l *boundedRateLimiter) allow(key string) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	entry, ok := l.entries[key]
	if !ok || now.Sub(entry.started) >= l.window {
		if !ok && len(l.entries) >= l.maxKeys {
			l.evictOldest()
		}
		l.entries[key] = rateLimitBucket{started: now, count: 1}
		return true
	}
	if entry.count >= l.limit {
		return false
	}
	entry.count++
	l.entries[key] = entry
	return true
}

func (l *boundedRateLimiter) evictOldest() {
	var oldestKey string
	var oldest time.Time
	for key, entry := range l.entries {
		if oldestKey == "" || entry.started.Before(oldest) {
			oldestKey, oldest = key, entry.started
		}
	}
	if oldestKey != "" {
		delete(l.entries, oldestKey)
	}
}

func newAPIRateLimiter() *boundedRateLimiter {
	return newBoundedRateLimiter(apiRateLimitMaxEntries, apiMutationLimit, apiRateLimitWindow)
}

func rateLimitClass(r *http.Request) (string, bool) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return "", false
	}
	path := r.URL.Path
	if !strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/api/v1/jobs/") || path == "/api/v1/runners" || path == "/api/v1/runners/heartbeat" {
		return "", false
	}
	if strings.HasPrefix(path, "/api/v1/auth/") {
		return "auth", true
	}
	return "mutation", true
}

func rateLimitClient(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	if r.RemoteAddr != "" {
		return r.RemoteAddr
	}
	return "unknown"
}
