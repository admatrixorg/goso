package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func registerGoClawCompatRoutes(mux *http.ServeMux, opt Options) {
	st := opt.Store
	mux.HandleFunc("GET /v1/agents/{id}/kg/graph", handleRealKGGraph(st))
	mux.HandleFunc("GET /v1/agents/{id}/kg/stats", handleRealKGStats(st))
	mux.HandleFunc("GET /v1/agents/{id}/kg/entities", handleRealKGEntities(st))
	mux.HandleFunc("GET /v1/vault/documents", remapJSON(handleListVaultDocs(st), func(m map[string]any) map[string]any {
		docs := goClawVaultDocList(m["docs"])
		m["docs"] = docs
		m["documents"] = docs
		return m
	}))
	mux.HandleFunc("GET /v1/vault/documents/{id}/links", handleGoClawVaultLinks(st))
	mux.HandleFunc("GET /api/vault/documents/{id}/links", handleGoClawVaultLinks(st))
	mux.HandleFunc("GET /v1/vault/documents/{id}", handleGoClawVaultDocGet(st))
	mux.HandleFunc("PUT /v1/vault/documents/{id}", handleGoClawVaultPut(st))
	mux.HandleFunc("DELETE /v1/vault/documents/{id}", handleGoClawVaultDelete(st))
	mux.HandleFunc("GET /v1/memory/documents", handleGoClawMemoryDocuments(st))
	mux.HandleFunc("GET /v1/channels/instances", handleGoClawChannelInstances(st))
	mux.HandleFunc("GET /v1/channels/instances/{id}", handleGoClawChannelInstance(st))
	mux.HandleFunc("GET /v1/usage", handleUsage(opt.Meter))
	mux.HandleFunc("GET /v1/usage/summary", handleGoClawUsageSummary(opt.Meter))
	mux.HandleFunc("GET /v1/stats", handleGoClawStats())
	mux.HandleFunc("GET /v1/tts/capabilities", handleGoClawTTSCaps())
	mux.HandleFunc("GET /v1/storage/files", remapJSON(handleStorageList(), func(m map[string]any) map[string]any {
		if m["files"] == nil {
			m["files"] = m["entries"]
		}
		return m
	}))
	mux.HandleFunc("GET /v1/embedding/status", handleVaultEmbeddings())
	mux.HandleFunc("GET /v1/system-configs", handleGoClawSystemConfigs(st))
	mux.HandleFunc("GET /v1/tools/builtin", emptyList("tools"))
	mux.HandleFunc("GET /v1/system/backup/s3/config", handleGoClawBackupS3())
	mux.HandleFunc("GET /v1/agents/{id}/episodic", handleGoClawEpisodic(st))
	mux.HandleFunc("GET /v1/agents/{id}/evolution/metrics", handleGoClawEvoMetrics(st))
	mux.HandleFunc("GET /v1/agents/{id}/evolution/suggestions", handleEvolution(st))
	mux.HandleFunc("GET /v1/agents/{id}/orchestration", handleGoClawOrchestration(st))
	mux.HandleFunc("GET /v1/agents/{id}/v3-flags", handleGoClawV3Flags())
}

func emptyList(key string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{key: []any{}})
	}
}

func remapJSON(h http.HandlerFunc, fn func(map[string]any) map[string]any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		h(rec, r)
		var m map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil || rec.Code != http.StatusOK {
			w.Header().Set("Content-Type", rec.Header().Get("Content-Type"))
			w.WriteHeader(rec.Code)
			_, _ = w.Write(rec.Body.Bytes())
			return
		}
		writeJSON(w, rec.Code, fn(m))
	}
}

func handleGoClawMemoryDocuments(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if st == nil {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		list, err := st.QueryMemories(store.MemoryQuery{AgentID: strings.TrimSpace(r.URL.Query().Get("agent_id"))})
		if err != nil {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		docs := []map[string]any{}
		for _, m := range list {
			if m == nil {
				continue
			}
			updated := int64(0)
			if !m.CreatedAt.IsZero() {
				updated = m.CreatedAt.UnixMilli()
			}
			docs = append(docs, map[string]any{"path": m.ID, "hash": m.ID, "updated_at": updated})
		}
		writeJSON(w, http.StatusOK, docs)
	}
}

func handleGoClawChannelInstances(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tid := requestTenant(r)
		out := []map[string]any{}
		if st != nil {
			for _, cfg := range st.ListChannelConfigs() {
				if cfg == nil {
					continue
				}
				if strings.Contains(cfg.Name, "/") {
					owner := strings.SplitN(cfg.Name, "/", 2)[0]
					if !store.SameTenant(owner, tid) && tid != store.DefaultTenant && tid != "master" {
						continue
					}
				}
				out = append(out, channelInstanceView(cfg))
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"instances": out, "total": len(out)})
	}
}

func handleGoClawChannelInstance(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if st == nil || id == "" {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		cfg, err := st.GetChannelConfig(id)
		if err != nil || cfg == nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		writeJSON(w, http.StatusOK, channelInstanceView(cfg))
	}
}

