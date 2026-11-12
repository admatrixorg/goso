// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package kgimport

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mqglobal/goso/gateway/internal/store"
)

const (
	tenantMinh = "11111111-1111-4111-8111-111111111111"
	tenantPhap = "22222222-2222-4222-8222-222222222222"
	tenantMQ   = "33333333-3333-4333-8333-333333333333"
	tenantNope = "44444444-4444-4444-8444-444444444444"

	agentMinhGC  = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1"
	agentPhapGC  = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2"
	agentNopeGC  = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa3"
	agentCrossGC = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa4"

	idPlace    = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb1"
	idOrg      = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb2"
	idPrivate  = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb3"
	idTeam     = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb4"
	idScope    = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb5"
	idNoAgent  = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb6"
	idPhap     = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb7"
	idPhap2    = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb8"
	idMQ       = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb9"
	idUnknown  = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbba"
	idEmbed    = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbc"
	idCross    = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbd"
	idWrongMap = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbe"

	relKeep     = "cccccccc-cccc-4ccc-8ccc-ccccccccccc1"
	relEndpoint = "cccccccc-cccc-4ccc-8ccc-ccccccccccc2"
	relPrivate  = "cccccccc-cccc-4ccc-8ccc-ccccccccccc3"
	relMissing  = "cccccccc-cccc-4ccc-8ccc-ccccccccccc4"
	relTeamEnd  = "cccccccc-cccc-4ccc-8ccc-ccccccccccc5"
	relTeamRow  = "cccccccc-cccc-4ccc-8ccc-ccccccccccc6"
	relPhap     = "cccccccc-cccc-4ccc-8ccc-ccccccccccc7"
	relMQ       = "cccccccc-cccc-4ccc-8ccc-ccccccccccc8"
	missingUUID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
)

func TestAdmitKeepsCustomerAndTeamOut(t *testing.T) {
	if admit("", "", "") != "" {
		t.Fatal("empty user_id should be admitted")
	}
	if admit("khach-a", "", "") != reasonCustomerPrivate {
		t.Fatal("user_id")
	}
	if admit("", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb0", "") != reasonTeamScope {
		t.Fatal("team_id")
	}
	if admit("", "", "rieng") != reasonTeamScope {
		t.Fatal("custom_scope")
	}
	if admit("khach-a", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb0", "rieng") != reasonTeamScope {
		t.Fatal("team/scope wins over user_id")
	}
}

func TestCompactPropertiesOrdersKeys(t *testing.T) {
	got, err := compactProperties(json.RawMessage(`{"b":1,"a":"mien tay"}`))
	if err != nil || got != `{"a":"mien tay","b":1}` {
		t.Fatalf("%q %v", got, err)
	}
	got, err = compactProperties(json.RawMessage(`{}`))
	if err != nil || got != "" {
		t.Fatalf("empty object %q %v", got, err)
	}
}

func TestMinhAnFixture(t *testing.T) {
	for _, name := range []string{"memory", "sqlite", "postgres"} {
		name := name
		t.Run(name, func(t *testing.T) {
			st := openBackend(t, name)
			runMinhAn(t, st)
		})
	}
}

