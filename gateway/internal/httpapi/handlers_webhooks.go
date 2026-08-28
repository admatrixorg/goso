// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/webhook"
)

func registerWebhookRoutes(mux *http.ServeMux, opt Options) {
	reg := opt.Webhooks
	if reg == nil {
		reg = webhook.NewWithStore(opt.Store)
	}
	st := opt.Store
	provider := opt.Provider
	reg.SetRunner(func(job webhook.Job) (string, error) {
		agentID := ""
		tid := store.DefaultTenant
		if p, err := reg.Get(job.WebhookID); err == nil && p != nil {
			agentID = p.AgentID
			tid = p.TenantID
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		reply, _, err := runWebhookChat(ctx, st, provider, "", job.Input, agentID, tid)
		return reply, err
	})
	reg.Start()

	aliasAPI(mux, "GET /api/webhooks", handleListWebhooks(reg))
	mux.HandleFunc("POST /api/webhooks", handleCreateWebhook(reg, st))
	aliasAPI(mux, "GET /api/webhooks/jobs/{id}", handleGetWebhookJob(reg))
	aliasAPI(mux, "GET /api/webhooks/{id}", handleGetWebhook(reg))
	mux.HandleFunc("POST /api/webhooks/{id}/rotate", handleRotateWebhook(reg))
	mux.HandleFunc("DELETE /api/webhooks/{id}", handleRevokeWebhook(reg))
	mux.HandleFunc("POST /api/webhooks/llm", handleWebhookLLM(reg, st, provider))
}

func handleListWebhooks(reg *webhook.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"webhooks": webhooksInTenant(reg.List(), requestTenant(r))})
	}
}

func handleGetWebhook(reg *webhook.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := reg.Get(r.PathValue("id"))
		if err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if hideWrongTenant(w, p.TenantID, requestTenant(r)) {
			return
		}
		writeJSON(w, http.StatusOK, p)
	}
}

func handleCreateWebhook(reg *webhook.Registry, st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var opts webhook.CreateOpts
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid body")
			return
		}
		if len(strings.TrimSpace(string(raw))) > 0 {
			var body struct {
				Name        string `json:"name"`
				Kind        string `json:"kind"`
				AgentID     string `json:"agent_id"`
				RequireHMAC bool   `json:"require_hmac"`
			}
			if err := json.Unmarshal(raw, &body); err != nil {
				writeErr(w, http.StatusBadRequest, "invalid json")
				return
			}
			opts = webhook.CreateOpts{Name: body.Name, Kind: body.Kind, AgentID: body.AgentID, RequireHMAC: body.RequireHMAC}
		}
		opts.TenantID = requestTenant(r)
		if aid := strings.TrimSpace(opts.AgentID); aid != "" {
			if _, err := agentVisible(st, aid, opts.TenantID); err != nil {
				writeErr(w, http.StatusBadRequest, "agent not found")
				return
			}
		}
		c, err := reg.CreateOpts(opts)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, c)
	}
}

func handleRotateWebhook(reg *webhook.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := reg.Get(r.PathValue("id"))
		if err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if hideWrongTenant(w, p.TenantID, requestTenant(r)) {
			return
		}
		c, err := reg.Rotate(r.PathValue("id"))
		if err != nil {
			if errors.Is(err, webhook.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "not found")
				return
			}
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, c)
	}
}

func handleRevokeWebhook(reg *webhook.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := reg.Get(r.PathValue("id"))
		if err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if hideWrongTenant(w, p.TenantID, requestTenant(r)) {
			return
		}
		if err := reg.Revoke(r.PathValue("id")); err != nil {
			if errors.Is(err, webhook.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "not found")
				return
			}
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func handleGetWebhookJob(reg *webhook.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		j, err := reg.GetJob(r.PathValue("id"))
		if err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		if j != nil && j.WebhookID != "" {
			p, gerr := reg.Get(j.WebhookID)
			if gerr != nil || p == nil || hideWrongTenant(w, p.TenantID, requestTenant(r)) {
				if gerr != nil || p == nil {
					writeErr(w, http.StatusNotFound, "not found")
				}
				return
			}
		}
		writeJSON(w, http.StatusOK, j)
	}
}

type webhookLLMBody struct {
	Input       string `json:"input"`
	Mode        string `json:"mode"`
	SessionID   string `json:"session_id"`
	CallbackURL string `json:"callback_url"`
}

