// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/auditlog"
	"github.com/mqglobal/goso/gateway/internal/channel"
	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/store"
)

type pendingView struct {
	channel.PublicGroup
	Agent string `json:"agent"`
}

func registerPendingRoutes(mux *http.ServeMux, opt Options) {
	buf := opt.Pending
	if buf == nil {
		buf = channel.DefaultPending()
	}
	aliasAPI(mux, "GET /api/pending-messages", handleListPending(buf, opt.Store))
	aliasAPI(mux, "POST /api/pending-messages/{id}/compact", handleCompactPending(buf, opt.Store, opt.Events, opt.Audit))
	aliasAPI(mux, "POST /api/pending-messages/{id}/clear", handleClearPending(buf, opt.Events, opt.Audit))
}

func pendingForbidden(w http.ResponseWriter) bool {
	if store.LiteEnabled() {
		writeErr(w, http.StatusForbidden, "lite: channels off")
		return true
	}
	return false
}

func handleListPending(buf *channel.Pending, st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if pendingForbidden(w) {
			return
		}
		now := time.Now().UTC()
		groups := buf.List(requestTenant(r), now)
		if groups == nil {
			groups = []channel.PublicGroup{}
		}
		out := make([]pendingView, 0, len(groups))
		for _, g := range groups {
			out = append(out, decoratePending(st, g))
		}
		writeJSON(w, http.StatusOK, map[string]any{"groups": out})
	}
}

func handleCompactPending(buf *channel.Pending, st store.StoreIface, ev *eventstore.Store, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if pendingForbidden(w) {
			return
		}
		id := strings.TrimSpace(r.PathValue("id"))
		confirm, ok := readPendingConfirm(w, r)
		if !ok {
			return
		}
		g, err := buf.Compact(id, requestTenant(r), confirm)
		if writePendingErr(w, err) {
			return
		}
		auditPending(ev, "compact", g.ID, true)
		recordAudit(al, r, auditlog.Record{
			Action: "compact", Entity: "pending", EntityID: g.ID,
			After: auditMeta(true, map[string]any{"count": g.Count}),
		})
		writeJSON(w, http.StatusOK, decoratePending(st, g))
	}
}

func handleClearPending(buf *channel.Pending, ev *eventstore.Store, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if pendingForbidden(w) {
			return
		}
		id := strings.TrimSpace(r.PathValue("id"))
		confirm, ok := readPendingConfirm(w, r)
		if !ok {
			return
		}
		tid := requestTenant(r)
		before, gerr := buf.Get(id, tid, time.Now().UTC())
		if writePendingErr(w, gerr) {
			return
		}
		if err := buf.Clear(id, tid, confirm); writePendingErr(w, err) {
			return
		}
		auditPending(ev, "clear", before.ID, true)
		recordAudit(al, r, auditlog.Record{
			Action: "clear", Entity: "pending", EntityID: before.ID,
			After: auditMeta(true, nil),
		})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": before.ID})
	}
}

func readPendingConfirm(w http.ResponseWriter, r *http.Request) (string, bool) {
	var body struct {
		Confirm string `json:"confirm"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 4096))
	if err := dec.Decode(&body); err != nil && err != io.EOF {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return "", false
	}
	return strings.TrimSpace(body.Confirm), true
}

func writePendingErr(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, channel.ErrPendingNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return true
	}
	if errors.Is(err, channel.ErrPendingBusy) {
		writeErr(w, http.StatusConflict, "compact in progress")
		return true
	}
	if errors.Is(err, channel.ErrPendingConfirmRequired) {
		writeErr(w, http.StatusBadRequest, "confirm is required")
		return true
	}
	if errors.Is(err, channel.ErrPendingConfirm) {
		writeErr(w, http.StatusBadRequest, "confirm does not match")
		return true
	}
	writeErr(w, http.StatusBadRequest, err.Error())
	return true
}

func decoratePending(st store.StoreIface, g channel.PublicGroup) pendingView {
	v := pendingView{PublicGroup: g, Agent: ""}
	if strings.TrimSpace(g.AgentID) == "" || st == nil {
		return v
	}
	a, err := st.GetAgent(g.AgentID)
	if err != nil || a == nil {
		return v
	}
	name := strings.TrimSpace(a.DisplayName)
	if name == "" {
		name = strings.TrimSpace(a.AgentKey)
	}
	v.Agent = name
	return v
}

func auditPending(ev *eventstore.Store, tool, id string, ok bool) {
	if ev == nil {
		return
	}
	kind := eventstore.KindSuccess
	if !ok {
		kind = eventstore.KindError
	}
	ev.Append(eventstore.Event{
		Connector: "pending-messages",
		Tool:      tool,
		Kind:      kind,
		Summary:   tool + " pending group " + strings.TrimSpace(id),
	})
}
