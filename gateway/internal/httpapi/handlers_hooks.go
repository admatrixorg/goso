// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/auditlog"
	"github.com/mqglobal/goso/gateway/internal/hooks"
	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// registerHookRoutes serves tenant-configured lifecycle hooks:
// GET/POST /api/hooks, GET/PATCH/DELETE /api/hooks/{id} (and /v1/hooks).
// Every call is scoped to the request tenant; a hook of another tenant
// answers 404. The signing secret is write-only: GET shows has_secret.
func registerHookRoutes(mux *http.ServeMux, opt Options) {
	st := opt.Store
	al := opt.Audit
	aliasAPI(mux, "GET /api/hooks", func(w http.ResponseWriter, r *http.Request) {
		hs := store.Hooks(st)
		if hs == nil {
			writeErr(w, http.StatusNotImplemented, "hooks need a store")
			return
		}
		list, err := hs.ListHooks(requestTenant(r))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "list hooks failed")
			return
		}
		out := make([]map[string]any, 0, len(list))
		for _, h := range list {
			out = append(out, publicHook(h))
		}
		writeJSON(w, http.StatusOK, map[string]any{"hooks": out})
	})
	aliasAPI(mux, "GET /api/hooks/{id}", func(w http.ResponseWriter, r *http.Request) {
		hs := store.Hooks(st)
		if hs == nil {
			writeErr(w, http.StatusNotImplemented, "hooks need a store")
			return
		}
		h, err := hs.GetHook(requestTenant(r), r.PathValue("id"))
		if err != nil {
			writeErr(w, http.StatusNotFound, "hook not found")
			return
		}
		writeJSON(w, http.StatusOK, publicHook(h))
	})
	aliasAPI(mux, "POST /api/hooks", func(w http.ResponseWriter, r *http.Request) {
		hs := store.Hooks(st)
		if hs == nil {
			writeErr(w, http.StatusNotImplemented, "hooks need a store")
			return
		}
		var body hookBody
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		tid := requestTenant(r)
		h := store.Hook{TenantID: tid, Enabled: true}
		if status, msg := body.apply(st, tid, &h); status != 0 {
			writeErr(w, status, msg)
			return
		}
		created, err := hs.CreateHook(h)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		hooks.Invalidate()
		recordAudit(al, r, auditlog.Record{Action: "create", Entity: "hook", EntityID: created.ID, After: auditMeta(true, auditHook(created))})
		writeJSON(w, http.StatusCreated, publicHook(created))
	})
	aliasAPI(mux, "PATCH /api/hooks/{id}", func(w http.ResponseWriter, r *http.Request) {
		hs := store.Hooks(st)
		if hs == nil {
			writeErr(w, http.StatusNotImplemented, "hooks need a store")
			return
		}
		tid := requestTenant(r)
		cur, err := hs.GetHook(tid, r.PathValue("id"))
		if err != nil {
			writeErr(w, http.StatusNotFound, "hook not found")
			return
		}
		var body hookBody
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		before := auditHook(cur)
		next := *cur
		if status, msg := body.apply(st, tid, &next); status != 0 {
			writeErr(w, status, msg)
			return
		}
		updated, err := hs.UpdateHook(next)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "hook not found")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		hooks.Invalidate()
		recordAudit(al, r, auditlog.Record{Action: "update", Entity: "hook", EntityID: updated.ID, Before: before, After: auditMeta(true, auditHook(updated))})
		writeJSON(w, http.StatusOK, publicHook(updated))
	})
	aliasAPI(mux, "DELETE /api/hooks/{id}", func(w http.ResponseWriter, r *http.Request) {
		hs := store.Hooks(st)
		if hs == nil {
			writeErr(w, http.StatusNotImplemented, "hooks need a store")
			return
		}
		tid := requestTenant(r)
		cur, err := hs.GetHook(tid, r.PathValue("id"))
		if err != nil {
			writeErr(w, http.StatusNotFound, "hook not found")
			return
		}
		if err := hs.DeleteHook(tid, cur.ID); err != nil {
			writeErr(w, http.StatusNotFound, "hook not found")
			return
		}
		hooks.Invalidate()
		recordAudit(al, r, auditlog.Record{Action: "delete", Entity: "hook", EntityID: cur.ID, Before: auditHook(cur), After: auditMeta(true, nil)})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
}

// hookBody is the create / patch payload. Absent fields keep their value
// on PATCH. secret sets the HMAC signing secret; "" removes it.
type hookBody struct {
	AgentID     *string         `json:"agent_id"`
	Event       *string         `json:"event"`
	Matcher     *string         `json:"matcher"`
	HandlerType *string         `json:"handler_type"`
	Config      json.RawMessage `json:"config"`
	Secret      *string         `json:"secret"`
	Enabled     *bool           `json:"enabled"`
}

func (b hookBody) apply(st store.StoreIface, tid string, h *store.Hook) (int, string) {
	if b.AgentID != nil {
		id := strings.TrimSpace(*b.AgentID)
		if id != "" {
			if _, err := agentVisible(st, id, tid); err != nil {
				return http.StatusBadRequest, "agent not found"
			}
		}
		h.AgentID = id
	}
	if b.Event != nil {
		h.Event = *b.Event
	}
	if b.Matcher != nil {
		h.Matcher = *b.Matcher
	}
	if b.HandlerType != nil {
		h.HandlerType = *b.HandlerType
	}
	if len(b.Config) > 0 && string(b.Config) != "null" {
		var obj map[string]any
		if err := json.Unmarshal(b.Config, &obj); err != nil {
			return http.StatusBadRequest, "config must be a JSON object"
		}
		h.Config = string(b.Config)
	}
	if b.Enabled != nil {
		h.Enabled = *b.Enabled
	}
	if err := hooks.Validate(h); err != nil {
		return http.StatusBadRequest, err.Error()
	}
	if b.Secret != nil {
		sec := strings.TrimSpace(*b.Secret)
		if sec == "" {
			h.SecretEnc = ""
		} else {
			enc, err := hooks.SealSecret(sec)
			if errors.Is(err, secrets.ErrNoMasterKey) {
				return http.StatusBadRequest, "GOSO_MASTER_KEY is required to store a signing secret"
			}
			if err != nil {
				return http.StatusInternalServerError, "seal secret failed"
			}
			h.SecretEnc = enc
		}
	}
	if h.HandlerType != hooks.HandlerHTTP {
		h.SecretEnc = ""
	}
	return 0, ""
}

func publicHook(h *store.Hook) map[string]any {
	cfg := map[string]any{}
	_ = json.Unmarshal([]byte(h.Config), &cfg)
	return map[string]any{
		"id":           h.ID,
		"tenant_id":    h.TenantID,
		"agent_id":     h.AgentID,
		"event":        h.Event,
		"matcher":      h.Matcher,
		"handler_type": h.HandlerType,
		"config":       cfg,
		"has_secret":   h.SecretEnc != "",
		"enabled":      h.Enabled,
		"created_at":   h.CreatedAt,
		"updated_at":   h.UpdatedAt,
	}
}

// auditHook is the audit view of a hook: no secret, and only the host of
// an http URL (a path or query may carry a token).
func auditHook(h *store.Hook) map[string]any {
	out := map[string]any{
		"event":        h.Event,
		"matcher":      h.Matcher,
		"handler_type": h.HandlerType,
		"agent_id":     h.AgentID,
		"enabled":      h.Enabled,
		"has_secret":   h.SecretEnc != "",
	}
	if cfg, err := hooks.ParseConfig(h.Config); err == nil && cfg.URL != "" {
		if u, err := url.Parse(cfg.URL); err == nil {
			out["url_host"] = u.Host
		}
	}
	return out
}
