// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/agent"
	"github.com/mqglobal/goso/gateway/internal/approval"
	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func registerConnectorRoutes(mux *http.ServeMux, opt Options) {
	mux.HandleFunc("POST /api/connectors", handleCreateConnector(opt))
	mux.HandleFunc("GET /api/connectors", handleListConnectors(opt))
	mux.HandleFunc("GET /api/connectors/{name}", handleGetConnector(opt))
	mux.HandleFunc("POST /api/agents/{id}/connectors", handleLinkAgentConnector(opt))
	mux.HandleFunc("GET /api/agents/{id}/connectors", handleListAgentConnectors(opt))
	mux.HandleFunc("POST /api/approvals/{id}/decision", handleApprovalDecision(opt))
	mux.HandleFunc("GET /api/approvals/{id}", handleGetApproval(opt))
	mux.HandleFunc("GET /api/events", handleListEvents(opt))
	mux.HandleFunc("POST /api/tools/invoke", handleToolInvoke(opt))
}

func handleCreateConnector(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body store.ConnectorRecord
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		body.Name = strings.TrimSpace(body.Name)
		if body.Name == "" {
			writeErr(w, http.StatusBadRequest, "name is required")
			return
		}
		if body.Transport == "" {
			body.Transport = connector.TransportHTTP
		}
		if err := mountConnector(opt, &body); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		rec, err := opt.Store.CreateConnector(body)
		if err != nil {
			if errors.Is(err, store.ErrExists) {
				writeErr(w, http.StatusConflict, "connector already exists")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if !rec.Enabled {
			_ = opt.Registry.SetEnabled(rec.Name, false)
		}
		writeJSON(w, http.StatusCreated, rec)
	}
}

func handleListConnectors(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list := opt.Store.ListConnectors()
		if list == nil {
			list = []*store.ConnectorRecord{}
		}
		out := make([]map[string]any, 0, len(list))
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		for _, rec := range list {
			item := map[string]any{
				"name":           rec.Name,
				"transport":      rec.Transport,
				"endpoint":       rec.Endpoint,
				"credential_ref": rec.CredentialRef,
				"schema_version": rec.SchemaVersion,
				"enabled":        rec.Enabled,
				"manifest_url":   rec.ManifestURL,
				"timeout_ms":     rec.TimeoutMS,
				"retries":        rec.Retries,
				"created_at":     rec.CreatedAt,
				"health":         "unknown",
			}
			c, err := opt.Registry.Lookup(rec.Name)
			if err != nil {
				item["health"] = "unregistered"
			} else if !rec.Enabled {
				item["health"] = "disabled"
			} else if herr := c.Health(ctx); herr != nil {
				item["health"] = "unavailable"
				item["health_error"] = herr.Error()
			} else {
				item["health"] = "ok"
			}
			out = append(out, item)
		}
		writeJSON(w, http.StatusOK, map[string]any{"connectors": out})
	}
}

func handleGetConnector(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		rec, err := opt.Store.GetConnector(name)
		if err != nil {
			writeErr(w, http.StatusNotFound, "connector not found")
			return
		}
		writeJSON(w, http.StatusOK, rec)
	}
}

func handleLinkAgentConnector(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentID := r.PathValue("id")
		var body struct {
			Connector string `json:"connector"`
			Name      string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		name := strings.TrimSpace(body.Connector)
		if name == "" {
			name = strings.TrimSpace(body.Name)
		}
		if name == "" {
			writeErr(w, http.StatusBadRequest, "connector is required")
			return
		}
		if err := opt.Store.LinkAgentConnector(agentID, name); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		names, _ := opt.Store.ListAgentConnectors(agentID)
		writeJSON(w, http.StatusCreated, map[string]any{"agent_id": agentID, "connectors": names})
	}
}

func handleListAgentConnectors(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentID := r.PathValue("id")
		names, err := opt.Store.ListAgentConnectors(agentID)
		if err != nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"agent_id": agentID, "connectors": names})
	}
}

