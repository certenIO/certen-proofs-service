// Copyright 2025 Certen Protocol
//
// Cross-cutting HTTP middleware: panic recovery and per-IP rate limiting.

package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/certen/proofs-service/pkg/server"
)

// recoverMiddleware turns a panic in the auth chain or any handler into a 500
// instead of aborting the request mid-write. (A panic in a detached goroutine
// still needs its own recover — see the export goroutines.)
func recoverMiddleware(logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Printf("[panic] %s %s: %v", r.Method, r.URL.Path, rec)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"success":false,"error":"Internal server error"}`))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// rateLimitMiddleware applies a per-IP fixed-window limit to NON-service callers.
// The gateway (a trusted service principal that enforces tenant quotas upstream)
// is exempt so internal traffic is never throttled; this protects the
// COUNT-heavy read endpoints from a single abusive authenticated/anonymous IP.
// Must run AFTER authMiddleware so the principal is in context.
func rateLimitMiddleware(next http.Handler) http.Handler {
	const (
		windowSec int64 = 60
		maxPerWin       = 600
	)
	type counter struct {
		count       int
		windowStart int64
	}
	var (
		mu       sync.Mutex
		counters = map[string]*counter{}
	)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p, ok := server.PrincipalFrom(r.Context()); ok && p.Type == server.PrincipalService {
			next.ServeHTTP(w, r)
			return
		}
		ip := clientIP(r)
		now := time.Now().Unix()
		mu.Lock()
		c := counters[ip]
		if c == nil || now-c.windowStart >= windowSec {
			c = &counter{windowStart: now}
			counters[ip] = c
			if len(counters) > 10000 {
				for k, v := range counters {
					if now-v.windowStart >= windowSec {
						delete(counters, k)
					}
				}
			}
		}
		c.count++
		over := c.count > maxPerWin
		mu.Unlock()
		if over {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"success":false,"error":"Rate limit exceeded"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	// Only trust X-Forwarded-For when explicitly behind a trusted proxy; otherwise
	// a client could spoof the header to rotate its rate-limit key per request.
	if os.Getenv("TRUST_PROXY") == "true" {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if i := strings.IndexByte(xff, ','); i >= 0 {
				return strings.TrimSpace(xff[:i])
			}
			return strings.TrimSpace(xff)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
