package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/skill"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// skillRowView maps a stored skill onto the row the skills page reads.
func skillRowView(r *store.SkillRow) map[string]any {
	if r == nil {
		return map[string]any{}
	}
	desc := r.Description
	if desc == "" {
		desc = skillDescriptionFromDisk(r.TenantID, r.Slug)
	}
	return map[string]any{
		"id": r.ID, "name": r.Name, "slug": r.Slug, "description": desc,
		"source": "registry", "visibility": r.Visibility, "version": r.Version,
		"is_system": r.IsSystem, "status": r.Status, "enabled": r.Enabled,
		"owner_id": r.OwnerID, "file_path": r.FilePath, "file_size": r.FileSize,
		"tags": r.Tags, "deps": r.Deps, "frontmatter": r.Frontmatter,
		"manager_agents": []any{}, "missing_deps": []any{},
		"created_at": r.CreatedAt, "updated_at": r.UpdatedAt,
	}
}

func skillDescriptionFromDisk(tenant, slug string) string {
	doc, err := skill.Load(tenant, slug)
	if err != nil || doc == nil {
		return ""
	}
	for _, line := range strings.Split(doc.Body, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(line, "#"))
		if line != "" {
			if len(line) > 200 {
				line = line[:200]
			}
			return line
		}
	}
	return ""
}

// skillRowsForPage prefers the registry and falls back to the on disk catalog.
func skillRowsForPage(st store.StoreIface, tenant string) []map[string]any {
	if x := store.Skills(st); x != nil {
		rows := x.ListSkillRows(tenant)
		if len(rows) > 0 {
			out := make([]map[string]any, 0, len(rows))
			for _, r := range rows {
				out = append(out, skillRowView(r))
			}
			return out
		}
	}
	infos, _ := skill.List(tenant)
	out := make([]map[string]any, 0, len(infos))
	for _, info := range infos {
		name := strings.TrimSpace(info.Name)
		if name == "" {
			continue
		}
		out = append(out, map[string]any{
			"id": name, "name": name, "slug": name,
			"description": skillDescriptionFromDisk(tenant, name),
			"source": "filesystem", "visibility": "tenant", "version": 1,
			"is_system": false, "status": "active", "enabled": true,
			"manager_agents": []any{}, "missing_deps": []any{},
		})
	}
	return out
}

// registerSkillRegistryRoutes serves the skill detail tabs and grants.
func registerSkillRegistryRoutes(mux *http.ServeMux, st store.StoreIface) {
	mux.HandleFunc("GET /v1/skills/{id}/versions", handleSkillVersions(st))
	mux.HandleFunc("GET /v1/skills/{id}/grants", handleSkillGrants(st))
	mux.HandleFunc("POST /v1/skills/{id}/grants", handleSkillGrantPut(st))
	mux.HandleFunc("DELETE /v1/skills/{id}/grants/{agentId}", handleSkillGrantDelete(st))
	mux.HandleFunc("GET /v1/skills/{id}/evolution", handleSkillEvolutionGet(st))
	mux.HandleFunc("PUT /v1/skills/{id}/evolution", handleSkillEvolutionPut(st))
	mux.HandleFunc("GET /v1/skills/{id}/metrics", handleSkillMetrics(st))
	mux.HandleFunc("GET /v1/skills/{id}/activity", handleSkillActivity(st))
	mux.HandleFunc("GET /v1/skills/{id}/suggestions", handleSkillSuggestions(st))
}

func handleSkillVersions(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.Skills(st)
		sk, err := x != nil, error(nil)
		_ = sk
		_ = err
		skillRow, gerr := skillRegistryLookup(x, requestTenant(r), r.PathValue("id"))
		if gerr != nil {
			writeJSON(w, http.StatusOK, map[string]any{"versions": []any{}, "current": 1})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"versions": []int{skillRow.Version}, "current": skillRow.Version})
	}
}

func skillRegistryLookup(x store.SkillStore, tenant, id string) (*store.SkillRow, error) {
	if x == nil {
		return nil, store.ErrNotFound
	}
	return x.GetSkillRow(tenant, id)
}

func handleSkillGrants(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.Skills(st)
		row, err := skillRegistryLookup(x, requestTenant(r), r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"grants": []any{}, "total": 0})
			return
		}
		grants := x.ListSkillGrants(requestTenant(r), row.ID)
		writeJSON(w, http.StatusOK, map[string]any{"grants": grants, "total": len(grants)})
	}
}

func handleSkillGrantPut(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.Skills(st)
		row, err := skillRegistryLookup(x, requestTenant(r), r.PathValue("id"))
		if err != nil {
			writeErr(w, http.StatusNotFound, "skill not found")
			return
		}
		var body struct {
			AgentID       string `json:"agent_id"`
			AgentID2      string `json:"agentId"`
			PinnedVersion int    `json:"pinned_version"`
			CanManage     bool   `json:"can_manage"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		agentID := body.AgentID
		if agentID == "" {
			agentID = body.AgentID2
		}
		if agentID == "" {
			writeErr(w, http.StatusBadRequest, "agent_id is required")
			return
		}
		g, perr := x.PutSkillGrant(store.SkillGrant{
			TenantID: requestTenant(r), SkillID: row.ID, AgentID: agentID,
			PinnedVersion: body.PinnedVersion, CanManage: body.CanManage, GrantedBy: "api",
		})
		if perr != nil {
			writeErr(w, http.StatusBadRequest, perr.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"grant": g, "ok": true})
	}
}

func handleSkillGrantDelete(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.Skills(st)
		row, err := skillRegistryLookup(x, requestTenant(r), r.PathValue("id"))
		if err != nil {
			writeErr(w, http.StatusNotFound, "skill not found")
			return
		}
		if err := x.DeleteSkillGrant(requestTenant(r), row.ID, r.PathValue("agentId")); err != nil {
			writeErr(w, http.StatusNotFound, "grant not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func handleSkillEvolutionGet(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.Skills(st)
		row, err := skillRegistryLookup(x, requestTenant(r), r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"enabled": false, "auto_apply": false, "min_sample_size": 20})
			return
		}
		writeJSON(w, http.StatusOK, x.GetSkillEvolution(requestTenant(r), row.ID))
	}
}

func handleSkillEvolutionPut(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.Skills(st)
		row, err := skillRegistryLookup(x, requestTenant(r), r.PathValue("id"))
		if err != nil {
			writeErr(w, http.StatusNotFound, "skill not found")
			return
		}
		var body store.SkillEvolution
		_ = json.NewDecoder(r.Body).Decode(&body)
		body.TenantID = requestTenant(r)
		body.SkillID = row.ID
		if err := x.PutSkillEvolution(body); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, x.GetSkillEvolution(requestTenant(r), row.ID))
	}
}

func handleSkillMetrics(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"uses": 0, "success_rate": 0, "avg_duration_ms": 0, "improvements": 0})
	}
}

func handleSkillActivity(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"activity": []any{}, "total": 0})
	}
}

func handleSkillSuggestions(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"suggestions": []any{}, "total": 0})
	}
}
