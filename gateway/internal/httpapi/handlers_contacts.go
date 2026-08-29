// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/auditlog"
	"github.com/mqglobal/goso/gateway/internal/channel"
	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/store"
)

type contactView struct {
	channel.PublicContact
	Agent string `json:"agent,omitempty"`
}

func registerContactsRoutes(mux *http.ServeMux, opt Options) {
	dir := opt.Contacts
	if dir == nil {
		dir = channel.DefaultContacts()
	}
	aliasAPI(mux, "GET /api/contacts", handleListContacts(dir, opt.Store))
	aliasAPI(mux, "GET /api/contacts/{id}", handleGetContact(dir, opt.Store))
	aliasAPI(mux, "POST /api/contacts/{id}/merge", handleMergeContact(dir, opt.Store, opt.Events, opt.Audit))
	aliasAPI(mux, "POST /api/contacts/{id}/undo", handleUndoContact(dir, opt.Store, opt.Events, opt.Audit))
}

func contactsForbidden(w http.ResponseWriter) bool {
	if store.LiteEnabled() {
		writeErr(w, http.StatusForbidden, "lite: channels off")
		return true
	}
	return false
}

func handleListContacts(dir *channel.Contacts, st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if contactsForbidden(w) {
			return
		}
		q := strings.TrimSpace(r.URL.Query().Get("q"))
		ch := strings.TrimSpace(r.URL.Query().Get("channel"))
		kind := strings.TrimSpace(r.URL.Query().Get("kind"))
		if kind == "" {
			kind = strings.TrimSpace(r.URL.Query().Get("type"))
		}
		rows := dir.List(requestTenant(r), q, ch, kind)
		if rows == nil {
			rows = []channel.PublicContact{}
		}
		out := make([]contactView, 0, len(rows))
		for _, row := range rows {
			out = append(out, decorateContact(st, row))
		}
		writeJSON(w, http.StatusOK, map[string]any{"contacts": out, "total": len(out)})
	}
}

func handleGetContact(dir *channel.Contacts, st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if contactsForbidden(w) {
			return
		}
		id := strings.TrimSpace(r.PathValue("id"))
		row, err := dir.Get(id, requestTenant(r))
		if writeContactErr(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, decorateContact(st, row))
	}
}

func handleMergeContact(dir *channel.Contacts, st store.StoreIface, ev *eventstore.Store, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if contactsForbidden(w) {
			return
		}
		id := strings.TrimSpace(r.PathValue("id"))
		body, ok := readContactAction(w, r)
		if !ok {
			return
		}
		row, err := dir.Merge(id, body.SourceID, requestTenant(r), body.Confirm)
		if writeContactErr(w, err) {
			return
		}
		auditContact(ev, "merge", row.ID+" "+strings.TrimSpace(body.SourceID), true)
		recordAudit(al, r, auditlog.Record{
			Action: "merge", Entity: "contact", EntityID: row.ID,
			After: auditMeta(true, map[string]any{"source_id": strings.TrimSpace(body.SourceID)}),
		})
		writeJSON(w, http.StatusOK, decorateContact(st, row))
	}
}

func handleUndoContact(dir *channel.Contacts, st store.StoreIface, ev *eventstore.Store, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if contactsForbidden(w) {
			return
		}
		id := strings.TrimSpace(r.PathValue("id"))
		body, ok := readContactAction(w, r)
		if !ok {
			return
		}
		row, err := dir.Undo(id, requestTenant(r), body.Confirm)
		if writeContactErr(w, err) {
			return
		}
		auditContact(ev, "undo", row.ID, true)
		recordAudit(al, r, auditlog.Record{
			Action: "undo", Entity: "contact", EntityID: row.ID,
			After: auditMeta(true, nil),
		})
		writeJSON(w, http.StatusOK, decorateContact(st, row))
	}
}

func readContactAction(w http.ResponseWriter, r *http.Request) (struct {
	SourceID string
	Confirm  string
}, bool) {
	var body struct {
		SourceID string `json:"source_id"`
		Confirm  string `json:"confirm"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 4096))
	if err := dec.Decode(&body); err != nil && err != io.EOF {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return struct {
			SourceID string
			Confirm  string
		}{}, false
	}
	return struct {
		SourceID string
		Confirm  string
	}{SourceID: strings.TrimSpace(body.SourceID), Confirm: strings.TrimSpace(body.Confirm)}, true
}

func writeContactErr(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, channel.ErrContactNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return true
	}
	if errors.Is(err, channel.ErrContactConfirmRequired) {
		writeErr(w, http.StatusBadRequest, "confirm is required")
		return true
	}
	if errors.Is(err, channel.ErrContactConfirm) {
		writeErr(w, http.StatusBadRequest, "confirm does not match")
		return true
	}
	if errors.Is(err, channel.ErrContactSelfMerge) {
		writeErr(w, http.StatusBadRequest, "cannot merge a contact into itself")
		return true
	}
	if errors.Is(err, channel.ErrContactMerged) {
		writeErr(w, http.StatusConflict, "contact already merged")
		return true
	}
	if errors.Is(err, channel.ErrContactNoUndo) {
		writeErr(w, http.StatusBadRequest, "nothing to undo")
		return true
	}
	writeErr(w, http.StatusBadRequest, err.Error())
	return true
}

func decorateContact(st store.StoreIface, row channel.PublicContact) contactView {
	v := contactView{PublicContact: row}
	if strings.TrimSpace(row.AgentID) == "" || st == nil {
		return v
	}
	a, err := st.GetAgent(row.AgentID)
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

func auditContact(ev *eventstore.Store, tool, id string, ok bool) {
	if ev == nil {
		return
	}
	kind := eventstore.KindSuccess
	if !ok {
		kind = eventstore.KindError
	}
	ev.Append(eventstore.Event{
		Connector: "contacts",
		Tool:      tool,
		Kind:      kind,
		Summary:   tool + " contact " + strings.TrimSpace(id),
	})
}
