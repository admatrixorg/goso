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
	"/api/events",
	"/api/activity",
	"/api/logs",
	"/api/tenant",
	"/api/tenants",
	"/api/api-keys",
	"/api/packages",
	"/v1/agents",
	"/v1/sessions",
	"/v1/pending-messages",
	"/v1/contacts",
	"/v1/nodes",
	"/v1/workstations",
	"/v1/storage",
	"/v1/events",
	"/v1/activity",
	"/v1/logs",
	"/v1/tenant",
	"/v1/tenants",
	"/v1/api-keys",
	"/v1/packages",
}

// Grant is a hashed issued API key that passed Accept. Secret is never stored here.
type Grant struct {
	ID     string
	Prefix string
	Scopes []string
}

// Has reports whether the grant includes scope (admin implies all).
func (g Grant) Has(scope string) bool {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "" {
		return false
	}
	for _, s := range g.Scopes {
		if strings.EqualFold(s, "admin") || strings.EqualFold(s, scope) {
			return true
		}
	}
	return false
}

// KeyGate looks up issued gateway keys by hash. Implementations must not return the secret.
type KeyGate interface {
	Accept(token string) (Grant, bool)
}

// Config is admin/view Bearer enforcement plus optional pairing grants and issued keys.
type Config struct {
	Admin   string
	View    string
	Bypass  []string
	Pairing *Pairing
	Keys    KeyGate
}

// RequireToken returns middleware that enforces Bearer token auth.
// An empty expected token rejects every non-bypass path with 401 (SPEC 016).
// Explicit passthrough is GOSO_DEV_MODE=1 in serve.New — do not pass "" here for that.
// Bypass is a list of path prefixes that don't require auth (e.g. "/healthz").
func RequireToken(token string, bypass []string) func(http.Handler) http.Handler {
	return RequireTokens(token, "", bypass)
}

// RequireTokens enforces GOSO_ADMIN_TOKEN (full) and optional GOSO_VIEW_TOKEN
// (GET /healthz /api/agents /api/sessions /api/nodes /api/workstations /api/storage /api/events /api/activity /api/logs /api/tenant /api/tenants /api/api-keys /api/packages and the matching /v1 aliases only).
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
			if cfg.Keys != nil {
				if g, ok := cfg.Keys.Accept(got); ok {
					if scopeAllows(g, r) {
						next.ServeHTTP(w, r)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusForbidden)
					_ = json.NewEncoder(w).Encode(map[string]string{"error": "forbidden"})
					return
				}
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

func scopeAllows(g Grant, r *http.Request) bool {
	if r == nil {
		return false
	}
	if g.Has("admin") {
		return true
	}
	path := r.URL.Path
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		if g.Has("read") && viewPathAllowed(path) {
			return true
		}
		if g.Has("approvals") && approvalsPath(path) {
			return true
		}
		return false
	}
	if privilegedWrite(path) {
		return false
	}
	if pairingWritePath(path) {
		return g.Has("pairing")
	}
	if provisionPath(path) {
		return g.Has("provision")
	}
	if approvalsPath(path) {
		return g.Has("approvals")
	}
	return g.Has("write")
}

func privilegedWrite(path string) bool {
	return hasPathPrefix(path, "/api/api-keys", "/v1/api-keys", "/api/tenants", "/v1/tenants", "/api/packages", "/v1/packages")
}

func pairingWritePath(path string) bool {
	switch path {
	case "/api/pairing", "/v1/pairing":
		return true
	default:
		return false
	}
}

func approvalsPath(path string) bool {
	return hasPathPrefix(path, "/api/approvals", "/v1/approvals")
}

func provisionPath(path string) bool {
	if path == "/api/nodes/request" || path == "/v1/nodes/request" {
		return false
	}
	return hasPathPrefix(path, "/api/nodes/", "/v1/nodes/", "/api/workstations", "/v1/workstations")
}

func hasPathPrefix(path string, prefixes ...string) bool {
	for _, p := range prefixes {
		if path == p || strings.HasPrefix(path, p+"/") || (strings.HasSuffix(p, "/") && strings.HasPrefix(path, p)) {
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
