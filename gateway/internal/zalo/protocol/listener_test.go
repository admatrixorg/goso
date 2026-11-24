package protocol

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
)

func frame(version byte, cmd uint16, sub byte, body map[string]any) []byte {
	raw, _ := json.Marshal(body)
	out := make([]byte, 0, 4+len(raw))
	out = append(out, version, byte(cmd), byte(cmd>>8), sub)
	return append(out, raw...)
}

func TestDecodeInboundTextAndObject(t *testing.T) {
	str := json.RawMessage(`"hello there"`)
	msg, ok := decodeInbound([]byte(`{"msgId":"m1","uidFrom":"u2","idTo":"u1","dName":"An","ts":"1700000000000","content":"hello there"}`), "u1", "user")
	if !ok || msg.Text != "hello there" || msg.FromID != "u2" || msg.ThreadID != "u2" || msg.Timestamp != 1700000000000 {
		t.Fatalf("string content %#v ok=%v", msg, ok)
	}
	_ = str
	obj, ok2 := decodeInbound([]byte(`{"msgId":"m2","uidFrom":"u3","idTo":"g1","content":{"title":"card title"}}`), "u1", "group")
	if !ok2 || obj.Text != "card title" || obj.ThreadType != "group" {
		t.Fatalf("object content %#v ok=%v", obj, ok2)
	}
}

func TestDecodeInboundSkipsSelfEcho(t *testing.T) {
	if _, ok := decodeInbound([]byte(`{"msgId":"m","uidFrom":"u1","idTo":"u9","content":"hi"}`), "u1", "user"); ok {
		t.Fatal("own message must be skipped")
	}
	if _, ok := decodeInbound([]byte(`{"msgId":"m","uidFrom":"u2","idTo":"u9","content":""}`), "u1", "user"); ok {
		t.Fatal("empty text must be skipped")
	}
	if _, ok := decodeInbound([]byte(`{"msgId":"m","uidFrom":"0","idTo":"u9","content":"hi"}`), "u1", "user"); ok {
		t.Fatal("uidFrom 0 is a self echo")
	}
}

func TestDecryptEventDataPlainAndGzip(t *testing.T) {
	l := &Listener{}
	plain, err := l.decryptEventData("hello", 0, "")
	if err != nil || string(plain) != "hello" {
		t.Fatalf("plain %q %v", plain, err)
	}
	// gzip of {"data":{"msgs":[]}} encoded as base64
	payload := `{"data":{"msgs":[]}}`
	gz := gzipForTest([]byte(payload))
	b64 := base64.StdEncoding.EncodeToString(gz)
	out, gerr := l.decryptEventData(b64, 1, "")
	if gerr != nil || string(out) != payload {
		t.Fatalf("gzip %q %v", out, gerr)
	}
}

// TestListenerReceivesFrame runs the read loop against a local websocket and
// asserts a plaintext user frame reaches the handler.
func TestListenerReceivesFrame(t *testing.T) {
	got := make(chan InboundMessage, 1)
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.WriteMessage(websocket.BinaryMessage, frame(1, 1, 1, map[string]any{"key": "cipher-key"}))
		body := map[string]any{
			"data": map[string]any{"msgs": []any{
				map[string]any{"msgId": "m9", "uidFrom": "u2", "idTo": "u1", "dName": "Bình", "ts": "1700000000000", "content": "ping from qc"},
			}},
		}
		_ = c.WriteMessage(websocket.BinaryMessage, frame(1, 501, 0, map[string]any{"encrypt": 0, "data": mustJSON(body)}))
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()

	cred, _ := ParseCredential(`{"cookies":[{"name":"zpw_sek","value":"x"}]}`)
	sess, err := NewSessionWithCredential(cred)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	sess.UID = "u1"
	sess.WSURL = "ws" + strings.TrimPrefix(srv.URL, "http")
	ln, lerr := NewListener(sess, func(m InboundMessage) { got <- m })
	if lerr != nil {
		t.Fatalf("listener: %v", lerr)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() { _ = ln.Start(ctx) }()
	select {
	case m := <-got:
		if m.Text != "ping from qc" || m.FromID != "u2" || m.FromName != "Bình" || m.ThreadID != "u2" {
			t.Fatalf("message %#v", m)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("no message reached the handler")
	}
	ln.Stop()
}

func TestNewListenerNeedsWebsocketURL(t *testing.T) {
	cred, _ := ParseCredential(`{"cookies":[{"name":"zpw_sek","value":"x"}]}`)
	sess, _ := NewSessionWithCredential(cred)
	if _, err := NewListener(sess, nil); err == nil {
		t.Fatal("expected an error without a websocket url")
	}
}

func mustJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}
