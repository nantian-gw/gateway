package admin

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type rateLimiter struct {
	maxRequests int64
	window      time.Duration

	mu        sync.Mutex
	lastReset time.Time
	counters  map[string]*int64
}

func newRateLimiter(maxRequests int64, window time.Duration) *rateLimiter {
	if maxRequests <= 0 || window <= 0 {
		return nil
	}
	return &rateLimiter{
		maxRequests: maxRequests,
		window:      window,
		counters:    make(map[string]*int64),
	}
}

func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	if rl == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.RemoteAddr

		rl.mu.Lock()
		now := time.Now().UTC()
		if now.Sub(rl.lastReset) >= rl.window {
			rl.counters = make(map[string]*int64)
			rl.lastReset = now
		}

		counter, ok := rl.counters[key]
		if !ok {
			var c int64 = 1
			rl.counters[key] = &c
			rl.mu.Unlock()
			next.ServeHTTP(w, r)
			return
		}

		count := atomic.AddInt64(counter, 1)
		rl.mu.Unlock()

		if count > rl.maxRequests {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}
