package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// registerMiscExtraRoutes closes small UI paths with honest states: an empty
// list when nothing is configured, a 400 when the action needs a provider.
func registerMiscExtraRoutes(mux *http.ServeMux, opt Options) {
	// TTS and voices have no provider in this deployment.
	mux.HandleFunc("GET /v1/tts/config", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"configured": false, "provider": "", "voices": []any{}, "format": "mp3",
		})
	})
	mux.HandleFunc("POST /v1/tts/synthesize", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusBadRequest, "no TTS provider is configured")
	})
	mux.HandleFunc("POST /v1/tts/test-connection", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "no TTS provider is configured"})
	})
	mux.HandleFunc("GET /v1/voices", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"voices": []any{}, "total": 0})
	})
	mux.HandleFunc("POST /v1/voices/refresh", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "voices": []any{}, "total": 0})
	})

	// Storage move and media upload map onto the existing storage handlers.
	mux.HandleFunc("POST /v1/storage/move", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			From string `json:"from"`
			To   string `json:"to"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.From == "" || body.To == "" {
			writeErr(w, http.StatusBadRequest, "from and to are required")
			return
		}
		writeErr(w, http.StatusBadRequest, "storage move is not enabled on this deployment")
	})
	mux.HandleFunc("POST /v1/media/upload", handleStorageUpload(opt.Events, opt.Audit))

	// Backup surfaces: configuration exists, execution is operator only.
	mux.HandleFunc("GET /v1/system/backup/s3/list", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"objects": []any{}, "total": 0})
	})
	mux.HandleFunc("POST /v1/system/backup/s3/backup", backupNotConfigured)
	mux.HandleFunc("POST /v1/system/backup/s3/upload", backupNotConfigured)
	mux.HandleFunc("POST /v1/tenant/backup", backupNotConfigured)
	mux.HandleFunc("POST /v1/tenant/restore", backupNotConfigured)

	// Small read stubs the UI probes on pages we do not execute.
	mux.HandleFunc("GET /v1/providers/claude-cli/auth-status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": false, "provider": "claude-cli"})
	})
	mux.HandleFunc("GET /v1/shell-deny-groups", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"groups": []any{}})
	})
	mux.HandleFunc("GET /v1/pending-messages/messages", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"messages": []any{}, "total": 0})
	})
	mux.HandleFunc("POST /v1/pending-messages/compact", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "removed": 0})
	})
	mux.HandleFunc("GET /v1/tools/builtin/knowledge_graph_search", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusBadRequest, "knowledge_graph_search runs through the WS tool call")
	})

	// OpenAI compatible facade over the first enabled agent.
	mux.HandleFunc("POST /v1/chat/completions", handleChatCompletions(opt.Store))
}

func backupNotConfigured(w http.ResponseWriter, r *http.Request) {
	writeErr(w, http.StatusBadRequest, "backup target is not configured on this deployment")
}

func handleChatCompletions(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			Stream bool `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Messages) == 0 {
			writeErr(w, http.StatusBadRequest, "messages are required")
			return
		}
		last := body.Messages[len(body.Messages)-1].Content
		agentID := ""
		if st != nil {
			for _, a := range st.ListAgents() {
				if a != nil && a.Enabled && store.SameTenant(a.TenantID, requestTenant(r)) {
					agentID = a.ID
					break
				}
			}
		}
		if agentID == "" {
			writeErr(w, http.StatusServiceUnavailable, "no enabled agent in this tenant")
			return
		}
		sess, err := st.CreateSession(store.Session{TenantID: requestTenant(r), AgentID: agentID, Label: "openai:" + agentID})
		if err != nil || sess == nil {
			writeErr(w, http.StatusServiceUnavailable, "session create failed")
			return
		}
		fire := FireSessionChat(wsD.Runtime, st, wsD.Provider, wsD.Meter)
		if fire == nil {
			writeErr(w, http.StatusServiceUnavailable, "runtime unavailable")
			return
		}
		if err := fire(r.Context(), sess.ID, last); err != nil {
			writeErr(w, http.StatusBadGateway, "chat failed")
			return
		}
		msgs, _ := st.ListMessages(sess.ID)
		content := ""
		for _, m := range msgs {
			if m != nil && m.Role == "assistant" {
				content = m.Content
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id": "chatcmpl-" + sess.ID, "object": "chat.completion", "model": body.Model,
			"choices": []map[string]any{{
				"index": 0, "finish_reason": "stop",
				"message": map[string]any{"role": "assistant", "content": content},
			}},
		})
	}
}

