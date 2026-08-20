// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package ratelimit

import (
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"
)

// Limiter limits requests per IP (requests per minute).
type Limiter struct {
	mu      sync.Mutex
	limit   int
	buckets map[string]*bucket
}

type bucket struct {
	count  int
	window time.Time
}

func New(limit int) *Limiter {
	return &Limiter{limit: limit, buckets: make(map[string]*bucket)}
}

// Middleware returns HTTP middleware. If limit <=0, passes through.
func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if l.limit <= 0 {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		ip := clientIP(r)
		now := time.Now()
		l.mu.Lock()
		b, ok := l.buckets[ip]
		if !ok || now.Sub(b.window) >= time.Minute {
			l.buckets[ip] = &bucket{count: 1, window: now}
			l.mu.Unlock()
			next.ServeHTTP(w, r)
			return
		}
		if b.count >= l.limit {
			retryAfter := int(time.Until(b.window.Add(time.Minute)).Seconds()) + 1
			if retryAfter < 1 {
				retryAfter = 1
			}
			l.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", itoa(retryAfter))
			w.WriteHeader(http.StatusTooManyRequests)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "rate limit exceeded"})
			return
		}
		b.count++
		l.mu.Unlock()
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	b := make([]byte, 0, 10)
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
