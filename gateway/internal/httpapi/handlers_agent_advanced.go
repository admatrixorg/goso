package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/pkgmgr"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// registerAgentAdvancedRoutes serves agent bundles, prompt preview, instances,
// KG traversal and the package runtime sub-paths the UI calls.
func registerAgentAdvancedRoutes(mux *http.ServeMux, opt Options) {
	mux.HandleFunc("GET /v1/agents/{id}/instances", handleAgentInstances(opt.Store))
	mux.HandleFunc("GET /v1/agents/{id}/system-prompt-preview", handleAgentPromptPreview(opt.Store))
	mux.HandleFunc("GET /v1/agents/{id}/export", handleAgentExport(opt.Store))
	mux.HandleFunc("GET /v1/agents/{id}/export/preview", handleAgentExportPreview(opt.Store))
	mux.HandleFunc("POST /v1/agents/import/preview", handleAgentImportPreview(opt.Store))
	mux.HandleFunc("POST /v1/agents/import", handleAgentImport(opt.Store))
	mux.HandleFunc("GET /v1/agents/{id}/kg/traverse", handleAgentKGTraverse(opt.Store))
	mux.HandleFunc("POST /v1/agents/{id}/kg/dedup/scan", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"candidates": []any{}, "scanned": 0})
	})
	mux.HandleFunc("GET /v1/agents/{id}/memory/chunks", handleAgentMemoryChunks(opt.Store))
	mux.HandleFunc("POST /v1/agents/{id}/resummon", handleAgentResummon(opt.Store))
	mux.HandleFunc("POST /v1/agents/{id}/regenerate", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusBadRequest, "regenerate needs a session message")
	})

	mux.HandleFunc("GET /v1/packages/runtimes", handlePackageRuntimes(opt.Packages))
	mux.HandleFunc("GET /v1/packages/updates", handlePackageUpdates(opt.Packages))
	mux.HandleFunc("POST /v1/packages/updates/refresh", handlePackageRefresh(opt.Packages))
	mux.HandleFunc("POST /v1/packages/updates/apply-all", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusBadRequest, "no package updates pending")
	})
	mux.HandleFunc("GET /v1/packages/github-releases", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"releases": []any{}})
	})
	mux.HandleFunc("POST /v1/packages/uninstall", packageByID(func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusBadRequest, "uninstall is not available here")
	}))
}

func packagesOf(m *pkgmgr.Manager) *pkgmgr.Manager {
	if m == nil {
		return pkgmgr.Default()
	}
	return m
}

func handlePackageRuntimes(m *pkgmgr.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snap := packagesOf(m).Snapshot()
		writeJSON(w, http.StatusOK, map[string]any{"runtimes": snap.Runtimes, "packages": snap.Packages})
	}
}

func handlePackageUpdates(m *pkgmgr.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snap := packagesOf(m).Snapshot()
		updates := []any{}
		writeJSON(w, http.StatusOK, map[string]any{"updates": updates, "jobs": snap.Jobs})
	}
}

func handlePackageRefresh(m *pkgmgr.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snap := packagesOf(m).Snapshot()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "runtimes": snap.Runtimes})
	}
}

