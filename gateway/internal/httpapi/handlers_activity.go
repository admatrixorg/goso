// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/auditlog"
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

func handleListActivity(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		al := opt.Audit
		if al == nil {
			writeJSON(w, http.StatusOK, auditlog.Page{Records: []auditlog.Record{}})
			return
		}
		page := al.Query(activityQuery(r))
		if page.Records == nil {
			page.Records = []auditlog.Record{}
		}
		writeJSON(w, http.StatusOK, page)
	}
}

func recordAudit(al *auditlog.Store, r *http.Request, rec auditlog.Record) {
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
