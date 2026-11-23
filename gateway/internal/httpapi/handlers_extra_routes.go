package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/channel"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// registerExtraRoutes closes the remaining UI paths on top of existing
// handlers and stores: vault reads and upload, contact merge/resolve,
// usage aggregates, storage size, user search.
func registerExtraRoutes(mux *http.ServeMux, opt Options) {
	st := opt.Store
	dir := opt.Contacts
	if dir == nil {
		dir = channel.DefaultContacts()
	}

	mux.HandleFunc("GET /v1/vault/search", handleSearchVault(st))
	mux.HandleFunc("GET /v1/vault/graph", handleVaultGraph(st))
	mux.HandleFunc("GET /v1/vault/tree", handleVaultTree(st))
	mux.HandleFunc("GET /v1/agents/{id}/memory/documents", handleAgentMemoryDocuments(st))
	mux.HandleFunc("GET /v1/vault/links", withDocID(handleVaultDocLinks(st)))
	mux.HandleFunc("POST /v1/vault/rescan", handleSyncVault(st))
	mux.HandleFunc("POST /v1/vault/enrichment/stop", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	mux.HandleFunc("POST /v1/contacts/merge", contactAction(handleMergeContact(dir, st, opt.Events, opt.Audit), "merge"))
	mux.HandleFunc("POST /v1/contacts/unmerge", contactAction(handleUndoContact(dir, st, opt.Events, opt.Audit), "undo"))
	mux.HandleFunc("POST /v1/contacts/resolve", handleContactResolve(dir, st))
	mux.HandleFunc("GET /v1/contacts/resolve", handleContactResolve(dir, st))

	mux.HandleFunc("GET /v1/usage/events", handleUsageEventList(opt.Store))
	mux.HandleFunc("GET /v1/usage/timeseries", handleUsageTimeseriesDB(opt.Store, opt.Meter))
	mux.HandleFunc("GET /v1/usage/breakdown", handleUsageBreakdownDB(opt.Store, opt.Meter))
	mux.HandleFunc("GET /v1/usage/events/summary", handleUsageEventSummary(opt.Store, opt.Meter))
	mux.HandleFunc("GET /v1/usage/events/breakdown", handleUsageEventBreakdown(opt.Store))
	mux.HandleFunc("GET /v1/usage/events/timeseries", handleUsageTimeseriesDB(opt.Store, opt.Meter))
	mux.HandleFunc("GET /v1/usage-caps/policies", handleUsageCapPolicies(opt.Store))
	mux.HandleFunc("POST /v1/usage-caps/policies", handleUsageCapPolicyCreate(opt.Store))
	mux.HandleFunc("PATCH /v1/usage-caps/policies/{id}", handleUsageCapPolicyPatch(opt.Store))
	mux.HandleFunc("DELETE /v1/usage-caps/policies/{id}", handleUsageCapPolicyDelete(opt.Store))
	mux.HandleFunc("GET /v1/usage-caps/events", handleUsageCapEvents(opt.Store))
	mux.HandleFunc("GET /v1/usage-caps/utilization", handleUsageCapUtilization(opt.Store))

	mux.HandleFunc("GET /v1/storage/size", handleStorageSize())
	mux.HandleFunc("GET /v1/files", remapJSON(handleStorageList(), func(m map[string]any) map[string]any {
		if m["files"] == nil {
			m["files"] = m["entries"]
		}
		return m
	}))

	mux.HandleFunc("GET /v1/users/search", handleUserSearch(st))
}

// withDocID lets a query parameter stand in for the {id} path value.
func withDocID(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(r.PathValue("id")) == "" {
			if id := strings.TrimSpace(r.URL.Query().Get("doc_id")); id != "" {
				r.SetPathValue("id", id)
			} else if id := strings.TrimSpace(r.URL.Query().Get("id")); id != "" {
				r.SetPathValue("id", id)
			}
		}
		h(w, r)
	}
}

