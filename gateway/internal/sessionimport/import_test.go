// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package sessionimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/agent"
	"github.com/mqglobal/goso/gateway/internal/approval"
	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

const (
	agentSrc   = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	contactSrc = "cccccccc-cccc-cccc-cccc-cccccccccccc"
)

func openStore(t *testing.T) *store.SQLiteStore {
	t.Helper()
	st, err := store.OpenSQLite(filepath.Join(t.TempDir(), "sessions.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func count(t *testing.T, st *store.SQLiteStore, q string, args ...any) int {
	t.Helper()
	db, _ := st.SQL()
	var n int
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func text(t *testing.T, st *store.SQLiteStore, q string, args ...any) string {
	t.Helper()
	db, _ := st.SQL()
	var s string
	if err := db.QueryRow(q, args...).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func seedAgent(t *testing.T, st *store.SQLiteStore, id, key, tenant string) {
	t.Helper()
	db, _ := st.SQL()
	_, err := db.Exec(`INSERT INTO agents(id, agent_key, display_name, model, llm_provider, instructions, orchestration_mode, created_at, tenant_id, enabled, updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		id, key, "Minh An sales", "", "", "", "", stubCreated, tenant, 1, stubCreated)
	if err != nil {
		t.Fatal(err)
	}
}

func seedContact(t *testing.T, st *store.SQLiteStore, id, tenant, channel, dest string) {
	t.Helper()
	db, _ := st.SQL()
	_, err := db.Exec(`INSERT INTO contacts(id, tenant_id, display, kind, channel, dest, seen_count, first_seen, last_seen, agent_id, merged, merged_into) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, tenant, "Minh An", "user", channel, dest, 1, stubCreated, stubCreated, "", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if channel == "" || dest == "" {
		return
	}
	_, err = db.Exec(`INSERT INTO contact_identifiers(tenant_id, channel, dest, contact_id, kind, permission, pos) VALUES(?,?,?,?,?,?,?)`,
		tenant, channel, dest, id, "user", "", 0)
	if err != nil {
		t.Fatal(err)
	}
}

func denyFile(t *testing.T, lines ...string) string {
	t.Helper()
	if len(lines) == 0 {
		lines = []string{"held-shop"}
	}
	path := filepath.Join(t.TempDir(), "deny.txt")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func minhOpt(t *testing.T) Options {
	t.Helper()
	return Options{Tenant: "minh-an", SourceTenant: "minh-an", DenyFile: denyFile(t)}
}

func openFixture(t *testing.T) *os.File {
	t.Helper()
	f, err := os.Open("testdata/minh-an-sessions.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func tenant(rep *Report, id string) TenantCount {
	for _, t := range rep.Tenants {
		if t.TenantID == id {
			return t
		}
	}
	return TenantCount{TenantID: id}
}

func has(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestDryRunWritesNothing(t *testing.T) {
	st := openStore(t)
	seedAgent(t, st, agentSrc, "minh-an-sales", "minh-an")
	rep, err := Import(st, openFixture(t), func() Options { o := minhOpt(t); o.DryRun = true; return o }())
	if err != nil {
		t.Fatal(err)
	}
	if !rep.DryRun {
		t.Fatal("dry_run")
	}
	got := tenant(rep, "minh-an")
	if got.New != 4 || got.Messages != 6 || got.Skipped != 2 || got.Invalid != 1 || got.Foreign != 0 {
		t.Fatalf("minh-an %+v", got)
	}
	if got.New+got.Skipped+got.NativeExists+got.IDCollision+got.Invalid+got.Foreign != got.Sessions {
		t.Fatalf("sum %+v", got)
	}
	other := tenant(rep, "ban-khac")
	if other.Foreign != 1 || other.New != 0 || other.Messages != 0 {
		t.Fatalf("ban-khac %+v", other)
	}
	if count(t, st, `SELECT COUNT(*) FROM sessions`) != 0 {
		t.Fatal("dry-run wrote a session")
	}
	if count(t, st, `SELECT COUNT(*) FROM messages`) != 0 {
		t.Fatal("dry-run wrote a message")
	}
	if count(t, st, `SELECT COUNT(*) FROM memories`) != 0 {
		t.Fatal("dry-run wrote a memory")
	}
	if count(t, st, `SELECT COUNT(*) FROM agents`) != 1 {
		t.Fatal("dry-run created an agent")
	}
	if count(t, st, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='session_import_links'`) != 0 {
		t.Fatal("dry-run created the link table")
	}
	for _, col := range []string{"summary", "metadata", "team_id", "channel", "messages.id"} {
		if !has(rep.NotImported, col) {
			t.Fatalf("missing %s in %v", col, rep.NotImported)
		}
	}
	if !has(rep.NotImported, "messages.tool_calls") || !has(rep.NotImported, "sessions.legacy_note") || !has(rep.NotImported, "messages.content.non_text") {
		t.Fatalf("extras %v", rep.NotImported)
	}
}

func TestImportMinhAnOrderAndRerun(t *testing.T) {
	st := openStore(t)
	seedAgent(t, st, agentSrc, "minh-an-sales", "minh-an")
	seedContact(t, st, contactSrc, "minh-an", "telegram", "ext-minh-an")
	rep, err := Import(st, openFixture(t), minhOpt(t))
	if err != nil {
		t.Fatal(err)
	}
	got := tenant(rep, "minh-an")
	if got.New != 4 || got.Messages != 6 || got.UnresolvedAgent != 1 || got.UnresolvedContact != 1 {
		t.Fatalf("apply %+v", got)
	}
	if count(t, st, `SELECT COUNT(*) FROM memories`) != 0 {
		t.Fatal("memory row created")
	}
	if count(t, st, `SELECT COUNT(*) FROM memory_fts WHERE session_id=?`, "s-minh-an-order") != 0 {
		t.Fatal("imported text was indexed as memory")
	}
	if count(t, st, `SELECT COUNT(*) FROM messages WHERE content LIKE '%không ghi memory%' OR content LIKE '%legacy-skip%' OR content LIKE '%skip-meta%' OR content LIKE '%example.invalid%' OR content LIKE '%lookup%' OR content LIKE '%khong ghi de%'`) != 0 {
		t.Fatal("unmapped text was stored on a message")
	}
	sess, err := st.GetSession("s-minh-an-order")
	if err != nil {
		t.Fatal(err)
	}
	if sess.TenantID != "minh-an" || sess.AgentID != agentSrc || sess.Label != "Minh An vải" || sess.ModelOverride != "demo-model" || sess.ProviderOverride != "demo" {
		t.Fatalf("session %#v", sess)
	}
	wantAt := time.Date(2026, 2, 1, 8, 0, 0, 0, time.UTC)
	if !sess.CreatedAt.Equal(wantAt) {
		t.Fatalf("session time %s", sess.CreatedAt)
	}
	msgs, err := st.ListMessages(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 3 {
		t.Fatalf("messages %d", len(msgs))
	}
	if msgs[0].Content != "Minh An hỏi giá mẫu vải." || msgs[0].Role != "user" || !msgs[0].CreatedAt.Equal(wantAt) {
		t.Fatalf("first %#v", msgs[0])
	}
	if msgs[1].Content != "Mẫu vải nằm trong kho. MAUVAI1842" || msgs[1].Role != "assistant" || msgs[1].Model != "demo-model" || msgs[1].Provider != "demo" {
		t.Fatalf("second %#v", msgs[1])
	}
	if !msgs[1].CreatedAt.Equal(wantAt.Add(time.Nanosecond)) {
		t.Fatalf("tie stamp %s", msgs[1].CreatedAt)
	}
	if msgs[2].Content != "Cảm ơn." || !msgs[2].CreatedAt.Equal(time.Date(2026, 2, 1, 8, 1, 0, 0, time.UTC)) {
		t.Fatalf("third %#v", msgs[2])
	}
	if text(t, st, `SELECT label FROM sessions WHERE id=?`, "s-minh-an-key-label") != "key-as-label" {
		t.Fatal("empty label did not use session_key")
	}
	if text(t, st, `SELECT agent_id FROM sessions WHERE id=?`, "s-minh-an-missing-agent") != unresolvedAgentID("minh-an") {
		t.Fatal("unresolved session was dropped")
	}
	if text(t, st, `SELECT agent_id FROM session_import_links WHERE source_id=?`, "s-minh-an-missing-agent") != "" {
		t.Fatal("unresolved agent was stored as resolved")
	}
	if text(t, st, `SELECT source_agent_id FROM session_import_links WHERE source_id=?`, "s-minh-an-missing-agent") != "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb" {
		t.Fatal("source agent was not recorded")
	}
	if text(t, st, `SELECT contact_id FROM session_import_links WHERE source_id=?`, "s-minh-an-missing-agent") != "" {
		t.Fatal("missing contact was linked")
	}
	if text(t, st, `SELECT source_user_id FROM session_import_links WHERE source_id=?`, "s-minh-an-missing-agent") != "no-such-contact" {
		t.Fatal("unresolved contact was not recorded")
	}
	if text(t, st, `SELECT contact_id FROM session_import_links WHERE source_id=?`, "s-minh-an-order") != contactSrc {
		t.Fatal("contact was not linked")
	}
	if text(t, st, `SELECT agent_id FROM session_import_links WHERE source_id=?`, "s-minh-an-no-agent") != "" {
		t.Fatal("absent agent looked resolved")
	}
	if count(t, st, `SELECT COUNT(*) FROM sessions WHERE tenant_id=?`, "ban-khac") != 0 {
		t.Fatal("foreign tenant was written")
	}
	hits, err := st.SearchMemory("MAUVAI1842")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Fatalf("search indexed import %d", len(hits))
	}
	if count(t, st, `SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name='messages_ai'`) != 1 {
		t.Fatal("message index trigger was not restored")
	}
	live, err := st.CreateSession(store.Session{AgentID: agentSrc, TenantID: "minh-an", Label: "live"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddMessage(store.Message{SessionID: live.ID, Role: "user", Content: "MAUVAI1842LIVE"}); err != nil {
		t.Fatal(err)
	}
	hits, err = st.SearchMemory("MAUVAI1842LIVE")
	if err != nil || len(hits) == 0 {
		t.Fatalf("live message was not indexed: %v %d", err, len(hits))
	}
	db, _ := st.SQL()
	if _, err := db.Exec(`UPDATE agents SET display_name=? WHERE id=?`, "Da doi", unresolvedAgentID("minh-an")); err != nil {
		t.Fatal(err)
	}
	again, err := Import(st, openFixture(t), minhOpt(t))
	if err != nil {
		t.Fatal(err)
	}
	if tenant(again, "minh-an").New != 0 || tenant(again, "minh-an").Skipped != 6 {
		t.Fatalf("rerun %+v", tenant(again, "minh-an"))
	}
	if count(t, st, `SELECT COUNT(*) FROM sessions WHERE tenant_id=?`, "minh-an") != 5 {
		t.Fatal("rerun changed the session count")
	}
	if text(t, st, `SELECT display_name FROM agents WHERE id=?`, unresolvedAgentID("minh-an")) != "Da doi" {
		t.Fatal("rerun rewrote the placeholder agent")
	}
	if count(t, st, `SELECT COUNT(*) FROM memories`) != 0 {
		t.Fatal("rerun wrote a memory")
	}
}

func TestTenantIsolation(t *testing.T) {
	st := openStore(t)
	seedAgent(t, st, agentSrc, "ban-khac-sales", "ban-khac")
	seedContact(t, st, "no-such-contact", "ban-khac", "telegram", "ext-ban")
	ag, err := st.GetAgent(agentSrc)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{ID: "ignored", AgentID: ag.ID, TenantID: "ban-khac", Label: "giu-ban-khac"})
	if err != nil {
		t.Fatal(err)
	}
	db, _ := st.SQL()
	if _, err := db.Exec(`UPDATE sessions SET id=? WHERE id=?`, "s-minh-an-order", sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddMessage(store.Message{SessionID: "s-minh-an-order", Role: "user", Content: "giu-than"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutMemory(store.Memory{SessionID: "s-minh-an-order", TenantID: "ban-khac", Body: "giu-memory"}); err != nil {
		t.Fatal(err)
	}
	rep, err := Import(st, openFixture(t), minhOpt(t))
	if err != nil {
		t.Fatal(err)
	}
	if tenant(rep, "minh-an").New != 3 || tenant(rep, "minh-an").IDCollision != 1 {
		t.Fatalf("new %+v", tenant(rep, "minh-an"))
	}
	if text(t, st, `SELECT tenant_id FROM sessions WHERE id=?`, "s-minh-an-order") != "ban-khac" {
		t.Fatal("existing session moved tenant")
	}
	if text(t, st, `SELECT label FROM sessions WHERE id=?`, "s-minh-an-order") != "giu-ban-khac" {
		t.Fatal("existing label was overwritten")
	}
	msgs, err := st.ListMessages("s-minh-an-order")
	if err != nil || len(msgs) != 1 || msgs[0].Content != "giu-than" {
		t.Fatalf("existing messages %#v %v", msgs, err)
	}
	if count(t, st, `SELECT COUNT(*) FROM memories`) != 1 {
		t.Fatal("memory count changed")
	}
	if text(t, st, `SELECT body FROM memories`) != "giu-memory" {
		t.Fatal("memory body changed")
	}
	if count(t, st, `SELECT COUNT(*) FROM sessions WHERE tenant_id=?`, "ban-khac") != 1 {
		t.Fatal("foreign session was inserted")
	}
	if text(t, st, `SELECT tenant_id FROM agents WHERE id=?`, agentSrc) != "ban-khac" {
		t.Fatal("other tenant agent was reused")
	}
	if text(t, st, `SELECT agent_id FROM sessions WHERE id=?`, "s-minh-an-key-label") != unresolvedAgentID("minh-an") {
		t.Fatal("session linked to the other tenant's agent")
	}
	if text(t, st, `SELECT contact_id FROM session_import_links WHERE source_id=?`, "s-minh-an-missing-agent") != "" {
		t.Fatal("session linked to the other tenant's contact")
	}
	if count(t, st, `SELECT COUNT(*) FROM sessions WHERE tenant_id=? AND id=?`, "minh-an", "s-ban-khac-1") != 0 {
		t.Fatal("foreign row landed in minh-an")
	}
}

func TestAddOnlyLeavesExisting(t *testing.T) {
	st := openStore(t)
	seedAgent(t, st, agentSrc, "minh-an-sales", "minh-an")
	if _, err := st.CreateSession(store.Session{AgentID: agentSrc, TenantID: "minh-an", Label: "temp"}); err != nil {
		t.Fatal(err)
	}
	db, _ := st.SQL()
	var id string
	if err := db.QueryRow(`SELECT id FROM sessions`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE sessions SET id=?, label=? WHERE id=?`, "s-minh-an-order", "keep-me", id); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddMessage(store.Message{SessionID: "s-minh-an-order", Role: "user", Content: "keep-body"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutMemory(store.Memory{SessionID: "s-minh-an-order", TenantID: "minh-an", Body: "keep-memory"}); err != nil {
		t.Fatal(err)
	}
	rep, err := Import(st, openFixture(t), minhOpt(t))
	if err != nil {
		t.Fatal(err)
	}
	if tenant(rep, "minh-an").NativeExists < 1 {
		t.Fatalf("native %+v", tenant(rep, "minh-an"))
	}
	if text(t, st, `SELECT label FROM sessions WHERE id=?`, "s-minh-an-order") != "keep-me" {
		t.Fatal("existing session was overwritten")
	}
	msgs, err := st.ListMessages("s-minh-an-order")
	if err != nil || len(msgs) != 1 || msgs[0].Content != "keep-body" {
		t.Fatalf("messages %#v %v", msgs, err)
	}
	if count(t, st, `SELECT COUNT(*) FROM memories`) != 1 || text(t, st, `SELECT body FROM memories`) != "keep-memory" {
		t.Fatal("memory changed")
	}
	if count(t, st, `SELECT COUNT(*) FROM messages WHERE content=?`, "khong ghi de") != 0 {
		t.Fatal("duplicate row was written")
	}
}

func TestAgentKeyAndContactDest(t *testing.T) {
	st := openStore(t)
	seedAgent(t, st, "ag-sales", "minh-an-sales", "minh-an")
	seedContact(t, st, "ct-minh", "minh-an", "telegram", "ext-minh-an")
	raw := strings.NewReader(`{"id":"s-by-key","tenant_id":"minh-an","session_key":"key-by-agent-key","agent_id":"minh-an-sales","user_id":"ext-minh-an","channel":"telegram","created_at":"2026-02-06T00:00:00Z","messages":[{"role":"user","content":"Theo khoa agent.","created_at":"2026-02-06T00:00:01Z"}]}` + "\n")
	rep, err := Import(st, raw, minhOpt(t))
	if err != nil {
		t.Fatal(err)
	}
	if tenant(rep, "minh-an").New != 1 || tenant(rep, "minh-an").UnresolvedAgent != 0 || tenant(rep, "minh-an").UnresolvedContact != 0 {
		t.Fatalf("%+v", tenant(rep, "minh-an"))
	}
	if text(t, st, `SELECT agent_id FROM sessions WHERE id=?`, "s-by-key") != "ag-sales" {
		t.Fatal("agent_key was not mapped")
	}
	if text(t, st, `SELECT contact_id FROM session_import_links WHERE source_id=?`, "s-by-key") != "ct-minh" {
		t.Fatal("contact dest was not mapped")
	}
}

func TestStreamBatchesAndLineLimit(t *testing.T) {
	st := openStore(t)
	var b strings.Builder
	for i := 0; i < 70; i++ {
		b.WriteString(`{"id":"stream-`)
		b.WriteString(pad6(i))
		b.WriteString(`","tenant_id":"minh-an","session_key":"k-`)
		b.WriteString(pad6(i))
		b.WriteString(`","created_at":"2026-03-01T00:00:00Z","messages":[`)
		if i == 0 {
			b.WriteString(`{"role":"user","content":"A","created_at":"2026-03-01T01:00:00Z"},{"role":"user","content":"B","created_at":"2026-03-01T01:00:00Z"}`)
		} else {
			b.WriteString(`{"role":"user","content":"Minh An dong","created_at":"2026-03-01T02:00:00Z"},{"role":"assistant","content":"Minh An tra loi","created_at":"2026-03-01T02:00:01Z"}`)
		}
		b.WriteString("]}\n")
	}
	rep, err := Import(st, strings.NewReader(b.String()), func() Options { o := minhOpt(t); o.DryRun = true; return o }())
	if err != nil {
		t.Fatal(err)
	}
	if tenant(rep, "minh-an").New != 70 || tenant(rep, "minh-an").Messages != 140 {
		t.Fatalf("dry %+v", tenant(rep, "minh-an"))
	}
	if count(t, st, `SELECT COUNT(*) FROM sessions`) != 0 {
		t.Fatal("dry-run wrote")
	}
	rep, err = Import(st, strings.NewReader(b.String()), minhOpt(t))
	if err != nil {
		t.Fatal(err)
	}
	if tenant(rep, "minh-an").New != 70 || tenant(rep, "minh-an").Messages != 140 {
		t.Fatalf("apply %+v", tenant(rep, "minh-an"))
	}
	if count(t, st, `SELECT COUNT(*) FROM sessions`) != 70 || count(t, st, `SELECT COUNT(*) FROM messages`) != 140 {
		t.Fatal("batch counts")
	}
	if count(t, st, `SELECT COUNT(*) FROM memories`) != 0 {
		t.Fatal("stream wrote memory")
	}
	msgs, err := st.ListMessages("stream-" + pad6(0))
	if err != nil || len(msgs) != 2 || msgs[0].Content != "A" || msgs[1].Content != "B" {
		t.Fatalf("order %#v %v", msgs, err)
	}
	if !msgs[1].CreatedAt.Equal(msgs[0].CreatedAt.Add(time.Nanosecond)) {
		t.Fatal("equal timestamps did not keep array order")
	}
	long := strings.Repeat("x", 500)
	raw := `{"id":"s-short","tenant_id":"minh-an","session_key":"k-short","created_at":"2026-04-01T00:00:00Z","messages":[{"role":"user","content":"nho","created_at":"2026-04-01T00:00:01Z"}]}` + "\n"
	rep, err = Import(st, strings.NewReader(long+"\n"+raw), func() Options { o := minhOpt(t); o.MaxLine = 400; return o }())
	if err != nil {
		t.Fatal(err)
	}
	if tenant(rep, "").Invalid != 1 || tenant(rep, "minh-an").New != 1 {
		t.Fatalf("limit %+v", rep.Tenants)
	}
	if _, err := st.GetSession("s-short"); err != nil {
		t.Fatal(err)
	}
}

func TestSourceTenantFilter(t *testing.T) {
	st := openStore(t)
	rep, err := Import(st, openFixture(t), Options{Tenant: "minh-an", SourceTenant: "ban-khac", DenyFile: denyFile(t)})
	if err != nil {
		t.Fatal(err)
	}
	if tenant(rep, "ban-khac").New != 1 || tenant(rep, "minh-an").New != 0 || tenant(rep, "minh-an").Foreign != 6 {
		t.Fatalf("%+v", rep.Tenants)
	}
	if text(t, st, `SELECT tenant_id FROM sessions WHERE id=?`, "s-ban-khac-1") != "minh-an" {
		t.Fatal("source tenant was not rewritten to the destination")
	}
	if count(t, st, `SELECT COUNT(*) FROM sessions`) != 1 {
		t.Fatal("other tenants were written")
	}
}

func TestDenylistStops(t *testing.T) {
	const phone = "09000000123"
	const peer = "zalo-peer-88421"
	raw := strings.NewReader(`{"id":"s-held","tenant_id":"held-shop","session_key":"k-held","user_id":"` + peer + `","created_at":"2026-05-01T00:00:00Z","messages":[{"role":"user","content":"call ` + phone + `","created_at":"2026-05-01T00:00:01Z"}]}` + "\n")
	st := openStore(t)
	rep, err := Import(st, raw, Options{Tenant: "held-shop", SourceTenant: "held-shop", DenyFile: denyFile(t, "held-shop")})
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("err %v", err)
	}
	if rep == nil || rep.Denied != 1 || len(rep.Tenants) != 0 {
		t.Fatalf("report %+v", rep)
	}
	blob, _ := json.Marshal(rep)
	if strings.Contains(string(blob), "held-shop") || strings.Contains(err.Error(), "held-shop") || strings.Contains(string(blob), phone) || strings.Contains(string(blob), peer) {
		t.Fatalf("name leaked %s %s", blob, err)
	}
	if count(t, st, `SELECT COUNT(*) FROM sessions`) != 0 {
		t.Fatal("denied tenant wrote a session")
	}
	for _, src := range []string{"", denyFile(t, "held-shop")} {
		_, err = Import(st, strings.NewReader("{}\n"), Options{Tenant: "minh-an", SourceTenant: "held-shop", DenyFile: src})
		if src == "" {
			if err == nil || !strings.Contains(err.Error(), "missing") {
				t.Fatalf("missing %v", err)
			}
		}
	}
	_, err = Import(st, strings.NewReader("{}\n"), Options{Tenant: "minh-an", SourceTenant: "held-shop", DenyFile: denyFile(t, "held-shop")})
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("source %v", err)
	}
	if count(t, st, `SELECT COUNT(*) FROM sessions`) != 0 {
		t.Fatal("denied source wrote a session")
	}
	empty := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(empty, []byte("# comment\n\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = Import(st, strings.NewReader("{}\n"), Options{Tenant: "minh-an", SourceTenant: "minh-an", DenyFile: empty})
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty %v", err)
	}
	bad := filepath.Join(t.TempDir(), "bad.txt")
	if err := os.WriteFile(bad, []byte("not a token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = Import(st, strings.NewReader("{}\n"), Options{Tenant: "minh-an", SourceTenant: "minh-an", DenyFile: bad})
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("malformed %v", err)
	}
	if count(t, st, `SELECT COUNT(*) FROM sessions`) != 0 {
		t.Fatal("bad denylist wrote a session")
	}
}

func TestDeniedNameStaysOutOfReport(t *testing.T) {
	const phone = "09000000123"
	const peer = "zalo-peer-88421"
	raw := `{"id":"s-ok","tenant_id":"minh-an","session_key":"k-ok","created_at":"2026-05-02T00:00:00Z","messages":[{"role":"user","content":"Minh An on.","created_at":"2026-05-02T00:00:01Z"}]}
{"id":"s-held","tenant_id":"held-shop","session_key":"k-held","user_id":"` + peer + `","created_at":"2026-05-02T00:00:00Z","messages":[{"role":"user","content":"call ` + phone + `","created_at":"2026-05-02T00:00:02Z"}]}
`
	st := openStore(t)
	rep, err := Import(st, strings.NewReader(raw), minhOpt(t))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Denied != 1 || tenant(rep, "minh-an").New != 1 {
		t.Fatalf("%+v denied %d", rep.Tenants, rep.Denied)
	}
	blob, _ := json.Marshal(rep)
	for _, secret := range []string{"held-shop", phone, peer} {
		if strings.Contains(string(blob), secret) {
			t.Fatalf("report contains %s: %s", secret, blob)
		}
	}
	if count(t, st, `SELECT COUNT(*) FROM sessions WHERE tenant_id=?`, "held-shop") != 0 {
		t.Fatal("denied row written")
	}
}

func TestUnresolvedSessionDoesNotCallModel(t *testing.T) {
	st := openStore(t)
	if _, err := Import(st, openFixture(t), minhOpt(t)); err != nil {
		t.Fatal(err)
	}
	ag, err := st.GetAgent(unresolvedAgentID("minh-an"))
	if err != nil || ag.Enabled {
		t.Fatalf("placeholder %#v %v", ag, err)
	}
	sess, err := st.GetSession("s-minh-an-no-agent")
	if err != nil || !sess.Archived {
		t.Fatalf("session %#v %v", sess, err)
	}
	before, _ := st.ListMessages(sess.ID)
	scripted := &llm.Scripted{Replies: []llm.Reply{{Text: "should-not-run"}}}
	rt := agent.New(st, connector.NewRegistry(), approval.New(0), eventstore.New(8), scripted)
	_, err = rt.Chat(context.Background(), sess.ID, "hello Minh An")
	if !errors.Is(err, agent.ErrSessionArchived) && !errors.Is(err, agent.ErrAgentInactive) {
		t.Fatalf("chat %v", err)
	}
	if len(scripted.Recorded) != 0 {
		t.Fatal("model was called")
	}
	after, _ := st.ListMessages(sess.ID)
	if len(after) != len(before) {
		t.Fatalf("messages changed %d -> %d", len(before), len(after))
	}
}

func TestTriggerStaysAndIndexesLiveMessage(t *testing.T) {
	st := openStore(t)
	duringTx = func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO messages(id, session_id, role, content, created_at, agent_id, mention_agent_id, model, provider) VALUES(?,?,?,?,?,?,?,?,?)`,
			"live-during", "s-minh-an-no-agent", "user", "LIVE-DURING-FTS", "2026-02-03T09:00:02.000000000Z", "", "", "", "")
		return err
	}
	t.Cleanup(func() { duringTx = nil })
	if _, err := Import(st, openFixture(t), minhOpt(t)); err != nil {
		t.Fatal(err)
	}
	if count(t, st, `SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name='messages_ai'`) != 1 {
		t.Fatal("trigger missing after import")
	}
	if count(t, st, `SELECT COUNT(*) FROM memory_fts WHERE id=?`, "live-during") != 1 {
		t.Fatal("live message was not indexed")
	}
	if count(t, st, `SELECT COUNT(*) FROM memory_fts WHERE session_id=?`, "s-minh-an-order") != 0 {
		t.Fatal("imported text stayed in memory_fts")
	}
	duringTx = func(tx *sql.Tx) error { return errors.New("boom") }
	before := count(t, st, `SELECT COUNT(*) FROM sessions`)
	fail := `{"id":"s-fail","tenant_id":"other-shop","session_key":"k-fail","created_at":"2026-06-01T00:00:00Z","messages":[{"role":"user","content":"nope","created_at":"2026-06-01T00:00:01Z"}]}` + "\n"
	if _, err := Import(st, strings.NewReader(fail), Options{Tenant: "other-shop", SourceTenant: "other-shop", DenyFile: denyFile(t)}); err == nil {
		t.Fatal("expected failed import")
	}
	if count(t, st, `SELECT COUNT(*) FROM sessions`) != before {
		t.Fatal("failed import wrote rows")
	}
	if count(t, st, `SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND name='messages_ai'`) != 1 {
		t.Fatal("failed import dropped the trigger")
	}
}

func TestRollbackKeepsNativeAndOtherTenant(t *testing.T) {
	st := openStore(t)
	opt := minhOpt(t)
	if _, err := Import(st, openFixture(t), opt); err != nil {
		t.Fatal(err)
	}
	seedAgent(t, st, "ag-native", "minh-an-native", "minh-an")
	native, err := st.CreateSession(store.Session{AgentID: "ag-native", TenantID: "minh-an", Label: "native-keep"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddMessage(store.Message{SessionID: native.ID, Role: "user", Content: "native-body"}); err != nil {
		t.Fatal(err)
	}
	seedAgent(t, st, "ag-other", "other-shop-agent", "other-shop")
	other, err := st.CreateSession(store.Session{AgentID: "ag-other", TenantID: "other-shop", Label: "other-keep"})
	if err != nil {
		t.Fatal(err)
	}
	db, _ := st.SQL()
	if _, err := db.Exec(`INSERT INTO session_import_links(tenant_id, source_id, source_key, session_id, source_agent_id, agent_id, source_user_id, contact_id) VALUES(?,?,?,?,?,?,?,?)`,
		"other-shop", "src-other", "key-other", other.ID, "", "ag-other", "", ""); err != nil {
		t.Fatal(err)
	}
	dry, err := Rollback(st, Options{Tenant: "minh-an", DenyFile: opt.DenyFile, DryRun: true})
	if err != nil || !dry.DryRun || dry.Links == 0 || dry.Sessions == 0 {
		t.Fatalf("dry %+v %v", dry, err)
	}
	if _, err := st.GetSession("s-minh-an-order"); err != nil {
		t.Fatal("dry-run removed an import")
	}
	applied, err := Rollback(st, Options{Tenant: "minh-an", DenyFile: opt.DenyFile})
	if err != nil || applied.DryRun {
		t.Fatalf("apply %+v %v", applied, err)
	}
	if _, err := st.GetSession("s-minh-an-order"); err == nil {
		t.Fatal("imported session survived rollback")
	}
	got, err := st.GetSession(native.ID)
	if err != nil || got.Label != "native-keep" {
		t.Fatalf("native %#v %v", got, err)
	}
	msgs, err := st.ListMessages(native.ID)
	if err != nil || len(msgs) != 1 || msgs[0].Content != "native-body" {
		t.Fatalf("native messages %#v %v", msgs, err)
	}
	if text(t, st, `SELECT label FROM sessions WHERE id=?`, other.ID) != "other-keep" {
		t.Fatal("other tenant session removed")
	}
	if count(t, st, `SELECT COUNT(*) FROM session_import_links WHERE tenant_id=?`, "minh-an") != 0 {
		t.Fatal("minh-an links remain")
	}
	if count(t, st, `SELECT COUNT(*) FROM session_import_links WHERE tenant_id=?`, "other-shop") != 1 {
		t.Fatal("other tenant link removed")
	}
}
