package httpapi

import (
	"net/http"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func registerAuthSessionRoutes(mux *http.ServeMux, st store.StoreIface) {
	aliasAPI(mux, "POST /api/auth/logout", handleAuthLogout(st))
	aliasAPI(mux, "GET /api/auth/sessions", handleAuthSessionList(st))
	aliasAPI(mux, "DELETE /api/auth/sessions/{id}", handleAuthSessionRevoke(st))
}

func authBearerToken(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func handleAuthLogout(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := authBearerToken(r)
		if token == "" {
			writeErr(w, http.StatusUnauthorized, "missing token")
			return
		}
		if sessions := store.AuthSessions(st); sessions != nil {
			_ = sessions.DeleteAuthSession(token)
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func handleAuthSessionList(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := auth.IdentityFrom(r.Context())
		if !ok || id.UserID == "" {
			writeJSON(w, http.StatusOK, map[string]any{"sessions": []any{}})
			return
		}
		sessions := store.AuthSessions(st)
		if sessions == nil {
			writeJSON(w, http.StatusOK, map[string]any{"sessions": []any{}})
			return
		}
		rows, err := sessions.ListAuthSessionsForUser(id.UserID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "session list failed")
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, s := range rows {
			out = append(out, map[string]any{
				"id": s.ID, "tenant_id": s.TenantID, "role": s.Role,
				"created_at": s.CreatedAt, "expires_at": s.ExpiresAt,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessions": out, "total": len(out)})
	}
}

func handleAuthSessionRevoke(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := auth.IdentityFrom(r.Context())
		if !ok || id.UserID == "" {
			writeErr(w, http.StatusUnauthorized, "sign in required")
			return
		}
		sessions := store.AuthSessions(st)
		if sessions == nil {
			writeErr(w, http.StatusNotImplemented, "session store unavailable")
			return
		}
		target := strings.TrimSpace(r.PathValue("id"))
		if target == "" {
			writeErr(w, http.StatusBadRequest, "session id required")
			return
		}
		if err := sessions.DeleteAuthSessionByID(id.UserID, target); err != nil {
			writeErr(w, http.StatusNotFound, "session not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}
