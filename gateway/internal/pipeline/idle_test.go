// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// idleStores returns memory + SQLite, plus a fresh-schema Postgres store when
// GOSO_TEST_POSTGRES_DSN (or GOSO_TEST_DATABASE_URL) is set.
func idleStores(t *testing.T) map[string]store.StoreIface {
	t.Helper()
	sq, err := store.OpenSQLite(filepath.Join(t.TempDir(), "idle.db"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]store.StoreIface{"memory": store.New(), "sqlite": sq}
	dsn := strings.TrimSpace(os.Getenv("GOSO_TEST_POSTGRES_DSN"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("GOSO_TEST_DATABASE_URL"))
	}
	if dsn == "" {
		return out
	}
	schema := fmt.Sprintf("idleflush_%d", time.Now().UnixNano())
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

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newFlusher(st store.StoreIface, clk *fakeClock, p llm.Provider) *IdleFlusher {
	return &IdleFlusher{Store: st, Idle: 10 * time.Minute, Now: clk.Now,
		Provider: func(*store.Session) llm.Provider { return p }}
}

func recallNoteIn(msgs []llm.Message) string {
	for _, m := range msgs {
		if m.Role == "system" && strings.Contains(m.Content, recallHeader) {
			return m.Content
		}
	}
	return ""
}

// A 2-message session is remembered after idle, once, and a new session recalls it.
func TestIdleFlush_ShortSessionWrittenOnceAndRecalled(t *testing.T) {
	t.Setenv("GOSO_MEMORY_RECALL", "")
	for name, st := range idleStores(t) {
		t.Run(name, func(t *testing.T) {
			ag, _ := st.CreateAgent(store.Agent{AgentKey: "sale-" + name})
			s1, _ := st.CreateSession(store.Session{AgentID: ag.ID})
			s2, _ := st.CreateSession(store.Session{AgentID: ag.ID})
			empty, _ := st.CreateSession(store.Session{AgentID: ag.ID})
			_ = empty

			chat := &llm.Scripted{Replies: []llm.Reply{{Text: "dạ em ghi nhận"}}, RepeatLast: true}
			r := NewRunner(st, nil, chat, nil)
			if _, err := r.Run(context.Background(), s1.ID, "Đơn SOFA-7731 sofa gỗ sồi màu walnut, giao 15/11", ModeFull); err != nil {
				t.Fatal(err)
			}
			if sum, _ := st.LatestSummary(s1.ID); sum != nil {
				t.Fatalf("2 messages must not hit the 12 threshold: %+v", sum)
			}

			summarizer := &llm.Scripted{Label: "fake-llm", Replies: []llm.Reply{{Text: "LLM SUMMARY"}}, RepeatLast: true}
			clk := &fakeClock{t: time.Now().UTC()}
			f := newFlusher(st, clk, summarizer)

			clk.advance(time.Minute)
			if n := f.Sweep(context.Background()); n != 0 {
				t.Fatalf("not idle yet, wrote %d", n)
			}
			clk.advance(10 * time.Minute)
			if n := f.Sweep(context.Background()); n != 1 {
				t.Fatalf("idle sweep wrote %d, want 1", n)
			}
			sum, err := st.LatestSummary(s1.ID)
			if err != nil || sum == nil || !strings.Contains(sum.Body, "SOFA-7731") {
				t.Fatalf("note missing: %+v %v", sum, err)
			}
			if len(summarizer.Recorded) != 0 {
				t.Fatal("short session must use the compact note, not the LLM")
			}
			if n := f.Sweep(context.Background()); n != 0 {
				t.Fatalf("second sweep wrote %d, want 0", n)
			}
			// Restart: a fresh flusher re-derives state from the DB.
			clk.advance(5 * time.Minute)
			if n := newFlusher(st, clk, summarizer).Sweep(context.Background()); n != 0 {
				t.Fatalf("after restart wrote %d, want 0", n)
			}
			mems, _ := st.ListMemories(s1.ID)
			ep := 0
			for _, m := range mems {
				if m.Kind == store.KindEpisodic {
					ep++
				}
			}
			if ep != 1 {
				t.Fatalf("want exactly 1 episodic note, got %d", ep)
			}
			if s, _ := st.LatestSummary(empty.ID); s != nil {
				t.Fatal("empty session must not get a note")
			}

			if _, err := r.Run(context.Background(), s2.ID, "Đơn SOFA-7731 sofa màu gì, giao ngày nào?", ModeFull); err != nil {
				t.Fatal(err)
			}
			if note := recallNoteIn(chat.Recorded[len(chat.Recorded)-1]); !strings.Contains(note, "walnut") {
				t.Fatalf("new session must recall the idle note: %q", note)
			}
		})
	}
}

// New messages after a flush make the session dirty again; the note is refreshed once.
func TestIdleFlush_NewBatchRefreshesNote(t *testing.T) {
	for name, st := range idleStores(t) {
		t.Run(name, func(t *testing.T) {
			ag, _ := st.CreateAgent(store.Agent{AgentKey: "a-" + name})
			s, _ := st.CreateSession(store.Session{AgentID: ag.ID})
			_, _ = st.AddMessage(store.Message{SessionID: s.ID, Role: "user", Content: "mã đơn KIWI-4821"})
			clk := &fakeClock{t: time.Now().UTC().Add(11 * time.Minute)}
			f := newFlusher(st, clk, nil)
			if n := f.Sweep(context.Background()); n != 1 {
				t.Fatalf("first flush %d", n)
			}
			time.Sleep(2 * time.Millisecond)
			_, _ = st.AddMessage(store.Message{SessionID: s.ID, Role: "user", Content: "đổi sang giao thứ bảy"})
			clk.t = time.Now().UTC().Add(time.Minute)
			if n := f.Sweep(context.Background()); n != 0 {
				t.Fatalf("new message is not idle yet, wrote %d", n)
			}
			clk.t = time.Now().UTC().Add(11 * time.Minute)
			if n := f.Sweep(context.Background()); n != 1 {
				t.Fatalf("refresh flush %d", n)
			}
			sum, _ := st.LatestSummary(s.ID)
			if sum == nil || !strings.Contains(sum.Body, "KIWI-4821") || !strings.Contains(sum.Body, "thứ bảy") {
				t.Fatalf("refreshed note: %+v", sum)
			}
			if n := f.Sweep(context.Background()); n != 0 {
				t.Fatalf("duplicate flush %d", n)
			}
		})
	}
}

func TestIdleFlush_SkipsNoUserTextChannelArchivedAndOld(t *testing.T) {
	for name, st := range idleStores(t) {
		t.Run(name, func(t *testing.T) {
			ag, _ := st.CreateAgent(store.Agent{AgentKey: "s-" + name})
			botOnly, _ := st.CreateSession(store.Session{AgentID: ag.ID})
			_, _ = st.AddMessage(store.Message{SessionID: botOnly.ID, Role: "assistant", Content: "xin chào"})
			blank, _ := st.CreateSession(store.Session{AgentID: ag.ID})
			_, _ = st.AddMessage(store.Message{SessionID: blank.ID, Role: "user", Content: "   "})
			ch, _ := st.CreateSession(store.Session{AgentID: ag.ID, Label: "zalo-personal:123"})
			_, _ = st.AddMessage(store.Message{SessionID: ch.ID, Role: "user", Content: "số điện thoại của tôi"})
			arch, _ := st.CreateSession(store.Session{AgentID: ag.ID})
			_, _ = st.AddMessage(store.Message{SessionID: arch.ID, Role: "user", Content: "phiên đã lưu trữ"})
			arch.Archived, arch.TouchArchive = true, true
			if _, err := st.UpdateSession(*arch); err != nil {
				t.Fatal(err)
			}
			clk := &fakeClock{t: time.Now().UTC().Add(11 * time.Minute)}
			if n := newFlusher(st, clk, nil).Sweep(context.Background()); n != 0 {
				t.Fatalf("wrote %d, want 0", n)
			}
			// Past the lookback window nothing is touched either.
			live, _ := st.CreateSession(store.Session{AgentID: ag.ID})
			_, _ = st.AddMessage(store.Message{SessionID: live.ID, Role: "user", Content: "cũ lắm rồi"})
			clk.t = time.Now().UTC().Add(48 * time.Hour)
			if n := newFlusher(st, clk, nil).Sweep(context.Background()); n != 0 {
				t.Fatalf("outside lookback wrote %d", n)
			}
		})
	}
}

// Longer sessions go through the LLM summary, like SummarizeStage.
func TestIdleFlush_LongSessionUsesLLM(t *testing.T) {
	st := store.New()
	ag, _ := st.CreateAgent(store.Agent{AgentKey: "long"})
	s, _ := st.CreateSession(store.Session{AgentID: ag.ID})
	for i := 0; i < 4; i++ {
		_, _ = st.AddMessage(store.Message{SessionID: s.ID, Role: "user", Content: fmt.Sprintf("câu hỏi số %d về sofa", i)})
		_, _ = st.AddMessage(store.Message{SessionID: s.ID, Role: "assistant", Content: "trả lời"})
	}
	p := &llm.Scripted{Label: "fake-llm", Replies: []llm.Reply{{Text: "Khách hỏi 4 câu về sofa."}}}
	clk := &fakeClock{t: time.Now().UTC().Add(11 * time.Minute)}
	if n := newFlusher(st, clk, p).Sweep(context.Background()); n != 1 {
		t.Fatalf("wrote %d", n)
	}
	if len(p.Recorded) != 1 {
		t.Fatalf("want 1 LLM call, got %d", len(p.Recorded))
	}
	if sum, _ := st.LatestSummary(s.ID); sum == nil || sum.Body != "Khách hỏi 4 câu về sofa." {
		t.Fatalf("note: %+v", sum)
	}
}

// Regression: the 12-message threshold and summarize=1 still write on the turn,
// and the idle sweep does not rewrite a note that already covers the session.
func TestSummarize_ThresholdAndForceUnchanged(t *testing.T) {
	for name, st := range idleStores(t) {
		t.Run(name, func(t *testing.T) {
			ag, _ := st.CreateAgent(store.Agent{AgentKey: "th-" + name})
			s, _ := st.CreateSession(store.Session{AgentID: ag.ID})
			r := NewRunner(st, nil, &llm.Scripted{Replies: []llm.Reply{{Text: "ok"}}, RepeatLast: true}, nil)
			for i := 1; i <= 6; i++ {
				if _, err := r.Run(context.Background(), s.ID, fmt.Sprintf("tin số %d", i), ModeFull); err != nil {
					t.Fatal(err)
				}
				sum, _ := st.LatestSummary(s.ID)
				if i < 6 && sum != nil {
					t.Fatalf("turn %d (%d msgs) wrote a note before the threshold", i, 2*i)
				}
				if i == 6 && sum == nil {
					t.Fatalf("turn 6 (%d msgs) must write the note", SummarizeMessageMin)
				}
			}
			clk := &fakeClock{t: time.Now().UTC().Add(11 * time.Minute)}
			if n := newFlusher(st, clk, nil).Sweep(context.Background()); n != 0 {
				t.Fatalf("idle sweep rewrote a covered note: %d", n)
			}

			s2, _ := st.CreateSession(store.Session{AgentID: ag.ID})
			r.ForceSummarize = true
			if _, err := r.Run(context.Background(), s2.ID, "summarize=1 vẫn ghi", ModeFull); err != nil {
				t.Fatal(err)
			}
			if sum, _ := st.LatestSummary(s2.ID); sum == nil {
				t.Fatal("summarize=1 must still write on the turn")
			}
		})
	}
}

func TestIdleFlushAfter_Env(t *testing.T) {
	cases := map[string]time.Duration{
		"": IdleFlushDefault, "0": 0, "off": 0, "15": 15 * time.Minute,
		"90s": 90 * time.Second, "2h": 2 * time.Hour, "junk": IdleFlushDefault, "-5": 0,
	}
	for v, want := range cases {
		t.Setenv("GOSO_MEMORY_IDLE_FLUSH", v)
		if got := IdleFlushAfter(); got != want {
			t.Fatalf("%q: got %s want %s", v, got, want)
		}
	}
}

func TestIdleFlushLoop_StopsWithContext(t *testing.T) {
	t.Setenv("GOSO_MEMORY_IDLE_FLUSH", "2s")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { IdleFlushLoop(ctx, store.New(), nil); close(done) }()
	time.Sleep(1200 * time.Millisecond) // at least one sweep
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not stop")
	}
	t.Setenv("GOSO_MEMORY_IDLE_FLUSH", "0")
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	finished := make(chan struct{})
	go func() { IdleFlushLoop(ctx2, store.New(), nil); close(finished) }()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("GOSO_MEMORY_IDLE_FLUSH=0 must return immediately")
	}
}
