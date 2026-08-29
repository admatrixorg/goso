// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/eventstore"
)

const eventsHeartbeat = 15 * time.Second

func registerEventRoutes(mux *http.ServeMux, opt Options) {
	aliasAPI(mux, "GET /api/events", handleListEvents(opt))
	aliasAPI(mux, "GET /api/events/stream", handleStreamEvents(opt))
}

func eventQuery(r *http.Request) eventstore.Query {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	after, _ := strconv.ParseInt(strings.TrimSpace(q.Get("after")), 10, 64)
	if after == 0 {
		after, _ = strconv.ParseInt(strings.TrimSpace(r.Header.Get("Last-Event-ID")), 10, 64)
	}
	return eventstore.Query{
		Kind:      strings.TrimSpace(q.Get("kind")),
		Connector: strings.TrimSpace(q.Get("connector")),
		Type:      strings.TrimSpace(q.Get("type")),
		Actor:     strings.TrimSpace(q.Get("actor")),
		Limit:     limit,
		AfterSeq:  after,
	}
}

func handleListEvents(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list := opt.Events.Query(eventQuery(r))
		writeJSON(w, http.StatusOK, map[string]any{"events": list})
	}
}

func handleStreamEvents(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := w.(http.Flusher); !ok {
			writeErr(w, http.StatusInternalServerError, "stream unsupported")
			return
		}
		q := eventQuery(r)
		live, cancel := opt.Events.Subscribe(32)
		defer cancel()
		sw := newSSEWriter(w)
		sw.start()
		sw.event("ready", `{"ok":true}`)
		last := q.AfterSeq
		if q.AfterSeq > 0 {
			replay := opt.Events.Query(eventstore.Query{
				Kind: q.Kind, Connector: q.Connector, Type: q.Type, Actor: q.Actor,
				AfterSeq: q.AfterSeq, Limit: eventstore.MaxFilterLimit,
			})
			for i := len(replay) - 1; i >= 0; i-- {
				writeOpsEvent(sw, replay[i])
				if replay[i].Seq > last {
					last = replay[i].Seq
				}
			}
		}
		tick := time.NewTicker(eventsHeartbeat)
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
				if e.Seq <= last || !eventstore.Match(e, q) {
					continue
				}
				pub := eventstore.Public(e)
				writeOpsEvent(sw, pub)
				if pub.Seq > last {
					last = pub.Seq
				}
			}
		}
	}
}

func writeOpsEvent(sw *sseWriter, e eventstore.Event) {
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	sw.idEvent(eventstore.SeqString(e.Seq), "ops", string(b))
}

func recordEvent(ev *eventstore.Store, e eventstore.Event) {
	if ev == nil {
		return
	}
	if strings.TrimSpace(e.Actor) == "" {
		e.Actor = "operator"
	}
	if strings.TrimSpace(e.Kind) == "" {
		e.Kind = eventstore.KindSuccess
	}
	ev.Append(e)
}

func operatorActor(_ *http.Request) string {
	return "operator"
}
