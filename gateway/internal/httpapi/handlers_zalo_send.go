package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/zalo/protocol"
)

// registerZaloSendRoutes serves one outbound text message on a stored cookie set.
func registerZaloSendRoutes(mux *http.ServeMux, st store.StoreIface) {
	mux.HandleFunc("POST /v1/zalo-sessions/{name}/send", handleZaloSend(st))
}

func handleZaloSend(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, ok := decodeChannelName(w, r)
		if !ok {
			return
		}
		var body struct {
			ThreadID   string `json:"thread_id"`
			ThreadID2  string `json:"toid"`
			ThreadType string `json:"thread_type"`
			Text       string `json:"text"`
			Message    string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		threadID := firstNonEmpty(body.ThreadID, body.ThreadID2)
		text := firstNonEmpty(body.Text, body.Message)
		if strings.TrimSpace(threadID) == "" || strings.TrimSpace(text) == "" {
			writeErr(w, http.StatusBadRequest, "thread_id and text are required")
			return
		}
		raw, err := secrets.Get(st, zaloCookieSecretName(name))
		if err != nil || len(raw) == 0 {
			writeJSON(w, http.StatusOK, map[string]any{
				"channel": name, "ok": false, "configured": false,
				"error": "no cookie credential stored for this channel",
			})
			return
		}
		cred, perr := protocol.ParseCredential(string(raw))
		if perr != nil {
			writeJSON(w, http.StatusOK, map[string]any{"channel": name, "ok": false, "error": perr.Error()})
			return
		}
		sess, serr := protocol.NewSessionWithCredential(cred)
		if serr != nil {
			writeJSON(w, http.StatusOK, map[string]any{"channel": name, "ok": false, "error": serr.Error()})
			return
		}
		// The service hosts come from the login handshake; without it a send has
		// nowhere to go, so say that plainly instead of guessing a host.
		var login struct {
			Data json.RawMessage `json:"data"`
		}
		_ = login
		kind := protocol.ThreadUser
		if strings.EqualFold(strings.TrimSpace(body.ThreadType), "group") {
			kind = protocol.ThreadGroup
		}
		ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
		defer cancel()
		res, sendErr := sess.SendMessage(ctx, threadID, kind, text)
		if sendErr != nil {
			payload := map[string]any{
				"channel": name, "ok": false, "configured": true, "error": sendErr.Error(),
			}
			if res != nil {
				payload["error_code"] = res.ErrorCode
			}
			writeJSON(w, http.StatusOK, payload)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"channel": name, "ok": true, "configured": true,
			"message_id": res.MessageID, "thread_id": threadID,
		})
	}
}
