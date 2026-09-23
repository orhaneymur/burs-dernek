package security

import (
	"sync"
	"time"
)

// Limiter bellek ici, anahtar basina token kovasi. Tek replica icin yeterli;
// harici bir servise (Redis vb.) ihtiyac duymaz.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64 // saniyede yenilenen jeton
	burst   float64
}

type bucket struct {
	tokens float64
	seen   time.Time
}

// NewLimiter her anahtar icin window suresinde en fazla n istek izin verir.
func NewLimiter(n int, window time.Duration) *Limiter {
	l := &Limiter{
		buckets: make(map[string]*bucket),
		rate:    float64(n) / window.Seconds(),
		burst:   float64(n),
	}
	go l.janitor()
	return l
}

func (l *Limiter) Allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[key]
	if !ok {
		l.buckets[key] = &bucket{tokens: l.burst - 1, seen: now}
		return true
	}
	b.tokens += now.Sub(b.seen).Seconds() * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.seen = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (l *Limiter) janitor() {
	for range time.Tick(10 * time.Minute) {
		cutoff := time.Now().Add(-30 * time.Minute)
		l.mu.Lock()
		for k, b := range l.buckets {
			if b.seen.Before(cutoff) {
				delete(l.buckets, k)
			}
		}
		l.mu.Unlock()
	}
}
