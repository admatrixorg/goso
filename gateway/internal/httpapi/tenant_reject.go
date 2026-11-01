// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"net/http"

	"github.com/mqglobal/goso/gateway/internal/tenant"
)

// RejectInvalidTenant fails closed: when multi-tenant is on and the request
// names a tenant that is not a valid id, answer 400 instead of silently
// serving the default tenant's data (SPEC 132).
func RejectInvalidTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tenant.Invalid(r) {
			writeErr(w, http.StatusBadRequest, "invalid tenant")
			return
		}
		next.ServeHTTP(w, r)
	})
}
