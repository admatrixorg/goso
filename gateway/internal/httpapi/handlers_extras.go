package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/mqglobal/goso/gateway/internal/auth"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// registerExtrasRoutes serves the MCP server registry, CLI credentials and
// pricing overrides on their GoClaw paths, backed by real rows.
func registerExtrasRoutes(mux *http.ServeMux, st store.StoreIface) {
	mux.HandleFunc("GET /v1/mcp/servers", handleMCPList(st))
	mux.HandleFunc("POST /v1/mcp/servers", handleMCPCreate(st))
	mux.HandleFunc("GET /v1/mcp/servers/{id}", handleMCPGet(st))
	mux.HandleFunc("PATCH /v1/mcp/servers/{id}", handleMCPPatch(st))
	mux.HandleFunc("DELETE /v1/mcp/servers/{id}", handleMCPDelete(st))
	mux.HandleFunc("GET /v1/cli-credentials", handleCLIList(st))
	mux.HandleFunc("GET /v1/cli-credentials/{id}", handleCLIGet(st))
	mux.HandleFunc("GET /v1/model-pricing/overrides", handlePricingList(st))
	mux.HandleFunc("POST /v1/model-pricing/overrides", handlePricingCreate(st))
	mux.HandleFunc("DELETE /v1/model-pricing/overrides/{id}", handlePricingDelete(st))
}

func extrasOrErr(st store.StoreIface, w http.ResponseWriter) store.ExtrasStore {
	x := store.Extras(st)
	if x == nil {
		writeErr(w, http.StatusNotImplemented, "extras store unavailable")
		return nil
	}
	return x
}

func handleMCPList(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := extrasOrErr(st, w)
		if x == nil {
			return
		}
		rows := x.ListMCPServers(requestTenant(r))
		if rows == nil {
			rows = []*store.MCPServer{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"servers": rows})
	}
}

func handleMCPGet(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := extrasOrErr(st, w)
		if x == nil {
			return
		}
		id := strings.TrimSpace(r.PathValue("id"))
		for _, m := range x.ListMCPServers(requestTenant(r)) {
			if m.ID == id || m.Name == id {
				writeJSON(w, http.StatusOK, m)
				return
			}
		}
		writeErr(w, http.StatusNotFound, "server not found")
	}
}

func handleMCPCreate(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := extrasOrErr(st, w)
		if x == nil {
			return
		}
		var body store.MCPServer
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		body.TenantID = requestTenant(r)
		saved, err := x.PutMCPServer(body)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, saved)
	}
}

