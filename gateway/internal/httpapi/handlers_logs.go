// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/logstore"
)

const logsHeartbeat = 15 * time.Second

func registerLogRoutes(mux *http.ServeMux, opt Options) {
	aliasAPI(mux, "GET /api/logs", handleListLogs(opt))
	aliasAPI(mux, "GET /api/logs/stream", handleStreamLogs(opt))
}

func logQuery(r *http.Request) logstore.Query {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	after, _ := strconv.ParseInt(strings.TrimSpace(q.Get("after")), 10, 64)
	if after == 0 {
		after, _ = strconv.ParseInt(strings.TrimSpace(r.Header.Get("Last-Event-ID")), 10, 64)
	}
	return logstore.Query{
		Component: strings.TrimSpace(q.Get("component")),
		Level:     strings.TrimSpace(q.Get("level")),
		Q:         strings.TrimSpace(q.Get("q")),
		Limit:     limit,
		AfterSeq:  after,
	}
}

func handleListLogs(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list := opt.Logs.Query(logQuery(r))
		writeJSON(w, http.StatusOK, map[string]any{
			"logs":       list,
			"components": opt.Logs.Components(),
		})
	}
}

func handleStreamLogs(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := w.(http.Flusher); !ok {
			writeErr(w, http.StatusInternalServerError, "stream unsupported")
			return
		}
		q := logQuery(r)
		live, cancel := opt.Logs.Subscribe(32)
		defer cancel()
		sw := newSSEWriter(w)
		sw.start()
		sw.event("ready", `{"ok":true}`)
		last := q.AfterSeq
		if q.AfterSeq > 0 {
			replay := opt.Logs.Query(logstore.Query{
				AfterSeq: q.AfterSeq, Limit: logstore.MaxFilterLimit,
			})
			for i := len(replay) - 1; i >= 0; i-- {
				writeLogEvent(sw, replay[i])
				if replay[i].Seq > last {
					last = replay[i].Seq
				}
			}
		}
		tick := time.NewTicker(logsHeartbeat)
		defer tick.Stop()
		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				sw.comment("ping")
			case e, ok := <-live:
				if !ok {
					return
				}
				if e.Seq <= last {
					continue
				}
				pub := logstore.Public(e)
				writeLogEvent(sw, pub)
				if pub.Seq > last {
					last = pub.Seq
				}
			}
		}
	}
}

func writeLogEvent(sw *sseWriter, e logstore.Entry) {
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	sw.idEvent(logstore.SeqString(e.Seq), "log", string(b))
}
