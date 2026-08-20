// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Router builds the HTTP mux.
func Router(st store.StoreIface, version string) http.Handler {
	mux := routerBase(st, version)
	mux.HandleFunc("GET /api/providers", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"providers": []string{"echo"}})
	})
	mux.HandleFunc("GET /api/channels", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"channels": []string{"telegram", "zalo-personal", "zalo-oa"}})
	})
	mux.HandleFunc("POST /api/chat", handleChat(st))
	return mux
}

func routerBase(st store.StoreIface, version string) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": version})
	})

	// Agents
	mux.HandleFunc("POST /api/agents", handleCreateAgent(st))
	mux.HandleFunc("GET /api/agents", handleListAgents(st))
	mux.HandleFunc("GET /api/agents/{id}", handleGetAgent(st))

	// Sessions
	mux.HandleFunc("POST /api/sessions", handleCreateSession(st))
	mux.HandleFunc("GET /api/sessions", handleListSessions(st))
	mux.HandleFunc("POST /api/sessions/{id}/messages", handleAddMessage(st))
	mux.HandleFunc("GET /api/sessions/{id}/messages", handleListMessages(st))

	// WebSocket is registered separately via RegisterWS to keep gorilla dep isolated.
	return mux
}

func handleCreateAgent(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			AgentKey    string `json:"agent_key"`
			DisplayName string `json:"display_name"`
			Model       string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		body.AgentKey = strings.TrimSpace(body.AgentKey)
		if body.AgentKey == "" {
			writeErr(w, http.StatusBadRequest, "agent_key is required")
			return
		}
		a, err := st.CreateAgent(store.Agent{AgentKey: body.AgentKey, DisplayName: body.DisplayName, Model: body.Model})
		if err != nil {
			if errors.Is(err, store.ErrExists) {
				writeErr(w, http.StatusConflict, "agent_key already exists")
				return
			}
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, a)
	}
}

func handleListAgents(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list := st.ListAgents()
		if list == nil {
			list = []*store.Agent{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"agents": list})
	}
}

func handleGetAgent(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		a, err := st.GetAgent(id)
		if err != nil {
			writeErr(w, http.StatusNotFound, "agent not found")
			return
		}
		writeJSON(w, http.StatusOK, a)
	}
}

func handleCreateSession(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			AgentID string `json:"agent_id"`
			Label   string `json:"label"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		body.AgentID = strings.TrimSpace(body.AgentID)
		if body.AgentID == "" {
			writeErr(w, http.StatusBadRequest, "agent_id is required")
			return
		}
		sess, err := st.CreateSession(store.Session{AgentID: body.AgentID, Label: body.Label})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, sess)
	}
}

func handleListSessions(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list := st.ListSessions()
		if list == nil {
			list = []*store.Session{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessions": list})
	}
}

func handleAddMessage(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sid := r.PathValue("id")
		var body struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if strings.TrimSpace(body.Content) == "" {
			writeErr(w, http.StatusBadRequest, "content is required")
			return
		}
		if body.Role == "" {
			body.Role = "user"
		}
		m, err := st.AddMessage(store.Message{SessionID: sid, Role: body.Role, Content: body.Content})
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, m)
	}
}

func handleListMessages(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sid := r.PathValue("id")
		msgs, err := st.ListMessages(sid)
		if err != nil {
			writeErr(w, http.StatusNotFound, "session not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"messages": msgs})
	}
}

func handleChat(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SessionID string `json:"session_id"`
			Message   string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if strings.TrimSpace(body.SessionID) == "" || strings.TrimSpace(body.Message) == "" {
			writeErr(w, http.StatusBadRequest, "session_id and message are required")
			return
		}
		if _, err := st.GetSession(body.SessionID); err != nil {
			writeErr(w, http.StatusNotFound, "session not found")
			return
		}
		// Persist user message and echo reply (stub for LLM in SPEC 003).
		_, _ = st.AddMessage(store.Message{SessionID: body.SessionID, Role: "user", Content: body.Message})
		reply := "echo: " + body.Message
		_, _ = st.AddMessage(store.Message{SessionID: body.SessionID, Role: "assistant", Content: reply})
		writeJSON(w, http.StatusOK, map[string]any{"reply": reply, "session_id": body.SessionID})
	}
}

func handleChatWithLLM(st store.StoreIface, provider llm.Provider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SessionID string `json:"session_id"`
			Message   string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if strings.TrimSpace(body.SessionID) == "" || strings.TrimSpace(body.Message) == "" {
			writeErr(w, http.StatusBadRequest, "session_id and message are required")
			return
		}
		if _, err := st.GetSession(body.SessionID); err != nil {
			writeErr(w, http.StatusNotFound, "session not found")
			return
		}
		_, _ = st.AddMessage(store.Message{SessionID: body.SessionID, Role: "user", Content: body.Message})
		history, _ := st.ListMessages(body.SessionID)
		var msgs []llm.Message
		for _, m := range history {
			msgs = append(msgs, llm.Message{Role: m.Role, Content: m.Content})
		}
		reply, err := provider.Chat(r.Context(), msgs)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		_, _ = st.AddMessage(store.Message{SessionID: body.SessionID, Role: "assistant", Content: reply})
		writeJSON(w, http.StatusOK, map[string]any{"reply": reply, "session_id": body.SessionID})
	}
}

// RouterWithDeps builds mux with LLM and channel deps (used in main).
func RouterWithDeps(st store.StoreIface, version string, provider llm.Provider, tgHandler http.HandlerFunc) http.Handler {
	// Extended by SPEC 004: zalo handlers are registered via RouterWithAllChannels when available.
	return RouterWithAllChannels(st, version, provider, tgHandler, nil, nil)
}

func RouterWithAllChannels(st store.StoreIface, version string, provider llm.Provider, tgHandler, zpHandler, zoHandler http.HandlerFunc) http.Handler {
	mux := routerBase(st, version)
	if zpHandler != nil {
		mux.HandleFunc("POST /api/channels/zalo-personal/webhook", zpHandler)
	}
	if zoHandler != nil {
		mux.HandleFunc("POST /api/channels/zalo-oa/webhook", zoHandler)
	}
	if tgHandler != nil {
		mux.HandleFunc("POST /api/channels/telegram/webhook", tgHandler)
	}
	mux.HandleFunc("GET /api/channels", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"channels": []string{"telegram", "zalo-personal", "zalo-oa"}})
	})
	mux.HandleFunc("GET /api/providers", func(w http.ResponseWriter, r *http.Request) {
		// provider name only, never expose keys
		name := "echo"
		if provider != nil {
			name = provider.Name()
		}
		writeJSON(w, http.StatusOK, map[string]any{"providers": []string{name}})
	})
	if provider != nil {
		mux.HandleFunc("POST /api/chat", handleChatWithLLM(st, provider))
	} else {
		mux.HandleFunc("POST /api/chat", handleChat(st))
	}
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
