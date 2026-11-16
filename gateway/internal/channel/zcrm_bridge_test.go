// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// fakeCRM is ZaloCRM's reply endpoint. It checks each reply's signature with
// the key it was given and keeps the replies.
type fakeCRM struct {
	srv    *httptest.Server
	secret string
	status int

	mu      sync.Mutex
	replies []map[string]any
	badSig  int
}

func newFakeCRM(t *testing.T, secret string) *fakeCRM {
	t.Helper()
	f := &fakeCRM{secret: secret, status: http.StatusOK}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.URL.Path != zcrmReplyPath || r.Header.Get("Content-Type") != zcrmContentType {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		want := ZCRMSign(f.secret, r.Header.Get(zcrmTimestampHeader), r.Header.Get(zcrmNonceHeader), body)
		if r.Header.Get(zcrmSignatureHeader) != want || r.Header.Get(zcrmVersionHeader) != "2" {
			f.badSig++
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var m map[string]any
		_ = json.Unmarshal(body, &m)
		f.replies = append(f.replies, m)
		w.WriteHeader(f.status)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeCRM) got() ([]map[string]any, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]map[string]any(nil), f.replies...), f.badSig
}

type failingLLM struct{}

func (failingLLM) Name() string { return "failing" }
func (failingLLM) Chat(context.Context, []llm.Message) (string, error) {
	return "", errors.New("provider down")
}

type bridgeFixture struct {
	st     store.StoreIface
	bridge *ZCRMBridge
	crm    *fakeCRM
	now    time.Time
	agentA *store.Agent
}

const (
	bridgeA = "tenant-a/zalo-personal/shop"
	bridgeB = "tenant-b/zalo-personal/shop"
	keyA    = "bridge-key-a"
	keyB    = "bridge-key-b"
)

