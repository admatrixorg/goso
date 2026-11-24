package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/zalo/protocol"
)

// saveZaloCookieCredential stores the cookie set an operator pastes for a
// zalo-personal channel. It returns saved=false with code 0 when the body is not
// a cookie credential, so the caller can fall through to the bridge path.
func saveZaloCookieCredential(st store.StoreIface, name string, r *http.Request) (bool, int, string) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil || len(bytes.TrimSpace(raw)) == 0 {
		return false, 0, ""
	}
	var body map[string]any
	if json.Unmarshal(raw, &body) != nil {
		return false, 0, ""
	}
	var cred protocol.Credential
	found := false
	for key, value := range body {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "cookies":
			found = true
			switch v := value.(type) {
			case string:
				var parsed []protocol.Cookie
				if json.Unmarshal([]byte(v), &parsed) == nil {
					cred.Cookies = parsed
				}
			case []any:
				blob, _ := json.Marshal(v)
				var parsed []protocol.Cookie
				if json.Unmarshal(blob, &parsed) == nil {
					cred.Cookies = parsed
				}
			case map[string]any:
				for k, vv := range v {
					sv, _ := vv.(string)
					cred.Cookies = append(cred.Cookies, protocol.Cookie{Name: k, Value: sv})
				}
			}
		case "bridge_secret":
			// The bridge field is the only credential input the dialog shows for this
			// type, so a pasted cookie set there is accepted as one.
			if s, ok := value.(string); ok {
				if parsed, perr := protocol.ParseCredential(s); perr == nil {
					cred = *parsed
					found = true
				}
			}
		case "secret_key":
			cred.SecretKey, _ = value.(string)
		case "imei":
			cred.IMEI, _ = value.(string)
		case "user_agent":
			cred.UserAgent, _ = value.(string)
		}
	}
	if !found {
		return false, 0, ""
	}
	if len(cred.Cookies) == 0 {
		return true, http.StatusBadRequest, "cookies must contain at least one entry"
	}
	encoded, merr := json.Marshal(cred)
	if merr != nil {
		return true, http.StatusBadRequest, "credential is not encodable"
	}
	if perr := secrets.Put(st, zaloCookieSecretName(name), encoded); perr != nil {
		return true, http.StatusInternalServerError, "save failed"
	}
	return true, 0, ""
}
