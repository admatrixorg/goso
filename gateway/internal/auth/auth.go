// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package auth

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/security"
)

var viewPrefixes = []string{
	"/healthz",
	"/api/agents",
	"/api/sessions",
	"/api/pending-messages",
	"/api/contacts",
	"/api/nodes",
	"/api/workstations",
	"/api/storage",
	"/v1/agents",
	"/v1/sessions",
	"/v1/pending-messages",
	"/v1/contacts",
	"/v1/nodes",
	"/v1/workstations",
	"/v1/storage",
}

// Config is admin/view Bearer enforcement plus optional pairing grants.
type Config struct {
	Admin   string
	View    string
	Bypass  []string
	Pairing *Pairing
}

// RequireToken returns middleware that enforces Bearer token auth.
// An empty expected token rejects every non-bypass path with 401 (SPEC 016).
// Explicit passthrough is GOSO_DEV_MODE=1 in serve.New — do not pass "" here for that.
// Bypass is a list of path prefixes that don't require auth (e.g. "/healthz").
func RequireToken(token string, bypass []string) func(http.Handler) http.Handler {
	return RequireTokens(token, "", bypass)
}

// RequireTokens enforces GOSO_ADMIN_TOKEN (full) and optional GOSO_VIEW_TOKEN
// (GET /healthz /api/agents /api/sessions /api/nodes /api/workstations /api/storage and the matching /v1 aliases only).
func RequireTokens(admin, view string, bypass []string) func(http.Handler) http.Handler {
	return Require(Config{Admin: admin, View: view, Bypass: bypass})
}

// Require is RequireTokens plus optional minted pairing grants (same GET-only matrix).
func Require(cfg Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if pairingExchangePath(r) || nodeRequestPath(r) {
				next.ServeHTTP(w, r)
				return
			}
			for _, p := range cfg.Bypass {
				if strings.HasPrefix(r.URL.Path, p) {
					next.ServeHTTP(w, r)
					return
				}
			}
			got := extractToken(r)
			if cfg.Admin != "" && security.Equal(got, cfg.Admin) {
				next.ServeHTTP(w, r)
				return
			}
			viewOK := cfg.View != "" && security.Equal(got, cfg.View)
			if !viewOK && cfg.Pairing != nil && cfg.Pairing.Accepts(got) {
				viewOK = true
			}
			if viewOK {
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

func pairingExchangePath(r *http.Request) bool {
	if r == nil || r.Method != http.MethodPost {
		return false
	}
	switch r.URL.Path {
	case "/api/pairing/exchange", "/v1/pairing/exchange":
		return true
	default:
		return false
	}
}

func nodeRequestPath(r *http.Request) bool {
	if r == nil || r.Method != http.MethodPost {
		return false
	}
	switch r.URL.Path {
	case "/api/nodes/request", "/v1/nodes/request":
		return true
	default:
		return false
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
	if security.Production() {
		return ""
	}
	if q := r.URL.Query().Get("token"); q != "" {
		return q
	}
	return ""
}
