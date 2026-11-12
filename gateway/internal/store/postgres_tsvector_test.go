// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

const (
	spec154Needle = "Ghế mẫu Minh An mã MA-WALNUT-77 làm bằng gỗ sồi."
	spec154Query  = "mã MA-WALNUT-77 làm bằng gỗ gì"
)

var spec154Fillers = []string{
	"Công ty mẫu Minh An gửi báo giá dạng PDF có VAT.",
	"Chiết khấu nội bộ Minh An đã chốt tám phần trăm.",
	"Kho mẫu Minh An đặt tại khu A và không giao trước chín giờ.",
}

func TestTsOrQueryQuotesTokens(t *testing.T) {
	got := tsOrQuery([]string{"mã", "walnut", "77"})
	if got != "'mã' | 'walnut' | '77'" {
		t.Fatalf("tsquery %s", got)
	}
	and := tsAndQuery([]string{"mã", "walnut", "a&b"})
	if and != "'mã' & 'walnut'" {
		t.Fatalf("and query %s", and)
	}
	if tsOrQuery([]string{"a&b", "ok"}) != "'ok'" {
		t.Fatal("unsafe token must be dropped")
	}
	if postgresTextSearchConfig != "simple" {
		t.Fatalf("config %s", postgresTextSearchConfig)
	}
}

func TestRecallORSQLStableShape(t *testing.T) {
	q := RecallQuery{ExcludeSession: "ask", CustomerScoped: true, CustomerKey: "cust"}
	if orRecallProbeBudget(1999) != 3998 || orRecallProbeBudget(0) != 0 {
		t.Fatalf("budget %d %d", orRecallProbeBudget(1999), orRecallProbeBudget(0))
	}
	probe, pargs := recallTSVORProbeQuery(q, "ten", "ag", "'mã' | 'ma'", 40, 20)
	bitmap, bargs := recallTSVORBitmapQuery(q, "ten", "ag", "'mã' | 'ma'", 20)
	for _, sql := range []string{probe, bitmap} {
		if strings.Contains(strings.ToLower(sql), "ts_rank") {
			t.Fatalf("ts_rank in %s", sql)
		}
	}
	if strings.Count(probe, "?") != len(pargs) || strings.Count(bitmap, "?") != len(bargs) {
		t.Fatalf("placeholders probe %d/%d bitmap %d/%d", strings.Count(probe, "?"), len(pargs), strings.Count(bitmap, "?"), len(bargs))
	}
	if !strings.Contains(probe, "ORDER BY created_at DESC") || !strings.Contains(probe, "LIMIT ?") {
		t.Fatal(probe)
	}
	if !strings.Contains(bitmap, "MATERIALIZED") || !strings.Contains(bitmap, "tenant_id = ?") || strings.Contains(bitmap, "tenant_id ||") {
		t.Fatal(bitmap)
	}
	if !strings.Contains(bitmap, "session_id IN (SELECT id FROM sessions WHERE agent_id = ? AND tenant_id = ?)") {
		t.Fatal(bitmap)
	}
	custQ, cargs := recallTSVORCustomerQuery(q, "ten", "ag", "'mã'", 20)
	if strings.Count(custQ, "?") != len(cargs) || !strings.Contains(custQ, "customer_key = ?") {
		t.Fatal(custQ)
	}
	agQ, aargs := recallTSVORAgentQuery(RecallQuery{ExcludeSession: "ask"}, "ten", "ag", "'mã'", 20)
	if strings.Count(agQ, "?") != len(aargs) || !strings.Contains(agQ, "agent_id = ?") || !strings.Contains(agQ, "OFFSET 0") {
		t.Fatal(agQ)
	}
	if !strings.Contains(bitmap, "customer_key = ?") || !strings.Contains(bitmap, "sess.agent_id = ?") {
		t.Fatal(bitmap)
	}
	plain, _ := recallTSVORBitmapQuery(RecallQuery{ExcludeSession: "ask"}, "ten", "", "'ok'", 5)
	if strings.Contains(plain, "customer_key = ?") || strings.Contains(plain, "agent_id") {
		t.Fatal(plain)
	}
}

