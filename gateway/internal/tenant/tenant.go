// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package tenant

import (
	"net/http"
	"os"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/auth"
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
//   - An issued API key always runs in its own tenant. The auth middleware has
//     already refused a key whose header names another tenant.
func Resolve(r *http.Request) string {
	if r == nil || !Enabled() {
		return Default
	}
	if g, ok := auth.GrantFrom(r.Context()); ok {
		return store.NormalizeTenant(g.KeyTenant())
	}
	if id, ok := auth.IdentityFrom(r.Context()); ok {
		if !id.IsOwner() {
			return store.NormalizeTenant(id.TenantID)
		}
	}
	raw := tenantHeader(r)
	id := store.NormalizeTenant(raw)
	if id == Default {
		return Default
	}
	if !isAdmin(r) {
		return Default
	}
	return id
}

// Invalid reports a request that names a tenant which is not a valid id
// while multi-tenant is on. Callers fail closed (400) instead of serving
// the default tenant's data under a mistyped header (SPEC 132).
func Invalid(r *http.Request) bool {
	if r == nil || !Enabled() {
		return false
	}
	raw := tenantHeader(r)
	return raw != "" && !store.TenantOK(raw)
}


func tenantHeader(r *http.Request) string {
	if r == nil {
		return ""
	}
	for _, k := range []string{Header, "X-GoClaw-Tenant-Id", "X-Goclaw-Tenant-Id"} {
		if v := strings.TrimSpace(r.Header.Get(k)); v != "" {
			return v
		}
	}
	return ""
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
