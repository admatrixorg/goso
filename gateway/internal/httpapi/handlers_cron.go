// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/agent"
	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/cron"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func registerCronRoutes(mux *http.ServeMux, opt Options) {
	aliasAPI(mux, "GET /api/cron", handleListCron(opt.Store))
	aliasAPI(mux, "POST /api/cron", handleCreateCron(opt.Store))
	aliasAPI(mux, "PATCH /api/cron/{id}", handlePatchCron(opt.Store))
	aliasAPI(mux, "DELETE /api/cron/{id}", handleDeleteCron(opt.Store))
}

func handleListCron(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list := []*store.CronJob{}
		if st != nil {
			if got := st.ListCronJobs(); got != nil {
				list = got
			}
		}
		out := make([]map[string]any, 0, len(list))
		for _, j := range list {
			out = append(out, publicCron(j))
		}
		writeJSON(w, http.StatusOK, map[string]any{"jobs": out})
	}
}

func handleCreateCron(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if st == nil {
			writeErr(w, http.StatusInternalServerError, "store required")
			return
		}
		var body struct {
			Spec      string `json:"spec"`
			SessionID string `json:"session_id"`
			Message   string `json:"message"`
			Enabled   *bool  `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if _, err := cron.ParseSpec(body.Spec); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid spec")
			return
		}
		enabled := true
		if body.Enabled != nil {
			enabled = *body.Enabled
		}
		job, err := st.CreateCronJob(store.CronJob{
			Spec:      body.Spec,
			SessionID: body.SessionID,
			Message:   body.Message,
			Enabled:   enabled,
		})
		if err != nil {
			if errors.Is(err, store.ErrCronCap) {
				writeErr(w, http.StatusBadRequest, "cron cap: max 20 jobs")
				return
			}
			if strings.Contains(err.Error(), "session not found") {
				writeErr(w, http.StatusNotFound, "session not found")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, publicCron(job))
	}
}

func handlePatchCron(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if st == nil {
			writeErr(w, http.StatusInternalServerError, "store required")
			return
		}
		id := strings.TrimSpace(r.PathValue("id"))
		if id == "" {
			writeErr(w, http.StatusBadRequest, "id is required")
			return
		}
		var body struct {
			Enabled *bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if body.Enabled == nil {
			writeErr(w, http.StatusBadRequest, "enabled is required")
			return
		}
		if err := st.SetCronEnabled(id, *body.Enabled); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "not found")
				return
			}
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		job, err := st.GetCronJob(id)
		if err != nil {
			writeErr(w, http.StatusNotFound, "not found")
			return
		}
		writeJSON(w, http.StatusOK, publicCron(job))
	}
}

func handleDeleteCron(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if st == nil {
			writeErr(w, http.StatusInternalServerError, "store required")
			return
		}
		id := strings.TrimSpace(r.PathValue("id"))
		if id == "" {
			writeErr(w, http.StatusBadRequest, "id is required")
			return
		}
		if err := st.DeleteCronJob(id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeErr(w, http.StatusNotFound, "not found")
				return
			}
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func publicCron(j *store.CronJob) map[string]any {
	if j == nil {
		return map[string]any{}
	}
	out := map[string]any{
		"id":         j.ID,
		"spec":       j.Spec,
		"session_id": j.SessionID,
		"message":    j.Message,
		"enabled":    j.Enabled,
	}
	if j.LastRun != nil {
		out["last_run"] = j.LastRun.UTC().Format(time.RFC3339)
	}
	if errMsg := strings.TrimSpace(j.LastError); errMsg != "" {
		out["last_error"] = redactConnectorError(errMsg, "")
	}
	return out
}

// FireSessionChat is the POST /api/chat equivalent used by the in-process ticker.
func FireSessionChat(rt *agent.Runtime, st store.StoreIface, provider llm.Provider, meter *billing.Store) cron.FireFunc {
	return func(ctx context.Context, sessionID, message string) error {
		sessionID = strings.TrimSpace(sessionID)
		message = strings.TrimSpace(message)
		if sessionID == "" || message == "" || st == nil {
			return nil
		}
		matched, block := security.InspectChat(message)
		if matched != "" {
			log.Printf("goso cron injection: matched %q", matched)
			if block {
				return nil
			}
		}
		sess, err := st.GetSession(sessionID)
		if err != nil {
			return err
		}
		if meter != nil && billing.DayLimit() > 0 {
			today := meter.TodayTotals(time.Now().UTC())
			if billing.Exceeded(today, billing.DayLimit()) {
				return nil
			}
		}
		if rt != nil {
			out, err := rt.Chat(ctx, sessionID, message)
			if err != nil {
				return err
			}
			name := "echo"
			if provider != nil {
				name = provider.Name()
			}
			reply := ""
			if out != nil {
				reply = out.Reply
			}
			recordUsage(meter, sess.AgentID, name, llm.EstimateUsage([]llm.Message{{Role: "user", Content: message}}, reply))
			return nil
		}
		_, _ = st.AddMessage(store.Message{SessionID: sessionID, Role: "user", Content: message})
		reply := "echo: " + message
		if provider != nil {
			history, _ := st.ListMessages(sessionID)
			msgs := make([]llm.Message, 0, len(history))
			for _, m := range history {
				msgs = append(msgs, llm.Message{Role: m.Role, Content: m.Content})
			}
			got, err := provider.Chat(ctx, msgs)
			if err != nil {
				return err
			}
			reply = got
		}
		_, _ = st.AddMessage(store.Message{SessionID: sessionID, Role: "assistant", Content: reply})
		name := "echo"
		if provider != nil {
			name = provider.Name()
		}
		recordUsage(meter, sess.AgentID, name, llm.EstimateUsage([]llm.Message{{Role: "user", Content: message}}, reply))
		return nil
	}
}