func TestSQLiteHasNoBodyTSV(t *testing.T) {
	s, err := OpenSQLite(filepath.Join(t.TempDir(), "no-tsv.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	has, err := s.hasColumn("memories", "body_tsv")
	if err != nil {
		t.Fatal(err)
	}
	if has || s.bodyTSV {
		t.Fatal("sqlite must not grow a tsvector column")
	}
}

func TestPostgresBodyTSVAdditive(t *testing.T) {
	st := openSpec154(t, "goso_spec154_add")
	if !st.bodyTSV {
		t.Fatal("bodyTSV not set")
	}
	original := map[string][]string{
		"memories":    {"id", "session_id", "kind", "body", "created_at", "tenant_id", "customer_key"},
		"vault_docs":  {"id", "title", "path", "sha256", "mtime", "body", "tenant_id", "owner_explicit"},
		"kg_entities": {"id", "tenant_id", "name", "kind", "body", "valid_from", "valid_until", "created_at", "agent_id", "provenance"},
	}
	for table, cols := range original {
		for _, col := range cols {
			want := "text"
			if table == "vault_docs" && col == "owner_explicit" {
				want = "integer"
			}
			assertColumn(t, st, table, col, want, "NEVER")
		}
		assertColumn(t, st, table, "body_tsv", "tsvector", "ALWAYS")
		var expr string
		if err := st.db.QueryRow(`SELECT generation_expression FROM information_schema.columns
			WHERE table_schema=current_schema() AND table_name=? AND column_name='body_tsv'`, table).Scan(&expr); err != nil {
			t.Fatal(err)
		}
		low := strings.ToLower(expr)
		if !strings.Contains(low, "to_tsvector") || !strings.Contains(low, "simple") || strings.Contains(low, "unaccent") {
			t.Fatalf("%s expression %s", table, expr)
		}
	}
	for _, idx := range []string{idxMemoriesBodyTSV, idxVaultBodyTSV, idxKGBodyTSV} {
		var am string
		if err := st.db.QueryRow(`SELECT am.amname
			FROM pg_class c
			JOIN pg_am am ON am.oid = c.relam
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = current_schema() AND c.relname = ?`, idx).Scan(&am); err != nil {
			t.Fatalf("index %s: %v", idx, err)
		}
		if am != "gin" {
			t.Fatalf("index %s method %s", idx, am)
		}
	}
	var am string
	if err := st.db.QueryRow(`SELECT am.amname
		FROM pg_class c
		JOIN pg_am am ON am.oid = c.relam
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = current_schema() AND c.relname = ?`, idxMemoriesTenantCreated).Scan(&am); err != nil {
		t.Fatal(err)
	}
	if am != "btree" {
		t.Fatalf("tenant created index method %s", am)
	}
	dsn := dsnFromStore(t, st)
	again, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	defer again.Close()
	if !again.bodyTSV {
		t.Fatal("second open lost bodyTSV")
	}
}

func TestPostgresBodyTSVBackfillAndSearch(t *testing.T) {
	st := openSpec154(t, "goso_spec154_backfill")
	ag, err := st.CreateAgent(Agent{AgentKey: "minh-an-sales", DisplayName: "Cố vấn Minh An"})
	if err != nil {
		t.Fatal(err)
	}
	oldSess, err := st.CreateSession(Session{AgentID: ag.ID, Label: "cu"})
	if err != nil {
		t.Fatal(err)
	}
	ask, err := st.CreateSession(Session{AgentID: ag.ID, Label: "hoi"})
	if err != nil {
		t.Fatal(err)
	}
	note, err := st.PutMemory(Memory{SessionID: oldSess.ID, TenantID: oldSess.TenantID, Kind: KindEpisodic, Body: spec154Needle})
	if err != nil {
		t.Fatal(err)
	}
	bodyBefore := note.Body
	if _, err := st.db.Exec(`ALTER TABLE memories DROP COLUMN body_tsv`); err != nil {
		t.Fatal(err)
	}
	dsn := dsnFromStore(t, st)
	re, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatalf("reopen after drop column: %v", err)
	}
	defer re.Close()
	got, err := re.GetMemory(note.ID)
	if err != nil || got.Body != bodyBefore {
		t.Fatalf("body changed on migrate: %#v %v", got, err)
	}
	assertColumn(t, re, "memories", "body", "text", "NEVER")
	hits, err := re.SearchMemory("MA-WALNUT-77")
	if err != nil || len(hits) != 1 || hits[0].ID != note.ID {
		t.Fatalf("memory search %#v %v", hits, err)
	}
	q := RecallQuery{TenantID: oldSess.TenantID, AgentID: ag.ID, ExcludeSession: ask.ID, Query: spec154Query}
	recalled, err := re.RecallMemories(q)
	if err != nil || len(recalled) != 1 || recalled[0].Memory.ID != note.ID {
		t.Fatalf("recall after backfill %#v %v", recalled, err)
	}

	doc, err := re.PutVaultDoc(VaultDoc{
		Title: "Nội dung khác", Path: "ghe.md", TenantID: DefaultTenant,
		Body: spec154Needle, SHA256: "s", OwnerExplicit: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	titleOnly, err := re.PutVaultDoc(VaultDoc{
		Title: "Bảng giá Minh An", Path: "bang-gia.md", TenantID: DefaultTenant,
		Body: "nội dung không trùng mã", SHA256: "t", OwnerExplicit: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	vh, err := re.SearchVault(DefaultTenant, "MA-WALNUT-77")
	if err != nil || len(vh) != 1 || vh[0].ID != doc.ID {
		t.Fatalf("vault body %#v %v", vh, err)
	}
	vh, err = re.SearchVault(DefaultTenant, "Bảng giá")
	if err != nil || len(vh) != 1 || vh[0].ID != titleOnly.ID {
		t.Fatalf("vault title %#v %v", vh, err)
	}
	byBody, err := re.PutKGEntity(KGEntity{Name: "Khác", TenantID: DefaultTenant, Body: spec154Needle})
	if err != nil {
		t.Fatal(err)
	}
	byName, err := re.PutKGEntity(KGEntity{Name: "Kho Minh An", TenantID: DefaultTenant, Body: "không có mã này"})
	if err != nil {
		t.Fatal(err)
	}
	kh, err := re.SearchProgressive("MA-WALNUT-77", DefaultTenant)
	if err != nil || !hitIDs(kh, byBody.ID) || hitIDs(kh, byName.ID) {
		t.Fatalf("kg body %#v %v", kh, err)
	}
	kh, err = re.SearchProgressive("Kho Minh An", DefaultTenant)
	if err != nil || !hitIDs(kh, byName.ID) {
		t.Fatalf("kg name %#v %v", kh, err)
	}
}

func TestPostgresTSVectorRollbackDropsIndexOnly(t *testing.T) {
	st := openSpec154(t, "goso_spec154_rollback")
	ag, err := st.CreateAgent(Agent{AgentKey: "minh-an-sales", DisplayName: "Cố vấn Minh An"})
	if err != nil {
		t.Fatal(err)
	}
	oldSess, err := st.CreateSession(Session{AgentID: ag.ID})
	if err != nil {
		t.Fatal(err)
	}
	ask, err := st.CreateSession(Session{AgentID: ag.ID})
	if err != nil {
		t.Fatal(err)
	}
	note, err := st.PutMemory(Memory{SessionID: oldSess.ID, Kind: KindEpisodic, Body: spec154Needle, TenantID: oldSess.TenantID})
	if err != nil {
		t.Fatal(err)
	}
	for _, idx := range []string{idxMemoriesBodyTSV, idxVaultBodyTSV, idxKGBodyTSV, idxMemoriesTenantCreated} {
		if _, err := st.db.Exec(`DROP INDEX IF EXISTS ` + idx); err != nil {
			t.Fatal(err)
		}
	}
	assertColumn(t, st, "memories", "body", "text", "NEVER")
	assertColumn(t, st, "memories", "body_tsv", "tsvector", "ALWAYS")
	var n int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM pg_class c
		JOIN pg_namespace ns ON ns.oid = c.relnamespace
		WHERE ns.nspname = current_schema() AND c.relname = ?`, idxMemoriesBodyTSV).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("index still present %d", n)
	}
	hits, err := st.RecallMemories(RecallQuery{TenantID: oldSess.TenantID, AgentID: ag.ID, ExcludeSession: ask.ID, Query: spec154Query})
	if err != nil || len(hits) != 1 || hits[0].Memory.ID != note.ID {
		t.Fatalf("recall after drop index %#v %v", hits, err)
	}
}

func TestRecallOlderThanScanCap(t *testing.T) {
	t.Setenv("GOSO_MEMORY_RECALL", "1")
	t.Setenv("GOSO_DATABASE_URL", "")
	if recallScanCap != 500 {
		t.Fatalf("spec assumes recallScanCap 500, got %d", recallScanCap)
	}
	stores := map[string]StoreIface{"memory": New()}
	sq, err := OpenSQLite(filepath.Join(t.TempDir(), "cap.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sq.Close() })
	stores["sqlite"] = sq
	if postgresTestDSN() != "" {
		stores["postgres"] = openSpec154(t, "goso_spec154_cap")
	} else if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
		t.Fatal("GOSO_TEST_POSTGRES_DSN is required in CI")
	}
	for name, st := range stores {
		t.Run(name, func(t *testing.T) {
			recallOlderThanCap(t, st)
		})
	}
}

func TestPostgresRecall50kUnder50ms(t *testing.T) {
	t.Setenv("GOSO_MEMORY_RECALL", "1")
	t.Setenv("GOSO_DATABASE_URL", "")
	st := openSpec154(t, "goso_spec154_50k")
	for _, body := range spec154Fillers {
		for _, tok := range RecallTokens(spec154Query) {
			if strings.Contains(strings.ToLower(body), tok) {
				t.Fatalf("filler %q contains query token %q", body, tok)
			}
		}
	}
	ag, err := st.CreateAgent(Agent{AgentKey: "minh-an-sales", DisplayName: "Cố vấn Minh An"})
	if err != nil {
		t.Fatal(err)
	}
	needleSess, err := st.CreateSession(Session{AgentID: ag.ID, Label: "cu"})
	if err != nil {
		t.Fatal(err)
	}
	fillerSess, err := st.CreateSession(Session{AgentID: ag.ID, Label: "moi"})
	if err != nil {
		t.Fatal(err)
	}
	ask, err := st.CreateSession(Session{AgentID: ag.ID, Label: "hoi"})
	if err != nil {
		t.Fatal(err)
	}
	const needleID = "spec154-needle"
	if _, err := st.db.Exec(`INSERT INTO memories(id, session_id, kind, body, created_at, tenant_id, customer_key) VALUES(?,?,?,?,?,?,?)`,
		needleID, needleSess.ID, KindEpisodic, spec154Needle, "2000-01-01T00:00:00Z", needleSess.TenantID, ""); err != nil {
		t.Fatal(err)
	}
	const fillers = 49999
	bulkMemories(t, st, fillerSess.ID, fillerSess.TenantID, fillers, spec154Fillers, "2026-06-01T00:00:00Z")
	if _, err := st.db.Exec(`ANALYZE memories`); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM memories`).Scan(&n); err != nil || n != fillers+1 {
		t.Fatalf("rows %d %v", n, err)
	}
	inCap, err := st.db.Query(`SELECT id FROM memories ORDER BY created_at DESC LIMIT ?`, recallScanCap)
	if err != nil {
		t.Fatal(err)
	}
	for inCap.Next() {
		var id string
		if err := inCap.Scan(&id); err != nil {
			t.Fatal(err)
		}
		if id == needleID {
			t.Fatal("needle is inside the newest recallScanCap rows")
		}
	}
	inCap.Close()
	q := RecallQuery{TenantID: needleSess.TenantID, AgentID: ag.ID, ExcludeSession: ask.ID, Query: spec154Query}
	match := tsOrQuery(RecallTokens(q.Query))
	plan := explain(t, st, `EXPLAIN SELECT m.id FROM memories m
		JOIN sessions sess ON sess.id = m.session_id
		WHERE m.body_tsv @@ to_tsquery('`+postgresTextSearchConfig+`', ?)
		AND m.tenant_id = ? AND sess.tenant_id = ?
		AND m.kind IN (?, ?)
		AND m.session_id <> ?`, match, q.TenantID, q.TenantID, KindEpisodic, KindDurable, ask.ID)
	if !strings.Contains(plan, idxMemoriesBodyTSV) {
		t.Fatalf("plan did not use %s\n%s", idxMemoriesBodyTSV, plan)
	}
	hits, err := st.RecallMemories(q)
	if err != nil || !recallHas(hits, needleID) {
		t.Fatalf("cold recall %#v %v", hits, err)
	}
	toks := RecallTokens(q.Query)
	_, n1, err := st.recallTSVStep(q, q.TenantID, q.AgentID, toks, tsAndQuery(toks), recallCandidateCap)
	if err != nil {
		t.Fatal(err)
	}
	logStep2Plan(t, st, q, toks, recallORLimit(n1), false)
	samples := timeRecall(t, st, q, needleID, 5)
	med := medianDuration(samples)
	max := maxDuration(samples)
	path := orStepPath(t, st, q)
	fmt.Printf("SPEC154 recall_50k rows=%d index=%s samples=%s median=%s max=%s path=%s\n",
		n, idxMemoriesBodyTSV, formatDurations(samples), med, max, path)
	if path != recallPathBitmapAnd {
		t.Fatalf("step2 path %s", path)
	}
	fmt.Printf("SPEC154 plan:\n%s\n", plan)
	if med >= 50*time.Millisecond || max >= 50*time.Millisecond {
		t.Fatalf("selective recall median %s max %s is not under 50ms", med, max)
	}
}

// Every filler shares one query token, so the match set is the whole table.
// The SQL cap must still return the older needle.
func TestPostgresRecall50kAllMatchWithinCap(t *testing.T) {
	t.Setenv("GOSO_MEMORY_RECALL", "1")
	t.Setenv("GOSO_DATABASE_URL", "")
	if recallCandidateCap != 2000 {
		t.Fatalf("recallCandidateCap = %d", recallCandidateCap)
	}
	st := openSpec154(t, "goso_spec154_50k_all")
	toks := RecallTokens(spec154Query)
	fillers := []string{
		"Công ty mẫu Minh An ghi mã riêng.",
		"Công ty mẫu Minh An ghi ma riêng.",
		"Công ty mẫu Minh An ghi walnut riêng.",
		"Công ty mẫu Minh An ghi 77 riêng.",
		"Công ty mẫu Minh An ghi làm riêng.",
		"Công ty mẫu Minh An ghi bằng riêng.",
		"Công ty mẫu Minh An ghi gỗ riêng.",
		"Công ty mẫu Minh An ghi gì riêng.",
	}
	for _, body := range fillers {
		n := 0
		for _, tok := range toks {
			if strings.Contains(strings.ToLower(body), tok) {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("filler %q matches %d query tokens, want 1", body, n)
		}
	}
	ag, err := st.CreateAgent(Agent{AgentKey: "minh-an-sales", DisplayName: "Cố vấn Minh An"})
	if err != nil {
		t.Fatal(err)
	}
	needleSess, err := st.CreateSession(Session{AgentID: ag.ID, Label: "cu"})
	if err != nil {
		t.Fatal(err)
	}
	fillerSess, err := st.CreateSession(Session{AgentID: ag.ID, Label: "moi"})
	if err != nil {
		t.Fatal(err)
	}
	ask, err := st.CreateSession(Session{AgentID: ag.ID, Label: "hoi"})
	if err != nil {
		t.Fatal(err)
	}
	const needleID = "spec154-needle-all"
	const nFill = 49999
	needleBody := spec154Needle + " gì"
	for _, tok := range toks {
		if !strings.Contains(strings.ToLower(needleBody), tok) {
			t.Fatalf("needle missing query token %q", tok)
		}
	}
	if _, err := st.db.Exec(`INSERT INTO memories(id, session_id, kind, body, created_at, tenant_id, customer_key) VALUES(?,?,?,?,?,?,?)`,
		needleID, needleSess.ID, KindEpisodic, needleBody, "2000-01-01T00:00:00Z", needleSess.TenantID, ""); err != nil {
		t.Fatal(err)
	}
	bulkMemories(t, st, fillerSess.ID, fillerSess.TenantID, nFill, fillers, "2026-06-01T00:00:00Z")
	if _, err := st.db.Exec(`ANALYZE memories`); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM memories`).Scan(&rows); err != nil || rows != nFill+1 {
		t.Fatalf("rows %d %v", rows, err)
	}
	inCap, err := st.db.Query(`SELECT id FROM memories ORDER BY created_at DESC LIMIT ?`, recallScanCap)
	if err != nil {
		t.Fatal(err)
	}
	for inCap.Next() {
		var id string
		if err := inCap.Scan(&id); err != nil {
			t.Fatal(err)
		}
		if id == needleID {
			t.Fatal("needle is inside the newest 500")
		}
	}
	inCap.Close()
	q := RecallQuery{TenantID: needleSess.TenantID, AgentID: ag.ID, ExcludeSession: ask.ID, Query: spec154Query, Limit: 20}
	hits, fetched, err := st.recallTSVFetch(q, q.TenantID, ag.ID, toks, q.Limit)
	if err != nil {
		t.Fatal(err)
	}
	if fetched > recallCandidateCap {
		t.Fatalf("fetched %d, cap %d", fetched, recallCandidateCap)
	}
	if !recallHas(hits, needleID) {
		t.Fatal("needle was not in the capped candidate set")
	}
	_, n1, err := st.recallTSVStep(q, q.TenantID, q.AgentID, toks, tsAndQuery(toks), recallCandidateCap)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("SPEC154 explain_and:\n%s\n", explainAnalyze(t, st, q, tsAndQuery(toks), recallCandidateCap))
	logStep2Plan(t, st, q, toks, recallORLimit(n1), true)
	samples := make([]time.Duration, 0, 5)
	for i := 0; i < 5; i++ {
		start := time.Now()
		got, err := st.RecallMemories(q)
		elapsed := time.Since(start)
		if err != nil || !recallHas(got, needleID) {
			t.Fatalf("sample %d %#v %v", i, got, err)
		}
		samples = append(samples, elapsed)
	}
	med := medianDuration(samples)
	max := samples[0]
	for _, d := range samples {
		if d > max {
			max = d
		}
	}
	path := orStepPath(t, st, q)
	fmt.Printf("SPEC154 worst_case rows=%d fetched=%d cap=%d samples=%s median=%s max=%s path=%s\n",
		rows, fetched, recallCandidateCap, formatDurations(samples), med, max, path)
	if path != recallPathTenantWindow {
		t.Fatalf("step2 path %s", path)
	}
	if max >= 50*time.Millisecond {
		t.Fatalf("worst-case recall max %s is not under 50ms (median %s)", max, med)
	}
}

// About 5% of the rows match the OR. The newest rows do not, so step 2 cannot
// fill the cap from the ordered index window and must use the GIN bitmap.
func TestPostgresRecall50kFivePercentWithinCap(t *testing.T) {
	t.Setenv("GOSO_MEMORY_RECALL", "1")
	t.Setenv("GOSO_DATABASE_URL", "")
	if recallCandidateCap != 2000 {
		t.Fatalf("recallCandidateCap = %d", recallCandidateCap)
	}
	st := openSpec154(t, "goso_spec154_50k_five")
	toks := RecallTokens(spec154Query)
	matchBody := "Công ty mẫu Minh An ghi mã riêng."
	missBody := spec154Fillers[0]
	nMatchTok := 0
	for _, tok := range toks {
		if strings.Contains(strings.ToLower(matchBody), tok) {
			nMatchTok++
		}
		if strings.Contains(strings.ToLower(missBody), tok) {
			t.Fatalf("non-match filler contains %q", tok)
		}
	}
	if nMatchTok != 1 {
		t.Fatalf("match filler matches %d query tokens, want 1", nMatchTok)
	}
	ag, err := st.CreateAgent(Agent{AgentKey: "minh-an-sales", DisplayName: "Cố vấn Minh An"})
	if err != nil {
		t.Fatal(err)
	}
	needleSess, err := st.CreateSession(Session{AgentID: ag.ID, Label: "cu"})
	if err != nil {
		t.Fatal(err)
	}
	fillerSess, err := st.CreateSession(Session{AgentID: ag.ID, Label: "moi"})
	if err != nil {
		t.Fatal(err)
	}
	ask, err := st.CreateSession(Session{AgentID: ag.ID, Label: "hoi"})
	if err != nil {
		t.Fatal(err)
	}
	const needleID = "spec154-needle-five"
	const nFill = 49999
	needleBody := spec154Needle + " gì"
	for _, tok := range toks {
		if !strings.Contains(strings.ToLower(needleBody), tok) {
			t.Fatalf("needle missing query token %q", tok)
		}
	}
	if _, err := st.db.Exec(`INSERT INTO memories(id, session_id, kind, body, created_at, tenant_id, customer_key) VALUES(?,?,?,?,?,?,?)`,
		needleID, needleSess.ID, KindEpisodic, needleBody, "2000-01-01T00:00:00Z", needleSess.TenantID, ""); err != nil {
		t.Fatal(err)
	}
	bulkMemoriesAt(t, st, fillerSess.ID, fillerSess.TenantID, nFill, "spec154-f", func(i int) (string, string) {
		if i%20 == 0 {
			return matchBody, "2026-06-01T00:00:00Z"
		}
		return missBody, "2026-06-02T00:00:00Z"
	})
	if _, err := st.db.Exec(`ANALYZE memories`); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM memories`).Scan(&rows); err != nil || rows != nFill+1 {
		t.Fatalf("rows %d %v", rows, err)
	}
	var orHits int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM memories WHERE body_tsv @@ to_tsquery('`+postgresTextSearchConfig+`', ?)`, tsOrQuery(toks)).Scan(&orHits); err != nil {
		t.Fatal(err)
	}
	// 2500 fillers (every 20th of 49999) plus the needle.
	if orHits < 2400 || orHits > 2600 {
		t.Fatalf("OR matches %d, want about 5%% of %d", orHits, rows)
	}
	q := RecallQuery{TenantID: needleSess.TenantID, AgentID: ag.ID, ExcludeSession: ask.ID, Query: spec154Query, Limit: 20}
	hits, fetched, err := st.recallTSVFetch(q, q.TenantID, ag.ID, toks, q.Limit)
	if err != nil {
		t.Fatal(err)
	}
	if fetched > recallCandidateCap {
		t.Fatalf("fetched %d, cap %d", fetched, recallCandidateCap)
	}
	if !recallHas(hits, needleID) {
		t.Fatal("needle was not in the capped candidate set")
	}
	_, n1, err := st.recallTSVStep(q, q.TenantID, q.AgentID, toks, tsAndQuery(toks), recallCandidateCap)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("SPEC154 explain_and:\n%s\n", explainAnalyze(t, st, q, tsAndQuery(toks), recallCandidateCap))
	logStep2Plan(t, st, q, toks, recallORLimit(n1), false)
	samples := make([]time.Duration, 0, 5)
	for i := 0; i < 5; i++ {
		start := time.Now()
		got, err := st.RecallMemories(q)
		elapsed := time.Since(start)
		if err != nil || !recallHas(got, needleID) {
			t.Fatalf("sample %d %#v %v", i, got, err)
		}
		samples = append(samples, elapsed)
	}
	med := medianDuration(samples)
	max := maxDuration(samples)
	path := orStepPath(t, st, q)
	fmt.Printf("SPEC154 five_percent rows=%d or_hits=%d fetched=%d cap=%d samples=%s median=%s max=%s path=%s\n",
		rows, orHits, fetched, recallCandidateCap, formatDurations(samples), med, max, path)
	if path != recallPathBitmapAnd {
		t.Fatalf("step2 path %s", path)
	}
	if max >= 50*time.Millisecond {
		t.Fatalf("5%% recall max %s is not under 50ms (median %s)", max, med)
	}
}

func TestPostgresRecall50kOtherTenantsSelective(t *testing.T) {
	t.Setenv("GOSO_MEMORY_RECALL", "1")
	t.Setenv("GOSO_DATABASE_URL", "")
	st := openSpec154(t, "goso_spec154_50k_others")
	toks := RecallTokens(spec154Query)
	const nFill = 49999
	const others = 4
	ag, ask, needleSess := spec154Agent(t, st, "tenant-a")
	const needleID = "spec154-needle-others"
	if _, err := st.db.Exec(`INSERT INTO memories(id, session_id, kind, body, created_at, tenant_id, customer_key) VALUES(?,?,?,?,?,?,?)`,
		needleID, needleSess.ID, KindEpisodic, spec154Needle, "2000-01-01T00:00:00Z", "tenant-a", ""); err != nil {
		t.Fatal(err)
	}
	bulkMemories(t, st, needleSess.ID, "tenant-a", nFill, []string{spec154Fillers[0]}, "2026-06-02T00:00:00Z")
	matchBody := "Công ty mẫu Minh An ghi mã riêng."
	for i := 0; i < others; i++ {
		tenant := fmt.Sprintf("tenant-%c", 'b'+i)
		other, err := st.CreateAgent(Agent{AgentKey: "other-" + tenant, DisplayName: "Khác", TenantID: tenant})
		if err != nil {
			t.Fatal(err)
		}
		sess, err := st.CreateSession(Session{AgentID: other.ID, TenantID: tenant, Label: tenant})
		if err != nil {
			t.Fatal(err)
		}
		bulkMemoriesAt(t, st, sess.ID, tenant, 50000, tenant, func(i int) (string, string) {
			return matchBody, "2026-07-01T00:00:00Z"
		})
	}
	if _, err := st.db.Exec(`ANALYZE memories`); err != nil {
		t.Fatal(err)
	}
	var global int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM memories WHERE body_tsv @@ to_tsquery('`+postgresTextSearchConfig+`', ?)`, tsOrQuery(toks)).Scan(&global); err != nil {
		t.Fatal(err)
	}
	if global < 200000 {
		t.Fatalf("global OR hits %d", global)
	}
	q := RecallQuery{TenantID: "tenant-a", AgentID: ag.ID, ExcludeSession: ask.ID, Query: spec154Query, Limit: 20}
	hits, fetched, err := st.recallTSVFetch(q, q.TenantID, ag.ID, toks, q.Limit)
	if err != nil {
		t.Fatal(err)
	}
	if fetched > recallCandidateCap || !recallHas(hits, needleID) {
		t.Fatalf("fetched %d hits needle %v", fetched, recallHas(hits, needleID))
	}
	_, n1, err := st.recallTSVStep(q, q.TenantID, q.AgentID, toks, tsAndQuery(toks), recallCandidateCap)
	if err != nil {
		t.Fatal(err)
	}
	logStep2Plan(t, st, q, toks, recallORLimit(n1), false)
	samples := timeRecall(t, st, q, needleID, 5)
	med, max := medianDuration(samples), maxDuration(samples)
	path := orStepPath(t, st, q)
	fmt.Printf("SPEC154 other_tenants rows=%d global_or=%d fetched=%d samples=%s median=%s max=%s path=%s\n",
		nFill+1+others*50000, global, fetched, formatDurations(samples), med, max, path)
	if path != recallPathBitmapAnd {
		t.Fatalf("step2 path %s", path)
	}
	if max >= 50*time.Millisecond {
		t.Fatalf("other-tenant recall max %s is not under 50ms (median %s)", max, med)
	}
}

func TestPostgresRecall50kCustomerScopeFew(t *testing.T) {
	t.Setenv("GOSO_MEMORY_RECALL", "1")
	t.Setenv("GOSO_DATABASE_URL", "")
	st := openSpec154(t, "goso_spec154_50k_cust")
	toks := RecallTokens(spec154Query)
	ag, ask, fillerSess := spec154Agent(t, st, "tenant-a")
	const needleID = "spec154-needle-cust"
	const cust = "zalo-personal:cust-a"
	if _, err := st.db.Exec(`INSERT INTO memories(id, session_id, kind, body, created_at, tenant_id, customer_key) VALUES(?,?,?,?,?,?,?)`,
		needleID, fillerSess.ID, KindEpisodic, spec154Needle, "2000-01-01T00:00:00Z", "tenant-a", cust); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if _, err := st.db.Exec(`INSERT INTO memories(id, session_id, kind, body, created_at, tenant_id, customer_key) VALUES(?,?,?,?,?,?,?)`,
			fmt.Sprintf("spec154-cust-%d", i), fillerSess.ID, KindEpisodic, "Công ty mẫu Minh An ghi mã riêng.", fmt.Sprintf("2026-01-0%dT00:00:00Z", i+1), "tenant-a", cust); err != nil {
			t.Fatal(err)
		}
	}
	bulkMemoriesAt(t, st, fillerSess.ID, "tenant-a", 49995, "spec154-o", func(i int) (string, string) {
		return "Công ty mẫu Minh An ghi mã riêng.", "2026-06-02T00:00:00Z"
	})
	if _, err := st.db.Exec(`UPDATE memories SET customer_key='zalo-personal:other' WHERE tenant_id='tenant-a' AND customer_key=''`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`ANALYZE memories`); err != nil {
		t.Fatal(err)
	}
	q := RecallQuery{TenantID: "tenant-a", AgentID: ag.ID, ExcludeSession: ask.ID, Query: spec154Query, Limit: 20, CustomerScoped: true, CustomerKey: cust}
	hits, fetched, err := st.recallTSVFetch(q, q.TenantID, ag.ID, toks, q.Limit)
	if err != nil {
		t.Fatal(err)
	}
	if fetched > recallCandidateCap || !recallHas(hits, needleID) {
		t.Fatalf("fetched %d needle %v", fetched, recallHas(hits, needleID))
	}
	match := tsOrQuery(toks)
	cq, cargs := recallTSVORCustomerQuery(q, q.TenantID, ag.ID, match, recallCandidateCap)
	plan := explainAnalyzeGUCs(t, st, recallORStableGUCs, cq, cargs...)
	fmt.Printf("SPEC154 explain_customer:\n%s\n", plan)
	requirePlan(t, plan, []string{"idx_memories_customer", "customer_key"}, []string{"Seq Scan on memories"})
	samples := timeRecall(t, st, q, needleID, 5)
	med, max := medianDuration(samples), maxDuration(samples)
	path := orStepPath(t, st, q)
	fmt.Printf("SPEC154 customer_few fetched=%d samples=%s median=%s max=%s path=%s\n",
		fetched, formatDurations(samples), med, max, path)
	if path != recallPathCustomerIndex {
		t.Fatalf("step2 path %s", path)
	}
	if max >= 50*time.Millisecond {
		t.Fatalf("customer recall max %s is not under 50ms (median %s)", max, med)
	}
}

func TestPostgresRecall50kOtherAgentFew(t *testing.T) {
	t.Setenv("GOSO_MEMORY_RECALL", "1")
	t.Setenv("GOSO_DATABASE_URL", "")
	st := openSpec154(t, "goso_spec154_50k_agent")
	toks := RecallTokens(spec154Query)
	ag, ask, needleSess := spec154Agent(t, st, "tenant-a")
	other, err := st.CreateAgent(Agent{AgentKey: "other-agent", DisplayName: "Khác", TenantID: "tenant-a"})
	if err != nil {
		t.Fatal(err)
	}
	otherSess, err := st.CreateSession(Session{AgentID: other.ID, TenantID: "tenant-a", Label: "khac"})
	if err != nil {
		t.Fatal(err)
	}
	const needleID = "spec154-needle-agent"
	if _, err := st.db.Exec(`INSERT INTO memories(id, session_id, kind, body, created_at, tenant_id, customer_key) VALUES(?,?,?,?,?,?,?)`,
		needleID, needleSess.ID, KindEpisodic, spec154Needle, "2000-01-01T00:00:00Z", "tenant-a", ""); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 15; i++ {
		if _, err := st.db.Exec(`INSERT INTO memories(id, session_id, kind, body, created_at, tenant_id, customer_key) VALUES(?,?,?,?,?,?,?)`,
			fmt.Sprintf("spec154-ag-%d", i), needleSess.ID, KindEpisodic, "Công ty mẫu Minh An ghi mã riêng.", fmt.Sprintf("2026-02-%02dT00:00:00Z", i+1), "tenant-a", ""); err != nil {
			t.Fatal(err)
		}
	}
	bulkMemories(t, st, otherSess.ID, "tenant-a", 49980, []string{"Công ty mẫu Minh An ghi mã riêng."}, "2026-06-02T00:00:00Z")
	if _, err := st.db.Exec(`ANALYZE memories`); err != nil {
		t.Fatal(err)
	}
	q := RecallQuery{TenantID: "tenant-a", AgentID: ag.ID, ExcludeSession: ask.ID, Query: spec154Query, Limit: 20}
	hits, fetched, err := st.recallTSVFetch(q, q.TenantID, ag.ID, toks, q.Limit)
	if err != nil {
		t.Fatal(err)
	}
	if fetched > recallCandidateCap || !recallHas(hits, needleID) {
		t.Fatalf("fetched %d needle %v", fetched, recallHas(hits, needleID))
	}
	match := tsOrQuery(toks)
	aq, aargs := recallTSVORAgentQuery(q, q.TenantID, ag.ID, match, recallCandidateCap)
	plan := explainAnalyzeGUCs(t, st, recallORAgentGUCs, aq, aargs...)
	fmt.Printf("SPEC154 explain_agent:\n%s\n", plan)
	requirePlan(t, plan, []string{"Nested Loop", "idx_memories_session"}, []string{"Seq Scan on memories", "idx_memories_tenant on memories"})
	samples := timeRecall(t, st, q, needleID, 5)
	med, max := medianDuration(samples), maxDuration(samples)
	path := orStepPath(t, st, q)
	fmt.Printf("SPEC154 other_agent fetched=%d samples=%s median=%s max=%s path=%s\n",
		fetched, formatDurations(samples), med, max, path)
	if path != recallPathAgentNested {
		t.Fatalf("step2 path %s", path)
	}
	if max >= 50*time.Millisecond {
		t.Fatalf("agent recall max %s is not under 50ms (median %s)", max, med)
	}
}

func spec154Agent(t *testing.T, st *PostgresStore, tenant string) (ag *Agent, ask, needle *Session) {
	t.Helper()
	var err error
	ag, err = st.CreateAgent(Agent{AgentKey: "minh-an-sales", DisplayName: "Cố vấn Minh An", TenantID: tenant})
	if err != nil {
		t.Fatal(err)
	}
	needle, err = st.CreateSession(Session{AgentID: ag.ID, TenantID: tenant, Label: "cu"})
	if err != nil {
		t.Fatal(err)
	}
	ask, err = st.CreateSession(Session{AgentID: ag.ID, TenantID: tenant, Label: "hoi"})
	if err != nil {
		t.Fatal(err)
	}
	return ag, ask, needle
}

func TestPostgresRecallORScopeIsolation(t *testing.T) {
	t.Setenv("GOSO_MEMORY_RECALL", "1")
	t.Setenv("GOSO_DATABASE_URL", "")
	st := openSpec154(t, "goso_spec154_scope")
	const matchBody = "Công ty mẫu Minh An ghi mã riêng."
	toks := RecallTokens(spec154Query)
	tests := []struct {
		name     string
		wantPath string
		lim      int
		scoped   bool
	}{
		{name: "tenant-window", wantPath: recallPathTenantWindow, lim: 4},
		{name: "customer-index", wantPath: recallPathCustomerIndex, lim: 10, scoped: true},
		{name: "agent-nested-loop", wantPath: recallPathAgentNested, lim: 10},
		{name: "bitmap-and", wantPath: recallPathBitmapAnd, lim: 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tenant := "iso-" + tc.name
			poisonTenant := tenant + "-x"
			const cust = "zalo-personal:cust-a"
			const otherCust = "zalo-personal:other"
			ag, err := st.CreateAgent(Agent{AgentKey: "sales-" + tenant, DisplayName: "Cố vấn Minh An", TenantID: tenant})
			if err != nil {
				t.Fatal(err)
			}
			good, err := st.CreateSession(Session{AgentID: ag.ID, TenantID: tenant, Label: "cu"})
			if err != nil {
				t.Fatal(err)
			}
			ask, err := st.CreateSession(Session{AgentID: ag.ID, TenantID: tenant, Label: "hoi"})
			if err != nil {
				t.Fatal(err)
			}
			otherAg, err := st.CreateAgent(Agent{AgentKey: "other-" + tenant, DisplayName: "Khác", TenantID: tenant})
			if err != nil {
				t.Fatal(err)
			}
			otherSess, err := st.CreateSession(Session{AgentID: otherAg.ID, TenantID: tenant, Label: "khac"})
			if err != nil {
				t.Fatal(err)
			}
			foreignAg, err := st.CreateAgent(Agent{AgentKey: "foreign-" + tenant, DisplayName: "Ngoại", TenantID: poisonTenant})
			if err != nil {
				t.Fatal(err)
			}
			foreignSess, err := st.CreateSession(Session{AgentID: foreignAg.ID, TenantID: poisonTenant, Label: "ngoai"})
			if err != nil {
				t.Fatal(err)
			}
			sessAgent := map[string]string{
				good.ID: ag.ID, ask.ID: ag.ID, otherSess.ID: otherAg.ID, foreignSess.ID: foreignAg.ID,
			}
			poison := map[string]bool{}
			put := func(id, sess, kind, created, ten, customer string) {
				t.Helper()
				if _, err := st.db.Exec(`INSERT INTO memories(id, session_id, kind, body, created_at, tenant_id, customer_key) VALUES(?,?,?,?,?,?,?)`,
					id, sess, kind, matchBody, created, ten, customer); err != nil {
					t.Fatal(err)
				}
			}
			mark := func(id, sess, kind, created, ten, customer string) {
				put(id, sess, kind, created, ten, customer)
				poison[id] = true
			}
			// Rows that must never come back, present on every path.
			mark(tc.name+"-foreign", foreignSess.ID, KindEpisodic, "2026-11-01T00:00:00Z", poisonTenant, cust)
			mark(tc.name+"-agent", otherSess.ID, KindEpisodic, "2026-11-02T00:00:00Z", tenant, cust)
			mark(tc.name+"-ask", ask.ID, KindEpisodic, "2026-11-03T00:00:00Z", tenant, cust)
			mark(tc.name+"-kind", good.ID, "summary", "2026-11-04T00:00:00Z", tenant, cust)
			switch tc.name {
			case "tenant-window":
				for i := 0; i < 8; i++ {
					put(fmt.Sprintf("%s-good-%d", tc.name, i), good.ID, KindEpisodic, "2026-12-01T00:00:00Z", tenant, cust)
				}
				mark(tc.name+"-cust", good.ID, KindEpisodic, "2026-01-01T00:00:00Z", tenant, otherCust)
			case "customer-index":
				for i := 0; i < 3; i++ {
					put(fmt.Sprintf("%s-good-%d", tc.name, i), good.ID, KindEpisodic, "2026-01-02T00:00:00Z", tenant, cust)
				}
				for i := 0; i < 20; i++ {
					mark(fmt.Sprintf("%s-cust-%d", tc.name, i), good.ID, KindEpisodic, "2026-12-01T00:00:00Z", tenant, otherCust)
				}
			case "agent-nested-loop":
				for i := 0; i < 3; i++ {
					put(fmt.Sprintf("%s-good-%d", tc.name, i), good.ID, KindEpisodic, "2026-01-02T00:00:00Z", tenant, cust)
				}
				put(tc.name+"-cust", good.ID, KindEpisodic, "2026-01-03T00:00:00Z", tenant, otherCust)
				for i := 0; i < 20; i++ {
					put(fmt.Sprintf("%s-newer-%d", tc.name, i), otherSess.ID, KindEpisodic, "2026-12-01T00:00:00Z", tenant, cust)
					poison[fmt.Sprintf("%s-newer-%d", tc.name, i)] = true
				}
			case "bitmap-and":
				for i := 0; i < 20; i++ {
					put(fmt.Sprintf("%s-good-%d", tc.name, i), good.ID, KindEpisodic, "2026-01-02T00:00:00Z", tenant, cust)
				}
				put(tc.name+"-cust", good.ID, KindEpisodic, "2026-01-03T00:00:00Z", tenant, otherCust)
				for i := 0; i < 24; i++ {
					put(fmt.Sprintf("%s-newer-%d", tc.name, i), otherSess.ID, KindEpisodic, "2026-12-01T00:00:00Z", tenant, cust)
					poison[fmt.Sprintf("%s-newer-%d", tc.name, i)] = true
				}
			}
			if len(poison) == 0 {
				t.Fatal("poison rows missing")
			}
			q := RecallQuery{TenantID: tenant, AgentID: ag.ID, ExcludeSession: ask.ID, Query: spec154Query, Limit: tc.lim}
			if tc.scoped {
				q.CustomerScoped = true
				q.CustomerKey = cust
			}
			hits, n, path, err := st.recallTSVORPath(q, q.TenantID, ag.ID, toks, tsOrQuery(toks), tc.lim)
			if err != nil {
				t.Fatal(err)
			}
			if path != tc.wantPath {
				t.Fatalf("path %s, want %s", path, tc.wantPath)
			}
			if len(hits) == 0 || n > tc.lim {
				t.Fatalf("hits %d fetched %d lim %d", len(hits), n, tc.lim)
			}
			for _, h := range hits {
				if poison[h.Memory.ID] {
					t.Errorf("poison row %s", h.Memory.ID)
				}
				if h.Memory.TenantID != tenant {
					t.Errorf("%s tenant %s", h.Memory.ID, h.Memory.TenantID)
				}
				if h.Memory.SessionID == ask.ID {
					t.Errorf("%s is the excluded session", h.Memory.ID)
				}
				if h.Memory.Kind != KindEpisodic && h.Memory.Kind != KindDurable {
					t.Errorf("%s kind %s", h.Memory.ID, h.Memory.Kind)
				}
				if sessAgent[h.Memory.SessionID] != ag.ID {
					t.Errorf("%s session agent %s", h.Memory.ID, sessAgent[h.Memory.SessionID])
				}
				if q.CustomerScoped && h.Memory.CustomerKey != cust {
					t.Errorf("%s customer %s", h.Memory.ID, h.Memory.CustomerKey)
				}
			}
		})
	}
}

func TestPostgresRecall50kAgentBitmapSwamp(t *testing.T) {
	t.Setenv("GOSO_MEMORY_RECALL", "1")
	t.Setenv("GOSO_DATABASE_URL", "")
	st := openSpec154(t, "goso_spec154_agent_swamp")
	toks := RecallTokens(spec154Query)
	ag, ask, good := spec154Agent(t, st, "tenant-a")
	other, err := st.CreateAgent(Agent{AgentKey: "swamp-other", DisplayName: "Khác", TenantID: "tenant-a"})
	if err != nil {
		t.Fatal(err)
	}
	otherSess, err := st.CreateSession(Session{AgentID: other.ID, TenantID: "tenant-a", Label: "khac"})
	if err != nil {
		t.Fatal(err)
	}
	const needleID = "spec154-needle-swamp-agent"
	if _, err := st.db.Exec(`INSERT INTO memories(id, session_id, kind, body, created_at, tenant_id, customer_key) VALUES(?,?,?,?,?,?,?)`,
		needleID, good.ID, KindEpisodic, spec154Needle, "2026-03-01T00:00:00Z", "tenant-a", ""); err != nil {
		t.Fatal(err)
	}
	bulkMemoriesAt(t, st, good.ID, "tenant-a", 4999, "swamp-a", func(int) (string, string) {
		return "Công ty mẫu Minh An ghi mã riêng.", "2026-01-01T00:00:00Z"
	})
	bulkMemoriesAt(t, st, otherSess.ID, "tenant-a", 45000, "swamp-b", func(int) (string, string) {
		return "Công ty mẫu Minh An ghi mã riêng.", "2026-06-02T00:00:00Z"
	})
	if _, err := st.db.Exec(`ANALYZE memories`); err != nil {
		t.Fatal(err)
	}
	q := RecallQuery{TenantID: "tenant-a", AgentID: ag.ID, ExcludeSession: ask.ID, Query: spec154Query, Limit: 20}
	hits, fetched, err := st.recallTSVFetch(q, q.TenantID, ag.ID, toks, q.Limit)
	if err != nil {
		t.Fatal(err)
	}
	if fetched > recallCandidateCap || !recallHas(hits, needleID) {
		t.Fatalf("fetched %d needle %v", fetched, recallHas(hits, needleID))
	}
	_, n1, err := st.recallTSVStep(q, q.TenantID, q.AgentID, toks, tsAndQuery(toks), recallCandidateCap)
	if err != nil {
		t.Fatal(err)
	}
	match := tsOrQuery(toks)
	bq, bargs := recallTSVORBitmapQuery(q, q.TenantID, ag.ID, match, recallORLimit(n1))
	plan := explainAnalyzeGUCs(t, st, recallORBitmapGUCs, bq, bargs...)
	fmt.Printf("SPEC154 explain_agent_swamp:\n%s\n", plan)
	requirePlan(t, plan, []string{"idx_memories_session"}, []string{"Seq Scan on memories"})
	samples := timeRecall(t, st, q, needleID, 5)
	med, max := medianDuration(samples), maxDuration(samples)
	path := orStepPath(t, st, q)
	fmt.Printf("SPEC154 agent_swamp fetched=%d samples=%s median=%s max=%s path=%s\n",
		fetched, formatDurations(samples), med, max, path)
	if path != recallPathBitmapAnd {
		t.Fatalf("step2 path %s", path)
	}
	if max >= 50*time.Millisecond {
		t.Fatalf("agent swamp recall max %s is not under 50ms (median %s)", max, med)
	}
}

func TestPostgresRecall50kCustomerBitmapSwamp(t *testing.T) {
	t.Setenv("GOSO_MEMORY_RECALL", "1")
	t.Setenv("GOSO_DATABASE_URL", "")
	st := openSpec154(t, "goso_spec154_cust_swamp")
	toks := RecallTokens(spec154Query)
	ag, ask, good := spec154Agent(t, st, "tenant-a")
	const needleID = "spec154-needle-swamp-cust"
	const cust = "zalo-personal:cust-a"
	if _, err := st.db.Exec(`INSERT INTO memories(id, session_id, kind, body, created_at, tenant_id, customer_key) VALUES(?,?,?,?,?,?,?)`,
		needleID, good.ID, KindEpisodic, spec154Needle, "2026-03-01T00:00:00Z", "tenant-a", cust); err != nil {
		t.Fatal(err)
	}
	bulkMemoriesAt(t, st, good.ID, "tenant-a", 4999, "swamp-c", func(int) (string, string) {
		return "Công ty mẫu Minh An ghi mã riêng.", "2026-01-01T00:00:00Z"
	})
	if _, err := st.db.Exec(`UPDATE memories SET customer_key=? WHERE id LIKE 'swamp-c-%'`, cust); err != nil {
		t.Fatal(err)
	}
	bulkMemoriesAt(t, st, good.ID, "tenant-a", 45000, "swamp-d", func(int) (string, string) {
		return "Công ty mẫu Minh An ghi mã riêng.", "2026-06-02T00:00:00Z"
	})
	if _, err := st.db.Exec(`UPDATE memories SET customer_key='zalo-personal:other' WHERE id LIKE 'swamp-d-%'`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`ANALYZE memories`); err != nil {
		t.Fatal(err)
	}
	q := RecallQuery{TenantID: "tenant-a", AgentID: ag.ID, ExcludeSession: ask.ID, Query: spec154Query, Limit: 20, CustomerScoped: true, CustomerKey: cust}
	hits, fetched, err := st.recallTSVFetch(q, q.TenantID, ag.ID, toks, q.Limit)
	if err != nil {
		t.Fatal(err)
	}
	if fetched > recallCandidateCap || !recallHas(hits, needleID) {
		t.Fatalf("fetched %d needle %v", fetched, recallHas(hits, needleID))
	}
	_, n1, err := st.recallTSVStep(q, q.TenantID, q.AgentID, toks, tsAndQuery(toks), recallCandidateCap)
	if err != nil {
		t.Fatal(err)
	}
	match := tsOrQuery(toks)
	bq, bargs := recallTSVORBitmapQuery(q, q.TenantID, ag.ID, match, recallORLimit(n1))
	plan := explainAnalyzeGUCs(t, st, recallORBitmapGUCs, bq, bargs...)
	fmt.Printf("SPEC154 explain_customer_swamp:\n%s\n", plan)
	requirePlan(t, plan, []string{"idx_memories_customer", "customer_key"}, []string{"Seq Scan on memories"})
	samples := timeRecall(t, st, q, needleID, 5)
	med, max := medianDuration(samples), maxDuration(samples)
	path := orStepPath(t, st, q)
	fmt.Printf("SPEC154 customer_swamp fetched=%d samples=%s median=%s max=%s path=%s\n",
		fetched, formatDurations(samples), med, max, path)
	if path != recallPathBitmapAnd {
		t.Fatalf("step2 path %s", path)
	}
	if max >= 50*time.Millisecond {
		t.Fatalf("customer swamp recall max %s is not under 50ms (median %s)", max, med)
	}
}

func TestPostgresRecallOldestFullMatch(t *testing.T) {
	t.Setenv("GOSO_MEMORY_RECALL", "1")
	t.Setenv("GOSO_DATABASE_URL", "")
	st := openSpec154(t, "goso_spec154_oldest")
	toks := RecallTokens(spec154Query)
	const filler = "Công ty mẫu Minh An ghi mã riêng."
	nTok := 0
	for _, tok := range toks {
		if strings.Contains(strings.ToLower(filler), tok) {
			nTok++
		}
	}
	if nTok != 1 {
		t.Fatalf("common filler matches %d query tokens", nTok)
	}
	ag, err := st.CreateAgent(Agent{AgentKey: "minh-an-sales", DisplayName: "Cố vấn Minh An"})
	if err != nil {
		t.Fatal(err)
	}
	needleSess, err := st.CreateSession(Session{AgentID: ag.ID, Label: "cu"})
	if err != nil {
		t.Fatal(err)
	}
	fillerSess, err := st.CreateSession(Session{AgentID: ag.ID, Label: "moi"})
	if err != nil {
		t.Fatal(err)
	}
	ask, err := st.CreateSession(Session{AgentID: ag.ID, Label: "hoi"})
	if err != nil {
		t.Fatal(err)
	}
	const needleID = "spec154-needle-oldest"
	const nFill = 49999
	needleBody := spec154Needle + " gì"
	for _, tok := range toks {
		if !strings.Contains(strings.ToLower(needleBody), tok) {
			t.Fatalf("needle missing query token %q", tok)
		}
	}
	if _, err := st.db.Exec(`INSERT INTO memories(id, session_id, kind, body, created_at, tenant_id, customer_key) VALUES(?,?,?,?,?,?,?)`,
		needleID, needleSess.ID, KindEpisodic, needleBody, "2000-01-01T00:00:00Z", needleSess.TenantID, ""); err != nil {
		t.Fatal(err)
	}
	bulkMemories(t, st, fillerSess.ID, fillerSess.TenantID, nFill, []string{filler}, "2026-06-01T00:00:00Z")
	if _, err := st.db.Exec(`ANALYZE memories`); err != nil {
		t.Fatal(err)
	}
	newest, err := st.db.Query(`SELECT id FROM memories ORDER BY created_at DESC LIMIT ?`, recallCandidateCap)
	if err != nil {
		t.Fatal(err)
	}
	for newest.Next() {
		var id string
		if err := newest.Scan(&id); err != nil {
			t.Fatal(err)
		}
		if id == needleID {
			t.Fatal("pure newest cap already contains the oldest needle")
		}
	}
	newest.Close()
	q := RecallQuery{TenantID: needleSess.TenantID, AgentID: ag.ID, ExcludeSession: ask.ID, Query: spec154Query, Limit: 20}
	hits, fetched, err := st.recallTSVFetch(q, q.TenantID, ag.ID, toks, q.Limit)
	if err != nil {
		t.Fatal(err)
	}
	if fetched > recallCandidateCap {
		t.Fatalf("fetched %d, cap %d", fetched, recallCandidateCap)
	}
	if !recallHas(hits, needleID) {
		t.Fatal("oldest full-match needle was not recalled")
	}
	got, err := st.RecallMemories(q)
	if err != nil || !recallHas(got, needleID) {
		t.Fatalf("RecallMemories %#v %v", got, err)
	}
}

func TestPostgresSearchMinDoesNotMatchMinh(t *testing.T) {
	t.Setenv("GOSO_DATABASE_URL", "")
	st := openSpec154(t, "goso_spec154_prefix")
	ag, err := st.CreateAgent(Agent{AgentKey: "minh-an-sales", DisplayName: "Cố vấn Minh An"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(Session{AgentID: ag.ID, Label: "cu"})
	if err != nil {
		t.Fatal(err)
	}
	note, err := st.PutMemory(Memory{SessionID: sess.ID, TenantID: sess.TenantID, Kind: KindEpisodic, Body: "Minh An gửi báo giá dạng PDF."})
	if err != nil {
		t.Fatal(err)
	}
	short, err := st.SearchMemory("Min")
	if err != nil {
		t.Fatal(err)
	}
	if len(short) != 0 {
		t.Fatalf("Min must not match Minh, got %#v", short)
	}
	full, err := st.SearchMemory("Minh")
	if err != nil || len(full) != 1 || full[0].ID != note.ID {
		t.Fatalf("Minh search %#v %v", full, err)
	}
}

func recallOlderThanCap(t *testing.T, st StoreIface) {
	t.Helper()
	for _, body := range spec154Fillers {
		for _, tok := range RecallTokens(spec154Query) {
			if strings.Contains(strings.ToLower(body), tok) {
				t.Fatalf("filler %q contains query token %q", body, tok)
			}
		}
	}
	ag, err := st.CreateAgent(Agent{AgentKey: "minh-an-sales", DisplayName: "Cố vấn Minh An"})
	if err != nil {
		t.Fatal(err)
	}
	oldSess, err := st.CreateSession(Session{AgentID: ag.ID, Label: "cu"})
	if err != nil {
		t.Fatal(err)
	}
	newSess, err := st.CreateSession(Session{AgentID: ag.ID, Label: "moi"})
	if err != nil {
		t.Fatal(err)
	}
	ask, err := st.CreateSession(Session{AgentID: ag.ID, Label: "hoi"})
	if err != nil {
		t.Fatal(err)
	}
	note, err := st.PutMemory(Memory{SessionID: oldSess.ID, TenantID: oldSess.TenantID, Kind: KindEpisodic, Body: spec154Needle})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < recallScanCap; i++ {
		if _, err := st.PutMemory(Memory{
			SessionID: newSess.ID, TenantID: newSess.TenantID, Kind: KindEpisodic,
			Body: spec154Fillers[i%len(spec154Fillers)],
		}); err != nil {
			t.Fatal(err)
		}
	}
	forceCreatedAt(t, st, note.ID, "2000-01-01T00:00:00Z")
	forceSessionCreatedAt(t, st, newSess.ID, "2026-06-01T00:00:00Z")
	q := RecallQuery{TenantID: oldSess.TenantID, AgentID: ag.ID, ExcludeSession: ask.ID, Query: spec154Query}
	hits, err := st.RecallMemories(q)
	if err != nil || !recallHas(hits, note.ID) {
		t.Fatalf("store recall %#v %v", hits, err)
	}
	for _, h := range hits {
		if h.Memory.ID == note.ID && h.Score <= 0 {
			t.Fatalf("needle score %v", h.Score)
		}
	}
}

func forceCreatedAt(t *testing.T, st StoreIface, id, ts string) {
	t.Helper()
	db := sqlDB(st)
	if db == nil {
		return
	}
	res, err := db.Exec(`UPDATE memories SET created_at=? WHERE id=?`, ts, id)
	if err != nil {
		t.Fatal(err)
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		t.Fatalf("backdate %s affected %d", id, n)
	}
}

func forceSessionCreatedAt(t *testing.T, st StoreIface, sessionID, ts string) {
	t.Helper()
	db := sqlDB(st)
	if db == nil {
		return
	}
	if _, err := db.Exec(`UPDATE memories SET created_at=? WHERE session_id=?`, ts, sessionID); err != nil {
		t.Fatal(err)
	}
}

func sqlDB(st StoreIface) *sqlHandle {
	switch s := st.(type) {
	case *SQLiteStore:
		return s.db
	case *PostgresStore:
		return s.db
	default:
		return nil
	}
}

func bulkMemories(t *testing.T, st *PostgresStore, sessionID, tenant string, n int, bodies []string, created string) {
	t.Helper()
	bulkMemoriesAt(t, st, sessionID, tenant, n, "spec154-f", func(i int) (string, string) {
		return bodies[i%len(bodies)], created
	})
}

func bulkMemoriesAt(t *testing.T, st *PostgresStore, sessionID, tenant string, n int, idPrefix string, row func(i int) (body, created string)) {
	t.Helper()
	tx, err := st.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	const batch = 400
	for seq := 0; seq < n; {
		end := seq + batch
		if end > n {
			end = n
		}
		var b strings.Builder
		args := make([]any, 0, (end-seq)*7)
		b.WriteString(`INSERT INTO memories(id, session_id, kind, body, created_at, tenant_id, customer_key) VALUES`)
		for i := seq; i < end; i++ {
			if i > seq {
				b.WriteByte(',')
			}
			b.WriteString(`(?,?,?,?,?,?,?)`)
			body, created := row(i)
			args = append(args, fmt.Sprintf("%s-%d", idPrefix, i), sessionID, KindEpisodic, body, created, tenant, "")
		}
		if _, err := tx.Exec(b.String(), args...); err != nil {
			t.Fatal(err)
		}
		seq = end
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	committed = true
}

func timeRecall(t *testing.T, st *PostgresStore, q RecallQuery, needleID string, n int) []time.Duration {
	t.Helper()
	out := make([]time.Duration, 0, n)
	for i := 0; i < n; i++ {
		start := time.Now()
		hits, err := st.RecallMemories(q)
		elapsed := time.Since(start)
		if err != nil || !recallHas(hits, needleID) {
			t.Fatalf("sample %d %#v %v", i, hits, err)
		}
		out = append(out, elapsed)
	}
	return out
}

func medianDuration(ds []time.Duration) time.Duration {
	cp := append([]time.Duration(nil), ds...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	return cp[len(cp)/2]
}

func maxDuration(ds []time.Duration) time.Duration {
	max := ds[0]
	for _, d := range ds[1:] {
		if d > max {
			max = d
		}
	}
	return max
}

func recallORLimit(step1Fetched int) int {
	lim := recallCandidateCap - step1Fetched
	if lim < 1 {
		return 1
	}
	return lim
}

func orStepPath(t *testing.T, st *PostgresStore, q RecallQuery) string {
	t.Helper()
	toks := RecallTokens(q.Query)
	_, n1, err := st.recallTSVStep(q, q.TenantID, q.AgentID, toks, tsAndQuery(toks), recallCandidateCap)
	if err != nil {
		t.Fatal(err)
	}
	_, _, path, err := st.recallTSVORPath(q, q.TenantID, q.AgentID, toks, tsOrQuery(toks), recallORLimit(n1))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func logStep2Plan(t *testing.T, st *PostgresStore, q RecallQuery, toks []string, orLim int, wantProbeFilled bool) {
	t.Helper()
	match := tsOrQuery(toks)
	budget := orRecallProbeBudget(orLim)
	pq, pargs := recallTSVORProbeQuery(q, q.TenantID, q.AgentID, match, budget, orLim)
	probePlan := explainAnalyzeGUCs(t, st, recallORStableGUCs, pq, pargs...)
	fmt.Printf("SPEC154 explain_or_probe budget=%d limit=%d:\n%s\n", budget, orLim, probePlan)
	requirePlan(t, probePlan,
		[]string{"Index Scan using " + idxMemoriesTenantCreated},
		[]string{"Seq Scan on memories", "Bitmap Index Scan on " + idxMemoriesBodyTSV})
	_, nProbe, err := st.execRecallTSV(toks, pq, pargs, recallORStableGUCs)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("SPEC154 or_probe_rows=%d limit=%d\n", nProbe, orLim)
	if wantProbeFilled {
		if nProbe < orLim {
			t.Fatalf("probe returned %d rows, want at least %d so step 2 stays on %s", nProbe, orLim, idxMemoriesTenantCreated)
		}
		fmt.Printf("SPEC154 step2_path=Index Scan using %s\n", idxMemoriesTenantCreated)
		return
	}
	if nProbe >= orLim {
		t.Fatalf("probe returned %d rows, want fewer than %d so step 2 uses the GIN bitmap", nProbe, orLim)
	}
	bq, bargs := recallTSVORBitmapQuery(q, q.TenantID, q.AgentID, match, orLim)
	bitPlan := explainAnalyzeGUCs(t, st, recallORBitmapGUCs, bq, bargs...)
	fmt.Printf("SPEC154 explain_or_bitmap limit=%d:\n%s\n", orLim, bitPlan)
	requirePlan(t, bitPlan, nil,
		[]string{"Seq Scan on memories", "Index Scan using " + idxMemoriesTenantCreated})
	fmt.Printf("SPEC154 step2_path=bitmap idx_memories_body_tsv\n")
}

func requirePlan(t *testing.T, plan string, must, mustNot []string) {
	t.Helper()
	for _, s := range must {
		if !strings.Contains(plan, s) {
			t.Fatalf("plan missing %q\n%s", s, plan)
		}
	}
	for _, s := range mustNot {
		if strings.Contains(plan, s) {
			t.Fatalf("plan contains %q\n%s", s, plan)
		}
	}
}

func explainAnalyzeGUCs(t *testing.T, st *PostgresStore, gucs []string, query string, args ...any) string {
	t.Helper()
	tx, err := st.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for _, g := range gucs {
		if _, err := tx.Exec(g); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := tx.Query(`EXPLAIN (ANALYZE, BUFFERS) `+query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func formatDurations(ds []time.Duration) string {
	parts := make([]string, len(ds))
	for i, d := range ds {
		parts[i] = d.String()
	}
	return strings.Join(parts, ",")
}

func explainAnalyze(t *testing.T, st *PostgresStore, q RecallQuery, match string, lim int) string {
	t.Helper()
	query, args := recallTSVStepQuery(q, q.TenantID, q.AgentID, match, lim)
	return explain(t, st, `EXPLAIN (ANALYZE, BUFFERS) `+query, args...)
}

func explain(t *testing.T, st *PostgresStore, query string, args ...any) string {
	t.Helper()
	rows, err := st.db.Query(query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func recallHas(hits []RecallHit, id string) bool {
	for _, h := range hits {
		if h.Memory.ID == id {
			return true
		}
	}
	return false
}

func hitIDs(hits []KGSearchHit, id string) bool {
	for _, h := range hits {
		if h.ID == id {
			return true
		}
	}
	return false
}

func assertColumn(t *testing.T, st *PostgresStore, table, column, dataType, generated string) {
	t.Helper()
	var gotType, gotGen string
	err := st.db.QueryRow(`SELECT data_type, is_generated FROM information_schema.columns
		WHERE table_schema=current_schema() AND table_name=? AND column_name=?`, table, column).Scan(&gotType, &gotGen)
	if err != nil {
		t.Fatalf("%s.%s: %v", table, column, err)
	}
	if gotType != dataType || gotGen != generated {
		t.Fatalf("%s.%s type=%s generated=%s, want %s %s", table, column, gotType, gotGen, dataType, generated)
	}
}

func openSpec154(t *testing.T, schema string) *PostgresStore {
	t.Helper()
	base := postgresTestDSN()
	if base == "" {
		if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
			t.Fatal("GOSO_TEST_POSTGRES_DSN is required in CI")
		}
		t.Skip("GOSO_TEST_POSTGRES_DSN unset")
	}
	dsn := prepareSchema(t, base, schema, false)
	st, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db, err := sql.Open("pgx", base)
		if err != nil {
			return
		}
		defer db.Close()
		_, _ = db.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
	})
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func dsnFromStore(t *testing.T, st *PostgresStore) string {
	t.Helper()
	var schema string
	if err := st.db.QueryRow(`SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	out, err := dsnWithSearchPath(postgresTestDSN(), schema)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
