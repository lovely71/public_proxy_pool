package ratelimit

import (
	"sync"
	"time"
)

type Limiter struct {
	rps   float64
	burst float64

	mu    sync.Mutex
	state map[string]*bucket
}

type bucket struct {
	tokens   float64
	last     time.Time
	lastSeen time.Time
}

func New(rps float64, burst int) *Limiter {
	b := float64(burst)
	if burst <= 0 {
		b = 1
	}
	return &Limiter{
		rps:   rps,
		burst: b,
		state: map[string]*bucket{},
	}
}

func (l *Limiter) Enabled() bool {
	return l != nil && l.rps > 0
}

func (l *Limiter) Allow(key string, now time.Time) bool {
	if l == nil || l.rps <= 0 {
		return true
	}
	if key == "" {
		key = "anon"
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.state[key]
	if !ok {
		l.state[key] = &bucket{tokens: l.burst - 1, last: now, lastSeen: now}
		return true
	}
	dt := now.Sub(b.last).Seconds()
	if dt < 0 {
		dt = 0
	}
	b.tokens += dt * l.rps
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = now
	b.lastSeen = now
	if b.tokens < 1 {
		return false
	}
	b.tokens -= 1
	return true
}

func (l *Limiter) Cleanup(olderThan time.Duration) {
	if l == nil {
		return
	}
	cutoff := time.Now().Add(-olderThan)
	l.mu.Lock()
	defer l.mu.Unlock()
	for k, b := range l.state {
		if b.lastSeen.Before(cutoff) {
			delete(l.state, k)
		}
	}
}

