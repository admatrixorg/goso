package protocol

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseCredentialCookieList(t *testing.T) {
	raw := `{"cookies":[{"name":"zpw_sek","value":"abc"},{"name":"zpsid","value":"xyz"}],"imei":"imei-1"}`
	cred, err := ParseCredential(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cred.Cookies) != 2 || cred.IMEI != "imei-1" {
		t.Fatalf("cred %#v", cred)
	}
}

func TestParseCredentialBareMap(t *testing.T) {
	cred, err := ParseCredential(`{"zpw_sek":"abc","zpsid":"xyz"}`)
	if err != nil {
		t.Fatalf("parse bare map: %v", err)
	}
	if len(cred.Cookies) != 2 {
		t.Fatalf("cookies %d", len(cred.Cookies))
	}
}

func TestParseCredentialRejectsEmpty(t *testing.T) {
	if _, err := ParseCredential(""); err == nil {
		t.Fatal("expected an error for an empty credential")
	}
	if _, err := ParseCredential("{}"); err == nil {
		t.Fatal("expected an error for a credential without cookies")
	}
}

func TestGetLoginInfoSendsCookies(t *testing.T) {
	var seenCookie, seenUA string
	replyKey := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 16)))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("zpw_sek"); err == nil {
			seenCookie = c.Value
		}
		seenUA = r.Header.Get("User-Agent")
		// The reply is encrypted with the key the request carried, which the
		// server rebuilds from the zcid and its extension.
		key, err := deriveEncryptKey(r.URL.Query().Get("zcid_ext"), r.URL.Query().Get("zcid"))
		if err != nil {
			t.Errorf("derive key: %v", err)
			return
		}
		inner, _ := json.Marshal(map[string]any{
			"error_code": 0,
			"data": map[string]any{
				"uid":          "1234567890",
				"display_name": "Zalo QC",
				"zpw_enk":      replyKey,
				"zpw_service_map_v3": map[string]any{
					"chat":    []string{"https://tt-chat-wpa.chat.zalo.me"},
					"profile": []string{"https://tt-profile-wpa.chat.zalo.me"},
				},
			},
		})
		enc, eerr := EncodeAESCBC([]byte(key), string(inner), false)
		if eerr != nil {
			t.Errorf("encrypt reply: %v", eerr)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"error_code": 0, "data": enc})
	}))
	defer srv.Close()

	cred, err := ParseCredential(`{"cookies":[{"name":"zpw_sek","value":"secret-value"}],"user_agent":"qc-agent"}`)
	if err != nil {
		t.Fatalf("cred: %v", err)
	}
	sess, err := NewSessionWithCredential(cred)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	SeedCookies(sess.Jar, srv.URL, cred.Cookies)

	info, err := sess.GetLoginInfoOn(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("login info: %v", err)
	}
	if info.UID != "1234567890" || info.Name != "Zalo QC" {
		t.Fatalf("info %#v", info)
	}
	if info.SecretKey != replyKey {
		t.Fatalf("payload key %q", info.SecretKey)
	}
	if sess.SecretKey == "" {
		t.Fatal("the session did not adopt the payload key from the reply")
	}
	if sess.ServiceURL("profile") != "https://tt-profile-wpa.chat.zalo.me" {
		t.Fatalf("profile service %q", sess.ServiceURL("profile"))
	}
	if seenCookie != "secret-value" {
		t.Fatalf("server saw cookie %q", seenCookie)
	}
	if seenUA != "qc-agent" {
		t.Fatalf("server saw user agent %q", seenUA)
	}
}

func TestGetLoginInfoRejectsErrorEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"error_code": 401, "error_message": "session expired"})
	}))
	defer srv.Close()
	cred, _ := ParseCredential(`{"cookies":[{"name":"zpw_sek","value":"x"}]}`)
	sess, _ := NewSessionWithCredential(cred)
	SeedCookies(sess.Jar, srv.URL, cred.Cookies)
	if _, err := sess.GetLoginInfoOn(context.Background(), srv.URL); err == nil {
		t.Fatal("expected the rejected login to fail")
	}
}
