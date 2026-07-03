package chat

import (
	"sync"
	"time"
)

// Limiter is a per-key token bucket: keys accrue `rate` tokens per second
// up to `burst`, and each allowed call spends one. Keys are session IDs, so
// one chatty user can't starve the others.
type Limiter struct {
	rate  float64
	burst float64
	now   func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

// NewLimiter returns a Limiter granting rate tokens/second with the given
// burst ceiling (which is also a new key's starting balance).
func NewLimiter(rate, burst float64) *Limiter {
	return &Limiter{rate: rate, burst: burst, now: time.Now, buckets: make(map[string]*bucket)}
}

// Allow spends one token for key, reporting false when the bucket is dry.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= 4096 {
			l.prune(now)
		}
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens += now.Sub(b.last).Seconds() * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// prune drops buckets that have sat at full balance long enough to be
// indistinguishable from new ones. Called under l.mu.
func (l *Limiter) prune(now time.Time) {
	idle := time.Duration(float64(time.Second) * (l.burst / l.rate))
	if idle < time.Minute {
		idle = time.Minute
	}
	for k, b := range l.buckets {
		if now.Sub(b.last) > idle {
			delete(l.buckets, k)
		}
	}
}