// contactAction adapts the flat GoClaw body onto the {id} path handler.
func contactAction(h http.HandlerFunc, kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			TargetID  string `json:"target_id"`
			TargetID2 string `json:"targetId"`
			SourceID  string `json:"source_id"`
			SourceID2 string `json:"sourceId"`
			Confirm   string `json:"confirm"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		target := firstNonEmpty(body.TargetID, body.TargetID2)
		source := firstNonEmpty(body.SourceID, body.SourceID2)
		if target == "" {
			writeErr(w, http.StatusBadRequest, "target_id is required")
			return
		}
		if kind == "merge" && source == "" {
			writeErr(w, http.StatusBadRequest, "source_id is required")
			return
		}
		r.SetPathValue("id", target)
		payload, _ := json.Marshal(map[string]string{"source_id": source, "confirm": body.Confirm})
		r.Body = io.NopCloser(bytes.NewReader(payload))
		h(w, r)
	}
}

func handleContactResolve(dir *channel.Contacts, st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sender := strings.TrimSpace(r.URL.Query().Get("sender"))
		ch := strings.TrimSpace(r.URL.Query().Get("channel"))
		if r.Method == http.MethodPost {
			var body struct {
				Sender  string `json:"sender"`
				UserID  string `json:"user_id"`
				Channel string `json:"channel"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if strings.TrimSpace(body.Sender) != "" {
				sender = strings.TrimSpace(body.Sender)
			}
			if strings.TrimSpace(body.UserID) != "" {
				sender = strings.TrimSpace(body.UserID)
			}
			if strings.TrimSpace(body.Channel) != "" {
				ch = strings.TrimSpace(body.Channel)
			}
		}
		rows, err := dir.List(requestTenant(r), sender, ch, "")
		if err != nil || len(rows) == 0 {
			writeJSON(w, http.StatusOK, map[string]any{"contact": nil})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"contact": decorateContact(st, rows[0])})
	}
}

type vaultTreeRow struct {
	Name      string
	Path      string
	DocID     string
	Scope     string
	Title     string
	UpdatedAt string
}

// vaultTreeCache keeps one lightweight tenant listing for a minute. The
// vault holds thousands of docs; rebuilding it per subtree request spiked
// memory enough to OOM the gateway.
var vaultTreeCache struct {
	mu   sync.Mutex
	at   map[string]time.Time
	rows map[string][]vaultTreeRow
}

func vaultTreeRows(st store.StoreIface, tenant string) []vaultTreeRow {
	if st == nil {
		return nil
	}
	now := time.Now()
	vaultTreeCache.mu.Lock()
	defer vaultTreeCache.mu.Unlock()
	if vaultTreeCache.at == nil {
		vaultTreeCache.at = map[string]time.Time{}
		vaultTreeCache.rows = map[string][]vaultTreeRow{}
	}
	if at, ok := vaultTreeCache.at[tenant]; ok && now.Sub(at) < time.Minute {
		return vaultTreeCache.rows[tenant]
	}
	rows := []vaultTreeRow{}
	for _, d := range st.ListVaultDocs() {
		if d == nil || !store.SameTenant(d.TenantID, tenant) {
			continue
		}
		p := strings.Trim(d.Path, "/")
		if p == "" {
			continue
		}
		rows = append(rows, vaultTreeRow{
			Name: p, Path: p, DocID: d.ID, Scope: d.Scope, Title: d.Title,
			UpdatedAt: d.Mtime.UTC().Format(time.RFC3339),
		})
	}
	vaultTreeCache.at[tenant] = now
	vaultTreeCache.rows[tenant] = rows
	return rows
}

