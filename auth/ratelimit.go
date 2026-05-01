// Copyright (c) 2026 Arnaud Guiovanna <https://www.aguiovanna.fr>
// GitHub: https://github.com/ArnaudGuiovanna
// SPDX-License-Identifier: MIT

package auth

import (
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type bucket struct {
	tokens   float64
	lastTime time.Time
}

// RateLimiter implements a per-IP token bucket rate limiter.
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64 // tokens per second
	burst   int     // max tokens
	stop    chan struct{}
}

// NewRateLimiter creates a rate limiter. rate is tokens/second, burst is max tokens.
// Starts a background goroutine to purge stale entries.
func NewRateLimiter(rate float64, burst int) *RateLimiter {
	rl := &RateLimiter{
		buckets: make(map[string]*bucket),
		rate:    rate,
		burst:   burst,
		stop:    make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

// Allow consumes one token for the given IP. Returns false if the bucket is empty.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	b, ok := rl.buckets[ip]
	if !ok {
		rl.buckets[ip] = &bucket{tokens: float64(rl.burst) - 1, lastTime: now}
		return true
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(b.lastTime).Seconds()
	b.tokens += elapsed * rl.rate
	if b.tokens > float64(rl.burst) {
		b.tokens = float64(rl.burst)
	}
	b.lastTime = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Stop shuts down the background cleanup goroutine.
func (rl *RateLimiter) Stop() {
	close(rl.stop)
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			cutoff := time.Now().Add(-10 * time.Minute)
			for ip, b := range rl.buckets {
				if b.lastTime.Before(cutoff) {
					delete(rl.buckets, ip)
				}
			}
			rl.mu.Unlock()
		case <-rl.stop:
			return
		}
	}
}

// trustedProxiesOnce parses TRUSTED_PROXY_CIDRS exactly once at first use.
// XFF is honored only when the direct peer (r.RemoteAddr) falls inside one of
// these CIDRs, preventing a client from spoofing its own bucket key.
var (
	trustedProxiesOnce sync.Once
	trustedProxies     []*net.IPNet
)

func loadTrustedProxies() []*net.IPNet {
	trustedProxiesOnce.Do(func() {
		raw := os.Getenv("TRUSTED_PROXY_CIDRS")
		if raw == "" {
			return
		}
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			_, cidr, err := net.ParseCIDR(part)
			if err != nil {
				slog.Warn("invalid TRUSTED_PROXY_CIDRS entry", "value", part, "err", err)
				continue
			}
			trustedProxies = append(trustedProxies, cidr)
		}
	})
	return trustedProxies
}

func remoteIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return net.ParseIP(host)
}

func isTrustedProxy(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, cidr := range loadTrustedProxies() {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// clientIP returns the bucket key. X-Forwarded-For is honored only when the
// direct peer is in TRUSTED_PROXY_CIDRS; otherwise a client could spoof its
// own bucket and bypass the per-IP limit. The leftmost XFF entry must be a
// well-formed IP — invalid values fall back to the direct peer address.
func clientIP(r *http.Request) string {
	peer := remoteIP(r)
	if isTrustedProxy(peer) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first := strings.TrimSpace(strings.Split(xff, ",")[0])
			if parsed := net.ParseIP(first); parsed != nil {
				return parsed.String() // canonical form (normalizes IPv6)
			}
		}
	}
	if peer != nil {
		return peer.String()
	}
	return r.RemoteAddr
}

// RateLimitMiddleware wraps an http.Handler with rate limiting.
// Returns 429 Too Many Requests when the limit is exceeded.
func RateLimitMiddleware(limiter *RateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !limiter.Allow(ip) {
			w.Header().Set("Retry-After", "5")
			http.Error(w, `{"error":"rate_limit_exceeded"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
