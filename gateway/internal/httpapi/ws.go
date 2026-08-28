// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// WS JSON frames: {op, payload}. ping→pong, chat {session_id, message}→reply text.
// If GOSO_WS_ORIGINS is empty, origin checks stay allow-all (previous behaviour).
// If set (comma-separated), only listed Origin values are accepted.

type wsFrame struct {
	Op      string          `json:"op"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type wsChatIn struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
}

func wsUpgrader() websocket.Upgrader {
	return websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			raw := strings.TrimSpace(os.Getenv("GOSO_WS_ORIGINS"))
			if raw == "" {
				return true
			}
			origin := r.Header.Get("Origin")
			for _, a := range strings.Split(raw, ",") {
				if strings.TrimSpace(a) == origin {
					return true
				}
			}
			return false
		},
	}
}

// RegisterWS registers GET /ws JSON RPC (not echo-only).
func RegisterWS(mux *http.ServeMux, st store.StoreIface, provider llm.Provider) {
	mux.HandleFunc("GET /ws", func(w http.ResponseWriter, r *http.Request) {
		up := wsUpgrader()
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetReadLimit(security.MaxWSRead)
		if provider == nil {
			provider = llm.Echo{}
		}
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var frame wsFrame
			if err := json.Unmarshal(msg, &frame); err != nil {
				_ = conn.WriteJSON(wsFrame{Op: "error", Payload: jsonRaw(`{"error":"invalid json"}`)})
				continue
			}
			switch strings.ToLower(strings.TrimSpace(frame.Op)) {
			case "ping":
				if err := conn.WriteJSON(wsFrame{Op: "pong", Payload: frame.Payload}); err != nil {
					return
				}
			case "chat":
				var in wsChatIn
				if len(frame.Payload) > 0 {
					_ = json.Unmarshal(frame.Payload, &in)
				}
				in.Message = strings.TrimSpace(in.Message)
				if in.Message == "" {
					_ = conn.WriteJSON(wsFrame{Op: "error", Payload: jsonRaw(`{"error":"message is required"}`)})
					continue
				}
				reply, sessID, chatErr := runWebhookChat(r.Context(), st, provider, in.SessionID, in.Message)
				if chatErr != nil {
					msg := chatErr.Error()
					if errors.Is(chatErr, llm.ErrProviderNotFound) {
						msg = "provider not found"
					}
					payload, _ := json.Marshal(map[string]string{"error": msg})
					if err := conn.WriteJSON(wsFrame{Op: "error", Payload: payload}); err != nil {
						return
					}
					continue
				}
				out, _ := json.Marshal(map[string]any{"session_id": sessID, "reply": reply})
				if err := conn.WriteJSON(wsFrame{Op: "chat", Payload: out}); err != nil {
					return
				}
			default:
				if err := conn.WriteJSON(wsFrame{Op: "error", Payload: jsonRaw(`{"error":"unknown op"}`)}); err != nil {
					return
				}
			}
		}
	})
}

func jsonRaw(s string) json.RawMessage { return json.RawMessage(s) }
