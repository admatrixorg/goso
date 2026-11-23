package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/store"
)


// usageSummaryStore returns the usage capability for the router store. The
// compat summary handler runs inside the same mux, so one package pointer
// carries the store it needs.
var usageStoreForSummary store.StoreIface

func usageSummaryStore(_ *http.Request) (store.UsageStore, bool) {
	if usageStoreForSummary == nil {
		return nil, false
	}
	x := store.Usage(usageStoreForSummary)
	return x, x != nil
}

func usageRange(r *http.Request) (string, string) {
	from := strings.TrimSpace(r.URL.Query().Get("from"))
	to := strings.TrimSpace(r.URL.Query().Get("to"))
	if from == "" {
		from = time.Now().UTC().AddDate(0, 0, -30).Format(time.RFC3339)
	}
	if to == "" {
		to = time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)
	}
	return from, to
}

func usagePaging(r *http.Request) (int, int) {
	limit, offset := 100, 0
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("offset")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}

// handleUsageEventList lists metered rows, newest first.
func handleUsageEventList(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.Usage(st)
		from, to := usageRange(r)
		limit, offset := usagePaging(r)
		if x == nil {
			writeJSON(w, http.StatusOK, map[string]any{"events": []any{}, "total": 0})
			return
		}
		rows, total := x.ListUsageEvents(requestTenant(r),
			strings.TrimSpace(r.URL.Query().Get("agent_id")),
			strings.TrimSpace(r.URL.Query().Get("provider")),
			strings.TrimSpace(r.URL.Query().Get("model")),
			strings.TrimSpace(r.URL.Query().Get("channel")),
			from, to, limit, offset)
		writeJSON(w, http.StatusOK, map[string]any{"events": rows, "total": total, "limit": limit, "offset": offset})
	}
}

// handleUsageEventSummary answers the event analytics cards, which count calls
// and errors per tool, skill, MCP tool or runtime tool.
func handleUsageEventSummary(st store.StoreIface, meter *billing.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		empty := map[string]any{
			"calls": 0, "errors": 0, "input_tokens": 0, "output_tokens": 0,
			"total_tokens": 0, "cost_usd": 0, "avg_duration_ms": 0,
		}
		x := store.Usage(st)
		from, to := usageRange(r)
		if x == nil {
			writeJSON(w, http.StatusOK, map[string]any{"summary": empty})
			return
		}
		m := x.UsageTotals(requestTenant(r), from, to)
		writeJSON(w, http.StatusOK, map[string]any{"summary": map[string]any{
			"calls": m["requests"], "errors": m["errors"],
			"input_tokens": m["input_tokens"], "output_tokens": m["output_tokens"],
			"total_tokens": m["total_tokens"], "cost_usd": m["cost"],
			"avg_duration_ms": m["avg_duration_ms"],
		}})
	}
}

// handleUsageEventBreakdown answers the event analytics rows.
func handleUsageEventBreakdown(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.Usage(st)
		from, to := usageRange(r)
		if x == nil {
			writeJSON(w, http.StatusOK, map[string]any{"rows": []any{}})
			return
		}
		groupBy := strings.TrimSpace(r.URL.Query().Get("group_by"))
		if groupBy == "" {
			groupBy = "provider"
		}
		raw := x.UsageBreakdown(requestTenant(r), groupBy, from, to)
		rows := make([]map[string]any, 0, len(raw))
		for _, row := range raw {
			rows = append(rows, map[string]any{
				"key":          row["key"],
				"event_type":   "llm_call",
				"resource_type": groupBy,
				"resource_name": row["key"],
				"source":       row["key"],
				"calls":        row["request_count"],
				"errors":       row["error_count"],
				"input_tokens": row["input_tokens"],
				"output_tokens": row["output_tokens"],
				"total_tokens": toFloat(row["input_tokens"]) + toFloat(row["output_tokens"]),
				"cost_usd":     row["total_cost"],
				"avg_duration_ms": row["avg_duration_ms"],
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"rows": rows, "group_by": groupBy})
	}
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	}
	return 0
}

func handleUsageTimeseriesDB(st store.StoreIface, meter *billing.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.Usage(st)
		if x == nil {
			writeJSON(w, http.StatusOK, map[string]any{"points": []any{}})
			return
		}
		from, to := usageRange(r)
		points := x.UsageSeries(requestTenant(r), r.URL.Query().Get("group_by"), from, to)
		writeJSON(w, http.StatusOK, map[string]any{"points": points})
	}
}

func handleUsageBreakdownDB(st store.StoreIface, meter *billing.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.Usage(st)
		if x == nil {
			writeJSON(w, http.StatusOK, map[string]any{"rows": []any{}})
			return
		}
		from, to := usageRange(r)
		groupBy := r.URL.Query().Get("group_by")
		rows := x.UsageBreakdown(requestTenant(r), groupBy, from, to)
		writeJSON(w, http.StatusOK, map[string]any{"rows": rows, "group_by": groupBy})
	}
}

func handleUsageCapPolicies(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.Usage(st)
		if x == nil {
			writeJSON(w, http.StatusOK, map[string]any{"policies": []any{}})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"policies": x.ListUsageCapPolicies(requestTenant(r))})
	}
}

func handleUsageCapPolicyCreate(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.Usage(st)
		if x == nil {
			writeErr(w, http.StatusNotImplemented, "usage store unavailable")
			return
		}
		var body store.UsageCapPolicy
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		body.ID = ""
		body.TenantID = requestTenant(r)
		if body.Window == "" {
			body.Window = strings.TrimSpace(r.URL.Query().Get("window"))
		}
		saved, err := x.PutUsageCapPolicy(body)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, saved)
	}
}

func handleUsageCapPolicyPatch(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.Usage(st)
		if x == nil {
			writeErr(w, http.StatusNotImplemented, "usage store unavailable")
			return
		}
		id := strings.TrimSpace(r.PathValue("id"))
		var body store.UsageCapPolicy
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		body.ID = id
		body.TenantID = requestTenant(r)
		saved, err := x.PutUsageCapPolicy(body)
		if err != nil {
			writeErr(w, http.StatusNotFound, "policy not found")
			return
		}
		writeJSON(w, http.StatusOK, saved)
	}
}

func handleUsageCapPolicyDelete(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.Usage(st)
		if x == nil {
			writeErr(w, http.StatusNotImplemented, "usage store unavailable")
			return
		}
		if err := x.DeleteUsageCapPolicy(requestTenant(r), r.PathValue("id")); err != nil {
			writeErr(w, http.StatusNotFound, "policy not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func handleUsageCapEvents(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"events": []any{}, "total": 0})
	}
}

func handleUsageCapUtilization(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		x := store.Usage(st)
		if x == nil {
			writeJSON(w, http.StatusOK, map[string]any{"rows": []any{}})
			return
		}
		type utilizer interface {
			UsageCapUtilization(string, time.Time) []map[string]any
		}
		u, ok := x.(utilizer)
		if !ok {
			writeJSON(w, http.StatusOK, map[string]any{"rows": []any{}})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"rows": u.UsageCapUtilization(requestTenant(r), time.Now().UTC())})
	}
}
