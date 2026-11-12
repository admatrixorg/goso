// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package episodicimport

import (
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mqglobal/goso/gateway/internal/store"
)

const (
	srcMinh    = "11111111-1111-4111-8111-111111111111"
	srcB       = "22222222-2222-4222-8222-222222222222"
	srcMQ      = "33333333-3333-4333-8333-333333333333"
	srcUnknown = "44444444-4444-4444-8444-444444444444"
	srcAgent1  = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1"
	srcAgent2  = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2"
	srcAgent3  = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa3"
	srcAgentB  = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb1"

	markerL0     = "L0MARKERdrop"
	markerEmbed  = "EMBEDMARKERvector"
	markerRecall = "RECALLMARKERdrop"
	markerTSV    = "TSVMARKERdrop"
	markerMQ     = "MQSKIPMARKER"
	liveBody     = "live summary two"
	liveInside   = "LIVEINSIDE minh an"
	liveInsideID = "live-inside-gc"
	otherGCID    = "gc-other:minh-an"
	customerX    = "zalo-personal:thread-minh-an-x"
	customerY    = "zalo-personal:thread-minh-an-y"
	goclawX      = "goclaw:user-minh-an-01"
	goclawY      = "goclaw:user-minh-an-02"
	goclawSender = "goclaw:ext-minh-an-0000"
)

var clock = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

func TestCustomerKeyAndBody(t *testing.T) {
	if CustomerKey("") != "" || CustomerKey("   ") != "" {
		t.Fatal("only an empty user_id may map to an empty customer_key")
	}
	for _, id := range []string{"user-minh-an-01", "ext-minh-an-0000", "user-minh-an-02"} {
		key := CustomerKey(" " + id + " ")
		if key != CustomerPrefix+id || key == "" || strings.HasPrefix(key, "zalo-") {
			t.Fatalf("key %q", key)
		}
	}
	got := Body("Bien ban Minh An.\n", []string{" hop dong ", "", "chiet khau"})
	if got != "Bien ban Minh An.\nTopics: hop dong, chiet khau" || strings.Count(got, "\n") != 1 {
		t.Fatalf("body %q", got)
	}
	if Body("   ", nil) != "" {
		t.Fatal("blank summary")
	}
	if Body("", []string{"van de"}) != "Topics: van de" {
		t.Fatal("topics only")
	}
	id := SessionID("agent-minh-an", "sess-minh-an-01")
	if id != SessionID("agent-minh-an", "sess-minh-an-01") || !strings.HasPrefix(id, "gc-sess:") || len(id) != len("gc-sess:")+64 {
		t.Fatalf("session id %s", id)
	}
	if SessionID("agent-minh-an", "sess-minh-an-01") == SessionID("agent-other", "sess-minh-an-01") {
		t.Fatal("session id must include the goso agent")
	}
	if MemoryID("10000000-0000-4000-8000-00000000000A") != "gc-ep:10000000-0000-4000-8000-00000000000a" {
		t.Fatal("memory id")
	}
}

func TestMappingFiles(t *testing.T) {
	tenants := readTenants(t)
	mq, ok := tenants.Lookup(srcMQ)
	if !ok || !mq.SkipMQ || mq.Slug != "mq-capital" {
		t.Fatalf("mq entry %+v %v", mq, ok)
	}
	minh, ok := tenants.Lookup(srcMinh)
	if !ok || minh.SkipMQ || minh.DestTenant != "minh-an" {
		t.Fatalf("minh %+v", minh)
	}
	agents := readAgents(t)
	missing, ok := agents.Lookup(srcAgent3)
	if !ok || missing.DestAgent != "missing-minh-an-agent" || missing.DestTenant != "minh-an" {
		t.Fatalf("missing agent %+v", missing)
	}
	dup := `{"source_tenant":"11111111-1111-4111-8111-111111111111","slug":"minh-an","dest_tenant":"minh-an"}
{"source_tenant":"11111111-1111-4111-8111-111111111111","slug":"minh-an","dest_tenant":"minh-an"}
`
	if _, err := LoadTenants(strings.NewReader(dup)); err == nil {
		t.Fatal("duplicate source must fail")
	}
	if _, err := LoadAgents(strings.NewReader(`{"source_agent":"a","dest_agent":"b","dest_tenant":"mq-capital"}` + "\n")); err == nil {
		t.Fatal("mq agent map must fail")
	}
}

