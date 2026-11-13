package widget

import (
	"site-assistant/internal/core"
	"sync"
	"time"
)

type window struct {
	n   int
	end time.Time
}
type Limiter struct {
	mu      sync.Mutex
	entries map[string]window
}

func NewLimiter() *Limiter { return &Limiter{entries: map[string]window{}} }
func (l *Limiter) Allow(key string, max int, period time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	for k, v := range l.entries {
		if v.end.Before(now) {
			delete(l.entries, k)
		}
	}
	key = core.TokenHash(key)
	v := l.entries[key]
	if v.n >= max || len(l.entries) >= 10000 {
		return false
	}
	if v.n == 0 {
		v.end = now.Add(period)
	}
	v.n++
	l.entries[key] = v
	return true
}
