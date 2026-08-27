// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/webhook"
)

func registerWebhookRoutes(mux *http.ServeMux, opt Options) {
	reg := opt.Webhooks
	if reg == nil {
		reg = webhook.New()
	}
	st := opt.Store
	provider := opt.Provider
	mux.HandleFunc("GET /api/webhooks", handleListWebhooks(reg))
	mux.HandleFunc("POST /api/webhooks", handleCreateWebhook(reg))
	mux.HandleFunc("POST /api/webhooks/llm", handleWebhookLLM(reg, st, provider))
}

func handleListWebhooks(reg *webhook.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"webhooks": reg.List()})
	}
}

func handleCreateWebhook(reg *webhook.Registry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := reg.Create()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, c)
	}
}

type webhookLLMBody struct {
	Input     string `json:"input"`
	Mode      string `json:"mode"`
	SessionID string `json:"session_id"`
}

func handleWebhookLLM(reg *webhook.Registry, st store.StoreIface, provider llm.Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid body")
			return
		}
		if err := reg.Authenticate(r.Header.Get("Authorization"), r.Header.Get("X-Goso-Signature"), raw); err != nil {
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
		if mode == "async" {
			job := reg.NewJob()
			go func() {
				reply, _ := runWebhookChat(context.Background(), st, provider, body.SessionID, body.Input)
				reg.CompleteJob(job.ID, reply)
			}()
			writeJSON(w, http.StatusAccepted, map[string]any{"id": job.ID, "status": job.Status})
			return
		}
		reply, sessID := runWebhookChat(r.Context(), st, provider, body.SessionID, body.Input)
		writeJSON(w, http.StatusOK, map[string]any{"reply": reply, "session_id": sessID})
	}
}

func runWebhookChat(ctx context.Context, st store.StoreIface, provider llm.Provider, sessionID, input string) (string, string) {
	if provider == nil {
		provider = llm.Echo{}
	}
	sessID := strings.TrimSpace(sessionID)
	if st != nil {
		if sessID == "" {
			agent := ensureWebhookAgent(st)
			sess, err := st.CreateSession(store.Session{AgentID: agent.ID, Label: "webhook"})
			if err == nil {
				sessID = sess.ID
			}
		} else if _, err := st.GetSession(sessID); err != nil {
			agent := ensureWebhookAgent(st)
			sess, err := st.CreateSession(store.Session{AgentID: agent.ID, Label: "webhook:" + sessID})
			if err == nil {
				sessID = sess.ID
			}
		}
		if sessID != "" {
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
	return reply, sessID
}

func ensureWebhookAgent(st store.StoreIface) *store.Agent {
	for _, a := range st.ListAgents() {
		if a.AgentKey == "webhook" {
			return a
		}
	}
	a, _ := st.CreateAgent(store.Agent{AgentKey: "webhook", DisplayName: "Webhook"})
	return a
}
