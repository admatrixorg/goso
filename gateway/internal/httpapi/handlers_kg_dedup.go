package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// registerKGDedupRoutes serves duplicate review: scan, list and merge.
func registerKGDedupRoutes(mux *http.ServeMux, st store.StoreIface) {
	mux.HandleFunc("POST /v1/kg/dedup/scan", handleKGDedupScan(st))
	mux.HandleFunc("GET /v1/kg/dedup/candidates", handleKGDedupList(st))
	mux.HandleFunc("POST /v1/kg/dedup/merge", handleKGMerge(st))
}

func handleKGDedupScan(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.KGDedup(st)
		if x == nil {
			writeErr(w, http.StatusNotImplemented, "kg dedup store unavailable")
			return
		}
		tenant := requestTenant(r)
		added, err := x.ScanKGDedupCandidates(tenant)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		pending := x.ListKGDedupCandidates(tenant, "pending")
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "added": added, "pending": len(pending),
		})
	}
}

func handleKGDedupList(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.KGDedup(st)
		if x == nil {
			writeJSON(w, http.StatusOK, map[string]any{"candidates": []any{}, "total": 0})
			return
		}
		status := strings.TrimSpace(r.URL.Query().Get("status"))
		if status == "" {
			status = "pending"
		}
		rows := x.ListKGDedupCandidates(requestTenant(r), status)
		writeJSON(w, http.StatusOK, map[string]any{
			"candidates": rows, "total": len(rows), "status": status,
		})
	}
}

func handleKGMerge(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.KGDedup(st)
		if x == nil {
			writeErr(w, http.StatusNotImplemented, "kg dedup store unavailable")
			return
		}
		var body struct {
			KeepID  string `json:"keep_id"`
			DropID  string `json:"drop_id"`
			KeepID2 string `json:"keep"`
			DropID2 string `json:"drop"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		keep := firstNonEmpty(body.KeepID, body.KeepID2)
		drop := firstNonEmpty(body.DropID, body.DropID2)
		if keep == "" || drop == "" {
			writeErr(w, http.StatusBadRequest, "keep_id and drop_id are required")
			return
		}
		res, err := x.MergeKGEntities(requestTenant(r), keep, drop)
		if err != nil {
			writeErr(w, http.StatusNotFound, "merge failed: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": res})
	}
}