func handleApprovalDecision(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body struct {
			Decision string `json:"decision"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		req, err := opt.Gate.Decide(r.Context(), id, body.Decision)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, approval.ErrNotFound) {
				status = http.StatusNotFound
			}
			writeErr(w, status, err.Error())
			return
		}
		opt.Events.Append(eventstore.Event{
			Connector: req.Connector,
			Tool:      req.Tool,
			Kind:      eventstore.KindHumanFeedback,
			Summary:   eventstore.SummarizeArgs(map[string]any{"approval_id": req.ID, "decision": req.Decision, "status": req.Status}),
		})
		writeJSON(w, http.StatusOK, req)
	}
}

func handleGetApproval(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := opt.Gate.Get(r.PathValue("id"))
		if err != nil {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, req)
	}
}

func handleListEvents(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit, _ := strconv.Atoi(q.Get("limit"))
		list := opt.Events.Filter(q.Get("kind"), q.Get("connector"), limit)
		if list == nil {
			list = nil
		}
		writeJSON(w, http.StatusOK, map[string]any{"events": list})
	}
}

func handleToolInvoke(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SessionID string         `json:"session_id"`
			Connector string         `json:"connector"`
			Tool      string         `json:"tool"`
			Arguments map[string]any `json:"arguments"`
			Args      map[string]any `json:"args"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		args := body.Arguments
		if args == nil {
			args = body.Args
		}
		if strings.TrimSpace(body.Connector) == "" || strings.TrimSpace(body.Tool) == "" {
			writeErr(w, http.StatusBadRequest, "connector and tool are required")
			return
		}
		cr, err := opt.Runtime.CallTool(r.Context(), body.Connector, body.Tool, args)
		if err != nil {
			status := http.StatusBadGateway
			if errors.Is(err, connector.ErrUnavailable) {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{
					"error":  "connector_unavailable",
					"detail": err.Error(),
					"trace":  cr.Trace,
				})
				return
			}
			writeJSON(w, status, map[string]any{"error": err.Error(), "trace": cr.Trace})
			return
		}
		if body.SessionID != "" && cr.Result != nil {
			_, _ = opt.Store.AddMessage(store.Message{
				SessionID: body.SessionID,
				Role:      "tool",
				Content:   mustJSON(cr.Result),
			})
		}
		writeJSON(w, http.StatusOK, cr)
	}
}

func handleChatRuntime(rt *agent.Runtime, st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SessionID string `json:"session_id"`
			Message   string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if strings.TrimSpace(body.SessionID) == "" || strings.TrimSpace(body.Message) == "" {
			writeErr(w, http.StatusBadRequest, "session_id and message are required")
			return
		}
		if _, err := st.GetSession(body.SessionID); err != nil {
			writeErr(w, http.StatusNotFound, "session not found")
			return
		}
		out, err := rt.Chat(r.Context(), body.SessionID, body.Message)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func mountConnector(opt Options, rec *store.ConnectorRecord) error {
	cfg := connector.Config{
		Name:          rec.Name,
		Transport:     rec.Transport,
		Endpoint:      rec.Endpoint,
		CredentialRef: rec.CredentialRef,
		SchemaVersion: rec.SchemaVersion,
		ManifestURL:   rec.ManifestURL,
		ManifestJSON:  rec.ManifestJSON,
		TimeoutMS:     rec.TimeoutMS,
		Retries:       rec.Retries,
	}
	c, err := connector.Build(cfg)
	if err != nil {
		return err
	}
	if err := opt.Registry.Replace(c); err != nil {
		return err
	}
	opt.Gate.Relayer = func(ctx context.Context, req *approval.Request, decision string) error {
		rec, err := opt.Store.GetConnector(req.Connector)
		if err != nil {
			return nil
		}
		return connector.RelayDecision(ctx, nil, rec.Endpoint, "", req.ID, decision, map[string]any{
			"tool": req.Tool,
		})
	}
	return nil
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
