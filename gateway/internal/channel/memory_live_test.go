// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mqglobal/goso/gateway/internal/guardrail"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Fake customers only: thread ids and notes below are fixtures, nothing is
// sent anywhere (Sender is a stub, the LLM is a recorder).

// recorder is an llm.Provider that keeps every prompt it was asked.
type recorder struct {
	mu    sync.Mutex
	calls [][]llm.Message
}

func (r *recorder) Name() string { return "recorder" }
func (r *recorder) Chat(_ context.Context, msgs []llm.Message) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, append([]llm.Message(nil), msgs...))
	return "ok", nil
}
func (r *recorder) last(t *testing.T) []llm.Message {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.calls) == 0 {
		t.Fatal("LLM never called")
	}
	return r.calls[len(r.calls)-1]
}

// systemOf asserts at most one system message, first, and returns it ("" if none).
func systemOf(t *testing.T, msgs []llm.Message) string {
	t.Helper()
	sys := ""
	for i, m := range msgs {
		if m.Role != "system" {
			continue
		}
		if i != 0 || sys != "" {
			t.Fatalf("system message must be single and first: %+v", msgs)
		}
		sys = m.Content
	}
	return sys
}

// liveStores is SQLite, plus a fresh Postgres schema when
// GOSO_TEST_POSTGRES_DSN (or GOSO_TEST_DATABASE_URL) is set.
func liveStores(t *testing.T) map[string]store.StoreIface {
	t.Helper()
	sq, err := store.OpenSQLite(filepath.Join(t.TempDir(), "chanmem.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sq.Close() })
	// The in-memory store lists sessions in map order, so "the live session
	// of this thread" is not deterministic once a second session shares its
	// label; production runs SQLite or Postgres (ORDER BY created_at).
	out := map[string]store.StoreIface{"sqlite": sq}
	dsn := strings.TrimSpace(os.Getenv("GOSO_TEST_POSTGRES_DSN"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("GOSO_TEST_DATABASE_URL"))
	}
	if dsn == "" {
		return out
	}
	schema := fmt.Sprintf("chanmem_%d", time.Now().UnixNano())
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
		_ = db.Close()
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("options", "-csearch_path="+schema)
	u.RawQuery = q.Encode()
	pg, err := store.OpenPostgres(u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pg.Close() })
	out["postgres"] = pg
	return out
}

func zaloSend(t *testing.T, z *ZaloPersonal, thread, text string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"thread_id": thread, "message": map[string]any{"text": text}})
	w := httptest.NewRecorder()
	z.HandleUpdate(w, httptest.NewRequest("POST", "/api/channels/zalo-personal/webhook", bytes.NewReader(body)))
	if w.Code != 200 {
		t.Fatalf("status %d %s", w.Code, w.Body.String())
	}
}

// seedNote stores note under a separate session with label (a prior session
// of that customer) and returns the session.
func seedNote(t *testing.T, st store.StoreIface, tenant, agentID, label, note string) {
	t.Helper()
	sess, err := st.CreateSession(store.Session{TenantID: tenant, AgentID: agentID, Label: label})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveSummary(sess.ID, note); err != nil {
		t.Fatal(err)
	}
}

const (
	noteA = "Khách A đặt bàn ghế gỗ sồi, mã đơn ZA-7777, giao Quận 7"
	noteB = "Khách B hỏi bảo hành máy lọc nước, mã đơn ZB-8888, ở Thủ Đức"
)