func channelInstanceView(cfg *store.ChannelConfig) map[string]any {
	chType := cfg.Name
	parts := strings.Split(cfg.Name, "/")
	if len(parts) >= 2 {
		chType = parts[1]
	}
	updated := ""
	if !cfg.UpdatedAt.IsZero() {
		updated = cfg.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return map[string]any{
		"id": cfg.Name, "name": cfg.Name, "display_name": cfg.Name, "channel_type": chType,
		"agent_id": cfg.AgentID, "config": cfg.Settings, "enabled": cfg.Enabled, "is_default": false,
		"has_credentials": false, "created_by": "", "created_at": updated, "updated_at": updated,
	}
}

func handleGoClawUsageSummary(meter *billing.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// The usage page reads current and previous windows with token, cost and
		// duration counters. The billing meter stays the fallback for tenants
		// whose rows predate the usage_events table.
		if x := store.Usage(nil); x != nil {
			_ = x
		}
		if st, ok := usageSummaryStore(r); ok {
			from, to := usageRange(r)
			cur := st.UsageTotals(requestTenant(r), from, to)
			prev := map[string]float64{"requests": 0}
			if ft, err1 := time.Parse(time.RFC3339, from); err1 == nil {
				if tt, err2 := time.Parse(time.RFC3339, to); err2 == nil {
					span := tt.Sub(ft)
					prev = st.UsageTotals(requestTenant(r), ft.Add(-span).Format(time.RFC3339), ft.Format(time.RFC3339))
				}
			}
			shape := func(m map[string]float64) map[string]any {
				return map[string]any{
					"requests": m["requests"], "input_tokens": m["input_tokens"],
					"output_tokens": m["output_tokens"], "cost": m["cost"], "errors": m["errors"],
					"unique_users": 0, "llm_calls": m["requests"], "tool_calls": 0,
					"avg_duration_ms": m["avg_duration_ms"], "total_tokens": m["total_tokens"],
				}
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"current": shape(cur), "previous": shape(prev), "summary": shape(cur),
				"byAgent": map[string]any{}, "totalRecords": int(cur["requests"]),
			})
			return
		}
		rec := httptest.NewRecorder()
		handleUsage(meter)(rec, r)
		var raw map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &raw)
		total := 0
		if v, ok := raw["calls"].(float64); ok {
			total = int(v)
		}
		writeJSON(w, http.StatusOK, map[string]any{"byAgent": map[string]any{}, "totalRecords": total})
	}
}

func handleGoClawStats() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "uptime_seconds": 0, "ws_up": WSMounted()})
	}
}

func handleGoClawTTSCaps() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"providers": []map[string]any{
			{"provider": "edge", "display_name": "Edge TTS", "requires_api_key": false, "models": []any{}, "voices": []any{}},
		}})
	}
}

func handleGoClawSystemConfigs(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out := map[string]string{}
		if st != nil {
			if s, err := st.GetGatewaySettings(); err == nil && s != nil {
				for k, v := range s.Values {
					out[k] = v
				}
			}
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func handleGoClawBackupS3() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"configured": false, "env_owned": false})
	}
}

func handleGoClawEpisodic(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if st == nil {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		list, _ := st.QueryMemories(store.MemoryQuery{AgentID: id})
		out := []map[string]any{}
		for _, m := range list {
			if m == nil {
				continue
			}
			out = append(out, map[string]any{"id": m.ID, "snippet": m.Body, "created_at": m.CreatedAt})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func handleGoClawEvoMetrics(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if st == nil {
			writeJSON(w, http.StatusOK, map[string]any{})
			return
		}
		writeJSON(w, http.StatusOK, st.GetAgentMetrics(id))
	}
}

func handleGoClawOrchestration(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		a, err := agentVisible(st, id, requestTenant(r))
		if err != nil || a == nil {
			writeJSON(w, http.StatusOK, map[string]any{"mode": ""})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"mode": a.OrchestrationMode})
	}
}

func handleGoClawV3Flags() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{})
	}
}

func handleGoClawVaultDelete(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if st == nil || id == "" {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		doc, err := st.GetVaultDoc(id)
		if err != nil || doc == nil || !store.SameTenant(doc.TenantID, requestTenant(r)) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if err := st.DeleteVaultDoc(id); err != nil {
			writeErr(w, http.StatusInternalServerError, "delete failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func handleGoClawVaultPut(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		if st == nil || id == "" {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		doc, err := st.GetVaultDoc(id)
		if err != nil || doc == nil || !store.SameTenant(doc.TenantID, requestTenant(r)) {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if v, ok := body["title"].(string); ok && strings.TrimSpace(v) != "" {
			doc.Title = strings.TrimSpace(v)
		}
		if v, ok := body["body"].(string); ok && v != doc.Body {
			snapshotVaultDoc(st, requestTenant(r), doc.ID, doc.Title, doc.Body, "edit")
			doc.Body = v
		}
		if v, ok := body["path"].(string); ok && strings.TrimSpace(v) != "" {
			doc.Path = strings.TrimSpace(v)
		}
		updated, err := st.PutVaultDoc(*doc)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, updated)
	}
}
