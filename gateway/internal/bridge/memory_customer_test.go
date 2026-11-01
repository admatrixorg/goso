// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package bridge

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/pipeline"
	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Fake customers only: tenants, accounts, conversations and notes below are
// fixtures; the LLM is fakeLLM and nothing leaves the test process.

const (
	tenantOne = "00000000-0000-0000-0000-0000000000a1"
	tenantTwo = "00000000-0000-0000-0000-0000000000b2"
	memAgent  = "agent-cust"
)

// customerStores is SQLite plus Postgres fresh + upgrade schemas when
// GOSO_TEST_POSTGRES_DSN is set. (The in-memory store lists sessions in map
// order, which makes "the live session of a label" ambiguous once the test
// seeds a second one; production runs SQLite or Postgres.)
func customerStores(t *testing.T) map[string]store.StoreIface {
	t.Helper()
	sq, err := store.OpenSQLite(filepath.Join(t.TempDir(), "bridge-mem.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sq.Close() })
	out := map[string]store.StoreIface{"sqlite": sq}
	base := strings.TrimSpace(os.Getenv("GOSO_TEST_POSTGRES_DSN"))
	if base == "" {
		base = strings.TrimSpace(os.Getenv("GOSO_TEST_DATABASE_URL"))
	}
	if base == "" {
		return out
	}
	for _, tc := range []struct {
		name string
		old  bool
	}{{"postgres-fresh", false}, {"postgres-upgrade", true}} {
		schema := "goso_bridge_mem_" + strings.TrimPrefix(tc.name, "postgres-")
		dsn := bridgeSchema(t, base, schema, tc.old)
		t.Cleanup(func() {
			if db, err := sql.Open("pgx", base); err == nil {
				_, _ = db.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
				_ = db.Close()
			}
		})
		pg, err := store.OpenPostgres(dsn)
		if err != nil {
			t.Fatal(err)
		}
		// Upgrade path runs the migrations twice: reopening must be a no-op.
		_ = pg.Close()
		if pg, err = store.OpenPostgres(dsn); err != nil {
			t.Fatalf("%s reopen: %v", tc.name, err)
		}
		p := pg
		t.Cleanup(func() { _ = p.Close() })
		out[tc.name] = pg
	}
	return out
}

type custEnv struct {
	t    *testing.T
	svc  *Service
	fake *fakeLLM
	base string
}

func newCustEnv(t *testing.T, st store.StoreIface) *custEnv {
	t.Helper()
	t.Setenv("GOSO_SSRF", "0")
	t.Setenv("GOSO_ENV", "demo")
	t.Setenv("GOSO_LLM_API_KEY", "")
	t.Setenv("GOSO_MEMORY_RECALL", "")
	t.Setenv("GOSO_BRIDGE_MEMORY", "")
	t.Setenv("GOSO_KNOWLEDGE_MAX_BYTES", "64")
	master, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", master)
	fake := &fakeLLM{text: "ok"}
	cat := newMem()
	svc := &Service{Secret: "test-secret", Version: "test", Catalog: cat, Store: st, Complete: fake.Complete}
	e := &custEnv{t: t, svc: svc, fake: fake, base: "http://" + startBridge(t, svc)}
	for _, ten := range []string{tenantOne, tenantTwo} {
		prov := []byte(`{"name":"p","provider_type":"custom","base_url":"http://9router:20128/v1","api_key":"fixture-key","models":["m1"],"is_active":true}`)
		if res := doSigned(t, http.MethodPut, e.base+"/v1/tenants/"+ten+"/providers/p1", svc.Secret, ten, prov); res.StatusCode != 200 {
			t.Fatalf("provider %d %s", res.StatusCode, res.Body)
		}
		agent := []byte(`{"name":"cust","harness_type":"GOSO","model_name":"m1","system_prompt":"Bạn là trợ lý bán hàng."}`)
		if res := doSigned(t, http.MethodPut, e.base+"/v1/tenants/"+ten+"/agents/"+memAgent, svc.Secret, ten, agent); res.StatusCode != 200 {
			t.Fatalf("agent %d %s", res.StatusCode, res.Body)
		}
		big := KnowledgeDoc{ID: "k1", FileName: "catalog.txt", Content: strings.Repeat("bảng giá ", 200)}
		if err := cat.ReplaceKnowledge(ten, memAgent, []KnowledgeDoc{big}); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

// send posts one Omni reply (unary or SSE) for a customer conversation and
// returns the single system message the model got.
func (e *custEnv) send(tenant, account, conv, prompt string, stream bool) string {
	e.t.Helper()
	body, _ := json.Marshal(map[string]any{
		"agent_id": memAgent, "model": "m1", "prompt": prompt,
		"metadata": map[string]string{
			"channel_type": "zalo", "channel_account_id": account, "external_conversation_id": conv,
		},
	})
	path := "/v1/chat/reply"
	if stream {
		path = "/v1/chat/reply/stream"
	}
	req, err := http.NewRequest(http.MethodPost, e.base+path, bytes.NewReader(body))
	if err != nil {
		e.t.Fatal(err)
	}
	stamp(req.Header, e.svc.Secret, tenant, body)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	raw, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 {
		e.t.Fatalf("%s %d %s", path, res.StatusCode, raw)
	}
	if stream && !strings.Contains(string(raw), "is_finished") {
		e.t.Fatalf("stream body %s", raw)
	}
	if !stream && !strings.Contains(string(raw), `"reply_text":"ok"`) {
		e.t.Fatalf("unary body %s", raw)
	}
	return singleSystem(e.t, e.fake.snapshot().Messages)
}

func singleSystem(t *testing.T, msgs []llm.Message) string {
	t.Helper()
	if len(msgs) == 0 || msgs[0].Role != "system" {
		t.Fatalf("first message must be system: %+v", msgs)
	}
	for _, m := range msgs[1:] {
		if m.Role == "system" {
			t.Fatalf("more than one system message: %+v", msgs)
		}
	}
	return msgs[0].Content
}

func (e *custEnv) session(label string) *store.Session {
	e.t.Helper()
	for _, s := range e.svc.Store.ListSessions() {
		if s != nil && s.Label == label {
			return s
		}
	}
	e.t.Fatalf("no session %q", label)
	return nil
}

func TestBridgeCustomerMemory(t *testing.T) {
	for name, st := range customerStores(t) {
		t.Run(name, func(t *testing.T) {
			e := newCustEnv(t, st)
			// Same channel account and the same external conversation id in two
			// tenants on purpose: only the tenant keeps them apart.
			e.send(tenantOne, "acc-1", "conv-A", "Tôi là Lan, đặt 20 bộ bàn ghế gỗ sồi mã ZA-7777", false)
			e.send(tenantOne, "acc-1", "conv-B", "Tôi là Minh, hỏi bảo hành máy lọc nước mã ZB-8888", false)
			e.send(tenantTwo, "acc-1", "conv-A", "Tôi là Hùng ở tenant hai, mã đơn TX-9999", false)

			labelA := customerLabel(ReplyInput{TenantID: tenantOne, Metadata: map[string]string{
				"channel_type": "zalo", "channel_account_id": "acc-1", "external_conversation_id": "conv-A"}})
			if !strings.HasPrefix(labelA, store.OmniCustomerPrefix) || store.CustomerKeyForLabel(labelA) != labelA {
				t.Fatalf("label %q", labelA)
			}
			sessA := e.session(labelA)
			if sum, err := st.LatestSummary(sessA.ID); err != nil || sum == nil || !strings.Contains(sum.Body, "ZA-7777") {
				t.Fatalf("compact note for A: %+v %v", sum, err)
			}

			for _, stream := range []bool{false, true} {
				sys := e.send(tenantOne, "acc-1", "conv-A", "đơn bàn ghế của tôi sao rồi?", stream)
				if !strings.Contains(sys, "ZA-7777") || !strings.Contains(sys, "Lan") {
					t.Fatalf("stream=%t A's memory missing:\n%s", stream, sys)
				}
				if strings.Contains(sys, "ZB-8888") || strings.Contains(sys, "TX-9999") {
					t.Fatalf("stream=%t leak into A:\n%s", stream, sys)
				}
				p, k, m := strings.Index(sys, "Bạn là trợ lý bán hàng."), strings.Index(sys, "[knowledge truncated]"), strings.Index(sys, "ZA-7777")
				if p != 0 || k < 0 || m < k {
					t.Fatalf("stream=%t order must be prompt, capped knowledge, memory:\n%s", stream, sys)
				}
			}

			sys := e.send(tenantOne, "acc-1", "conv-B", "đơn bàn ghế gỗ sồi ZA-7777 của Lan sao rồi?", false)
			if !strings.Contains(sys, "ZB-8888") || strings.Contains(sys, "ZA-7777 giao") || strings.Contains(sys, "Tôi là Lan") {
				t.Fatalf("B's prompt:\n%s", sys)
			}
			sys = e.send(tenantTwo, "acc-1", "conv-A", "mã đơn ZA-7777 bàn ghế gỗ sồi của tôi?", true)
			if !strings.Contains(sys, "TX-9999") || strings.Contains(sys, "Tôi là Lan") {
				t.Fatalf("tenant two prompt:\n%s", sys)
			}

			// Cross-session recall: a prior session of customer A (same agent +
			// customer key) is recalled for A, never for B.
			prior, err := st.CreateSession(store.Session{TenantID: sessA.TenantID, AgentID: sessA.AgentID, Label: labelA})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := st.SaveSummary(prior.ID, "Lần trước Lan xin giao hàng Quận 7 vào thứ Sáu"); err != nil {
				t.Fatal(err)
			}
			if got := e.session(labelA); got.ID != sessA.ID {
				t.Fatalf("live session changed %s -> %s", sessA.ID, got.ID)
			}
			sys = e.send(tenantOne, "acc-1", "conv-A", "giao hàng Quận 7 thứ Sáu được không?", false)
			if !strings.Contains(sys, "Lần trước Lan xin giao hàng Quận 7") {
				t.Fatalf("A must recall the prior session:\n%s", sys)
			}
			sys = e.send(tenantOne, "acc-1", "conv-B", "giao hàng Quận 7 thứ Sáu được không?", false)
			if strings.Contains(sys, "Lần trước Lan") {
				t.Fatalf("prior A note leaked into B:\n%s", sys)
			}

			// GOSO_BRIDGE_MEMORY=0: nothing injected, nothing written.
			t.Setenv("GOSO_BRIDGE_MEMORY", "0")
			sys = e.send(tenantOne, "acc-1", "conv-A", "giao hàng Quận 7 thứ Sáu được không?", false)
			if strings.Contains(sys, "ZA-7777") || strings.Contains(sys, "Lần trước") || strings.Contains(sys, pipeline.PreviousSummaryPrefix) {
				t.Fatalf("memory off must not inject:\n%s", sys)
			}
			e.send(tenantOne, "acc-1", "conv-C", "Tôi là Tú, mã đơn ZC-1111", false)
			sessC := e.session(customerLabel(ReplyInput{TenantID: tenantOne, Metadata: map[string]string{
				"channel_type": "zalo", "channel_account_id": "acc-1", "external_conversation_id": "conv-C"}}))
			if sum, _ := st.LatestSummary(sessC.ID); sum != nil {
				t.Fatalf("memory off must not write: %+v", sum)
			}
		})
	}
}

type recallDown struct{ store.StoreIface }

func (recallDown) RecallMemories(store.RecallQuery) ([]store.RecallHit, error) {
	return nil, errors.New("recall down")
}

func TestBridgeCustomerMemoryRecallErrorStillReplies(t *testing.T) {
	sq, err := store.OpenSQLite(filepath.Join(t.TempDir(), "down.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sq.Close() })
	e := newCustEnv(t, recallDown{sq})
	e.send(tenantOne, "acc-1", "conv-A", "Tôi là Lan, đặt 20 bộ bàn ghế gỗ sồi mã ZA-7777", false)
	for _, stream := range []bool{false, true} {
		sys := e.send(tenantOne, "acc-1", "conv-A", "đơn bàn ghế gỗ sồi của tôi sao rồi?", stream)
		// Recall failed; the reply still went out and the session's own note
		// is still there.
		if !strings.Contains(sys, "ZA-7777") || !strings.HasPrefix(sys, "Bạn là trợ lý bán hàng.") {
			t.Fatalf("stream=%t prompt:\n%s", stream, sys)
		}
	}
}

func TestCustomerLabel(t *testing.T) {
	md := func(kv ...string) map[string]string {
		m := map[string]string{}
		for i := 0; i+1 < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return m
	}
	cases := []struct {
		name string
		in   ReplyInput
		want string
	}{
		{"none", ReplyInput{TenantID: "t"}, ""},
		{"no tenant", ReplyInput{Metadata: md("channel_account_id", "a", "external_conversation_id", "c")}, ""},
		{"no account", ReplyInput{TenantID: "t", Metadata: md("external_conversation_id", "c")}, ""},
		{"no conversation", ReplyInput{TenantID: "t", Metadata: md("channel_account_id", "a")}, ""},
		{"full", ReplyInput{TenantID: "t", Metadata: md("channel_type", "zalo", "channel_account_id", "a", "external_conversation_id", "c")}, "omni-chat:t:zalo:a:c"},
		{"grpc account field", ReplyInput{TenantID: "t", ChannelAccountID: "a", Metadata: md("external_conversation_id", "c")}, "omni-chat:t::a:c"},
		{"escaped", ReplyInput{TenantID: "t", Metadata: md("channel_account_id", "a:b", "external_conversation_id", "c")}, "omni-chat:t::a%3Ab:c"},
	}
	for _, tc := range cases {
		if got := customerLabel(tc.in); got != tc.want {
			t.Errorf("%s: %q want %q", tc.name, got, tc.want)
		}
		if tc.want != "" && !memoryScoped(tc.in) {
			t.Errorf("%s: must be memory scoped", tc.name)
		}
	}
}