func TestChannelLiveReplyRecallsOnlyOwnCustomer(t *testing.T) {
	for name, st := range liveStores(t) {
		t.Run(name, func(t *testing.T) {
			_ = st.PutChannelConfig(store.ChannelConfig{Name: "zalo-personal", DMPolicy: "open"})
			rec := &recorder{}
			z := &ZaloPersonal{Store: st, LLM: rec, Sender: func(context.Context, string, string) error { return nil }}

			// First turns create the agent and both live sessions.
			zaloSend(t, z, "fakeA", "xin chào")
			zaloSend(t, z, "fakeB", "xin chào")
			agent := z.ensureAgent()
			agent.Instructions = "Bạn là trợ lý bán hàng."
			if _, err := st.UpdateAgent(*agent); err != nil {
				t.Fatal(err)
			}
			tenant := z.ensureSession(agent.ID, "fakeA").TenantID
			seedNote(t, st, tenant, agent.ID, "zalo-personal:fakeA", noteA)
			seedNote(t, st, tenant, agent.ID, "zalo-personal:fakeB", noteB)
			// Same thread label in another tenant: must never be recalled.
			other, err := st.CreateAgent(store.Agent{TenantID: "tenant-other", AgentKey: "zalo-personal-other", DisplayName: "Zalo Personal"})
			if err != nil {
				t.Fatal(err)
			}
			seedNote(t, st, "tenant-other", other.ID, "zalo-personal:fakeA", "Ghi chú tenant khác mã đơn ZA-7777 Quận 7 bàn ghế gỗ sồi")

			zaloSend(t, z, "fakeA", "đơn bàn ghế gỗ sồi ZA-7777 giao Quận 7 tới đâu rồi?")
			sys := systemOf(t, rec.last(t))
			if !strings.Contains(sys, noteA) {
				t.Fatalf("A's note missing from A's reply prompt:\n%s", sys)
			}
			if strings.Contains(sys, "ZB-8888") || strings.Contains(sys, "tenant khác") {
				t.Fatalf("leak into A's prompt:\n%s", sys)
			}
			if !strings.HasPrefix(sys, "Bạn là trợ lý bán hàng.") || strings.Index(sys, noteA) < len("Bạn là trợ lý bán hàng.") {
				t.Fatalf("order must be instructions then memory:\n%s", sys)
			}

			zaloSend(t, z, "fakeB", "máy lọc nước ZB-8888 bảo hành Thủ Đức thế nào?")
			sys = systemOf(t, rec.last(t))
			if !strings.Contains(sys, noteB) || strings.Contains(sys, "ZA-7777") {
				t.Fatalf("B's prompt wrong:\n%s", sys)
			}
			// B asking about A's order still never sees A's note.
			zaloSend(t, z, "fakeB", "đơn bàn ghế gỗ sồi ZA-7777 giao Quận 7 tới đâu rồi?")
			if sys := systemOf(t, rec.last(t)); strings.Contains(sys, noteA) || strings.Contains(sys, "tenant khác") {
				t.Fatalf("A leaked into B:\n%s", sys)
			}
		})
	}
}

// failingRecall breaks only recall.
type failingRecall struct{ store.StoreIface }

func (failingRecall) RecallMemories(store.RecallQuery) ([]store.RecallHit, error) {
	return nil, errors.New("recall down")
}

func TestChannelLiveReplySurvivesRecallError(t *testing.T) {
	base := store.New()
	st := failingRecall{base}
	_ = st.PutChannelConfig(store.ChannelConfig{Name: "zalo-personal", DMPolicy: "open"})
	rec := &recorder{}
	var sent string
	z := &ZaloPersonal{Store: st, LLM: rec, Sender: func(_ context.Context, _, text string) error { sent = text; return nil }}
	zaloSend(t, z, "fakeA", "đơn bàn ghế gỗ sồi ZA-7777 giao Quận 7 tới đâu rồi?")
	if sent != "ok" {
		t.Fatalf("reply not sent: %q", sent)
	}
	msgs := rec.last(t)
	// No instructions and a failed recall: the system message is just the
	// no-invention guardrail, then the thread.
	if sys := systemOf(t, msgs); sys != guardrail.Reply {
		t.Fatalf("no instructions, failed recall: want only the guardrail, got %q", sys)
	}
	if len(msgs) != 2 || msgs[1].Role != "user" {
		t.Fatalf("prompt changed: %+v", msgs)
	}
}

// The generic inbound path (slack, whatsapp, feishu, discord) and telegram
// share answerTurn; check slack-style labels recall per customer too.
func TestReplyInboundRecallsPerCustomer(t *testing.T) {
	st := liveStores(t)["sqlite"]
	rec := &recorder{}
	d := inboundDeps{Store: st, LLM: rec}
	run := func(dest, text string) {
		w := httptest.NewRecorder()
		replyInbound(w, httptest.NewRequest("POST", "/x", nil), d, "slack", "Slack", dest, text)
		if w.Code != 200 {
			t.Fatalf("status %d", w.Code)
		}
	}
	run("C-fakeA", "xin chào")
	agent := ensureNamedAgent(st, "slack", "Slack")
	seedNote(t, st, "", agent.ID, "slack:C-fakeA", noteA)
	seedNote(t, st, "", agent.ID, "slack:C-fakeB", noteB)
	run("C-fakeA", "đơn bàn ghế gỗ sồi ZA-7777 giao Quận 7 tới đâu rồi?")
	sys := systemOf(t, rec.last(t))
	if !strings.Contains(sys, noteA) || strings.Contains(sys, noteB) {
		t.Fatalf("slack A prompt:\n%s", sys)
	}
	run("C-fakeB", "đơn bàn ghế gỗ sồi ZA-7777 giao Quận 7 tới đâu rồi?")
	if sys := systemOf(t, rec.last(t)); strings.Contains(sys, noteA) {
		t.Fatalf("slack A leaked into B:\n%s", sys)
	}
}