// Additional GoClaw surfaces: MCP bundles, CLI presets, pricing root,
// teams import and the batch vault links POST.
func registerMiscExtra2Routes(mux *http.ServeMux, opt Options) {
	st := opt.Store

	mux.HandleFunc("GET /v1/mcp/export/preview", handleMCPExportPreview(st))
	mux.HandleFunc("GET /v1/mcp/export", handleMCPExport(st))
	mux.HandleFunc("POST /v1/mcp/import", handleMCPImport(st))
	mux.HandleFunc("POST /v1/mcp/servers/test", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "MCP probe is not enabled on this deployment"})
	})

	mux.HandleFunc("POST /v1/cli-credentials/check-binary", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name   string `json:"name"`
			Binary string `json:"binary_name"`
			Path   string `json:"binary_path"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		name := firstNonEmpty(body.Name, body.Binary)
		if name == "" {
			writeErr(w, http.StatusBadRequest, "binary name is required")
			return
		}
		available := false
		if p := strings.TrimSpace(body.Path); p != "" {
			if _, err := os.Stat(p); err == nil {
				available = true
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"binary": name, "available": available, "path": body.Path})
	})
	mux.HandleFunc("GET /v1/cli-credentials/presets", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"presets": []any{}})
	})

	mux.HandleFunc("GET /v1/model-pricing", handlePricingList(st))
	mux.HandleFunc("POST /v1/model-pricing", handlePricingCreate(st))
	mux.HandleFunc("POST /v1/model-pricing/sync-openrouter", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusBadRequest, "OpenRouter pricing sync is not enabled; publish overrides directly")
	})

	mux.HandleFunc("GET /v1/teams/import", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"teams": []any{}, "hint": "POST a teams bundle to import"})
	})
	mux.HandleFunc("POST /v1/teams/import", handleTeamsImport(st))
	mux.HandleFunc("GET /v1/vault/links/batch", handleVaultLinksBatchPost(st))
	mux.HandleFunc("POST /v1/vault/links/batch", handleVaultLinksBatchPost(st))
	mux.HandleFunc("GET /v1/test", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "service": "goso-gateway"})
	})
	mux.HandleFunc("GET /v1/files/", remapJSON(handleStorageList(), func(m map[string]any) map[string]any {
		if m["files"] == nil {
			m["files"] = m["entries"]
		}
		return m
	}))
	mux.HandleFunc("GET /v1/system-configs/{key}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"key": r.PathValue("key"), "value": nil})
	})
}

type mcpBundleRow struct {
	Name        string          `json:"name"`
	DisplayName string          `json:"display_name"`
	Transport   string          `json:"transport"`
	Command     string          `json:"command"`
	Args        json.RawMessage `json:"args"`
	URL         string          `json:"url"`
	Headers     json.RawMessage `json:"headers"`
	Env         json.RawMessage `json:"env"`
	ToolPrefix  string          `json:"tool_prefix"`
	TimeoutSec  int             `json:"timeout_sec"`
	Enabled     bool            `json:"enabled"`
}

func handleMCPExportPreview(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.Extras(st)
		if x == nil {
			writeJSON(w, http.StatusOK, map[string]any{"servers": 0})
			return
		}
		rows := x.ListMCPServers(requestTenant(r))
		writeJSON(w, http.StatusOK, map[string]any{"servers": len(rows), "bytes": 0})
	}
}

func handleMCPExport(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.Extras(st)
		out := []map[string]any{}
		if x != nil {
			for _, m := range x.ListMCPServers(requestTenant(r)) {
				out = append(out, map[string]any{
					"name": m.Name, "display_name": m.DisplayName, "transport": m.Transport,
					"command": m.Command, "args": m.Args, "url": m.URL,
					"headers": m.Headers, "env": m.Env, "tool_prefix": m.ToolPrefix,
					"timeout_sec": m.TimeoutSec, "enabled": m.Enabled,
				})
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"version": 1, "servers": out, "tenant": requestTenant(r)})
	}
}

func handleMCPImport(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.Extras(st)
		if x == nil {
			writeErr(w, http.StatusNotImplemented, "MCP store unavailable")
			return
		}
		var body struct {
			Servers []mcpBundleRow `json:"servers"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Servers) == 0 {
			writeErr(w, http.StatusBadRequest, "servers are required")
			return
		}
		created, failed := 0, []string{}
		for _, row := range body.Servers {
			if strings.TrimSpace(row.Name) == "" {
				continue
			}
			_, err := x.PutMCPServer(store.MCPServer{
				TenantID: requestTenant(r), Name: row.Name, DisplayName: row.DisplayName,
				Transport: row.Transport, Command: row.Command, Args: row.Args, URL: row.URL,
				Headers: row.Headers, Env: row.Env, ToolPrefix: row.ToolPrefix,
				TimeoutSec: row.TimeoutSec, Enabled: row.Enabled,
			})
			if err != nil {
				failed = append(failed, row.Name)
				continue
			}
			created++
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": len(failed) == 0, "created": created, "failed": failed})
	}
}

func handleTeamsImport(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Teams []struct {
				Name        string `json:"name"`
				LeadAgentID string `json:"lead_agent_id"`
			} `json:"teams"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Teams) == 0 {
			writeErr(w, http.StatusBadRequest, "teams are required")
			return
		}
		created, failed := 0, []string{}
		for _, row := range body.Teams {
			name := strings.TrimSpace(row.Name)
			if name == "" || st == nil {
				continue
			}
			t, err := st.CreateTeam(store.Team{TenantID: requestTenant(r), Name: name, LeadAgentID: row.LeadAgentID})
			if err != nil {
				failed = append(failed, name)
				continue
			}
			if row.LeadAgentID != "" {
				_, _ = st.AddTeamMember(store.TeamMember{TeamID: t.ID, AgentID: row.LeadAgentID, Role: "lead"})
			}
			created++
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": len(failed) == 0, "created": created, "failed": failed})
	}
}

func handleVaultLinksBatchPost(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ids := []string{}
		if r.Method == http.MethodPost {
			var body struct {
				IDs []string `json:"ids"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			ids = body.IDs
		}
		if len(ids) == 0 {
			ids = strings.Split(strings.TrimSpace(r.URL.Query().Get("ids")), ",")
		}
		out := map[string]any{}
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" || st == nil {
				continue
			}
			if ob, ib, err := vaultSvc(st).Links(id); err == nil {
				out[id] = map[string]any{"outbound": ob, "inbound": ib}
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"links": out})
	}
}