func runMinhAn(t *testing.T, st testStore) {
	t.Helper()
	minh, phap := seedAgents(t, st)
	ex := minhAnExport(t)
	opt := options(t, minh.ID, phap.ID)

	dry, err := Import(st, ex, opt)
	if err != nil {
		t.Fatal(err)
	}
	assertFirstCounts(t, dry)
	if hit, _ := st.SearchProgressive("KEEPMARKER", "minh-an"); len(hit) != 0 {
		t.Fatal("dry-run wrote a row")
	}

	opt.DryRun = false
	rep, err := Import(st, ex, opt)
	if err != nil {
		t.Fatal(err)
	}
	assertFirstCounts(t, rep)
	assertStored(t, st, minh.ID)
	assertIsolation(t, st)
	assertNoLeak(t, st)
	assertNoCustomerKey(t, st)

	again, err := Import(st, ex, opt)
	if err != nil {
		t.Fatal(err)
	}
	assertSecondCounts(t, again)
	assertStored(t, st, minh.ID)

	live, other, liveRel := seedLive(t, st, minh.ID)
	seedPrefixRows(t, st, minh)
	dryRemove, err := Remove(st, "minh-an", true)
	if err != nil {
		t.Fatal(err)
	}
	if dryRemove.Entities != 3 || dryRemove.Relations != 1 || !dryRemove.DryRun {
		t.Fatalf("dry remove %+v", dryRemove)
	}
	if _, err := st.GetKGEntity("gc-kg:" + idPlace); err != nil {
		t.Fatal("dry remove deleted a row")
	}
	removed, err := Remove(st, "minh-an", false)
	if err != nil {
		t.Fatal(err)
	}
	if removed.Entities != 3 || removed.Relations != 1 {
		t.Fatalf("remove %+v", removed)
	}
	if _, err := st.GetKGEntity("gc-kg:" + idPlace); err != store.ErrNotFound {
		t.Fatal("imported entity remains")
	}
	if _, err := st.GetKGEntity("gc-kg:" + idPhap); err != nil {
		t.Fatal("other tenant imported row was deleted")
	}
	if _, err := st.GetKGEntity(live.ID); err != nil {
		t.Fatal("live entity was deleted")
	}
	if _, err := st.GetKGEntity(other.ID); err != nil {
		t.Fatal("live peer was deleted")
	}
	rels, err := st.ListKGRelations(live.ID)
	if err != nil || len(rels) != 1 || rels[0].ID != liveRel.ID || rels[0].ValidUntil != nil {
		t.Fatalf("live relation %+v %v", rels, err)
	}
	assertPrefixRows(t, st)
	if _, err := Remove(st, "mq-capital", false); err == nil {
		t.Fatal("mq-capital removal should be refused")
	}
}

func TestConflictDoesNotOverwrite(t *testing.T) {
	for _, name := range []string{"memory", "sqlite", "postgres"} {
		name := name
		t.Run(name, func(t *testing.T) {
			st := openBackend(t, name)
			minh, phap := seedAgents(t, st)
			ok, err := st.InsertImportedKGEntity(store.KGEntity{
				ID: "gc-kg:" + idPlace, TenantID: "minh-an", Name: "LIVEBODY", Kind: "place",
				Body: "LIVEBODY", Source: "posted", AgentID: minh.ID,
				ValidFrom: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
				CreatedAt: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
			})
			if err != nil || !ok {
				t.Fatal(err)
			}
			opt := options(t, minh.ID, phap.ID)
			opt.DryRun = false
			rep, err := Import(st, minhAnExport(t), opt)
			if err != nil {
				t.Fatal(err)
			}
			sec := mustSection(t, rep, tenantMinh, "kg_entities")
			if sec.WillWrite != 2 || sec.Skipped[reasonExists] != 1 {
				t.Fatalf("conflict counts %+v", sec)
			}
			got, err := st.GetKGEntity("gc-kg:" + idPlace)
			if err != nil || got.Name != "LIVEBODY" || got.Body != "LIVEBODY" {
				t.Fatalf("seed overwritten: %+v %v", got, err)
			}
			prov, err := st.KGProvenance("kg_entities", "gc-kg:"+idPlace)
			if err != nil || prov != "posted" {
				t.Fatalf("provenance %q %v", prov, err)
			}
		})
	}
}

