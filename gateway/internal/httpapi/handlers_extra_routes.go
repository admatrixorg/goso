package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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
	mux.HandleFunc("GET /v1/vault/links", withDocID(handleVaultDocLinks(st)))
	mux.HandleFunc("POST /v1/vault/upload", handleVaultUpload(st, opt.Audit))
	mux.HandleFunc("POST /v1/vault/rescan", handleSyncVault(st))
	mux.HandleFunc("POST /v1/vault/enrichment/stop", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	mux.HandleFunc("POST /v1/contacts/merge", contactAction(handleMergeContact(dir, st, opt.Events, opt.Audit), "merge"))
	mux.HandleFunc("POST /v1/contacts/unmerge", contactAction(handleUndoContact(dir, st, opt.Events, opt.Audit), "undo"))
	mux.HandleFunc("POST /v1/contacts/resolve", handleContactResolve(dir, st))
	mux.HandleFunc("GET /v1/contacts/resolve", handleContactResolve(dir, st))

	mux.HandleFunc("GET /v1/usage/timeseries", handleUsageTimeseries(opt.Meter))
	mux.HandleFunc("GET /v1/usage/breakdown", handleUsageBreakdown(opt.Meter))
	mux.HandleFunc("GET /v1/usage/events/summary", handleUsageBreakdown(opt.Meter))
	mux.HandleFunc("GET /v1/usage/events/breakdown", handleUsageBreakdown(opt.Meter))
	mux.HandleFunc("GET /v1/usage/events/timeseries", handleUsageTimeseries(opt.Meter))
	mux.HandleFunc("GET /v1/usage-caps/policies", emptyList("policies"))
	mux.HandleFunc("GET /v1/usage-caps/events", emptyList("events"))
	mux.HandleFunc("GET /v1/usage-caps/utilization", emptyList("utilization"))

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

func handleVaultTree(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type node struct {
			Path  string `json:"path"`
			Title string `json:"title"`
		}
		docs := []node{}
		if st != nil {
			for _, d := range vaultSvc(st).List() {
				if d == nil || !store.SameTenant(d.TenantID, requestTenant(r)) {
					continue
				}
				docs = append(docs, node{Path: d.Path, Title: d.Title})
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"documents": docs, "tree": docs, "total": len(docs)})
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
