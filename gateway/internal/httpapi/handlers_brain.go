package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/brain"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func registerBrainRoutes(mux *http.ServeMux, opt Options) {
	aliasAPI(mux, "GET /api/brain/pages", handleBrainList(opt))
	aliasAPI(mux, "GET /api/brain/pages/{id}", handleBrainGet(opt))
	aliasAPI(mux, "POST /api/brain/compile", handleBrainCompile(opt))
	aliasAPI(mux, "GET /api/brain/search", handleBrainSearch(opt))
	aliasAPI(mux, "GET /api/brain/sources", handleBrainSources(opt))
	aliasAPI(mux, "GET /api/brain/plans", handleBrainPlans(opt))
	aliasAPI(mux, "GET /api/brain/graph", handleBrainGraph(opt))
	aliasAPI(mux, "GET /api/brain/pages/{id}/versions", handleBrainVersions(opt))
	aliasAPI(mux, "POST /api/brain/pages/{id}/rollback", handleBrainRollback(opt))
	aliasAPI(mux, "GET /api/brain/pages/{id}/links", handleBrainLinks(opt))
	aliasAPI(mux, "GET /api/brain/drafts", handleBrainDrafts(opt))
	aliasAPI(mux, "POST /api/brain/drafts", handleBrainDraftCreate(opt))
	aliasAPI(mux, "POST /api/brain/drafts/{id}/review", handleBrainDraftReview(opt))
}

func brainTenant(r *http.Request) string {
	if q := strings.TrimSpace(r.URL.Query().Get("tenant")); q != "" {
		return store.NormalizeTenant(q)
	}
	return requestTenant(r)
}

func brainSQL(opt Options) *store.SQLiteStore {
	switch s := opt.Store.(type) {
	case *store.SQLiteStore:
		return s
	case *store.PostgresStore:
		return s.SQLiteStore
	default:
		return nil
	}
}

func handleBrainList(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenant := brainTenant(r)
		pages, err := brain.List(opt.Store, tenant)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if pages == nil {
			pages = []store.BrainPage{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"tenant": tenant, "pages": pages})
	}
}

func handleBrainGet(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, err := brain.Get(opt.Store, brainTenant(r), strings.TrimSpace(r.PathValue("id")))
		if err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		writeJSON(w, http.StatusOK, page)
	}
}

func handleBrainSearch(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		hits, err := brain.Search(opt.Store, brainTenant(r), q, 20)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"hits": hits})
	}
}

func handleBrainCompile(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Tenant string `json:"tenant"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		raw := strings.TrimSpace(body.Tenant)
		if raw == "*" || strings.EqualFold(raw, "all") {
			out, err := brain.CompileAllTenants(r.Context(), opt.Store, opt.Provider)
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"tenants": out})
			return
		}
		tenant := brainTenant(r)
		if raw != "" {
			tenant = store.NormalizeTenant(raw)
		}
		n, err := brain.CompileTenant(r.Context(), opt.Store, opt.Provider, tenant)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tenant": tenant, "pages_upserted": n})
	}
}

func handleBrainSources(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sql := brainSQL(opt)
		if sql == nil {
			writeJSON(w, http.StatusOK, map[string]any{"sources": []any{}})
			return
		}
		srcs, err := sql.ListBrainSources(brainTenant(r))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"sources": srcs})
	}
}

func handleBrainPlans(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sql := brainSQL(opt)
		if sql == nil {
			writeJSON(w, http.StatusOK, map[string]any{"plans": []any{}})
			return
		}
		plans, err := sql.ListBrainPlans(brainTenant(r))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"plans": plans})
	}
}

func handleBrainGraph(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sql := brainSQL(opt)
		if sql == nil {
			writeJSON(w, http.StatusOK, map[string]any{"edges": []any{}})
			return
		}
		edges, err := sql.ListBrainGraph(brainTenant(r), 200)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"edges": edges})
	}
}

func handleBrainVersions(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sql := brainSQL(opt)
		if sql == nil {
			writeJSON(w, http.StatusOK, map[string]any{"versions": []any{}})
			return
		}
		vers, err := sql.ListBrainVersions(brainTenant(r), strings.TrimSpace(r.PathValue("id")))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"versions": vers})
	}
}

func handleBrainRollback(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sql := brainSQL(opt)
		if sql == nil {
			writeErr(w, http.StatusInternalServerError, "store required")
			return
		}
		var body struct {
			Rev int `json:"rev"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		page, err := sql.RollbackBrainPage(brainTenant(r), strings.TrimSpace(r.PathValue("id")), body.Rev)
		if err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		writeJSON(w, http.StatusOK, page)
	}
}

func handleBrainLinks(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sql := brainSQL(opt)
		if sql == nil {
			writeJSON(w, http.StatusOK, map[string]any{"outbound": []any{}, "inbound": []any{}})
			return
		}
		out, in, err := sql.ListBrainLinks(brainTenant(r), strings.TrimSpace(r.PathValue("id")))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"outbound": out, "inbound": in})
	}
}

func handleBrainDrafts(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sql := brainSQL(opt)
		if sql == nil {
			writeJSON(w, http.StatusOK, map[string]any{"drafts": []any{}})
			return
		}
		drafts, err := sql.ListBrainDrafts(brainTenant(r), strings.TrimSpace(r.URL.Query().Get("status")))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"drafts": drafts})
	}
}

func handleBrainDraftCreate(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sql := brainSQL(opt)
		if sql == nil {
			writeErr(w, http.StatusInternalServerError, "store required")
			return
		}
		var body store.BrainDraft
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		body.TenantID = brainTenant(r)
		d, err := sql.PutBrainDraft(body)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, d)
	}
}

func handleBrainDraftReview(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sql := brainSQL(opt)
		if sql == nil {
			writeErr(w, http.StatusInternalServerError, "store required")
			return
		}
		var body struct {
			Status string `json:"status"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		st := strings.ToLower(strings.TrimSpace(body.Status))
		switch st {
		case "approved", "rejected", "needs_revision", "withdrawn":
		default:
			writeErr(w, http.StatusBadRequest, "status must be approved, rejected, needs_revision, withdrawn")
			return
		}
		tenant := brainTenant(r)
		id := strings.TrimSpace(r.PathValue("id"))
		d, err := sql.SetBrainDraftStatus(tenant, id, st)
		if err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if st == "approved" && d.PageID != "" {
			cur, err := sql.GetBrainPage(tenant, d.PageID)
			if err == nil && cur != nil {
				if d.Title != "" {
					cur.Title = d.Title
				}
				cur.Body = d.Body
				_, _ = sql.UpsertBrainPage(*cur)
			}
		}
		writeJSON(w, http.StatusOK, d)
	}
}
