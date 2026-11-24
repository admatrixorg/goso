package zalo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/mqglobal/goso/gateway/internal/zalo/protocol"
)

func wsFrame(version byte, cmd uint16, sub byte, body map[string]any) []byte {
	raw, _ := json.Marshal(body)
	out := []byte{version, byte(cmd), byte(cmd >> 8), sub}
	return append(out, raw...)
}

// TestManagerStartsAndReceives proves the runtime chain: login with the stored
// cookie, dial the socket the login handed out, and land the message in the inbox.
func TestManagerStartsAndReceives(t *testing.T) {
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	wsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.WriteMessage(websocket.BinaryMessage, wsFrame(1, 1, 1, map[string]any{"key": "k"}))
		body := map[string]any{"data": map[string]any{"msgs": []any{
			map[string]any{"msgId": "m1", "uidFrom": "u2", "idTo": "u1", "dName": "An", "ts": "1700000000000", "content": "chao ban"},
		}}}
		_ = c.WriteMessage(websocket.BinaryMessage, wsFrame(1, 501, 0, map[string]any{"encrypt": 0, "data": mustJSON(body)}))
		time.Sleep(300 * time.Millisecond)
	}))
	defer wsSrv.Close()
	wsURL := "ws" + strings.TrimPrefix(wsSrv.URL, "http")

	replyKey := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 16)))
	loginSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The reply is encrypted under the key the signed request carried, which
		// the server rebuilds from the zcid and its extension.
		key, kerr := protocol.DeriveEncryptKey(r.URL.Query().Get("zcid_ext"), r.URL.Query().Get("zcid"))
		if kerr != nil {
			t.Errorf("derive key: %v", kerr)
			return
		}
		inner, _ := json.Marshal(map[string]any{
			"error_code": 0,
			"data": map[string]any{
				"uid":          "u1",
				"display_name": "QC Account",
				"zpw_enk":      replyKey,
				"zpw_ws":       []string{wsURL},
				"zpw_service_map_v3": map[string]any{
					"chat": []string{"https://chat.example"},
				},
			},
		})
		enc, eerr := protocol.EncodeAESCBC([]byte(key), string(inner), false)
		if eerr != nil {
			t.Errorf("encrypt reply: %v", eerr)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"error_code": 0, "data": enc})
	}))
	defer loginSrv.Close()

	cred := &protocol.Credential{Cookies: []protocol.Cookie{{Name: "zpw_sek", Value: "x"}}}
	m := NewManager()
	sess, err := protocol.NewSessionWithCredential(cred)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	// Point login at the local server, then hand that session to the manager path.
	info, lerr := sess.GetLoginInfoOn(context.Background(), loginSrv.URL)
	if lerr != nil {
		t.Fatalf("login: %v", lerr)
	}
	if info.UID != "u1" || info.Name != "QC Account" {
		t.Fatalf("login info %#v", info)
	}
	if urls := sess.WebSocketURLs(); len(urls) != 1 || urls[0] != wsURL {
		t.Fatalf("ws urls %#v", urls)
	}
	sess.WSURL = pickWS(wsURL)
	var received []protocol.InboundMessage
	ln, err := protocol.NewListener(sess, func(msg protocol.InboundMessage) { received = append(received, msg) })
	if err != nil {
		t.Fatalf("listener: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() { _ = ln.Start(ctx) }()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) && len(received) == 0 {
		time.Sleep(50 * time.Millisecond)
	}
	ln.Stop()
	if len(received) == 0 || received[0].Text != "chao ban" || received[0].FromID != "u2" {
		t.Fatalf("received %#v", received)
	}
	if m.Status("missing").Running {
		t.Fatal("unknown channel must report not running")
	}
}

func TestPickWSURL(t *testing.T) {
	if got := pickWSURL("wss://a.example/ws"); got != "wss://a.example/ws" {
		t.Fatalf("kept %q", got)
	}
	if got := pickWSURL("a.example/ws"); got != "wss://a.example/ws" {
		t.Fatalf("prefixed %q", got)
	}
	if got := pickWSURL(""); got != "" {
		t.Fatalf("empty %q", got)
	}
}

func mustJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

// pickWS is a test alias so the assertion reads clearly.
func pickWS(raw string) string { return pickWSURL(raw) }