func newBridgeFixture(t *testing.T) *bridgeFixture {
	t.Helper()
	key, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", key)
	st := store.New()
	crm := newFakeCRM(t, keyA)
	agentA, _ := st.CreateAgent(store.Agent{AgentKey: "minh-an", DisplayName: "Minh An", TenantID: "tenant-a"})
	agentB, _ := st.CreateAgent(store.Agent{AgentKey: "minh-an-b", DisplayName: "Minh An B", TenantID: "tenant-b"})
	for _, c := range []struct{ name, acc, key, agent string }{
		{bridgeA, "acc-a", keyA, agentA.ID},
		{bridgeB, "acc-b", keyB, agentB.ID},
	} {
		if err := st.PutChannelConfig(store.ChannelConfig{
			Name: c.name, Enabled: true, AgentID: c.agent,
			Settings: map[string]string{SettingAccountID: c.acc, SettingCRMBaseURL: crm.srv.URL},
		}); err != nil {
			t.Fatal(err)
		}
		if err := secrets.Put(st, SecretName(c.name, KindBridge), []byte(c.key)); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	b := &ZCRMBridge{Store: st, LLM: llm.Echo{}, HTTPClient: crm.srv.Client()}
	b.now = func() time.Time { return now }
	b.run = func(f func()) { f() }
	return &bridgeFixture{st: st, bridge: b, crm: crm, now: now, agentA: agentA}
}

type bridgeCall struct {
	account, conv, text, thread, agentKey, key, nonce string
	at                                                time.Time
}

func (fx *bridgeFixture) post(t *testing.T, c bridgeCall) *httptest.ResponseRecorder {
	t.Helper()
	if c.account == "" {
		c.account = "acc-a"
	}
	if c.conv == "" {
		c.conv = "conv-1"
	}
	if c.key == "" {
		c.key = keyA
	}
	if c.nonce == "" {
		c.nonce = "nonce-" + c.conv + "-" + c.text
	}
	if c.at.IsZero() {
		c.at = fx.now
	}
	lease := int64(7)
	ev := map[string]any{
		"eventId": "zcrm:m1", "event": "message.received", "timestamp": c.at.Format(time.RFC3339),
		"data": map[string]any{
			"accountId": c.account, "channel": "zalo", "messageId": "m-" + c.text, "conversationId": c.conv,
			"senderUid": "u1", "senderName": "Khách", "content": c.text, "contentType": "text",
			"threadType": c.thread, "agentLease": lease, "agentKey": c.agentKey,
		},
	}
	body, _ := json.Marshal(ev)
	ts := strconv.FormatInt(c.at.UnixMilli(), 10)
	req := httptest.NewRequest(http.MethodPost, ZCRMBridgePath, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", zcrmContentType)
	req.Header.Set(zcrmVersionHeader, "2")
	req.Header.Set(zcrmTimestampHeader, ts)
	req.Header.Set(zcrmNonceHeader, c.nonce)
	req.Header.Set(zcrmSignatureHeader, ZCRMSign(c.key, ts, c.nonce, body))
	w := httptest.NewRecorder()
	fx.bridge.HandleUpdate(w, req)
	return w
}

func TestZCRMBridgeAnswersThroughCRM(t *testing.T) {
	fx := newBridgeFixture(t)
	if w := fx.post(t, bridgeCall{text: "chào shop"}); w.Code != http.StatusAccepted {
		t.Fatalf("bridge call %d %s", w.Code, w.Body.String())
	}
	replies, bad := fx.crm.got()
	if bad != 0 || len(replies) != 1 {
		t.Fatalf("replies %v bad signatures %d", replies, bad)
	}
	r := replies[0]
	if r["conversationId"] != "conv-1" || r["idempotencyKey"] != "goso:m-chào shop" || r["agentLease"] != float64(7) {
		t.Fatalf("reply %v", r)
	}
	if c, _ := r["content"].(string); !strings.Contains(c, "chào shop") {
		t.Fatalf("reply content %q", r["content"])
	}
	// The turn is kept in the bound agent's session for this conversation.
	var found bool
	for _, s := range fx.st.ListSessions() {
		if s.AgentID == fx.agentA.ID && s.Label == bridgeA+":conv-1" && s.TenantID == "tenant-a" {
			found = true
		}
	}
	if !found {
		t.Fatal("no session for the conversation")
	}
}

func TestZCRMBridgeRefusesWrongKeysAndReplays(t *testing.T) {
	fx := newBridgeFixture(t)
	cases := []struct {
		name string
		call bridgeCall
		code int
	}{
		{"another tenant's key", bridgeCall{text: "a", key: keyB}, http.StatusUnauthorized},
		{"unknown account", bridgeCall{text: "b", account: "acc-x"}, http.StatusUnauthorized},
		{"old call", bridgeCall{text: "c", at: fx.now.Add(-10 * time.Minute)}, http.StatusUnauthorized},
	}
	for _, tc := range cases {
		if w := fx.post(t, tc.call); w.Code != tc.code {
			t.Fatalf("%s: %d %s", tc.name, w.Code, w.Body.String())
		}
	}
	first := bridgeCall{text: "d", nonce: "same"}
	if w := fx.post(t, first); w.Code != http.StatusAccepted {
		t.Fatalf("first %d", w.Code)
	}
	if w := fx.post(t, first); w.Code != http.StatusConflict {
		t.Fatalf("replay %d", w.Code)
	}
	if replies, _ := fx.crm.got(); len(replies) != 1 {
		t.Fatalf("replies %d, want only the first call answered", len(replies))
	}
}

func TestZCRMBridgeStaysQuietWhenItShould(t *testing.T) {
	fx := newBridgeFixture(t)
	// Groups are off unless the channel says so.
	if w := fx.post(t, bridgeCall{text: "nhóm", thread: "group"}); w.Code != http.StatusOK {
		t.Fatalf("group %d", w.Code)
	}
	// A model error never reaches the customer.
	fx.bridge.LLM = failingLLM{}
	if w := fx.post(t, bridgeCall{text: "lỗi"}); w.Code != http.StatusAccepted {
		t.Fatalf("llm error call %d", w.Code)
	}
	fx.bridge.LLM = llm.Echo{}
	// A disabled channel tells ZaloCRM so instead of dropping the message.
	cfg, _ := fx.st.GetChannelConfig(bridgeA)
	cfg.Enabled = false
	_ = fx.st.PutChannelConfig(*cfg)
	if w := fx.post(t, bridgeCall{text: "tắt"}); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("disabled %d", w.Code)
	}
	if replies, _ := fx.crm.got(); len(replies) != 0 {
		t.Fatalf("replies %v, want none", replies)
	}
	// With groups on, a group message is answered.
	cfg.Enabled = true
	cfg.Settings[SettingGroupEnabled] = "true"
	_ = fx.st.PutChannelConfig(*cfg)
	if w := fx.post(t, bridgeCall{text: "nhóm 2", thread: "group"}); w.Code != http.StatusAccepted {
		t.Fatalf("group on %d", w.Code)
	}
	if replies, _ := fx.crm.got(); len(replies) != 1 {
		t.Fatalf("replies %d", len(replies))
	}
}

func TestZCRMBridgeIgnoresAgentOfAnotherTenant(t *testing.T) {
	fx := newBridgeFixture(t)
	if w := fx.post(t, bridgeCall{text: "hỏi", agentKey: "minh-an-b"}); w.Code != http.StatusAccepted {
		t.Fatalf("call %d", w.Code)
	}
	for _, s := range fx.st.ListSessions() {
		if s.TenantID == "tenant-b" {
			t.Fatal("tenant-b's agent answered tenant-a's customer")
		}
	}
}

func TestZCRMSelfAuth(t *testing.T) {
	for _, tc := range []struct {
		method, path string
		sig          bool
		want         bool
	}{
		{http.MethodPost, ZCRMBridgePath, true, true},
		{http.MethodPost, ZCRMBridgePath, false, false},
		{http.MethodGet, ZCRMBridgePath, true, false},
		{http.MethodPost, ZCRMBridgePath + "/x", true, false},
		{http.MethodPost, "/api/channels", true, false},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		if tc.sig {
			r.Header.Set(zcrmSignatureHeader, "x")
		}
		if got := ZCRMSelfAuth(r); got != tc.want {
			t.Fatalf("%s %s sig=%v: %v", tc.method, tc.path, tc.sig, got)
		}
	}
}

func TestBridgeChannelHealth(t *testing.T) {
	fx := newBridgeFixture(t)
	if row := InstanceInfo(fx.st, nil, bridgeA); row.Health != "running" || row.Transport != "bridge" || !row.SecretSet {
		t.Fatalf("health %+v", row)
	}
	_ = secrets.Delete(fx.st, SecretName(bridgeA, KindBridge))
	if row := InstanceInfo(fx.st, nil, bridgeA); row.Health != "missing" {
		t.Fatalf("no key health %+v", row)
	}
	if got := WritableFields(bridgeA); len(got) != 1 || got[0] != "bridge_secret" {
		t.Fatalf("writable %v", got)
	}
	if got := WritableFields("zalo-personal"); got != nil {
		t.Fatalf("legacy zalo-personal writable %v", got)
	}
}
