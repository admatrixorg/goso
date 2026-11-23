// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/auditlog"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func registerActivityRoutes(mux *http.ServeMux, opt Options) {
	aliasAPI(mux, "GET /api/activity", handleListActivity(opt))
}

func activityQuery(r *http.Request) auditlog.Query {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	before, _ := strconv.ParseInt(strings.TrimSpace(q.Get("before")), 10, 64)
	return auditlog.Query{
		Action:    strings.TrimSpace(q.Get("action")),
		Actor:     strings.TrimSpace(q.Get("actor")),
		Entity:    strings.TrimSpace(q.Get("entity")),
		IP:        strings.TrimSpace(q.Get("ip")),
		Since:     parseTimeParam(q.Get("since")),
		Until:     parseTimeParam(q.Get("until")),
		Limit:     limit,
		BeforeSeq: before,
	}
}

func parseTimeParam(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.UTC()
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC()
	}
	return time.Time{}
}

// handleListActivity answers both shapes. The pages read logs, total, limit and
// offset from the persisted rows; records keeps the older ring buffer contract
// alive for callers that still ask for before/next_before cursors.
func handleListActivity(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 50
		offset := 0
		if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
				limit = n
			}
		}
		if v := strings.TrimSpace(r.URL.Query().Get("offset")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				offset = n
			}
		}
		if x := store.Telemetry(opt.Store); x != nil {
			rows, total := x.ListActivity(requestTenant(r),
				strings.TrimSpace(r.URL.Query().Get("action")),
				strings.TrimSpace(r.URL.Query().Get("entity_type")),
				strings.TrimSpace(r.URL.Query().Get("actor_id")), limit, offset)
			logs := make([]map[string]any, 0, len(rows))
			records := make([]map[string]any, 0, len(rows))
			for _, a := range rows {
				logs = append(logs, map[string]any{
					"id": a.ID, "actor_type": a.ActorType, "actor_id": a.ActorID, "action": a.Action,
					"entity_type": a.EntityType, "entity_id": a.EntityID, "details": a.Details,
					"ip_address": a.IPAddress, "created_at": a.CreatedAt,
				})
				records = append(records, map[string]any{
					"id": a.ID, "action": a.Action, "actor": a.ActorID, "entity": a.EntityType,
					"entity_id": a.EntityID, "ip": a.IPAddress, "ts": a.CreatedAt,
				})
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"logs": logs, "records": records, "total": total, "limit": limit, "offset": offset,
			})
			return
		}
		al := opt.Audit
		if al == nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"logs": []any{}, "records": []any{}, "total": 0, "limit": limit, "offset": offset,
			})
			return
		}
		page := al.Query(activityQuery(r))
		if page.Records == nil {
			page.Records = []auditlog.Record{}
		}
		logs := make([]map[string]any, 0, len(page.Records))
		for _, rec := range page.Records {
			logs = append(logs, map[string]any{
				"id": rec.ID, "actor_type": "operator", "actor_id": rec.Actor, "action": rec.Action,
				"entity_type": rec.Entity, "entity_id": rec.EntityID, "details": "",
				"ip_address": rec.IP, "created_at": rec.TS.UTC().Format(time.RFC3339),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"records": page.Records, "logs": logs, "total": page.Total,
			"limit": page.Limit, "offset": offset, "next_before": page.NextBefore,
		})
	}
}

// activitySink persists audit rows outside the in-memory ring when set.
var activitySink func(auditlog.Record, *http.Request)

func recordAudit(al *auditlog.Store, r *http.Request, rec auditlog.Record) {
	if activitySink != nil {
		activitySink(rec, r)
	}
	if al == nil {
		return
	}
	if strings.TrimSpace(rec.Actor) == "" {
		rec.Actor = operatorActor(r)
	}
	if strings.TrimSpace(rec.IP) == "" {
		rec.IP = requestIP(r)
	}
	al.Append(rec)
}

func requestIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			xff = strings.TrimSpace(xff[:i])
		}
		if len(xff) > 64 {
			xff = xff[:64]
		}
		return xff
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func auditMeta(ok bool, extra map[string]any) map[string]any {
	out := map[string]any{"ok": ok}
	for k, v := range extra {
		out[k] = v
	}
	return out
}