func handleWebhookLLM(reg *webhook.Registry, st store.StoreIface, provider llm.Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid body")
			return
		}
		auth, err := reg.AuthenticateRecord(r.Header.Get("Authorization"), r.Header.Get("X-Goso-Signature"), raw)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		var body webhookLLMBody
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				writeErr(w, http.StatusBadRequest, "invalid json")
				return
			}
		}
		body.Input = strings.TrimSpace(body.Input)
		if body.Input == "" {
			writeErr(w, http.StatusBadRequest, "input is required")
			return
		}
		mode := strings.ToLower(strings.TrimSpace(body.Mode))
		if mode == "" {
			mode = "sync"
		}
		if mode != "sync" && mode != "async" {
			writeErr(w, http.StatusBadRequest, "mode must be sync or async")
			return
		}
		idem := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if len(idem) > 255 {
			writeErr(w, http.StatusBadRequest, "idempotency key too long")
			return
		}
		hash := webhook.BodyHash(raw)
		if idem != "" {
			existing, lerr := reg.LookupIdempotency(auth.ID, idem, hash)
			if errors.Is(lerr, webhook.ErrConflict) {
				writeErr(w, http.StatusConflict, "idempotency conflict")
				return
			}
			if lerr == nil && existing != nil {
				if mode == "async" {
					writeJSON(w, http.StatusAccepted, map[string]any{"id": existing.ID, "status": existing.Status})
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{"id": existing.ID, "reply": existing.Reply, "session_id": body.SessionID})
				return
			}
		}
		if mode == "async" {
			job, qerr := reg.Enqueue(auth, body.Input, body.CallbackURL, idem, hash)
			if qerr != nil {
				if errors.Is(qerr, webhook.ErrConflict) {
					writeErr(w, http.StatusConflict, "idempotency conflict")
					return
				}
				if strings.Contains(qerr.Error(), "ssrf") {
					writeErr(w, http.StatusBadRequest, "callback_url blocked")
					return
				}
				writeErr(w, http.StatusBadRequest, qerr.Error())
				return
			}
			writeJSON(w, http.StatusAccepted, map[string]any{"id": job.ID, "status": job.Status})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		whTenant := store.DefaultTenant
		if p, gerr := reg.Get(auth.ID); gerr == nil && p != nil {
			whTenant = p.TenantID
		}
		reply, sessID, chatErr := runWebhookChat(ctx, st, provider, body.SessionID, body.Input, auth.AgentID, whTenant)
		errStr := ""
		if chatErr != nil {
			if errors.Is(chatErr, llm.ErrProviderNotFound) {
				writeErr(w, http.StatusBadRequest, "provider not found")
				return
			}
			errStr = chatErr.Error()
			writeErr(w, http.StatusBadGateway, chatErr.Error())
			return
		}
		job, _ := reg.FinishSync(auth, body.Input, idem, hash, reply, errStr)
		id := ""
		if job != nil {
			id = job.ID
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "reply": reply, "session_id": sessID})
	}
}

func runWebhookChat(ctx context.Context, st store.StoreIface, provider llm.Provider, sessionID, input, agentID, tenantID string) (string, string, error) {
	if provider == nil {
		provider = llm.Echo{}
	}
	tenantID = store.NormalizeTenant(tenantID)
	sessID := strings.TrimSpace(sessionID)
	if st != nil {
		if sessID != "" {
			sess, err := st.GetSession(sessID)
			if err != nil || sess == nil || !store.SameTenant(sess.TenantID, tenantID) {
				sessID = ""
			}
		}
		if sessID == "" {
			agent := pickWebhookAgent(st, agentID, tenantID)
			if agent != nil {
				sess, err := st.CreateSession(store.Session{TenantID: tenantID, AgentID: agent.ID, Label: "webhook"})
				if err == nil {
					sessID = sess.ID
				}
			}
		}
		if sessID != "" {
			if sess, err := st.GetSession(sessID); err == nil && sess != nil {
				if a, err := st.GetAgent(sess.AgentID); err == nil && a != nil && store.SameTenant(a.TenantID, tenantID) {
					p, rerr := llm.Resolve(st, a.LLMProvider, a.Model, provider)
					if rerr != nil {
						return "", sessID, rerr
					}
					provider = p
				}
			}
			_, _ = st.AddMessage(store.Message{SessionID: sessID, Role: "user", Content: input})
		}
	}
	msgs := []llm.Message{{Role: "user", Content: input}}
	if st != nil && sessID != "" {
		history, _ := st.ListMessages(sessID)
		msgs = msgs[:0]
		for _, m := range history {
			msgs = append(msgs, llm.Message{Role: m.Role, Content: m.Content})
		}
	}
	reply, err := provider.Chat(ctx, msgs)
	if err != nil {
		reply = "echo: " + input
	}
	if st != nil && sessID != "" {
		_, _ = st.AddMessage(store.Message{SessionID: sessID, Role: "assistant", Content: reply})
	}
	return reply, sessID, nil
}

func pickWebhookAgent(st store.StoreIface, agentID, tenantID string) *store.Agent {
	tenantID = store.NormalizeTenant(tenantID)
	agentID = strings.TrimSpace(agentID)
	if agentID != "" {
		if a, err := st.GetAgent(agentID); err == nil && a != nil && store.SameTenant(a.TenantID, tenantID) {
			return a
		}
	}
	return ensureWebhookAgent(st, tenantID)
}

func ensureWebhookAgent(st store.StoreIface, tenantID string) *store.Agent {
	tenantID = store.NormalizeTenant(tenantID)
	key := "webhook"
	if tenantID != store.DefaultTenant {
		key = "webhook-" + tenantID
	}
	for _, a := range st.ListAgents() {
		if a != nil && a.AgentKey == key && store.SameTenant(a.TenantID, tenantID) {
			return a
		}
	}
	a, _ := st.CreateAgent(store.Agent{TenantID: tenantID, AgentKey: key, DisplayName: "Webhook"})
	return a
}
