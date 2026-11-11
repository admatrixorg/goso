// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/contextfile"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func registerContextFileRoutes(mux *http.ServeMux, st store.StoreIface) {
	aliasAPI(mux, "PUT /api/context-files", handlePutContextFile(st))
	aliasAPI(mux, "GET /api/context-files", handleGetContextFiles(st))
	aliasAPI(mux, "DELETE /api/context-files", handleDeleteContextFile(st))
}

func contextFilesOff(w http.ResponseWriter) bool {
	if contextfile.Enabled() {
		return false
	}
	writeErr(w, http.StatusNotFound, "not found")
	return true
}

func writeContextErr(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrContextOwnerCap):
		writeErr(w, http.StatusBadRequest, "too_large")
	case errors.Is(err, store.ErrContextTooLarge):
		writeErr(w, http.StatusBadRequest, "context file too large")
	case errors.Is(err, store.ErrContextInvalid):
		writeErr(w, http.StatusBadRequest, "invalid context file")
	default:
		contextfile.LogFailure(op, err)
		writeErr(w, http.StatusInternalServerError, "store")
	}
}

type contextFileBody struct {
	Scope    string `json:"scope"`
	AgentID  string `json:"agent_id"`
	UserKey  string `json:"user_key"`
	Name     string `json:"name"`
	Body     string `json:"body"`
	TenantID string `json:"tenant_id"`
}

func contextOwnerFrom(scope, agentID, userKey string) (string, error) {
	scope = strings.ToLower(strings.TrimSpace(scope))
	agentID = strings.TrimSpace(agentID)
	userKey = strings.TrimSpace(userKey)
	switch scope {
	case store.ContextScopeAgent:
		if agentID == "" || userKey != "" {
			return "", store.ErrContextInvalid
		}
		return agentID, nil
	case store.ContextScopeUser:
		if userKey == "" || agentID != "" || !store.UserKeyOK(userKey) {
			return "", store.ErrContextInvalid
		}
		return userKey, nil
	default:
		return "", store.ErrContextInvalid
	}
}

func readContextJSON(w http.ResponseWriter, r *http.Request, dest any) bool {
	raw, err := io.ReadAll(io.LimitReader(r.Body, store.MaxContextFileBytes+8192+1))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return false
	}
	if len(raw) > store.MaxContextFileBytes+8192 {
		writeErr(w, http.StatusBadRequest, "context file too large")
		return false
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return false
	}
	return true
}

func handlePutContextFile(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if contextFilesOff(w) {
			return
		}
		var body contextFileBody
		if !readContextJSON(w, r, &body) {
			return
		}
		owner, err := contextOwnerFrom(body.Scope, body.AgentID, body.UserKey)
		if err != nil {
			writeContextErr(w, "put", err)
			return
		}
		saved, err := st.PutContextFile(store.ContextFile{
			TenantID: requestTenant(r),
			Scope:    body.Scope,
			OwnerID:  owner,
			Name:     body.Name,
			Body:     body.Body,
		})
		if err != nil {
			writeContextErr(w, "put", err)
			return
		}
		writeJSON(w, http.StatusOK, saved)
	}
}

func handleGetContextFiles(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if contextFilesOff(w) {
			return
		}
		q := r.URL.Query()
		owner, err := contextOwnerFrom(q.Get("scope"), q.Get("agent_id"), q.Get("user_key"))
		if err != nil {
			writeContextErr(w, "get", err)
			return
		}
		scope := strings.ToLower(strings.TrimSpace(q.Get("scope")))
		name := strings.TrimSpace(q.Get("name"))
		tid := requestTenant(r)
		if name != "" {
			got, err := st.GetContextFile(tid, scope, owner, name)
			if err != nil {
				writeContextErr(w, "get", err)
				return
			}
			writeJSON(w, http.StatusOK, got)
			return
		}
		list, err := st.ListContextFiles(tid, scope, owner)
		if err != nil {
			writeContextErr(w, "list", err)
			return
		}
		if list == nil {
			list = []*store.ContextFile{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"files": list})
	}
}

func handleDeleteContextFile(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if contextFilesOff(w) {
			return
		}
		q := r.URL.Query()
		owner, err := contextOwnerFrom(q.Get("scope"), q.Get("agent_id"), q.Get("user_key"))
		if err != nil {
			writeContextErr(w, "delete", err)
			return
		}
		name := strings.TrimSpace(q.Get("name"))
		if name == "" {
			writeContextErr(w, "delete", store.ErrContextInvalid)
			return
		}
		if err := st.DeleteContextFile(requestTenant(r), strings.ToLower(strings.TrimSpace(q.Get("scope"))), owner, name); err != nil {
			writeContextErr(w, "delete", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}
}

// chatContext attaches the turn's user key only when context files are enabled.
// The header and body are ignored while the flag is off.
func chatContext(r *http.Request, body chatBody) (context.Context, error) {
	if r == nil {
		return context.Background(), nil
	}
	ctx := r.Context()
	if !contextfile.Enabled() {
		return ctx, nil
	}
	key, err := contextfile.ResolveUserKey(r.Header.Get(contextfile.UserHeader), body.UserKey)
	if err != nil {
		return ctx, err
	}
	return contextfile.WithUser(ctx, key), nil
}
