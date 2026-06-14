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
}

type tokenBucket struct {
	tokens     float64
	lastRefill time.Time
}

func newRateLimiter(rps, burst int64) *rateLimiter {
	if rps <= 0 {
		return nil
	}
	if burst <= 0 {
		burst = rps
	}
	return &rateLimiter{
		rate:   float64(rps),
		burst:  float64(burst),
		now:    func() time.Time { return time.Now().UTC() },
		client: make(map[string]tokenBucket),
	}
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
		if !rl.allow(r.RemoteAddr) {
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