func handleVaultTree(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		prefix := strings.Trim(strings.TrimSpace(r.URL.Query().Get("path")), "/")
		type entry struct {
			Name        string `json:"name"`
			Path        string `json:"path"`
			IsDir       bool   `json:"isDir"`
			HasChildren bool   `json:"hasChildren,omitempty"`
			DocID       string `json:"docId,omitempty"`
			DocType     string `json:"docType,omitempty"`
			Scope       string `json:"scope,omitempty"`
			Title       string `json:"title,omitempty"`
			UpdatedAt   string `json:"updatedAt,omitempty"`
		}
		files := []entry{}
		dirs := map[string]entry{}
		for _, d := range vaultTreeRows(st, requestTenant(r)) {
			p := d.Path
			if prefix != "" {
				if !strings.HasPrefix(p, prefix+"/") {
					continue
				}
				p = strings.TrimPrefix(p, prefix+"/")
			}
			if i := strings.Index(p, "/"); i >= 0 {
				dirName := p[:i]
				dirPath := dirName
				if prefix != "" {
					dirPath = prefix + "/" + dirName
				}
				dirs[dirPath] = entry{Name: dirName, Path: dirPath, IsDir: true, HasChildren: true}
				continue
			}
			if p == "" {
				continue
			}
			full := p
			if prefix != "" {
				full = prefix + "/" + p
			}
			files = append(files, entry{
				Name: p, Path: full, DocID: d.DocID, DocType: "md",
				Scope: d.Scope, Title: d.Title, UpdatedAt: d.UpdatedAt,
			})
		}
		entries := make([]entry, 0, len(dirs)+len(files))
		dirNames := make([]string, 0, len(dirs))
		for k := range dirs {
			dirNames = append(dirNames, k)
		}
		sort.Strings(dirNames)
		for _, k := range dirNames {
			entries = append(entries, dirs[k])
		}
		sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
		entries = append(entries, files...)
		writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "total": len(entries)})
	}
}

func handleAgentMemoryDocuments(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agent, err := agentVisible(st, r.PathValue("id"), requestTenant(r))
		if err != nil || agent == nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		list, _ := st.QueryMemories(store.MemoryQuery{AgentID: agent.ID})
		docs := []map[string]any{}
		for _, m := range list {
			if m == nil {
				continue
			}
			updated := int64(0)
			if !m.CreatedAt.IsZero() {
				updated = m.CreatedAt.UnixMilli()
			}
			docs = append(docs, map[string]any{"path": m.ID, "hash": m.ID, "agent_id": agent.ID, "updated_at": updated})
		}
		writeJSON(w, http.StatusOK, docs)
	}
}

func handleStorageSize() http.HandlerFunc {
	inner := handleStorageList()
	return func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		inner(rec, r)
		var m map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"used_bytes": 0, "max_bytes": 0})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"used_bytes": m["used_bytes"], "max_bytes": m["max_bytes"], "configured": m["configured"]})
	}
}

func usageWindow(r *http.Request) billing.Query {
	days := 30
	if v := strings.TrimSpace(r.URL.Query().Get("days")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 365 {
			days = n
		}
	}
	now := time.Now().UTC()
	return billing.Query{From: now.AddDate(0, 0, -days), To: now}
}

func handleUsageTimeseries(meter *billing.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if meter == nil {
			writeJSON(w, http.StatusOK, map[string]any{"points": []any{}})
			return
		}
		sum := meter.Query(usageWindow(r))
		points := make([]map[string]any, 0, len(sum.ByDay))
		for _, b := range sum.ByDay {
			points = append(points, map[string]any{
				"ts": b.Date, "calls": b.Calls,
				"prompt_tokens": b.PromptTokens, "completion_tokens": b.CompletionTokens, "total_tokens": b.TotalTokens,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"points": points})
	}
}

func handleUsageBreakdown(meter *billing.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if meter == nil {
			writeJSON(w, http.StatusOK, map[string]any{"items": []any{}})
			return
		}
		sum := meter.Query(usageWindow(r))
		items := make([]map[string]any, 0, len(sum.ByDay))
		for _, b := range sum.ByDay {
			items = append(items, map[string]any{
				"ts": b.Date, "calls": b.Calls, "total_tokens": b.TotalTokens,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "by_day": sum.ByDay})
	}
}

func handleUserSearch(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		users := store.AuthUsers(st)
		out := []map[string]any{}
		if users != nil {
			for _, u := range users.ListAuthUsers() {
				if q != "" && !strings.Contains(strings.ToLower(u.Email), q) && !strings.Contains(strings.ToLower(u.DisplayName), q) {
					continue
				}
				out = append(out, map[string]any{
					"id": u.ID, "email": u.Email, "display_name": u.DisplayName, "status": u.Status,
				})
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"users": out})
	}
}
