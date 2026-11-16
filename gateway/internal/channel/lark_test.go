// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
)

const (
	larkA      = "tenant-a/feishu/team"
	larkAppID  = "cli_demo"
	larkSecret = "app-secret-demo"
	larkBotID  = "ou_bot"
)

// fakeLark is Lark's open platform for one app: the token, bot info and long
// connection endpoints, the socket itself, and the reply endpoint.
type fakeLark struct {
	srv *httptest.Server
	// events are sent down each new socket, one frame list per event.
	events [][]larkFrame
	// expireOnce makes the first reply fail with a stale-token code.
	expireOnce bool

	mu      sync.Mutex
	tokens  int
	replies []map[string]string
	frames  []larkFrame
}

func newFakeLark(t *testing.T) *fakeLark {
	t.Helper()
	f := &fakeLark{}
	up := websocket.Upgrader{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var in map[string]any
		_ = json.Unmarshal(body, &in)
		switch {
		case r.URL.Path == "/open-apis/auth/v3/tenant_access_token/internal":
			if in["app_id"] != larkAppID || in["app_secret"] != larkSecret {
				_, _ = w.Write([]byte(`{"code":10014,"msg":"app secret invalid"}`))
				return
			}
			f.mu.Lock()
			f.tokens++
			f.mu.Unlock()
			_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"t-demo","expire":7200}`))
		case r.URL.Path == "/open-apis/bot/v3/info":
			if r.Header.Get("Authorization") != "Bearer t-demo" {
				_, _ = w.Write([]byte(`{"code":99991663,"msg":"token invalid"}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"bot":{"open_id":"` + larkBotID + `","app_name":"Minh An"}}`))
		case r.URL.Path == "/callback/ws/endpoint":
			if in["AppID"] != larkAppID || in["AppSecret"] != larkSecret {
				_, _ = w.Write([]byte(`{"code":514,"msg":"forbidden"}`))
				return
			}
			ws := "ws" + strings.TrimPrefix(f.srv.URL, "http") + "/ws?device_id=d1&service_id=7"
			_, _ = w.Write([]byte(`{"code":0,"data":{"URL":"` + ws + `","ClientConfig":{"PingInterval":1}}}`))
		case r.URL.Path == "/ws":
			conn, err := up.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			for _, ev := range f.events {
				for _, fr := range ev {
					_ = conn.WriteMessage(websocket.BinaryMessage, fr.encode())
				}
			}
			for {
				_, raw, err := conn.ReadMessage()
				if err != nil {
					return
				}
				fr, err := decodeLarkFrame(raw)
				if err != nil {
					continue
				}
				f.mu.Lock()
				f.frames = append(f.frames, fr)
				f.mu.Unlock()
			}
		case strings.HasSuffix(r.URL.Path, "/reply"):
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.expireOnce {
				f.expireOnce = false
				_, _ = w.Write([]byte(`{"code":99991663,"msg":"token expired"}`))
				return
			}
			var content map[string]string
			s, _ := in["content"].(string)
			_ = json.Unmarshal([]byte(s), &content)
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/open-apis/im/v1/messages/"), "/reply")
			f.replies = append(f.replies, map[string]string{"message_id": id, "text": content["text"]})
			_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeLark) got() (replies []map[string]string, frames []larkFrame, tokens int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]string(nil), f.replies...), append([]larkFrame(nil), f.frames...), f.tokens
}

// larkEventJSON is an im.message.receive_v1 event.
func larkEventJSON(msgID, chatType, text string, mentionBot bool) []byte {
	mentions := []map[string]any{}
	if mentionBot {
		mentions = append(mentions, map[string]any{"key": "@_user_1", "id": map[string]any{"open_id": larkBotID}, "name": "Minh An"})
		text = "@_user_1 " + text
	}
	content, _ := json.Marshal(map[string]string{"text": text})
	b, _ := json.Marshal(map[string]any{
		"schema": "2.0",
		"header": map[string]any{"event_id": "ev-" + msgID, "event_type": "im.message.receive_v1"},
		"event": map[string]any{
			"sender": map[string]any{"sender_id": map[string]any{"open_id": "ou_khach"}, "sender_type": "user"},
			"message": map[string]any{
				"message_id": msgID, "chat_id": "oc_" + chatType, "chat_type": chatType,
				"message_type": "text", "content": string(content), "mentions": mentions,
			},
		},
	})
	return b
}

// eventFrames splits an event into n data frames, as Lark does with big ones.
func eventFrames(id string, payload []byte, n int) []larkFrame {
	var out []larkFrame
	size := (len(payload) + n - 1) / n
	for i := 0; i < n; i++ {
		end := (i + 1) * size
		if end > len(payload) {
			end = len(payload)
		}
		out = append(out, larkFrame{
			SeqID: uint64(i + 1), Service: 7, Method: larkFrameData,
			Headers: [][2]string{{"type", "event"}, {"message_id", id}, {"sum", itoa(n)}, {"seq", itoa(i)}},
			Payload: payload[i*size : end],
		})
	}
	return out
}

func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }

func larkStore(t *testing.T, settings map[string]string) store.StoreIface {
	t.Helper()
	key, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", key)
	st := store.New()
	agent, _ := st.CreateAgent(store.Agent{AgentKey: "minh-an", DisplayName: "Minh An", TenantID: "tenant-a"})
	if settings == nil {
		settings = map[string]string{SettingAppID: larkAppID}
	}
	if err := st.PutChannelConfig(store.ChannelConfig{Name: larkA, Enabled: true, AgentID: agent.ID, DMPolicy: "open", GroupPolicy: "open", RequireMention: true, Settings: settings}); err != nil {
		t.Fatal(err)
	}
	if err := secrets.Put(st, SecretName(larkA, KindAppSecret), []byte(larkSecret)); err != nil {
		t.Fatal(err)
	}
	return st
}

// The manager holds the app's long connection: it pings, acknowledges each
// event, joins an event split across frames, answers it once, and stops when
// the channel is turned off.
func TestLarkAnswersOverLongConnection(t *testing.T) {
	t.Setenv("GOSO_LITE", "")
	st := larkStore(t, nil)
	api := newFakeLark(t)
	ev := larkEventJSON("om_1", "p2p", "báo giá gói A", false)
	api.events = [][]larkFrame{eventFrames("om_1", ev, 2), eventFrames("om_1b", ev, 1)}
	m := NewManager()
	m.Store = st
	m.newLark = func(name string) *Lark {
		return &Lark{Instance: name, Store: st, LLM: llm.Echo{}, APIBase: api.srv.URL}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.StartAll(ctx)
	defer m.StopAll()

	waitFor(t, "reply and acks", func() bool {
		replies, frames, _ := api.got()
		acks := 0
		for _, f := range frames {
			if f.Method == larkFrameData {
				acks++
			}
		}
		return len(replies) == 1 && acks == 2
	})
	replies, frames, _ := api.got()
	if replies[0]["message_id"] != "om_1" || !strings.Contains(replies[0]["text"], "báo giá gói A") {
		t.Fatalf("reply %v", replies[0])
	}
	var pinged bool
	for _, f := range frames {
		if f.Method == larkFrameControl && f.header("type") == "ping" && f.Service == 7 {
			pinged = true
		}
		if f.Method == larkFrameData && (string(f.Payload) != `{"code":200,"headers":null,"data":null}` || f.header("biz_rt") == "") {
			t.Fatalf("ack %+v", f)
		}
	}
	if !pinged {
		t.Fatal("no ping")
	}
	waitFor(t, "running", func() bool { return m.Running(larkA) })
	if row := InstanceInfo(st, m, larkA); row.Health != "running" || row.Transport != "websocket" || !row.SecretSet {
		t.Fatalf("row %+v", row)
	}
	cfg, _ := st.GetChannelConfig(larkA)
	cfg.Enabled = false
	_ = st.PutChannelConfig(*cfg)
	m.Reconcile()
	if m.Lark(larkA) != nil || m.Running(larkA) {
		t.Fatal("disabled bot still running")
	}
}

func TestLarkGroupAnswersOnlyWhenMentioned(t *testing.T) {
	st := larkStore(t, nil)
	api := newFakeLark(t)
	l := &Lark{Instance: larkA, Store: st, LLM: llm.Echo{}, APIBase: api.srv.URL, run: func(f func()) { f() }}
	if err := l.ProbeToken(context.Background()); err != nil {
		t.Fatal(err)
	}
	l.dispatchEvent(larkEventJSON("g1", "group", "ai trực hôm nay", false))
	l.dispatchEvent(larkEventJSON("g2", "group", "ai trực hôm nay", true))
	replies, _, _ := api.got()
	if len(replies) != 1 || replies[0]["message_id"] != "g2" {
		t.Fatalf("replies %v", replies)
	}
	if strings.Contains(replies[0]["text"], "@_user_1") {
		t.Fatalf("mention key reached the agent: %q", replies[0]["text"])
	}
}

func TestLarkStaysQuietWhenItShould(t *testing.T) {
	st := larkStore(t, nil)
	var sent []string
	l := &Lark{Instance: larkA, Store: st, LLM: failingLLM{}, run: func(f func()) { f() }}
	l.Sender = func(_ context.Context, _ LarkMessage, text string) error {
		sent = append(sent, text)
		return nil
	}
	// A model error never reaches the customer.
	l.dispatchEvent(larkEventJSON("e1", "p2p", "lỗi", false))
	// A bot's own message and non-text messages are not answered.
	l.LLM = llm.Echo{}
	l.dispatchEvent([]byte(strings.Replace(string(larkEventJSON("b1", "p2p", "x", false)), `"sender_type":"user"`, `"sender_type":"app"`, 1)))
	l.dispatchEvent([]byte(strings.Replace(string(larkEventJSON("i1", "p2p", "x", false)), `"message_type":"text"`, `"message_type":"image"`, 1)))
	// A disabled channel does not answer.
	cfg, _ := st.GetChannelConfig(larkA)
	cfg.Enabled = false
	_ = st.PutChannelConfig(*cfg)
	l.dispatchEvent(larkEventJSON("d1", "p2p", "tắt", false))
	if len(sent) != 0 {
		t.Fatalf("sent %v, want none", sent)
	}
}

func TestLarkRenewsStaleTokenOnReply(t *testing.T) {
	st := larkStore(t, nil)
	api := newFakeLark(t)
	api.expireOnce = true
	l := &Lark{Instance: larkA, Store: st, LLM: llm.Echo{}, APIBase: api.srv.URL, run: func(f func()) { f() }}
	l.dispatchEvent(larkEventJSON("r1", "p2p", "chào", false))
	replies, _, tokens := api.got()
	if len(replies) != 1 || tokens < 2 {
		t.Fatalf("replies %v tokens %d", replies, tokens)
	}
}

// With no domain set the app is tried on Lark, then on Feishu.
func TestLarkFindsItsDomain(t *testing.T) {
	st := larkStore(t, nil)
	api := newFakeLark(t)
	wrong := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":10003,"msg":"invalid app_id"}`))
	}))
	defer wrong.Close()
	l := &Lark{Instance: larkA, Store: st, tryHosts: []string{wrong.URL, api.srv.URL}}
	if err := l.ProbeToken(context.Background()); err != nil {
		t.Fatal(err)
	}
	if l.base != api.srv.URL || l.botID != larkBotID {
		t.Fatalf("base %q bot %q", l.base, l.botID)
	}
	l2 := &Lark{Instance: larkA, Store: larkStore(t, map[string]string{SettingAppID: larkAppID, SettingDomain: "feishu"})}
	if h := l2.hosts(); len(h) != 1 || h[0] != feishuHost {
		t.Fatalf("feishu hosts %v", h)
	}
}

