package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/zalo/protocol"
)

// zaloCookieSecretName is where a personal account cookie set is stored. It is a
// separate key from the channel secrets so the existing channel contract, which
// only exposes bridge_secret for the bridge path, keeps working.
func zaloCookieSecretName(channelName string) string {
	return "zalo_cookies:" + strings.TrimSpace(channelName)
}

// registerZaloPersonalRoutes lets an operator paste a cookie set and validate it.
func registerZaloPersonalRoutes(mux *http.ServeMux, st store.StoreIface) {
	mux.HandleFunc("PUT /v1/zalo-sessions/{name}", handleZaloSessionPut(st))
	mux.HandleFunc("GET /v1/zalo-sessions/{name}", handleZaloSessionGet(st))
	mux.HandleFunc("POST /v1/zalo-sessions/{name}/check", handleZaloSessionCheck(st))
}

func decodeChannelName(w http.ResponseWriter, r *http.Request) (string, bool) {
	raw := r.PathValue("name")
	if unescaped, err := url.PathUnescape(raw); err == nil {
		raw = unescaped
	}
	name := strings.TrimSpace(raw)
	if name == "" {
		writeErr(w, http.StatusBadRequest, "channel name is required")
		return "", false
	}
	return name, true
}

func handleZaloSessionPut(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, ok := decodeChannelName(w, r)
		if !ok {
			return
		}
		var body protocol.Credential
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		if len(body.Cookies) == 0 {
			writeErr(w, http.StatusBadRequest, "cookies are required")
			return
		}
		raw, err := json.Marshal(body)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "credential is not encodable")
			return
		}
		if err := secrets.Put(st, zaloCookieSecretName(name), raw); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true, "channel": name, "cookies": len(body.Cookies),
			"has_secret_key": strings.TrimSpace(body.SecretKey) != "",
		})
	}
}

func handleZaloSessionGet(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, ok := decodeChannelName(w, r)
		if !ok {
			return
		}
		raw, err := secrets.Get(st, zaloCookieSecretName(name))
		if err != nil || len(raw) == 0 {
			writeJSON(w, http.StatusOK, map[string]any{"channel": name, "configured": false})
			return
		}
		cred, perr := protocol.ParseCredential(string(raw))
		if perr != nil {
			writeJSON(w, http.StatusOK, map[string]any{"channel": name, "configured": false, "error": perr.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"channel": name, "configured": true, "cookies": len(cred.Cookies),
			"imei_set":       strings.TrimSpace(cred.IMEI) != "",
			"has_secret_key": strings.TrimSpace(cred.SecretKey) != "",
		})
	}
}

func handleZaloSessionCheck(st store.StoreIface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, ok := decodeChannelName(w, r)
		if !ok {
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
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		info, ierr := sess.GetLoginInfo(ctx)
		if ierr != nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"channel": name, "ok": false, "configured": true, "error": ierr.Error(),
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"channel": name, "ok": true, "configured": true, "uid": info.UID, "name": info.Name,
		})
	}
}