func TestMinhAnEpisodic(t *testing.T) {
	rows := readFixture(t)
	for name, open := range backends(t) {
		t.Run(name, func(t *testing.T) {
			st := open(t)
			db := asDB(t, st)
			agent1 := mustAgent(t, st, "minh-an", "minh-an-cskh")
			agent2 := mustAgent(t, st, "minh-an", "minh-an-sales")
			agentB := mustAgent(t, st, "tenant-b", "tenant-b-cskh")
			agents := mustAgents(t, agent1, agent2, agentB)
			opt := Options{Tenants: readTenants(t), Agents: agents, Now: clock, DryRun: true}
			dry, err := Import(db, rows, opt)
			if err != nil {
				t.Fatal(err)
			}
			assertReport(t, dry, true)
			if _, err := db.GetMemory(MemoryID("10000000-0000-4000-8000-000000000001")); !errors.Is(err, store.ErrNotFound) {
				t.Fatal("dry-run wrote")
			}

			opt.DryRun = false
			rep, err := Import(db, rows, opt)
			if err != nil {
				t.Fatal(err)
			}
			assertReport(t, rep, false)
			assertWritten(t, db, agent1, agent2, agentB)

			assertRecall(t, st, agent1)
			live := assertLiveWrite(t, st, agent1, db)

			opt2 := opt
			again, err := Import(db, rows, opt2)
			if err != nil {
				t.Fatal(err)
			}
			minh := section(t, again, srcMinh)
			if minh.Write != 0 || minh.Sessions != 0 || minh.Already != 9 {
				t.Fatalf("rerun %+v", minh)
			}
			b := section(t, again, srcB)
			if b.Write != 0 || b.Sessions != 0 || b.Already != 1 {
				t.Fatalf("rerun tenant b %+v", b)
			}
			first := mustMemory(t, db, "10000000-0000-4000-8000-000000000001")
			if !strings.Contains(first.Body, "khach X") || first.CustomerKey != goclawX {
				t.Fatalf("rerun changed first note %+v", first)
			}

			sess := mustSession(t, db, first.SessionID)
			renamed, err := st.UpdateSession(store.Session{ID: sess.ID, TouchLabel: true, Label: "renamed-minh-an"})
			if err != nil || renamed.Label != "renamed-minh-an" {
				t.Fatal(err)
			}
			if _, err := Import(db, rows, opt); err != nil {
				t.Fatal(err)
			}
			if got := mustSession(t, db, sess.ID); got.Label != "renamed-minh-an" || !got.Archived {
				t.Fatalf("second run rewrote session %+v", got)
			}

			latest, err := st.LatestSummary(sess.ID)
			if err != nil {
				t.Fatal(err)
			}
			earlierID := MemoryID("10000000-0000-4000-8000-000000000001")
			if latest.ID == earlierID {
				t.Fatal("expected the later note to be latest")
			}
			over, err := st.SaveSummary(sess.ID, "summary overwritten in archive")
			if err != nil {
				t.Fatal(err)
			}
			if over.ID != latest.ID || mustMemory(t, db, earlierID).Body == over.Body {
				t.Fatal("archive SaveSummary must change only the latest episodic")
			}
			if _, err := Import(db, rows, opt); err != nil {
				t.Fatal(err)
			}
			if mustMemory(t, db, latest.ID).Body != "summary overwritten in archive" {
				t.Fatal("rerun restored an overwritten note")
			}
			if mustMemory(t, db, earlierID).CustomerKey != goclawX {
				t.Fatal("earlier note changed")
			}
			if got, err := st.GetMemory(live.ID); err != nil || got.Body != liveBody {
				t.Fatalf("live note %v %v", got, err)
			}

			insertPlain(t, st, liveInsideID, sess.ID, liveInside, "minh-an", customerX, "2026-02-01T00:00:00Z")
			insertPlain(t, st, otherGCID, sess.ID, "gc other minh an", "minh-an", goclawX, "2026-02-01T00:00:00Z")
			otherSess := mustMemory(t, db, "10000000-0000-4000-8000-000000000007").SessionID
			if otherSess == sess.ID {
				t.Fatal("fixture session")
			}
			memN, sessN, err := db.DeleteGCPrefix("minh-an")
			if err != nil {
				t.Fatal(err)
			}
			if memN != 10 || sessN != 7 {
				t.Fatalf("removed memories=%d sessions=%d", memN, sessN)
			}
			if _, err := db.GetMemory(earlierID); !errors.Is(err, store.ErrNotFound) {
				t.Fatal("imported note survived removal")
			}
			if _, err := db.GetMemory(otherGCID); !errors.Is(err, store.ErrNotFound) {
				t.Fatal("gc- prefix note survived removal")
			}
			kept, err := db.GetMemory(liveInsideID)
			if err != nil || kept.Body != liveInside || kept.CustomerKey != customerX {
				t.Fatalf("live note inside gc session %v %v", kept, err)
			}
			if _, err := db.GetSession(sess.ID); err != nil {
				t.Fatal("session with a live note was deleted")
			}
			if _, err := db.GetMemory(MemoryID("10000000-0000-4000-8000-00000000000c")); err != nil {
				t.Fatal("tenant b was removed with minh-an")
			}
			if got, err := st.GetMemory(live.ID); err != nil || got.Body != liveBody {
				t.Fatal("live session note was removed")
			}
			if _, err := db.GetSession(otherSess); !errors.Is(err, store.ErrNotFound) {
				t.Fatal("empty gc session survived removal")
			}
			if _, _, err := db.DeleteGCPrefix("tenant-b"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.GetMemory(MemoryID("10000000-0000-4000-8000-00000000000c")); !errors.Is(err, store.ErrNotFound) {
				t.Fatal("tenant b import survived its own removal")
			}
			if _, err := st.GetMemory(live.ID); err != nil {
				t.Fatal("tenant b removal touched minh-an")
			}
		})
	}
}

func assertReport(t *testing.T, rep *Report, dry bool) {
	t.Helper()
	if rep.DryRun != dry || rep.Table != "episodic_summaries" {
		t.Fatalf("report header %+v", rep)
	}
	minh := section(t, rep, srcMinh)
	if minh.DestTenant != "minh-an" || minh.Slug != "minh-an" || minh.Read != 14 || minh.Write != 9 || minh.Already != 0 || minh.Sessions != 8 {
		t.Fatalf("minh %+v", minh)
	}
	if minh.Skipped != (SkipCounts{Expired: 1, TeamScope: 2, Unmapped: 1, Invalid: 1}) {
		t.Fatalf("minh skips %+v", minh.Skipped)
	}
	b := section(t, rep, srcB)
	if b.DestTenant != "tenant-b" || b.Read != 1 || b.Write != 1 || b.Sessions != 1 || b.Skipped != (SkipCounts{}) {
		t.Fatalf("tenant b %+v", b)
	}
	mq := section(t, rep, srcMQ)
	if mq.Read != 2 || mq.Write != 0 || mq.Sessions != 0 || mq.Skipped.MQ != 2 || mq.Skipped.Expired != 0 || mq.DestTenant != "" {
		t.Fatalf("mq %+v", mq)
	}
	slug := section(t, rep, "mq-capital")
	if slug.Read != 1 || slug.Write != 0 || slug.Skipped.MQ != 1 || slug.Sessions != 0 {
		t.Fatalf("mq slug %+v", slug)
	}
	unknown := section(t, rep, srcUnknown)
	if unknown.Read != 1 || unknown.Write != 0 || unknown.Skipped.Unmapped != 1 || unknown.DestTenant != "" {
		t.Fatalf("unknown %+v", unknown)
	}
}

func assertWritten(t *testing.T, db DB, agent1, agent2, agentB string) {
	t.Helper()
	first := mustMemory(t, db, "10000000-0000-4000-8000-000000000001")
	if first.Body != "Bien ban Minh An hop dong mau khach X.\nTopics: hop dong, chiet khau" {
		t.Fatalf("body %q", first.Body)
	}
	if strings.Count(first.Body, "\n") != 1 {
		t.Fatal("expected one topics line")
	}
	for _, marker := range []string{markerL0, markerEmbed, markerRecall, markerTSV, markerMQ} {
		if strings.Contains(first.Body, marker) {
			t.Fatalf("dropped column leaked: %s", marker)
		}
	}
	wantAt, err := time.Parse(time.RFC3339Nano, "2026-01-02T03:04:05.5+07:00")
	if err != nil {
		t.Fatal(err)
	}
	if !first.CreatedAt.Equal(wantAt) || first.CreatedAt.Location() != time.UTC {
		t.Fatalf("created_at %s", first.CreatedAt.Format(time.RFC3339Nano))
	}
	if first.Kind != store.KindEpisodic || first.CustomerKey != goclawX || first.TenantID != "minh-an" {
		t.Fatalf("first %+v", first)
	}
	sess := mustSession(t, db, first.SessionID)
	if sess.ID != SessionID(agent1, "sess-minh-an-01") || !sess.Archived || sess.Label != "gc-import" || sess.AgentID != agent1 || !sess.CreatedAt.Equal(wantAt) {
		t.Fatalf("session %+v", sess)
	}
	second := mustMemory(t, db, "10000000-0000-4000-8000-000000000002")
	if second.SessionID != first.SessionID || second.CustomerKey != goclawX {
		t.Fatalf("same session %+v", second)
	}
	checks := map[string]string{
		"10000000-0000-4000-8000-000000000003": goclawY,
		"10000000-0000-4000-8000-000000000004": "",
		"10000000-0000-4000-8000-000000000006": "goclaw:user-minh-an-04",
		"10000000-0000-4000-8000-000000000007": "goclaw:user-minh-an-05",
		"10000000-0000-4000-8000-00000000000b": goclawSender,
		"10000000-0000-4000-8000-000000000011": "goclaw:user-minh-an-12",
		"10000000-0000-4000-8000-000000000013": "",
		"10000000-0000-4000-8000-00000000000c": "goclaw:user-tenant-b-01",
	}
	for id, key := range checks {
		m := mustMemory(t, db, id)
		if m.CustomerKey != key {
			t.Fatalf("%s customer_key %q want %q", id, m.CustomerKey, key)
		}
		if key == "" && strings.TrimSpace(m.CustomerKey) != "" {
			t.Fatal("empty key")
		}
		if key != "" && (m.CustomerKey == "" || !strings.HasPrefix(m.CustomerKey, "goclaw:")) {
			t.Fatalf("non-empty user stored as %q", m.CustomerKey)
		}
	}
	topics := mustMemory(t, db, "10000000-0000-4000-8000-000000000011")
	if topics.Body != "Topics: van de" {
		t.Fatalf("topics only %q", topics.Body)
	}
	sender := mustMemory(t, db, "10000000-0000-4000-8000-00000000000b")
	if sender.SessionID != SessionID(agent2, "sess-minh-an-10") || strings.Contains(sender.CustomerKey, "zalo") {
		t.Fatalf("sender %+v", sender)
	}
	other := mustMemory(t, db, "10000000-0000-4000-8000-00000000000c")
	if other.TenantID != "tenant-b" || other.SessionID != SessionID(agentB, "sess-tenant-b-01") {
		t.Fatalf("tenant b %+v", other)
	}
	for _, id := range []string{
		"10000000-0000-4000-8000-000000000005",
		"10000000-0000-4000-8000-000000000008",
		"10000000-0000-4000-8000-000000000009",
		"10000000-0000-4000-8000-00000000000a",
		"10000000-0000-4000-8000-000000000010",
		"10000000-0000-4000-8000-00000000000d",
		"10000000-0000-4000-8000-00000000000e",
		"10000000-0000-4000-8000-00000000000f",
		"10000000-0000-4000-8000-000000000012",
	} {
		if _, err := db.GetMemory(MemoryID(id)); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("skipped row %s was written (%v)", id, err)
		}
	}
}