func TestLarkCredentialsAndHealth(t *testing.T) {
	if got := WritableFields(larkA); len(got) != 1 || got[0] != "app_secret" {
		t.Fatalf("writable %v", got)
	}
	if k, ok := FieldKind(larkA, "app_secret"); !ok || k != KindAppSecret {
		t.Fatalf("app_secret kind %q %v", k, ok)
	}
	if got := WritableFields("feishu"); got != nil {
		t.Fatalf("legacy feishu writable %v", got)
	}
	// Without the app id setting the channel is missing its credentials.
	st := larkStore(t, map[string]string{})
	if row := InstanceInfo(st, nil, larkA); row.Health != "missing" || row.SecretSet {
		t.Fatalf("no app id row %+v", row)
	}
	// A refused secret fails the channel without showing it.
	t.Setenv("GOSO_LITE", "")
	st = larkStore(t, nil)
	_ = secrets.Put(st, SecretName(larkA, KindAppSecret), []byte("wrong-secret-xyz"))
	api := newFakeLark(t)
	m := NewManager()
	l := &Lark{Instance: larkA, Store: st, APIBase: api.srv.URL}
	l.Start(context.Background(), m)
	if !m.Failed(larkA) || strings.Contains(m.LastError(larkA), "wrong-secret-xyz") {
		t.Fatalf("failed=%v err=%q", m.Failed(larkA), m.LastError(larkA))
	}
}

func TestLarkFrameRoundTrip(t *testing.T) {
	in := larkFrame{SeqID: 3, LogID: 9, Service: 7, Method: larkFrameData, Headers: [][2]string{{"type", "event"}, {"sum", "1"}}, PayloadType: "json", Payload: []byte(`{"a":1}`), LogIDNew: "x"}
	out, err := decodeLarkFrame(in.encode())
	if err != nil {
		t.Fatal(err)
	}
	if out.SeqID != 3 || out.LogID != 9 || out.Service != 7 || out.Method != 1 || out.header("type") != "event" || string(out.Payload) != `{"a":1}` || out.LogIDNew != "x" || out.PayloadType != "json" {
		t.Fatalf("frame %+v", out)
	}
	if _, err := decodeLarkFrame([]byte{0xff, 0xff, 0xff}); err == nil {
		t.Fatal("garbage decoded")
	}
}
