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
	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/node"
)

func registerNodeRoutes(mux *http.ServeMux, opt Options) {
	reg := opt.Nodes
	if reg == nil {
		reg = node.Default()
	}
	aliasAPI(mux, "GET /api/nodes", handleListNodes(reg))
	aliasAPI(mux, "POST /api/nodes/request", handleRequestNode(reg, opt.Events))
	aliasAPI(mux, "POST /api/nodes/{id}/approve", handleNodeAction(reg, opt.Events, opt.Audit, "approve"))
	aliasAPI(mux, "POST /api/nodes/{id}/deny", handleNodeAction(reg, opt.Events, opt.Audit, "deny"))
	aliasAPI(mux, "POST /api/nodes/{id}/revoke", handleNodeAction(reg, opt.Events, opt.Audit, "revoke"))
}

func handleListNodes(reg *node.Nodes) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().UTC()
		tid := requestTenant(r)
		pending := reg.ListPending(tid, now)
		paired := reg.ListPaired(tid, now)
		if pending == nil {
			pending = []node.Public{}
		}
		if paired == nil {
			paired = []node.Public{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"pending": pending, "paired": paired})
	}
}

func handleRequestNode(reg *node.Nodes, ev *eventstore.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Display string `json:"display"`
			Kind    string `json:"kind"`
		}
		dec := json.NewDecoder(io.LimitReader(r.Body, 4096))
		if err := dec.Decode(&body); err != nil && err != io.EOF {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		row, err := reg.RequestAccess(node.Request{
			Display:  strings.TrimSpace(body.Display),
			Kind:     strings.TrimSpace(body.Kind),
			TenantID: requestTenant(r),
		})
		if writeNodeErr(w, err) {
			return
		}
		auditNode(ev, "request", row.ID, true)
		writeJSON(w, http.StatusCreated, row)
	}
}

func handleNodeAction(reg *node.Nodes, ev *eventstore.Store, al *auditlog.Store, action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		confirm, ok := readNodeConfirm(w, r)
		if !ok {
			return
		}
		tid := requestTenant(r)
		now := time.Now().UTC()
		var (
			row node.Public
			err error
		)
		switch action {
		case "approve":
			row, err = reg.Approve(id, tid, confirm, now)
		case "deny":
			row, err = reg.Deny(id, tid, confirm, now)
		case "revoke":
			row, err = reg.Revoke(id, tid, confirm, now)
		default:
			writeErr(w, http.StatusBadRequest, "unknown action")
			return
		}
		if writeNodeErr(w, err) {
			return
		}
		auditNode(ev, action, row.ID, true)
		recordAudit(al, r, auditlog.Record{
			Action: action, Entity: "node", EntityID: row.ID,
			After: auditMeta(true, map[string]any{"status": row.Status}),
		})
		writeJSON(w, http.StatusOK, row)
	}
}

func readNodeConfirm(w http.ResponseWriter, r *http.Request) (string, bool) {
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

func writeNodeErr(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, node.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return true
	}
	if errors.Is(err, node.ErrConfirmRequired) {
		writeErr(w, http.StatusBadRequest, "confirm is required")
		return true
	}
	if errors.Is(err, node.ErrConfirm) {
		writeErr(w, http.StatusBadRequest, "confirm does not match")
		return true
	}
	if errors.Is(err, node.ErrDisplayRequired) {
		writeErr(w, http.StatusBadRequest, "display is required")
		return true
	}
	if errors.Is(err, node.ErrExpired) {
		writeErr(w, http.StatusConflict, "pairing request expired")
		return true
	}
	if errors.Is(err, node.ErrCap) {
		writeErr(w, http.StatusConflict, "too many pending pairing requests")
		return true
	}
	if errors.Is(err, node.ErrStatus) {
		writeErr(w, http.StatusConflict, "node not pending")
		return true
	}
	if errors.Is(err, node.ErrNotPaired) {
		writeErr(w, http.StatusConflict, "node not paired")
		return true
	}
	writeErr(w, http.StatusBadRequest, err.Error())
	return true
}

func auditNode(ev *eventstore.Store, tool, id string, ok bool) {
	if ev == nil {
		return
	}
	kind := eventstore.KindSuccess
	if !ok {
		kind = eventstore.KindError
	}
	ev.Append(eventstore.Event{
		Connector: "nodes",
		Tool:      tool,
		Kind:      kind,
		Summary:   tool + " node " + strings.TrimSpace(id),
	})
}
