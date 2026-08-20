// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package auth

import (
	"encoding/json"
	"net/http"
	"strings"
)

// RequireToken returns middleware that enforces Bearer token auth.
// If token == "", middleware passes through (dev mode).
// Bypass is a list of path prefixes that don't require auth (e.g. "/healthz").
func RequireToken(token string, bypass []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}
			for _, p := range bypass {
				if strings.HasPrefix(r.URL.Path, p) {
					next.ServeHTTP(w, r)
					return
				}
			}
			got := extractToken(r)
			if got != token {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func extractToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); h != "" {
		if strings.HasPrefix(h, "Bearer ") {
			return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
		}
	}
	if q := r.URL.Query().Get("token"); q != "" {
		return q
	}
	return ""
}
