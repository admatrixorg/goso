package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// registerRunTimelineRoutes exposes the run timeline over HTTP so the run detail
// page and a probe can read it without a websocket.
func registerRunTimelineRoutes(mux *http.ServeMux, st store.StoreIface) {
	mux.HandleFunc("GET /v1/runs/{runId}/timeline", handleRunTimelineHTTP(st))
}

func handleRunTimelineHTTP(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.Telemetry(st)
		runID := strings.TrimSpace(r.PathValue("runId"))
		if x == nil || runID == "" {
			writeJSON(w, http.StatusOK, map[string]any{"items": []any{}, "total": 0, "run_id": runID})
			return
		}
		limit := 100
		if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
				limit = n
			}
		}
		rows := x.ListRunTimeline(requestTenant(r), runID, limit)
		writeJSON(w, http.StatusOK, map[string]any{
			"items": rows, "total": len(rows), "run_id": runID, "limit": limit,
		})
	}
}
