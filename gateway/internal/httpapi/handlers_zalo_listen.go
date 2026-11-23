package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/zalo"
	"github.com/mqglobal/goso/gateway/internal/zalo/protocol"
)

// zaloManager owns the running personal sessions for this process.
var zaloManager = zalo.NewManager()

// registerZaloListenRoutes starts and stops a personal session and reports what
// it has received.
func registerZaloListenRoutes(mux *http.ServeMux, st store.StoreIface) {
	mux.HandleFunc("POST /v1/zalo-sessions/{name}/listen", handleZaloListenStart(st))
	mux.HandleFunc("POST /v1/zalo-sessions/{name}/listen/stop", handleZaloListenStop(st))
	mux.HandleFunc("GET /v1/zalo-sessions/{name}/listen", handleZaloListenStatus(st))
	mux.HandleFunc("GET /v1/zalo-sessions/{name}/inbox", handleZaloInbox(st))
}

func handleZaloListenStart(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, ok := decodeChannelName(w, r)
		if !ok {
			return
		}
		cred, cerr := storedZaloCredential(st, name)
		if cerr != nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"channel": name, "running": false, "error": cerr.Error(),
			})
			return
		}
		status, err := zaloManager.Start(r.Context(), name, cred)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"channel": name, "running": false, "error": err.Error(),
			})
			return
		}
		writeJSON(w, http.StatusOK, status)
	}
}

func handleZaloListenStop(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, ok := decodeChannelName(w, r)
		if !ok {
			return
		}
		zaloManager.Stop(name)
		writeJSON(w, http.StatusOK, map[string]any{"channel": name, "running": false})
	}
}

func handleZaloListenStatus(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, ok := decodeChannelName(w, r)
		if !ok {
			return
		}
		status := zaloManager.Status(name)
		_, cerr := storedZaloCredential(st, name)
		writeJSON(w, http.StatusOK, map[string]any{
			"status": status, "configured": cerr == nil,
		})
	}
}

func handleZaloInbox(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, ok := decodeChannelName(w, r)
		if !ok {
			return
		}
		limit := 50
		if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = n
			}
		}
		items := zaloManager.Inbox(name, limit)
		writeJSON(w, http.StatusOK, map[string]any{
			"channel": name, "messages": items, "total": len(items),
		})
	}
}

// storedZaloCredential loads and parses the cookie set for a channel.
func storedZaloCredential(st store.StoreIface, name string) (*protocol.Credential, error) {
	raw, err := secrets.Get(st, zaloCookieSecretName(name))
	if err != nil || len(raw) == 0 {
		return nil, errNoZaloCredential
	}
	cred, perr := protocol.ParseCredential(string(raw))
	if perr != nil {
		return nil, perr
	}
	if strings.TrimSpace(cred.SecretKey) == "" {
		return nil, errNoZaloSecret
	}
	return cred, nil
}

var (
	errNoZaloCredential = &zaloCred{msg: "no cookie credential stored for this channel"}
	errNoZaloSecret     = &zaloCred{msg: "the stored credential has no secret key"}
)

type zaloCred struct{ msg string }

func (e *zaloCred) Error() string { return e.msg }

var _ = json.Marshal