func packageByID(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ID string `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if strings.TrimSpace(body.ID) == "" {
			writeErr(w, http.StatusBadRequest, "id is required")
			return
		}
		r.SetPathValue("id", body.ID)
		next(w, r)
	}
}

func agentByKeyOrID(st store.StoreIface, id, tenant string) *store.Agent {
	a, err := agentVisible(st, id, tenant)
	if err != nil {
		return nil
	}
	return a
}

func handleAgentInstances(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a := agentByKeyOrID(st, r.PathValue("id"), requestTenant(r))
		if a == nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		rows := []map[string]any{}
		for _, cfg := range st.ListChannelConfigs() {
			if cfg == nil || cfg.AgentID != a.ID {
				continue
			}
			rows = append(rows, channelInstanceView(cfg))
		}
		writeJSON(w, http.StatusOK, map[string]any{"instances": rows, "total": len(rows)})
	}
}

func handleAgentPromptPreview(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a := agentByKeyOrID(st, r.PathValue("id"), requestTenant(r))
		if a == nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		var b strings.Builder
		name := strings.TrimSpace(a.DisplayName)
		if name == "" {
			name = a.AgentKey
		}
		b.WriteString("You are " + name + ", a GOSO gateway assistant.\n")
		if strings.TrimSpace(a.Instructions) != "" {
			b.WriteString("\n" + strings.TrimSpace(a.Instructions) + "\n")
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"prompt": b.String(), "agent_id": a.ID, "agent_key": a.AgentKey,
			"model": a.Model, "provider": a.LLMProvider,
		})
	}
}

type agentBundle struct {
	Version    int             `json:"version"`
	Agent      store.Agent     `json:"agent"`
	Channels   []string        `json:"channels,omitempty"`
	SystemText string          `json:"system_text,omitempty"`
	Extra      json.RawMessage `json:"extra,omitempty"`
}

func buildAgentBundle(st store.StoreIface, a *store.Agent) agentBundle {
	b := agentBundle{Version: 1, Agent: *a}
	b.Channels = []string{}
	for _, cfg := range st.ListChannelConfigs() {
		if cfg != nil && cfg.AgentID == a.ID {
			b.Channels = append(b.Channels, cfg.Name)
		}
	}
	return b
}

func handleAgentExport(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a := agentByKeyOrID(st, r.PathValue("id"), requestTenant(r))
		if a == nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		writeJSON(w, http.StatusOK, buildAgentBundle(st, a))
	}
}

func handleAgentExportPreview(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a := agentByKeyOrID(st, r.PathValue("id"), requestTenant(r))
		if a == nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		b := buildAgentBundle(st, a)
		writeJSON(w, http.StatusOK, map[string]any{
			"agent_key": a.AgentKey, "display_name": a.DisplayName,
			"channels": len(b.Channels), "bytes": len(b.Agent.Instructions),
		})
	}
}

func decodeAgentBundle(r *http.Request) (*agentBundle, error) {
	var b agentBundle
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		return nil, err
	}
	return &b, nil
}

func handleAgentImportPreview(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := decodeAgentBundle(r)
		if err != nil || strings.TrimSpace(b.Agent.AgentKey) == "" {
			writeErr(w, http.StatusBadRequest, "agent_key is required")
			return
		}
		exists := false
		if a := agentByKeyOrID(st, b.Agent.AgentKey, requestTenant(r)); a != nil {
			exists = true
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"agent_key": b.Agent.AgentKey, "display_name": b.Agent.DisplayName,
			"exists": exists, "channels": len(b.Channels),
		})
	}
}

func handleAgentImport(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := decodeAgentBundle(r)
		if err != nil || strings.TrimSpace(b.Agent.AgentKey) == "" {
			writeErr(w, http.StatusBadRequest, "agent_key is required")
			return
		}
		tenant := requestTenant(r)
		if a := agentByKeyOrID(st, b.Agent.AgentKey, tenant); a != nil {
			writeErr(w, http.StatusConflict, "agent already exists")
			return
		}
		in := b.Agent
		in.ID = ""
		in.TenantID = tenant
		in.CreatedAt = time.Time{}
		in.UpdatedAt = time.Time{}
		saved, err := st.CreateAgent(in)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, publicAgentView(saved, false, false))
	}
}

func handleAgentKGTraverse(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a := agentByKeyOrID(st, r.PathValue("id"), requestTenant(r))
		if a == nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		entityID := strings.TrimSpace(r.URL.Query().Get("entity_id"))
		if entityID == "" {
			entityID = strings.TrimSpace(r.URL.Query().Get("id"))
		}
		if entityID == "" {
			writeErr(w, http.StatusBadRequest, "entity_id is required")
			return
		}
		ent, err := st.GetKGEntity(entityID, store.KGScope{})
		if err != nil || ent == nil || !store.SameTenant(ent.TenantID, a.TenantID) {
			writeErr(w, http.StatusNotFound, "entity not found")
			return
		}
		rels, _ := st.ListKGRelations(ent.ID, store.KGScope{})
		out := make([]map[string]any, 0, len(rels))
		for _, rel := range rels {
			if rel == nil || !store.SameTenant(rel.TenantID, a.TenantID) {
				continue
			}
			out = append(out, map[string]any{"id": rel.ID, "from_id": rel.FromID, "to_id": rel.ToID, "rel": rel.Rel})
		}
		writeJSON(w, http.StatusOK, map[string]any{"entity": ent, "relations": out})
	}
}

func handleAgentMemoryChunks(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a := agentByKeyOrID(st, r.PathValue("id"), requestTenant(r))
		if a == nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		limit := store.MemoryQueryCap
		if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
			if n, err := strconvAtoi(v); err == nil && n > 0 && n < limit {
				limit = n
			}
		}
		rows, _ := st.QueryMemories(store.MemoryQuery{AgentID: a.ID, Limit: limit})
		if rows == nil {
			rows = []*store.Memory{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"chunks": rows, "total": len(rows)})
	}
}

func handleAgentResummon(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a := agentByKeyOrID(st, r.PathValue("id"), requestTenant(r))
		if a == nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		fresh, ferr := st.GetAgent(a.ID)
		if ferr != nil || fresh == nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		fresh.Enabled = true
		fresh.UpdatedAt = time.Time{}
		saved, err := st.UpdateAgent(*fresh)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, publicAgentView(saved, false, false))
	}
}

// strconvAtoi keeps the handler import list small.
func strconvAtoi(s string) (int, error) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errBadIntLocal
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

var errBadIntLocal = &badIntLocal{}

type badIntLocal struct{}

func (*badIntLocal) Error() string { return "bad integer" }
