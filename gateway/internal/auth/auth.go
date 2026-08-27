// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package auth

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/security"
)

var viewPrefixes = []string{"/healthz", "/api/agents", "/api/sessions", "/v1/agents", "/v1/sessions"}

// RequireToken returns middleware that enforces Bearer token auth.
// An empty expected token rejects every non-bypass path with 401 (SPEC 016).
// Explicit passthrough is GOSO_DEV_MODE=1 in serve.New — do not pass "" here for that.
// Bypass is a list of path prefixes that don't require auth (e.g. "/healthz").
func RequireToken(token string, bypass []string) func(http.Handler) http.Handler {
	return RequireTokens(token, "", bypass)
}

// RequireTokens enforces GOSO_ADMIN_TOKEN (full) and optional GOSO_VIEW_TOKEN
// (GET /healthz /api/agents /api/sessions and the matching /v1 aliases only).
func RequireTokens(admin, view string, bypass []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, p := range bypass {
				if strings.HasPrefix(r.URL.Path, p) {
					next.ServeHTTP(w, r)
					return
				}
			}
			got := extractToken(r)
			if admin != "" && security.Equal(got, admin) {
				next.ServeHTTP(w, r)
				return
			}
			if view != "" && security.Equal(got, view) {
				if r.Method == http.MethodGet && viewPathAllowed(r.URL.Path) {
					next.ServeHTTP(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "forbidden"})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		})
	}
}

func viewPathAllowed(path string) bool {
	for _, p := range viewPrefixes {
		if path == p {
			return true
		}
		if p == "/healthz" {
			continue
		}
		rest, ok := strings.CutPrefix(path, p+"/")
		if ok && rest != "" && !strings.Contains(rest, "/") {
			return true
		}
	}
	return false
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
