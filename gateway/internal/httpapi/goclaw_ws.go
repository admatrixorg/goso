package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"os"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
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

func handleGoClawReq(ctx context.Context, conn *websocket.Conn, st store.StoreIface, provider llm.Provider, raw []byte, authed bool) (bool, error) {
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
		if admin != "" && subtle.ConstantTimeCompare([]byte(admin), []byte(token)) != 1 {
			return false, writeGoClawErr(conn, req.ID, "UNAUTHORIZED", "invalid token")
		}
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
	case "tenants.mine":
		return authed, writeGoClaw(conn, req.ID, map[string]any{"tenants": []any{}})
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
		list := st.ListAgents()
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
	case "cron.list":
		return authed, writeGoClaw(conn, req.ID, map[string]any{"jobs": []any{}})
	case "channels.status":
		return authed, writeGoClaw(conn, req.ID, map[string]any{"channels": map[string]any{}})
	case "chat.send", "chat.history", "chat.abort", "chat.session.status", "sessions.list", "sessions.delete", "teams.tasks.active-by-session":
		return authed, handleGoClawChat(ctx, conn, st, provider, req)
	default:
		return authed, writeGoClawErr(conn, req.ID, "NOT_IMPLEMENTED", req.Method+" is not implemented on GOSO WS")
	}
}
