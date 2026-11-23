package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"os"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/tenant"
)

type goclawReq struct {
	Type   string          `json:"type"`
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type goclawErr struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type goclawRes struct {
	Type    string     `json:"type"`
	ID      string     `json:"id"`
	OK      bool       `json:"ok"`
	Payload any        `json:"payload,omitempty"`
	Error   *goclawErr `json:"error,omitempty"`
}

func writeGoClaw(conn *websocket.Conn, id string, payload any) error {
	return conn.WriteJSON(goclawRes{Type: "res", ID: id, OK: true, Payload: payload})
}

func writeGoClawErr(conn *websocket.Conn, id, code, msg string) error {
	return conn.WriteJSON(goclawRes{Type: "res", ID: id, OK: false, Error: &goclawErr{Code: code, Message: msg}})
}

func handleGoClawReq(ctx context.Context, conn *websocket.Conn, st store.StoreIface, provider llm.Provider, raw []byte, authed bool, state *wsConnState) (bool, error) {
	var req goclawReq
	if err := json.Unmarshal(raw, &req); err != nil {
		return authed, conn.WriteJSON(goclawRes{Type: "res", OK: false, Error: &goclawErr{Code: "INVALID_JSON", Message: "invalid json"}})
	}
	if req.Method != "connect" && !authed {
		return false, writeGoClawErr(conn, req.ID, "UNAUTHORIZED", "connect first")
	}
	switch req.Method {
	case "connect":
		var p struct {
			Token  string `json:"token"`
			UserID string `json:"user_id"`
		}
		_ = json.Unmarshal(req.Params, &p)
		admin := strings.TrimSpace(os.Getenv("GOSO_ADMIN_TOKEN"))
		token := strings.TrimSpace(p.Token)
		var who auth.Identity
		switch {
		case admin != "" && subtle.ConstantTimeCompare([]byte(admin), []byte(token)) == 1:
			who = auth.Identity{UserID: "system", Role: tenant.RoleOwner, TenantID: tenant.Default, Owner: true}
		case token != "":
			sessions := store.AuthSessions(st)
			if sessions == nil {
				return false, writeGoClawErr(conn, req.ID, "UNAUTHORIZED", "invalid token")
			}
			sess, serr := sessions.LookupAuthSession(token)
			if serr != nil || sess == nil {
				return false, writeGoClawErr(conn, req.ID, "UNAUTHORIZED", "invalid token")
			}
			who = auth.Identity{UserID: sess.UserID, Email: sess.Email, TenantID: sess.TenantID, Role: sess.Role, Owner: strings.EqualFold(sess.Role, "owner")}
		default:
			return false, writeGoClawErr(conn, req.ID, "UNAUTHORIZED", "invalid token")
		}
		if state != nil {
			state.Identity = &who
		}
		name := "GOSO"
		if reg := tenant.DefaultRegistry(); reg != nil {
			if row, rerr := reg.Get(who.TenantID); rerr == nil && strings.TrimSpace(row.Name) != "" {
				name = row.Name
			}
		}
		return true, writeGoClaw(conn, req.ID, map[string]any{
			"role": who.Role, "status": "ok", "is_owner": who.IsOwner(), "is_master_scope": who.IsOwner(),
			"edition": "standard", "tenant_id": who.TenantID, "tenant_name": name, "tenant_slug": who.TenantID,
			"server": map[string]any{"name": "goso-gateway", "version": "0.1.0"},
		})
		return true, writeGoClaw(conn, req.ID, map[string]any{
			"role":            "admin",
			"status":          "ok",
			"is_owner":        true,
			"is_master_scope": true,
			"edition":         "standard",
			"tenant_id":       "default",
			"tenant_name":     "GOSO",
			"tenant_slug":     "default",
			"server":          map[string]any{"name": "goso-gateway", "version": "0.1.0"},
		})
	case "tenants.mine", "tenants.list", "tenants.get", "tenants.create", "tenants.users.list", "tenants.users.add", "tenants.users.remove":
		return authed, handleGoClawTenants(conn, req, state.Identity)
	case "health":
		return authed, writeGoClaw(conn, req.ID, map[string]any{"status": "ok", "version": "0.1.0", "uptime": 0, "database": "sqlite"})
	case "status":
		list := st.ListAgents()
		agents := make([]map[string]any, 0, len(list))
		for _, a := range list {
			agents = append(agents, map[string]any{"id": a.ID, "model": a.Model, "isRunning": a.Enabled})
		}
		return authed, writeGoClaw(conn, req.ID, map[string]any{"agents": agents, "agentTotal": len(agents)})
	case "agents.list":
		list := wsVisibleAgents(st.ListAgents(), state)
		agents := make([]map[string]any, 0, len(list))
		for _, a := range list {
			status := "disabled"
			if a.Enabled {
				status = "active"
			}
			agents = append(agents, map[string]any{
				"id": a.ID, "agent_key": a.AgentKey, "display_name": a.DisplayName,
				"model": a.Model, "isRunning": a.Enabled, "status": status, "enabled": a.Enabled,
			})
		}
		return authed, writeGoClaw(conn, req.ID, map[string]any{"agents": agents})
	case "quota.usage":
		return authed, writeGoClaw(conn, req.ID, map[string]any{"enabled": false, "entries": []any{}})
	case "cron.list", "cron.create", "cron.toggle", "cron.delete", "cron.run", "cron.runs", "channels.status", "agents.files.list", "agents.files.get", "agents.files.set", "heartbeat.get", "heartbeat.set", "heartbeat.toggle", "heartbeat.test", "heartbeat.logs", "heartbeat.checklist.get", "heartbeat.checklist.set", "heartbeat.targets", "config.permissions.list", "config.permissions.grant", "config.permissions.revoke", "config.permissions.check":
		return authed, handleGoClawExtra(conn, st, req)
	case "teams.list", "teams.get", "teams.create", "teams.update", "teams.delete", "teams.members.add", "teams.members.remove", "teams.tasks.list", "teams.tasks.create", "teams.tasks.update", "config.get", "config.patch", "config.apply", "config.defaults", "skills.list", "skills.get", "exec.approval.list", "exec.approval.approve", "exec.approval.deny", "usage.get", "usage.summary", "pairing.list", "pairing.approve", "pairing.deny", "pairing.revoke", "logs.tail", "sessions.patch", "sessions.reset", "agents.links.list", "agents.links.create", "agents.links.update", "agents.links.delete", "cron.update", "chat.behavior.preview":
		return authed, handleGoClawMore(conn, st, req)
	case "chat.send", "chat.history", "chat.abort", "chat.session.status", "sessions.list", "sessions.delete", "teams.tasks.active-by-session":
		return authed, handleGoClawChat(ctx, conn, st, provider, req)
	default:
		return authed, writeGoClawErr(conn, req.ID, "NOT_IMPLEMENTED", req.Method+" is not implemented on GOSO WS")
	}
}

func wsIdentityTenant(state *wsConnState) string {
	if state != nil && state.Identity != nil && strings.TrimSpace(state.Identity.TenantID) != "" {
		return state.Identity.TenantID
	}
	return tenant.Default
}
type wsStateKey struct{}

func withWSState(ctx context.Context, s *wsConnState) context.Context {
	return context.WithValue(ctx, wsStateKey{}, s)
}

func wsStateFromCtx(ctx context.Context) *wsConnState {
	if ctx == nil {
		return nil
	}
	if s, ok := ctx.Value(wsStateKey{}).(*wsConnState); ok {
		return s
	}
	return nil
}

// wsVisibleAgents keeps only the signed-in user's tenant unless the caller is
// an operator or owner token.
func wsVisibleAgents(list []*store.Agent, state *wsConnState) []*store.Agent {
	if state == nil || state.Identity == nil || state.Identity.IsOwner() {
		return list
	}
	tid := state.Identity.TenantID
	out := make([]*store.Agent, 0, len(list))
	for _, a := range list {
		if a != nil && store.SameTenant(a.TenantID, tid) {
			out = append(out, a)
		}
	}
	return out
}

// wsScopeTenant is the tenant filter for list queries: empty means every
// tenant (operator), otherwise only the signed-in user's own tenant.
func wsScopeTenant(state *wsConnState) string {
	if state == nil || state.Identity == nil || state.Identity.IsOwner() {
		return ""
	}
	return state.Identity.TenantID
}