func assertRecall(t *testing.T, st store.StoreIface, agent1 string) {
	t.Helper()
	xSess := mustSessionCreate(t, st, agent1, "minh-an", customerX)
	if _, err := st.PutMemory(store.Memory{SessionID: xSess, TenantID: "minh-an", Kind: store.KindEpisodic, Body: "Bien ban khach that X"}); err != nil {
		t.Fatal(err)
	}
	ySess := mustSessionCreate(t, st, agent1, "minh-an", customerY)
	if _, err := st.PutMemory(store.Memory{SessionID: ySess, TenantID: "minh-an", Kind: store.KindEpisodic, Body: "Bien ban khach that Y"}); err != nil {
		t.Fatal(err)
	}
	hits := mustRecall(t, st, store.RecallQuery{
		TenantID: "minh-an", AgentID: agent1, ExcludeSession: "ask-minh-an",
		Query: "Bien ban", Limit: 30, CustomerScoped: true, CustomerKey: customerX,
	})
	if len(hits) == 0 {
		t.Fatal("customer X recall returned nothing")
	}
	for _, h := range hits {
		if h.Memory.CustomerKey != customerX || strings.HasPrefix(h.Memory.CustomerKey, "goclaw:") || h.Memory.TenantID != "minh-an" {
			t.Fatalf("customer X saw %+v", h.Memory)
		}
		if strings.Contains(h.Memory.Body, "khach Y") || strings.Contains(h.Memory.Body, "tenant B") || strings.Contains(h.Memory.Body, markerMQ) {
			t.Fatalf("customer X saw body %q", h.Memory.Body)
		}
	}
	hits = mustRecall(t, st, store.RecallQuery{
		TenantID: "minh-an", AgentID: agent1, ExcludeSession: "ask-minh-an",
		Query: "Bien ban", Limit: 30, CustomerScoped: true, CustomerKey: goclawX,
	})
	seen := map[string]bool{}
	for _, h := range hits {
		seen[h.Memory.ID] = true
		if h.Memory.CustomerKey != goclawX || h.Memory.TenantID != "minh-an" {
			t.Fatalf("goclaw X saw %+v", h.Memory)
		}
	}
	if !seen[MemoryID("10000000-0000-4000-8000-000000000001")] || !seen[MemoryID("10000000-0000-4000-8000-000000000002")] {
		t.Fatalf("missing own notes %v", seen)
	}
	for _, id := range []string{
		"10000000-0000-4000-8000-000000000003",
		"10000000-0000-4000-8000-000000000004",
		"10000000-0000-4000-8000-00000000000b",
		"10000000-0000-4000-8000-00000000000c",
	} {
		if seen[MemoryID(id)] {
			t.Fatalf("customer X saw %s", id)
		}
	}
	bMem, err := st.GetMemory(MemoryID("10000000-0000-4000-8000-00000000000c"))
	if err != nil {
		t.Fatal(err)
	}
	bSess, err := st.GetSession(bMem.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	bHits := mustRecall(t, st, store.RecallQuery{
		TenantID: "tenant-b", AgentID: bSess.AgentID, ExcludeSession: "ask-b",
		Query: "Bien ban", Limit: 20, CustomerScoped: true, CustomerKey: "goclaw:user-tenant-b-01",
	})
	if len(bHits) == 0 {
		t.Fatal("tenant b recall returned nothing")
	}
	for _, h := range bHits {
		if h.Memory.TenantID != "tenant-b" || h.Memory.CustomerKey != "goclaw:user-tenant-b-01" {
			t.Fatalf("tenant b saw %+v", h.Memory)
		}
	}
}

func assertLiveWrite(t *testing.T, st store.StoreIface, agent1 string, db DB) *store.Memory {
	t.Helper()
	before := map[string]store.Memory{}
	for _, id := range []string{
		"10000000-0000-4000-8000-000000000001",
		"10000000-0000-4000-8000-000000000002",
		"10000000-0000-4000-8000-000000000003",
		"10000000-0000-4000-8000-00000000000c",
	} {
		m := mustMemory(t, db, id)
		before[m.ID] = *m
	}
	liveSess := mustSessionCreate(t, st, agent1, "minh-an", "web-minh-an")
	if _, err := st.SaveSummary(liveSess, "live summary one"); err != nil {
		t.Fatal(err)
	}
	live, err := st.SaveSummary(liveSess, liveBody)
	if err != nil {
		t.Fatal(err)
	}
	for id, prev := range before {
		got := mustMemory(t, db, id)
		if got.Body != prev.Body || !got.CreatedAt.Equal(prev.CreatedAt) || got.CustomerKey != prev.CustomerKey || got.SessionID != prev.SessionID {
			t.Fatalf("live write changed imported %s", id)
		}
	}
	if live.SessionID == before[MemoryID("10000000-0000-4000-8000-000000000001")].SessionID {
		t.Fatal("live session reused an import session")
	}
	return live
}

func mustRecall(t *testing.T, st store.StoreIface, q store.RecallQuery) []store.RecallHit {
	t.Helper()
	hits, err := st.RecallMemories(q)
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

func mustSessionCreate(t *testing.T, st store.StoreIface, agent, tenant, label string) string {
	t.Helper()
	sess, err := st.CreateSession(store.Session{AgentID: agent, TenantID: tenant, Label: label})
	if err != nil {
		t.Fatal(err)
	}
	return sess.ID
}

func mustAgent(t *testing.T, st store.StoreIface, tenant, key string) string {
	t.Helper()
	ag, err := st.CreateAgent(store.Agent{TenantID: tenant, AgentKey: key, DisplayName: key})
	if err != nil {
		t.Fatal(err)
	}
	return ag.ID
}

func mustAgents(t *testing.T, agent1, agent2, agentB string) AgentMap {
	t.Helper()
	raw := strings.Join([]string{
		`{"source_agent":"` + srcAgent1 + `","dest_agent":"` + agent1 + `","dest_tenant":"minh-an"}`,
		`{"source_agent":"` + srcAgent2 + `","dest_agent":"` + agent2 + `","dest_tenant":"minh-an"}`,
		`{"source_agent":"` + srcAgent3 + `","dest_agent":"missing-minh-an-agent","dest_tenant":"minh-an"}`,
		`{"source_agent":"` + srcAgentB + `","dest_agent":"` + agentB + `","dest_tenant":"tenant-b"}`,
	}, "\n") + "\n"
	m, err := LoadAgents(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func section(t *testing.T, rep *Report, source string) TenantReport {
	t.Helper()
	for _, s := range rep.Tenants {
		if s.SourceTenant == source {
			return s
		}
	}
	t.Fatalf("missing tenant %s in %+v", source, rep.Tenants)
	return TenantReport{}
}

func mustMemory(t *testing.T, db DB, sourceOrID string) *store.Memory {
	t.Helper()
	id := sourceOrID
	if !strings.HasPrefix(id, "gc-") {
		id = MemoryID(id)
	}
	m, err := db.GetMemory(id)
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	return m
}

func mustSession(t *testing.T, db DB, id string) *store.Session {
	t.Helper()
	s, err := db.GetSession(id)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func readFixture(t *testing.T) []Parsed {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "minh-an-episodic.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := ReadExport(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 19 {
		t.Fatalf("fixture lines %d", len(rows))
	}
	return rows
}

func readTenants(t *testing.T) TenantMap {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "tenants.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	m, err := LoadTenants(f)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func readAgents(t *testing.T) AgentMap {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "agents.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	m, err := LoadAgents(f)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

type gcDeleter interface {
	DB
	DeleteGCPrefix(string) (int, int, error)
}

func asDB(t *testing.T, st store.StoreIface) gcDeleter {
	t.Helper()
	switch s := st.(type) {
	case *store.SQLiteStore:
		return s
	case *store.PostgresStore:
		return s
	default:
		t.Fatalf("store %T", st)
		return nil
	}
}

func backends(t *testing.T) map[string]func(t *testing.T) store.StoreIface {
	t.Helper()
	out := map[string]func(t *testing.T) store.StoreIface{
		"sqlite": func(t *testing.T) store.StoreIface {
			s, err := store.OpenSQLite(filepath.Join(t.TempDir(), "episodic.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			return s
		},
	}
	if pg := openPostgres(t); pg != nil {
		out["postgres"] = func(t *testing.T) store.StoreIface { return pg }
	}
	return out
}

func openPostgres(t *testing.T) store.StoreIface {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("GOSO_TEST_POSTGRES_DSN"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("GOSO_TEST_DATABASE_URL"))
	}
	if dsn == "" {
		if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
			t.Fatal("GOSO_TEST_POSTGRES_DSN is required in CI")
		}
		return nil
	}
	schema := "episodic179"
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`) })
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
	return pg
}

func insertPlain(t *testing.T, st store.StoreIface, id, sessionID, body, tenant, customer, created string) {
	t.Helper()
	db, pg := sqlOf(t, st)
	q := `INSERT INTO memories(id, session_id, kind, body, created_at, tenant_id, customer_key) VALUES(?,?,?,?,?,?,?)`
	if pg {
		q = rewriteQ(q)
	}
	if _, err := db.Exec(q, id, sessionID, store.KindEpisodic, body, created, tenant, customer); err != nil {
		t.Fatal(err)
	}
}

func sqlOf(t *testing.T, st store.StoreIface) (*sql.DB, bool) {
	t.Helper()
	switch s := st.(type) {
	case *store.SQLiteStore:
		db, pg := s.SQL()
		return db, pg
	case *store.PostgresStore:
		db, pg := s.SQLiteStore.SQL()
		return db, pg
	default:
		t.Fatalf("store %T", st)
		return nil, false
	}
}

func rewriteQ(q string) string {
	n := 0
	var b strings.Builder
	for i := 0; i < len(q); i++ {
		if q[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(q[i])
	}
	return b.String()
}
