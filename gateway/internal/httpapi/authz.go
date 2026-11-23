package httpapi

import (
	"net/http"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/auth"
)

// Role names stored on a tenant membership.
const (
	roleOwner  = "owner"
	roleAdmin  = "admin"
	roleMember = "member"
	roleViewer = "viewer"
)

func roleRank(role string) int {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case roleOwner:
		return 4
	case roleAdmin:
		return 3
	case roleMember:
		return 2
	case roleViewer:
		return 1
	default:
		return 0
	}
}

// callerRank returns the caller's capability. A request with no session
// identity is the operator token and keeps full access.
func callerRank(r *http.Request) (rank int, ok bool) {
	id, has := auth.IdentityFrom(r.Context())
	if !has {
		return 4, false
	}
	if id.IsOwner() {
		return 4, true
	}
	return roleRank(id.Role), true
}

// requiredRank maps a mutating request to the role it needs. Read requests
// need only a signed-in user (rank 1).
func requiredRank(r *http.Request) int {
	if r == nil {
		return 4
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return 1
	}
	p := r.URL.Path
	if strings.HasPrefix(p, "/v1/") {
		p = "/api/" + strings.TrimPrefix(p, "/v1/")
	}
	if strings.HasPrefix(p, "/api/auth/") {
		return 1
	}
	// Operator-only surfaces.
	for _, pre := range []string{"/api/tenants", "/api/api-keys", "/api/packages", "/api/system/", "/api/import-export"} {
		if strings.HasPrefix(p, pre) {
			return 4
		}
	}
	// Tenant administration surfaces.
	for _, pre := range []string{
		"/api/users", "/api/providers", "/api/mcp", "/api/cli-credentials", "/api/channels",
		"/api/hooks", "/api/webhooks", "/api/config", "/api/tools", "/api/agents", "/api/teams",
		"/api/skills", "/api/cron", "/api/storage", "/api/vault", "/api/memory", "/api/kg",
		"/api/connectors", "/api/nodes", "/api/workstations", "/api/tts",
	} {
		if strings.HasPrefix(p, pre) {
			return 3
		}
	}
	// Chat and session work for members.
	return 2
}

// RequireRole enforces requiredRank on every request after authentication.
func RequireRole(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rank, hasIdentity := callerRank(r)
		need := requiredRank(r)
		if rank < need {
			writeErr(w, http.StatusForbidden, "forbidden for role")
			return
		}
		_ = hasIdentity
		next.ServeHTTP(w, r)
	})
}

// wsRequiredRank maps a mutating WS method to the role it needs.
func wsRequiredRank(method string) int {
	m := strings.ToLower(strings.TrimSpace(method))
	switch {
	case m == "connect" || m == "health" || m == "status" || m == "quota.usage":
		return 1
	case strings.HasPrefix(m, "chat.") || strings.HasPrefix(m, "sessions.") || strings.HasPrefix(m, "heartbeat.get") || strings.HasPrefix(m, "agents.files.get") || strings.HasPrefix(m, "agents.list") || strings.HasPrefix(m, "tenants.list") || strings.HasPrefix(m, "tenants.mine") || strings.HasPrefix(m, "tenants.get") || strings.HasPrefix(m, "teams.list") || strings.HasPrefix(m, "teams.get") || strings.HasPrefix(m, "teams.tasks.list") || strings.HasPrefix(m, "teams.tasks.get") || strings.HasPrefix(m, "usage.") || strings.HasPrefix(m, "cron.list") || strings.HasPrefix(m, "channels.status") || strings.HasPrefix(m, "skills.list") || strings.HasPrefix(m, "skills.get") || strings.HasPrefix(m, "logs.") || strings.HasPrefix(m, "config.get") || strings.HasPrefix(m, "exec.approval.list") || strings.HasPrefix(m, "pairing.list") || strings.HasPrefix(m, "agents.links.list") || strings.HasPrefix(m, "channels.instances.list") || strings.HasPrefix(m, "teams.tasks.comments") || strings.HasPrefix(m, "teams.workspace.list") || strings.HasPrefix(m, "teams.workspace.read") || strings.HasPrefix(m, "teams.events.list") || m == "teams.scopes" || strings.HasPrefix(m, "run.") || strings.HasPrefix(m, "browser.") || strings.HasPrefix(m, "chat_behavior."):
		return 1
	case strings.HasPrefix(m, "tenants.create") || strings.HasPrefix(m, "tenants.update"):
		return 4
	default:
		return 3
	}
}

// wsAuthorize reports whether the signed-in identity may call method.
func wsAuthorize(method string, who *auth.Identity) bool {
	if who == nil || who.IsOwner() {
		return true
	}
	return roleRank(who.Role) >= wsRequiredRank(method)
}
