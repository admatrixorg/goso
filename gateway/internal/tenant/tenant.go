// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package tenant

import (
	"net/http"
	"os"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Header is the optional tenant selector. Honored only when GOSO_MULTI_TENANT=1.
const Header = "X-Goso-Tenant"

// Default is store.DefaultTenant ("default").
const Default = store.DefaultTenant

// Enabled reports GOSO_MULTI_TENANT=1/true/yes/on.
func Enabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GOSO_MULTI_TENANT"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// Resolve picks the request tenant.
//
//   - GOSO_MULTI_TENANT unset/off → always default (header ignored; demo/single-tenant).
//   - Empty or invalid header → default.
//   - Non-default requires the admin bearer; view-token cannot switch tenants.
func Resolve(r *http.Request) string {
	if r == nil || !Enabled() {
		return Default
	}
	raw := strings.TrimSpace(r.Header.Get(Header))
	id := store.NormalizeTenant(raw)
	if id == Default {
		return Default
	}
	if !isAdmin(r) {
		return Default
	}
	return id
}

func isAdmin(r *http.Request) bool {
	admin := strings.TrimSpace(os.Getenv("GOSO_ADMIN_TOKEN"))
	if admin == "" {
		return false
	}
	got := extractToken(r)
	if got == "" {
		return false
	}
	return security.Equal(got, admin)
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