func assertFirstCounts(t *testing.T, rep *Report) {
	t.Helper()
	want := []struct {
		source, table     string
		read, write       int
		priv, team, unmap int
		mq, missing, bad  int
		exists            int
	}{
		{tenantMinh, "kg_entities", 10, 3, 1, 2, 3, 0, 0, 1, 0},
		{tenantMinh, "kg_relations", 6, 1, 1, 1, 0, 0, 3, 0, 0},
		{tenantPhap, "kg_entities", 2, 2, 0, 0, 0, 0, 0, 0, 0},
		{tenantPhap, "kg_relations", 1, 1, 0, 0, 0, 0, 0, 0, 0},
		{tenantMQ, "kg_entities", 1, 0, 0, 0, 0, 1, 0, 0, 0},
		{tenantMQ, "kg_relations", 1, 0, 0, 0, 0, 1, 0, 0, 0},
		{tenantNope, "kg_entities", 1, 0, 0, 0, 1, 0, 0, 0, 0},
	}
	if len(rep.Tables) != len(want) {
		t.Fatalf("sections %d %+v", len(rep.Tables), rep.Tables)
	}
	for _, w := range want {
		s := mustSection(t, rep, w.source, w.table)
		if s.Read != w.read || s.WillWrite != w.write {
			t.Fatalf("%s %s read/write %d/%d", w.table, w.source, s.Read, s.WillWrite)
		}
		if s.Skipped[reasonCustomerPrivate] != w.priv || s.Skipped[reasonTeamScope] != w.team || s.Skipped[reasonUnmapped] != w.unmap || s.Skipped[reasonMQ] != w.mq || s.Skipped[reasonMissingEndpoint] != w.missing || s.Skipped[reasonInvalid] != w.bad || s.Skipped[reasonExists] != w.exists || s.Skipped[reasonExpired] != 0 {
			t.Fatalf("%s %s skipped %+v", w.table, w.source, s.Skipped)
		}
		if w.mq > 0 && s.DestTenant != "" {
			t.Fatal("mq section names a dest")
		}
	}
}

func assertSecondCounts(t *testing.T, rep *Report) {
	t.Helper()
	for _, pair := range []struct{ source, table string }{
		{tenantMinh, "kg_entities"},
		{tenantMinh, "kg_relations"},
		{tenantPhap, "kg_entities"},
		{tenantPhap, "kg_relations"},
	} {
		s := mustSection(t, rep, pair.source, pair.table)
		if s.WillWrite != 0 || s.Skipped[reasonExists] == 0 {
			t.Fatalf("second run %+v", s)
		}
	}
	minh := mustSection(t, rep, tenantMinh, "kg_entities")
	if minh.Skipped[reasonExists] != 3 || minh.Skipped[reasonCustomerPrivate] != 1 {
		t.Fatalf("second entities %+v", minh.Skipped)
	}
	rel := mustSection(t, rep, tenantMinh, "kg_relations")
	if rel.Skipped[reasonExists] != 1 || rel.Skipped[reasonMissingEndpoint] != 3 {
		t.Fatalf("second relations %+v", rel.Skipped)
	}
}

func assertStored(t *testing.T, st testStore, agentID string) {
	t.Helper()
	place, err := st.GetKGEntity("gc-kg:" + idPlace)
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	if place.Name != "Kho Cần Thơ" || place.Kind != "place" || place.TenantID != "minh-an" || place.AgentID != agentID {
		t.Fatalf("place %+v", place)
	}
	if place.Body != "KEEPMARKER\n{\"khu\":\"mien tay\"}" {
		t.Fatalf("body %q", place.Body)
	}
	if !place.ValidFrom.Equal(from) || !place.CreatedAt.Equal(from) || place.ValidUntil != nil {
		t.Fatalf("place times %+v", place)
	}
	prov, err := st.KGProvenance("kg_entities", place.ID)
	if err != nil || prov != "goclaw:"+idPlace {
		t.Fatalf("provenance %q %v", prov, err)
	}
	org, err := st.GetKGEntity("gc-kg:" + idOrg)
	if err != nil {
		t.Fatal(err)
	}
	until := time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	validFrom := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	if !org.ValidFrom.Equal(validFrom) || !org.CreatedAt.Equal(created) || org.ValidUntil == nil || !org.ValidUntil.Equal(until) {
		t.Fatalf("org times %+v", org)
	}
	if org.Body != "" || org.Kind != "org" {
		t.Fatalf("org %+v", org)
	}
	embed, err := st.GetKGEntity("gc-kg:" + idEmbed)
	if err != nil || embed.Body != "Bang gia mau\n{\"sku\":\"MA-1\"}" || strings.Contains(embed.Body, "EMBEDMARKER") {
		t.Fatalf("embed %+v %v", embed, err)
	}
	rels, err := st.ListKGRelations(place.ID)
	if err != nil {
		t.Fatal(err)
	}
	var edge *store.KGRelation
	for _, r := range rels {
		if r.ID == "gc-kg:"+relKeep {
			edge = r
		}
	}
	if edge == nil || edge.Rel != "cung_cap" || edge.FromID != place.ID || edge.ToID != "gc-kg:"+idOrg || edge.Body != `{"note":"mau"}` || edge.TenantID != "minh-an" {
		t.Fatalf("edge %+v", edge)
	}
	if !edge.ValidFrom.Equal(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("edge time %s", edge.ValidFrom)
	}
	rprov, err := st.KGProvenance("kg_relations", edge.ID)
	if err != nil || rprov != "goclaw:"+relKeep {
		t.Fatalf("rel provenance %q %v", rprov, err)
	}
	for _, id := range []string{idPrivate, idTeam, idScope, idNoAgent, idMQ, idUnknown, idCross, idWrongMap} {
		if _, err := st.GetKGEntity("gc-kg:" + id); err != store.ErrNotFound {
			t.Fatalf("skipped entity stored %s", id)
		}
	}
}

