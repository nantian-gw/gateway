package admin

import (
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type rateLimiter struct {
	rate   float64
	burst  float64
	now    func() time.Time
	mu     sync.Mutex
	client map[string]tokenBucket
	done   chan struct{}
}

type tokenBucket struct {
	tokens     float64
	lastRefill time.Time
	expiresAt  time.Time
}

func newRateLimiter(rps int64, burstOrWindow any) *rateLimiter {
	if rps <= 0 {
		return nil
	}

	rate := float64(rps)
	burst := rps
	switch v := burstOrWindow.(type) {
	case int:
		if v > 0 {
			burst = int64(v)
		}
	case int64:
		if v > 0 {
			burst = v
		}
	case uint:
		if v > 0 {
			burst = int64(v)
		}
	case uint64:
		if v > 0 {
			burst = int64(v)
		}
	case time.Duration:
		if v > 0 {
			// Legacy callers pass a fixed window. Model the same effective limit
			// with a token bucket by refilling rps tokens per window.
			rate = float64(rps) / v.Seconds()
		}
	default:
		// Unknown input types fall back to the RPS-derived burst.
	}

	rl := &rateLimiter{
		rate:   rate,
		burst:  float64(burst),
		now:    func() time.Time { return time.Now().UTC() },
		client: make(map[string]tokenBucket),
		done:   make(chan struct{}),
	}
	go rl.periodicCleanup()
	return rl
}

func (rl *rateLimiter) periodicCleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-rl.done:
			return
		case <-ticker.C:
		}
		rl.mu.Lock()
		now := rl.now()
		for key, entry := range rl.client {
			if now.After(entry.expiresAt) {
				delete(rl.client, key)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *rateLimiter) Stop() {
	close(rl.done)
}

func (rl *rateLimiter) allow(remoteAddr string) bool {
	if rl == nil {
		return true
	}

	key := normalizeClientIP(remoteAddr)
	now := rl.now()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	bucket := rl.client[key]
	if bucket.lastRefill.IsZero() {
		bucket.tokens = rl.burst
		bucket.lastRefill = now
	}

	if elapsed := now.Sub(bucket.lastRefill).Seconds(); elapsed > 0 {
		bucket.tokens = math.Min(rl.burst, bucket.tokens+elapsed*rl.rate)
		bucket.lastRefill = now
	}

	bucket.expiresAt = now.Add(5 * time.Minute)

	if bucket.tokens < 1 {
		rl.client[key] = bucket
		return false
	}

	bucket.tokens--
	rl.client[key] = bucket
	return true
}

func (rl *rateLimiter) middleware(next http.Handler) http.Handler {
	if rl == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isProbePath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if !rl.allow(clientIP(r)) {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func normalizeClientIP(remoteAddr string) string {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if remoteAddr == "" {
		return ""
	}

	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}

	return remoteAddr
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.IndexByte(xff, ','); idx >= 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}
