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
	"github.com/mqglobal/goso/gateway/internal/workstation"
)

func registerWorkstationRoutes(mux *http.ServeMux, opt Options) {
	reg := opt.Workstations
	if reg == nil {
		reg = workstation.Default()
	}
	aliasAPI(mux, "GET /api/workstations", handleListWorkstations(reg))
	aliasAPI(mux, "POST /api/workstations", handleCreateWorkstation(reg, opt.Events, opt.Audit))
	aliasAPI(mux, "GET /api/workstations/{id}", handleGetWorkstation(reg))
	aliasAPI(mux, "PATCH /api/workstations/{id}", handlePatchWorkstation(reg, opt.Events, opt.Audit))
	aliasAPI(mux, "POST /api/workstations/{id}/test", handleTestWorkstation(reg, opt.Events))
	aliasAPI(mux, "POST /api/workstations/{id}/disconnect", handleWorkstationConfirm(reg, opt.Events, opt.Audit, "disconnect"))
	aliasAPI(mux, "POST /api/workstations/{id}/delete", handleWorkstationConfirm(reg, opt.Events, opt.Audit, "delete"))
}

type workstationBody struct {
	Display     string `json:"display"`
	Backend     string `json:"backend"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	User        string `json:"user"`
	IdentityRef string `json:"identity_ref"`
	AgentID     string `json:"agent_id"`
}

func handleListWorkstations(reg *workstation.Workstations) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows := reg.List(requestTenant(r))
		if rows == nil {
			rows = []workstation.Public{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"workstations": rows})
	}
}

func handleGetWorkstation(reg *workstation.Workstations) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		row, err := reg.Get(strings.TrimSpace(r.PathValue("id")), requestTenant(r))
		if writeWorkstationErr(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, row)
	}
}

func handleCreateWorkstation(reg *workstation.Workstations, ev *eventstore.Store, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _, ok := readWorkstationBody(w, r)
		if !ok {
			return
		}
		row, err := reg.Create(workstation.Input{
			Display:     body.Display,
			Backend:     body.Backend,
			Host:        body.Host,
			Port:        body.Port,
			User:        body.User,
			IdentityRef: body.IdentityRef,
			AgentID:     body.AgentID,
			TenantID:    requestTenant(r),
		})
		if writeWorkstationErr(w, err) {
			return
		}
		auditWorkstation(ev, "create", row.ID, true)
		recordAudit(al, r, auditlog.Record{
			Action: "create", Entity: "workstation", EntityID: row.ID,
			After: auditMeta(true, map[string]any{"backend": row.Backend, "host": row.Host}),
		})
		writeJSON(w, http.StatusCreated, row)
	}
}

func handlePatchWorkstation(reg *workstation.Workstations, ev *eventstore.Store, al *auditlog.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, raw, ok := readWorkstationBody(w, r)
		if !ok {
			return
		}
		_, identitySet := raw["identity_ref"]
		_, agentSet := raw["agent_id"]
		row, err := reg.Update(strings.TrimSpace(r.PathValue("id")), requestTenant(r), workstation.Input{
			Display:     body.Display,
			Backend:     body.Backend,
			Host:        body.Host,
			Port:        body.Port,
			User:        body.User,
			IdentityRef: body.IdentityRef,
			AgentID:     body.AgentID,
		}, identitySet, agentSet, time.Time{})
		if writeWorkstationErr(w, err) {
			return
		}
		auditWorkstation(ev, "update", row.ID, true)
		recordAudit(al, r, auditlog.Record{
			Action: "update", Entity: "workstation", EntityID: row.ID,
			After: auditMeta(true, map[string]any{"backend": row.Backend, "host": row.Host}),
		})
		writeJSON(w, http.StatusOK, row)
	}
}

func handleTestWorkstation(reg *workstation.Workstations, ev *eventstore.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tr, _, err := reg.Test(strings.TrimSpace(r.PathValue("id")), requestTenant(r), time.Time{})
		if writeWorkstationErr(w, err) {
			return
		}
		auditWorkstation(ev, "test", strings.TrimSpace(r.PathValue("id")), tr.OK)
		writeJSON(w, http.StatusOK, tr)
	}
}

func handleWorkstationConfirm(reg *workstation.Workstations, ev *eventstore.Store, al *auditlog.Store, action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.PathValue("id"))
		confirm, ok := readWorkstationConfirm(w, r)
		if !ok {
			return
		}
		tid := requestTenant(r)
		var (
			row workstation.Public
			err error
		)
		switch action {
		case "disconnect":
			row, err = reg.Disconnect(id, tid, confirm, time.Time{})
		case "delete":
			row, err = reg.Delete(id, tid, confirm)
		default:
			writeErr(w, http.StatusBadRequest, "unknown action")
			return
		}
		if writeWorkstationErr(w, err) {
			return
		}
		auditWorkstation(ev, action, row.ID, true)
		recordAudit(al, r, auditlog.Record{
			Action: action, Entity: "workstation", EntityID: row.ID,
			After: auditMeta(true, map[string]any{"health": row.Health}),
		})
		writeJSON(w, http.StatusOK, row)
	}
}

func readWorkstationConfirm(w http.ResponseWriter, r *http.Request) (string, bool) {
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

func readWorkstationBody(w http.ResponseWriter, r *http.Request) (workstationBody, map[string]json.RawMessage, bool) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 8192))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return workstationBody{}, nil, false
	}
	probe := map[string]json.RawMessage{}
	if len(strings.TrimSpace(string(raw))) > 0 {
		if err := json.Unmarshal(raw, &probe); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return workstationBody{}, nil, false
		}
	}
	for k := range probe {
		if workstationForbiddenField(k) {
			writeErr(w, http.StatusBadRequest, "identity is a path/ref, never a private key")
			return workstationBody{}, nil, false
		}
	}
	var body workstationBody
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return workstationBody{}, nil, false
		}
	}
	return body, probe, true
}

func workstationForbiddenField(k string) bool {
	switch strings.ToLower(strings.TrimSpace(k)) {
	case "private_key", "password", "secret", "token", "key", "pem", "ssh_key", "identity", "identity_file", "hmac", "bot_token", "content", "api_key":
		return true
	default:
		return false
	}
}

func writeWorkstationErr(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, workstation.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not found")
		return true
	}
	if errors.Is(err, workstation.ErrConfirmRequired) {
		writeErr(w, http.StatusBadRequest, "confirm is required")
		return true
	}
	if errors.Is(err, workstation.ErrConfirm) {
		writeErr(w, http.StatusBadRequest, "confirm does not match")
		return true
	}
	if errors.Is(err, workstation.ErrDisplayRequired) {
		writeErr(w, http.StatusBadRequest, "display is required")
		return true
	}
	if errors.Is(err, workstation.ErrHostRequired) {
		writeErr(w, http.StatusBadRequest, "host is required")
		return true
	}
	if errors.Is(err, workstation.ErrBackend) {
		writeErr(w, http.StatusBadRequest, "backend must be ssh or docker")
		return true
	}
	if errors.Is(err, workstation.ErrHost) {
		writeErr(w, http.StatusBadRequest, "host is invalid")
		return true
	}
	if errors.Is(err, workstation.ErrPort) {
		writeErr(w, http.StatusBadRequest, "port is invalid")
		return true
	}
	if errors.Is(err, workstation.ErrUserRequired) {
		writeErr(w, http.StatusBadRequest, "user is required for ssh")
		return true
	}
	if errors.Is(err, workstation.ErrUser) {
		writeErr(w, http.StatusBadRequest, "user is invalid")
		return true
	}
	if errors.Is(err, workstation.ErrIdentity) || errors.Is(err, workstation.ErrKeyMaterial) {
		writeErr(w, http.StatusBadRequest, "identity is a path/ref, never a private key")
		return true
	}
	if errors.Is(err, workstation.ErrAgent) {
		writeErr(w, http.StatusBadRequest, "agent id is invalid")
		return true
	}
	if errors.Is(err, workstation.ErrCap) {
		writeErr(w, http.StatusConflict, "too many workstations")
		return true
	}
	if errors.Is(err, workstation.ErrNotDisconnected) {
		writeErr(w, http.StatusConflict, "workstation already disconnected")
		return true
	}
	writeErr(w, http.StatusBadRequest, err.Error())
	return true
}

func auditWorkstation(ev *eventstore.Store, tool, id string, ok bool) {
	if ev == nil {
		return
	}
	kind := eventstore.KindSuccess
	if !ok {
		kind = eventstore.KindError
	}
	ev.Append(eventstore.Event{
		Connector: "workstations",
		Tool:      tool,
		Kind:      kind,
		Summary:   tool + " workstation " + strings.TrimSpace(id),
	})
}