func handleMCPPatch(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := extrasOrErr(st, w)
		if x == nil {
			return
		}
		id := strings.TrimSpace(r.PathValue("id"))
		var cur *store.MCPServer
		for _, m := range x.ListMCPServers(requestTenant(r)) {
			if m.ID == id || m.Name == id {
				cur = m
				break
			}
		}
		if cur == nil {
			writeErr(w, http.StatusNotFound, "server not found")
			return
		}
		var body struct {
			DisplayName *string `json:"display_name"`
			Enabled     *bool   `json:"enabled"`
			URL         *string `json:"url"`
			Command     *string `json:"command"`
			ToolPrefix  *string `json:"tool_prefix"`
			TimeoutSec  *int    `json:"timeout_sec"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if body.DisplayName != nil {
			cur.DisplayName = *body.DisplayName
		}
		if body.Enabled != nil {
			cur.Enabled = *body.Enabled
		}
		if body.URL != nil {
			cur.URL = *body.URL
		}
		if body.Command != nil {
			cur.Command = *body.Command
		}
		if body.ToolPrefix != nil {
			cur.ToolPrefix = *body.ToolPrefix
		}
		if body.TimeoutSec != nil && *body.TimeoutSec > 0 {
			cur.TimeoutSec = *body.TimeoutSec
		}
		saved, err := x.PutMCPServer(*cur)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, saved)
	}
}

func handleMCPDelete(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := extrasOrErr(st, w)
		if x == nil {
			return
		}
		id := strings.TrimSpace(r.PathValue("id"))
		for _, m := range x.ListMCPServers(requestTenant(r)) {
			if m.ID == id || m.Name == id {
				id = m.ID
				break
			}
		}
		if err := x.DeleteMCPServer(requestTenant(r), id); err != nil {
			writeErr(w, http.StatusNotFound, "server not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func handleCLIList(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := extrasOrErr(st, w)
		if x == nil {
			return
		}
		rows := x.ListCLIBinaries(requestTenant(r))
		if rows == nil {
			rows = []*store.CLIBinary{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"binaries": rows})
	}
}

func handleCLIGet(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := extrasOrErr(st, w)
		if x == nil {
			return
		}
		id := strings.TrimSpace(r.PathValue("id"))
		for _, b := range x.ListCLIBinaries(requestTenant(r)) {
			if b.ID == id || b.BinaryName == id {
				writeJSON(w, http.StatusOK, map[string]any{
					"binary":      b,
					"credentials": x.ListCLICredentials(requestTenant(r), b.ID),
				})
				return
			}
		}
		writeErr(w, http.StatusNotFound, "binary not found")
	}
}

func handlePricingList(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := extrasOrErr(st, w)
		if x == nil {
			return
		}
		rows := x.ListPricingOverrides(requestTenant(r))
		if rows == nil {
			rows = []*store.PricingOverride{}
		}
		// The catalog carries the synced list price per model, so a caller can
		// compute a cost without an override on every model.
		catalog := []*store.PricingCatalogEntry{}
		if u := store.Usage(st); u != nil {
			catalog = u.ListPricingCatalog()
		}
		writeJSON(w, http.StatusOK, map[string]any{"overrides": rows, "catalog": catalog, "catalog_total": len(catalog)})
	}
}

func handlePricingCreate(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := extrasOrErr(st, w)
		if x == nil {
			return
		}
		var body store.PricingOverride
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		body.TenantID = requestTenant(r)
		saved, err := x.PutPricingOverride(body)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, saved)
	}
}

func handlePricingDelete(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := extrasOrErr(st, w)
		if x == nil {
			return
		}
		if err := x.DeletePricingOverride(requestTenant(r), r.PathValue("id")); err != nil {
			writeErr(w, http.StatusNotFound, "override not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

// --- KG: real rows ---

func kgQueryFrom(r *http.Request) store.KGGraphQuery {
	q := store.KGGraphQuery{
		Tenant:  requestTenant(r),
		AgentID: strings.TrimSpace(r.URL.Query().Get("agent_id")),
		Q:       strings.TrimSpace(r.URL.Query().Get("q")),
	}
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q.Limit = n
		}
	}
	return q
}

func handleRealKGGraph(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := kgQueryFrom(r)
		q.Limit = store.ClampKGGraphLimit(q.Limit)
		g, err := st.ListKGGraph(q)
		if err != nil || g == nil {
			writeJSON(w, http.StatusOK, map[string]any{"nodes": []any{}, "edges": []any{}})
			return
		}
		writeJSON(w, http.StatusOK, g)
	}
}

func handleRealKGStats(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := kgQueryFrom(r)
		q.Limit = 1
		g, err := st.ListKGGraph(q)
		if err != nil || g == nil {
			writeJSON(w, http.StatusOK, map[string]any{"entities": 0, "relations": 0})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"entities": g.TotalNodes, "relations": g.TotalEdges})
	}
}

func handleRealKGEntities(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := kgQueryFrom(r)
		q.Limit = store.ClampKGGraphLimit(q.Limit)
		g, err := st.ListKGGraph(q)
		if err != nil || g == nil {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		writeJSON(w, http.StatusOK, g.Nodes)
	}
}

// --- heartbeat WS ---

func wsHeartbeat(conn *websocket.Conn, st store.StoreIface, req goclawReq, who *auth.Identity) error {
	x := store.Extras(st)
	tenant := tenantOfIdentity(who)
	switch req.Method {
	case "heartbeat.get":
		var p struct {
			AgentID  string `json:"agentId"`
			AgentID2 string `json:"agent_id"`
		}
		_ = json.Unmarshal(req.Params, &p)
		aid := firstNonEmpty(p.AgentID, p.AgentID2)
		aid = resolveAgentID(st, tenant, aid)
		if x == nil || aid == "" {
			return writeGoClaw(conn, req.ID, map[string]any{"heartbeat": nil})
		}
		h, err := x.GetHeartbeat(tenant, aid)
		if err != nil || h == nil {
			return writeGoClaw(conn, req.ID, map[string]any{"heartbeat": nil})
		}
		return writeGoClaw(conn, req.ID, map[string]any{"heartbeat": wsHeartbeatView(h)})
	case "heartbeat.set":
		var p struct {
			AgentID          string  `json:"agentId"`
			AgentID2         string  `json:"agent_id"`
			Enabled          *bool   `json:"enabled"`
			IntervalSec      *int    `json:"intervalSec"`
			Prompt           *string `json:"prompt"`
			Model            *string `json:"model"`
			IsolatedSession  *bool   `json:"isolatedSession"`
			LightContext     *bool   `json:"lightContext"`
			AckMaxChars      *int    `json:"ackMaxChars"`
			MaxRetries       *int    `json:"maxRetries"`
			ActiveHoursStart *string `json:"activeHoursStart"`
			ActiveHoursEnd   *string `json:"activeHoursEnd"`
			Timezone         *string `json:"timezone"`
			Channel          *string `json:"channel"`
			ChatID           *string `json:"chatId"`
		}
		_ = json.Unmarshal(req.Params, &p)
		aid := firstNonEmpty(p.AgentID, p.AgentID2)
		aid = resolveAgentID(st, tenant, aid)
		if x == nil || aid == "" {
			return writeGoClawErr(conn, req.ID, "INVALID", "agentId is required")
		}
		h, err := x.GetHeartbeat(tenant, aid)
		if err != nil || h == nil {
			h = &store.AgentHeartbeat{TenantID: tenant, AgentID: aid, IntervalSec: 1800, AckMaxChars: 300, MaxRetries: 2, IsolatedSession: true}
		}
		if p.Enabled != nil {
			h.Enabled = *p.Enabled
		}
		if p.IntervalSec != nil && *p.IntervalSec > 0 {
			h.IntervalSec = *p.IntervalSec
		}
		if p.Prompt != nil {
			h.Prompt = *p.Prompt
		}
		if p.Model != nil {
			h.Model = *p.Model
		}
		if p.IsolatedSession != nil {
			h.IsolatedSession = *p.IsolatedSession
		}
		if p.LightContext != nil {
			h.LightContext = *p.LightContext
		}
		if p.AckMaxChars != nil && *p.AckMaxChars > 0 {
			h.AckMaxChars = *p.AckMaxChars
		}
		if p.MaxRetries != nil && *p.MaxRetries >= 0 {
			h.MaxRetries = *p.MaxRetries
		}
		if p.ActiveHoursStart != nil {
			h.ActiveHoursStart = *p.ActiveHoursStart
		}
		if p.ActiveHoursEnd != nil {
			h.ActiveHoursEnd = *p.ActiveHoursEnd
		}
		if p.Timezone != nil {
			h.Timezone = *p.Timezone
		}
		if p.Channel != nil {
			h.Channel = *p.Channel
		}
		if p.ChatID != nil {
			h.ChatID = *p.ChatID
		}
		saved, err := x.PutHeartbeat(*h)
		if err != nil {
			return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
		}
		return writeGoClaw(conn, req.ID, map[string]any{"heartbeat": wsHeartbeatView(saved)})
	case "heartbeat.toggle":
		var p struct {
			AgentID  string `json:"agentId"`
			AgentID2 string `json:"agent_id"`
			Enabled  *bool  `json:"enabled"`
		}
		_ = json.Unmarshal(req.Params, &p)
		aid := firstNonEmpty(p.AgentID, p.AgentID2)
		aid = resolveAgentID(st, tenant, aid)
		if x == nil || aid == "" {
			return writeGoClawErr(conn, req.ID, "INVALID", "agentId is required")
		}
		h, err := x.GetHeartbeat(tenant, aid)
		if err != nil || h == nil {
			return writeGoClawErr(conn, req.ID, "NOT_FOUND", "heartbeat not configured")
		}
		if p.Enabled != nil {
			h.Enabled = *p.Enabled
		} else {
			h.Enabled = !h.Enabled
		}
		saved, err := x.PutHeartbeat(*h)
		if err != nil {
			return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
		}
		return writeGoClaw(conn, req.ID, map[string]any{"heartbeat": wsHeartbeatView(saved)})
	case "heartbeat.test":
		var p struct {
			AgentID  string `json:"agentId"`
			AgentID2 string `json:"agent_id"`
		}
		_ = json.Unmarshal(req.Params, &p)
		aid := firstNonEmpty(p.AgentID, p.AgentID2)
		aid = resolveAgentID(st, tenant, aid)
		if x == nil || aid == "" {
			return writeGoClawErr(conn, req.ID, "INVALID", "agentId is required")
		}
		h, err := x.GetHeartbeat(tenant, aid)
		if err != nil || h == nil || strings.TrimSpace(h.Prompt) == "" {
			return writeGoClawErr(conn, req.ID, "NOT_FOUND", "heartbeat prompt is empty")
		}
		go runHeartbeatOnce(st, tenant, aid, h.Prompt)
		return writeGoClaw(conn, req.ID, map[string]any{"ok": true, "ran": true})
	case "heartbeat.logs":
		return writeGoClaw(conn, req.ID, map[string]any{"logs": []any{}})
	case "heartbeat.checklist.get":
		var p struct {
			AgentID  string `json:"agentId"`
			AgentID2 string `json:"agent_id"`
		}
		_ = json.Unmarshal(req.Params, &p)
		aid := firstNonEmpty(p.AgentID, p.AgentID2)
		text := ""
		if x != nil && aid != "" {
			if h, err := x.GetHeartbeat(tenant, aid); err == nil && h != nil {
				text = h.Checklist
			}
		}
		return writeGoClaw(conn, req.ID, map[string]any{"content": text})
	case "heartbeat.checklist.set":
		var p struct {
			AgentID  string `json:"agentId"`
			AgentID2 string `json:"agent_id"`
			Content  string `json:"content"`
		}
		_ = json.Unmarshal(req.Params, &p)
		aid := firstNonEmpty(p.AgentID, p.AgentID2)
		aid = resolveAgentID(st, tenant, aid)
		if x == nil || aid == "" {
			return writeGoClawErr(conn, req.ID, "INVALID", "agentId is required")
		}
		h, err := x.GetHeartbeat(tenant, aid)
		if err != nil || h == nil {
			h = &store.AgentHeartbeat{TenantID: tenant, AgentID: aid, IntervalSec: 1800, AckMaxChars: 300, MaxRetries: 2, IsolatedSession: true}
		}
		h.Checklist = p.Content
		if _, err := x.PutHeartbeat(*h); err != nil {
			return writeGoClawErr(conn, req.ID, "INVALID", err.Error())
		}
		return writeGoClaw(conn, req.ID, map[string]any{"ok": true, "content": p.Content})
	case "heartbeat.targets":
		return writeGoClaw(conn, req.ID, map[string]any{"targets": []any{}})
	default:
		return writeGoClawErr(conn, req.ID, "NOT_IMPLEMENTED", req.Method)
	}
}

func wsHeartbeatView(h *store.AgentHeartbeat) map[string]any {
	if h == nil {
		return map[string]any{}
	}
	return map[string]any{
		"id": h.ID, "agentId": h.AgentID, "enabled": h.Enabled, "intervalSec": h.IntervalSec,
		"prompt": h.Prompt, "model": h.Model, "isolatedSession": h.IsolatedSession,
		"lightContext": h.LightContext, "ackMaxChars": h.AckMaxChars, "maxRetries": h.MaxRetries,
		"activeHoursStart": h.ActiveHoursStart, "activeHoursEnd": h.ActiveHoursEnd,
		"timezone": h.Timezone, "channel": h.Channel, "chatId": h.ChatID,
		"nextRunAt": h.NextRunAt, "lastRunAt": h.LastRunAt, "lastStatus": h.LastStatus,
		"lastError": h.LastError, "runCount": h.RunCount, "suppressCount": h.SuppressCount,
	}
}

// resolveAgentID accepts an agent id or agent key and returns the canonical id.
func resolveAgentID(st store.StoreIface, tenant, idOrKey string) string {
	idOrKey = strings.TrimSpace(idOrKey)
	if st == nil || idOrKey == "" {
		return idOrKey
	}
	if a, err := agentVisible(st, idOrKey, tenant); err == nil && a != nil {
		return a.ID
	}
	for _, a := range st.ListAgents() {
		if a != nil && a.AgentKey == idOrKey && store.SameTenant(a.TenantID, tenant) {
			return a.ID
		}
	}
	return idOrKey
}

func tenantOfIdentity(who *auth.Identity) string {
	if who != nil && strings.TrimSpace(who.TenantID) != "" {
		return who.TenantID
	}
	return "default"
}

// runHeartbeatOnce performs one heartbeat turn in a fresh session.
func runHeartbeatOnce(st store.StoreIface, tenant, agentID, prompt string) {
	x := store.Extras(st)
	if st == nil || x == nil {
		return
	}
	sess, err := st.CreateSession(store.Session{TenantID: tenant, AgentID: agentID, Label: "heartbeat:" + agentID})
	if err != nil || sess == nil {
		_ = x.MarkHeartbeatRun(tenant, agentID, "error", "session create failed")
		return
	}
	fire := FireSessionChat(wsD.Runtime, st, wsD.Provider, wsD.Meter)
	err = fire(context.Background(), sess.ID, prompt)
	status := "ok"
	msg := ""
	if err != nil {
		status = "error"
		msg = "run failed"
	}
	_ = x.MarkHeartbeatRun(tenant, agentID, status, msg)
}