func assertIsolation(t *testing.T, st testStore) {
	t.Helper()
	minh, err := st.ListKGGraph(store.KGGraphQuery{Tenant: "minh-an", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	phap, err := st.ListKGGraph(store.KGGraphQuery{Tenant: "phap-che", Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if minh.TotalNodes != 3 || phap.TotalNodes != 2 {
		t.Fatalf("nodes minh %d phap %d", minh.TotalNodes, phap.TotalNodes)
	}
	for _, n := range minh.Nodes {
		if strings.Contains(n.ID, idPhap) || strings.Contains(n.Name, "Điều khoản") {
			t.Fatal("minh graph has phap")
		}
	}
	for _, n := range phap.Nodes {
		if strings.Contains(n.ID, idPlace) || strings.Contains(n.Name, "Kho") {
			t.Fatal("phap graph has minh")
		}
	}
	if hit, _ := st.SearchProgressive("Điều khoản", "minh-an"); len(hit) != 0 {
		t.Fatal("minh search returned phap")
	}
	if hit, _ := st.SearchProgressive("KEEPMARKER", "phap-che"); len(hit) != 0 {
		t.Fatal("phap search returned minh")
	}
	if hit, _ := st.SearchProgressive("KEEPMARKER", "minh-an"); len(hit) != 1 {
		t.Fatalf("minh search %d", len(hit))
	}
}

func assertNoLeak(t *testing.T, st testStore) {
	t.Helper()
	for _, marker := range []string{"PRIVMARKER", "TEAMMARKER", "SCOPEMARKER", "MQMARKER", "EMBEDMARKER", "TSVMARKER", "UNMAPPEDMARKER", "BADMARKER"} {
		for _, tenant := range []string{"minh-an", "phap-che", "default"} {
			hit, err := st.SearchProgressive(marker, tenant)
			if err != nil || len(hit) != 0 {
				t.Fatalf("leaked %s into %s (%d, %v)", marker, tenant, len(hit), err)
			}
		}
	}
}

func assertNoCustomerKey(t *testing.T, st testStore) {
	t.Helper()
	db, ok := st.(interface{ SQL() (*sql.DB, bool) })
	if !ok {
		return
	}
	raw, pg := db.SQL()
	var n int
	var err error
	if pg {
		err = raw.QueryRow(`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name IN ('kg_entities','kg_relations') AND column_name='customer_key'`).Scan(&n)
	} else {
		err = raw.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('kg_entities') WHERE name='customer_key'`).Scan(&n)
		if err == nil && n == 0 {
			var rel int
			err = raw.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('kg_relations') WHERE name='customer_key'`).Scan(&rel)
			n += rel
		}
	}
	if err != nil || n != 0 {
		t.Fatalf("customer_key columns %d %v", n, err)
	}
}

type testStore interface {
	Target
	CreateAgent(store.Agent) (*store.Agent, error)
	CreateSession(store.Session) (*store.Session, error)
	PutMemory(store.Memory) (*store.Memory, error)
	GetMemory(string) (*store.Memory, error)
	PutKGEntity(store.KGEntity) (*store.KGEntity, error)
	PutKGRelation(store.KGRelation) (*store.KGRelation, error)
	GetKGEntity(string) (*store.KGEntity, error)
	ListKGRelations(string) ([]*store.KGRelation, error)
	ListKGGraph(store.KGGraphQuery) (*store.KGGraph, error)
	SearchProgressive(string, string) ([]store.KGSearchHit, error)
	KGProvenance(string, string) (string, error)
}

func seedAgents(t *testing.T, st testStore) (*store.Agent, *store.Agent) {
	t.Helper()
	minh, err := st.CreateAgent(store.Agent{TenantID: "minh-an", AgentKey: "co-van-minh-an", DisplayName: "Cố vấn Minh An"})
	if err != nil {
		t.Fatal(err)
	}
	phap, err := st.CreateAgent(store.Agent{TenantID: "phap-che", AgentKey: "co-van-phap-che", DisplayName: "Cố vấn pháp chế"})
	if err != nil {
		t.Fatal(err)
	}
	return minh, phap
}

func seedLive(t *testing.T, st testStore, agentID string) (*store.KGEntity, *store.KGEntity, *store.KGRelation) {
	t.Helper()
	live, err := st.PutKGEntity(store.KGEntity{TenantID: "minh-an", Name: "Kho song", Kind: "place", AgentID: agentID})
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.PutKGEntity(store.KGEntity{TenantID: "minh-an", Name: "Dai ly song", Kind: "org", AgentID: agentID})
	if err != nil {
		t.Fatal(err)
	}
	rel, err := st.PutKGRelation(store.KGRelation{TenantID: "minh-an", FromID: live.ID, ToID: other.ID, Rel: "song"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: agentID, TenantID: "minh-an", Label: "song"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.PutMemory(store.Memory{SessionID: sess.ID, TenantID: "minh-an", Body: "MEMLIVE", Kind: store.KindDurable}); err != nil {
		t.Fatal(err)
	}
	return live, other, rel
}

func seedPrefixRows(t *testing.T, st testStore, minh *store.Agent) {
	t.Helper()
	db, ok := st.(interface{ SQL() (*sql.DB, bool) })
	if !ok {
		return
	}
	raw, pg := db.SQL()
	exec := func(q string, args ...any) {
		t.Helper()
		if pg {
			q = toPg(q)
		}
		if _, err := raw.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	sess, err := st.CreateSession(store.Session{AgentID: minh.ID, TenantID: "minh-an", Label: "prefix"})
	if err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO kg_entities(id, tenant_id, name, kind, body, valid_from, created_at, agent_id, provenance) VALUES('GC-kept','minh-an','GC kept','org','GCKEPT','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z','','posted')`)
	exec(`INSERT INTO memories(id, session_id, kind, body, created_at, tenant_id, customer_key) VALUES('gc-sess:minh-an',?,'durable','MEMLIVEPREFIX','2026-01-01T00:00:00Z','minh-an','')`, sess.ID)
}

func assertPrefixRows(t *testing.T, st testStore) {
	t.Helper()
	assertLiveMemory(t, st)
	db, ok := st.(interface{ SQL() (*sql.DB, bool) })
	if !ok {
		return
	}
	raw, pg := db.SQL()
	qEnt := `SELECT body FROM kg_entities WHERE id='GC-kept'`
	qMem := `SELECT body FROM memories WHERE id='gc-sess:minh-an'`
	if pg {
		// queries have no placeholders
	}
	var body string
	if err := raw.QueryRow(qEnt).Scan(&body); err != nil || body != "GCKEPT" {
		t.Fatalf("GC-kept %q %v", body, err)
	}
	if err := raw.QueryRow(qMem).Scan(&body); err != nil || body != "MEMLIVEPREFIX" {
		t.Fatalf("memory prefix %q %v", body, err)
	}
	assertLiveMemory(t, st)
}

func assertLiveMemory(t *testing.T, st testStore) {
	t.Helper()
	mems, err := st.SearchProgressive("MEMLIVE", "minh-an")
	if err != nil || len(mems) == 0 {
		t.Fatal("live memory missing")
	}
}

func toPg(q string) string {
	n := 0
	var b strings.Builder
	for i := 0; i < len(q); i++ {
		if q[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(itoa(n))
			continue
		}
		b.WriteByte(q[i])
	}
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d [4]byte
	i := len(d)
	for n > 0 {
		i--
		d[i] = byte('0' + n%10)
		n /= 10
	}
	return string(d[i:])
}

func options(t *testing.T, minhID, phapID string) Options {
	t.Helper()
	tenants, err := ReadTenantMap(strings.NewReader(`{"tenants":[
		{"source_tenant":"` + tenantMinh + `","dest_tenant":"minh-an","slug":"minh-an"},
		{"source_tenant":"` + tenantPhap + `","dest_tenant":"phap-che","slug":"phap-che"},
		{"source_tenant":"` + tenantMQ + `","dest_tenant":"mq-capital","slug":"mq-capital"}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	agents, err := ReadAgentMap(strings.NewReader(`{"agents":[
		{"goclaw_agent_id":"` + agentMinhGC + `","goso_agent_id":"` + minhID + `","source_tenant":"` + tenantMinh + `"},
		{"goclaw_agent_id":"` + agentPhapGC + `","goso_agent_id":"` + phapID + `","source_tenant":"` + tenantPhap + `"},
		{"goclaw_agent_id":"` + agentCrossGC + `","goso_agent_id":"` + phapID + `","source_tenant":"` + tenantMinh + `"}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	return Options{Tenants: tenants, Agents: agents, DryRun: true}
}

func minhAnExport(t *testing.T) *Export {
	t.Helper()
	lines := []map[string]any{
		rel("kg_relations", relKeep, tenantMinh, agentMinhGC, "", "", idPlace, idOrg, "cung_cap", map[string]any{"note": "mau"}, "2026-03-01T00:00:00Z", nil, nil),
		ent(idPlace, tenantMinh, agentMinhGC, "", "", "", "Kho Cần Thơ", "place", "KEEPMARKER", map[string]any{"khu": "mien tay"}, "2026-01-15T00:00:00Z", nil, nil),
		ent(idOrg, tenantMinh, agentMinhGC, "", "", "", "Đại lý mẫu", "org", "", map[string]any{}, "2026-01-01T00:00:00Z", "2026-02-01T00:00:00Z", "2027-06-01T00:00:00Z"),
		ent(idPrivate, tenantMinh, agentMinhGC, "khach-a", "", "", "Ghi chu khach", "person", "PRIVMARKER", nil, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", nil),
		ent(idTeam, tenantMinh, agentMinhGC, "", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb0", "", "Nhom mau", "team", "TEAMMARKER", nil, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", nil),
		ent(idScope, tenantMinh, agentMinhGC, "", "", "rieng", "Pham vi", "note", "SCOPEMARKER", nil, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", nil),
		ent(idNoAgent, tenantMinh, agentNopeGC, "", "", "", "Khong agent", "fact", "UNMAPPEDMARKER", nil, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", nil),
		{"table": "kg_entities", "id": "not-a-uuid", "tenant_id": tenantMinh, "agent_id": agentMinhGC, "user_id": "", "name": "Sai", "entity_type": "fact", "description": "BADMARKER", "created_at": "2026-01-01T00:00:00Z"},
		ent(idEmbed, tenantMinh, agentMinhGC, "", "", "", "Bang gia mau", "doc", "Bang gia mau", map[string]any{"sku": "MA-1"}, "2026-04-01T00:00:00Z", "2026-04-01T00:00:00Z", nil),
		ent(idCross, tenantMinh, agentCrossGC, "", "", "", "Nham agent", "fact", "UNMAPPEDMARKER", nil, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", nil),
		ent(idWrongMap, tenantMinh, agentPhapGC, "", "", "", "Agent khac tenant", "fact", "UNMAPPEDMARKER", nil, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", nil),
		ent(idPhap, tenantPhap, agentPhapGC, "", "", "", "Điều khoản mẫu", "policy", "PHAPMARKER", nil, "2026-05-01T00:00:00Z", "2026-05-01T00:00:00Z", nil),
		ent(idPhap2, tenantPhap, agentPhapGC, "", "", "", "Phụ lục mẫu", "policy", "", nil, "2026-05-02T00:00:00Z", "2026-05-02T00:00:00Z", nil),
		ent(idMQ, tenantMQ, agentMinhGC, "", "", "", "Khong nhap", "org", "MQMARKER", nil, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", nil),
		ent(idUnknown, tenantNope, agentMinhGC, "", "", "", "Khong map", "org", "UNMAPPEDMARKER", nil, "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z", nil),
		rel("kg_relations", relEndpoint, tenantMinh, agentMinhGC, "", "", idPlace, idPrivate, "thuoc", nil, "2026-03-02T00:00:00Z", "2026-03-02T00:00:00Z", nil),
		rel("kg_relations", relPrivate, tenantMinh, agentMinhGC, "khach-a", "", idPrivate, idOrg, "mua", map[string]any{"x": "PRIVMARKER"}, "2026-03-03T00:00:00Z", "2026-03-03T00:00:00Z", nil),
		rel("kg_relations", relMissing, tenantMinh, agentMinhGC, "", "", idPlace, missingUUID, "thieu", nil, "2026-03-04T00:00:00Z", "2026-03-04T00:00:00Z", nil),
		rel("kg_relations", relTeamEnd, tenantMinh, agentMinhGC, "", "", idTeam, idPlace, "nam_trong", nil, "2026-03-05T00:00:00Z", "2026-03-05T00:00:00Z", nil),
		rel("kg_relations", relTeamRow, tenantMinh, agentMinhGC, "", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb0", idPlace, idOrg, "noi_bo", nil, "2026-03-06T00:00:00Z", "2026-03-06T00:00:00Z", nil),
		rel("kg_relations", relPhap, tenantPhap, agentPhapGC, "", "", idPhap, idPhap2, "dan_chieu", nil, "2026-05-03T00:00:00Z", "2026-05-03T00:00:00Z", nil),
		rel("kg_relations", relMQ, tenantMQ, agentMinhGC, "", "", idMQ, idMQ, "tu", nil, "2026-01-02T00:00:00Z", "2026-01-02T00:00:00Z", nil),
	}
	lines[8]["embedding"] = "EMBEDMARKER"
	lines[8]["tsv"] = "TSVMARKER"
	lines[8]["confidence"] = 0.4
	lines[8]["external_id"] = "ext-ma"
	lines[8]["source_id"] = "src-ma"
	var buf bytes.Buffer
	for _, line := range lines {
		b, err := json.Marshal(line)
		if err != nil {
			t.Fatal(err)
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	ex, err := ReadExport(&buf)
	if err != nil {
		t.Fatal(err)
	}
	return ex
}

func ent(id, tenant, agent, user, team, scope, name, kind, desc string, props map[string]any, created string, from, until any) map[string]any {
	return map[string]any{
		"table": "kg_entities", "id": id, "tenant_id": tenant, "agent_id": agent,
		"user_id": user, "team_id": emptyNil(team), "custom_scope": emptyNil(scope),
		"name": name, "entity_type": kind, "description": desc, "properties": props,
		"created_at": created, "valid_from": from, "valid_until": until,
	}
}

func rel(table, id, tenant, agent, user, team, fromID, toID, kind string, props map[string]any, created string, from, until any) map[string]any {
	return map[string]any{
		"table": table, "id": id, "tenant_id": tenant, "agent_id": agent,
		"user_id": user, "team_id": emptyNil(team), "custom_scope": nil,
		"source_entity_id": fromID, "target_entity_id": toID, "relation_type": kind,
		"properties": props, "created_at": created, "valid_from": from, "valid_until": until,
	}
}

func emptyNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func mustSection(t *testing.T, rep *Report, source, table string) Section {
	t.Helper()
	for _, s := range rep.Tables {
		if s.SourceTenant == source && s.Table == table {
			return s
		}
	}
	t.Fatalf("missing %s %s", table, source)
	return Section{}
}

func openBackend(t *testing.T, name string) testStore {
	t.Helper()
	switch name {
	case "memory":
		return store.New()
	case "sqlite":
		s, err := store.OpenSQLite(filepath.Join(t.TempDir(), "kg.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = s.Close() })
		return s
	case "postgres":
		return openPostgres(t)
	default:
		t.Fatal(name)
		return nil
	}
}

func openPostgres(t *testing.T) testStore {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("GOSO_TEST_POSTGRES_DSN"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("GOSO_TEST_DATABASE_URL"))
	}
	if dsn == "" {
		if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
			t.Fatal("GOSO_TEST_POSTGRES_DSN is required in CI")
		}
		t.Skip("GOSO_TEST_POSTGRES_DSN is unset")
	}
	schema := "kgimport181"
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
	t.Cleanup(func() {
		_, _ = db.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
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
	return pg
}
