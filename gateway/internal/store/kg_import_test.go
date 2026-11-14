// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestImportSQLLeavesSchemaColumnsAlone(t *testing.T) {
	for _, q := range []string{insertImportedKGEntitySQL, insertImportedKGRelationSQL} {
		if strings.Contains(q, "embedding") || strings.Contains(q, "customer_key") || strings.Contains(q, "tsv") {
			t.Fatalf("import SQL touches a column this spec does not write: %s", q)
		}
		if !strings.Contains(q, "ON CONFLICT (id) DO NOTHING") {
			t.Fatalf("missing conflict clause: %s", q)
		}
	}
}

func TestInsertImportedKGDoesNotOverwrite(t *testing.T) {
	s := openImportSQLite(t)
	until := time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)
	first := KGEntity{
		ID: "gc-kg:bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb1", TenantID: "minh-an",
		Name: "LIVEBODY", Kind: "place", Body: "LIVEBODY",
		AgentID: "agent-minh", Source: "posted",
		ValidFrom: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		CreatedAt: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	}
	ok, err := s.InsertImportedKGEntity(first)
	if err != nil || !ok {
		t.Fatalf("insert %v %v", ok, err)
	}
	second := first
	second.Name = "replaced"
	second.Body = "replaced"
	second.Source = "goclaw:bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb1"
	second.ValidUntil = &until
	ok, err = s.InsertImportedKGEntity(second)
	if err != nil || ok {
		t.Fatalf("second insert %v %v", ok, err)
	}
	got, err := s.GetKGEntity(first.ID, KGScope{})
	if err != nil || got.Name != "LIVEBODY" || got.Body != "LIVEBODY" || got.ValidUntil != nil {
		t.Fatalf("row changed: %+v %v", got, err)
	}
	prov, err := s.KGProvenance("kg_entities", first.ID)
	if err != nil || prov != "posted" {
		t.Fatalf("provenance %q %v", prov, err)
	}

	rel := KGRelation{
		ID: "gc-kg:cccccccc-cccc-4ccc-8ccc-ccccccccccc1", TenantID: "minh-an",
		FromID: first.ID, ToID: first.ID, Rel: "cung_cap", Body: "keep",
		Source:    "goclaw:cccccccc-cccc-4ccc-8ccc-ccccccccccc1",
		ValidFrom: first.ValidFrom,
	}
	if ok, err = s.InsertImportedKGRelation(rel); err != nil || !ok {
		t.Fatalf("relation %v %v", ok, err)
	}
	rel.Body = "replaced"
	if ok, err = s.InsertImportedKGRelation(rel); err != nil || ok {
		t.Fatalf("relation overwrite %v %v", ok, err)
	}
	prov, err = s.KGProvenance("kg_relations", rel.ID)
	if err != nil || prov != "goclaw:cccccccc-cccc-4ccc-8ccc-ccccccccccc1" {
		t.Fatalf("relation provenance %q %v", prov, err)
	}
}

func TestDeleteImportedKGLeavesLiveRows(t *testing.T) {
	s := openImportSQLite(t)
	a, err := s.CreateAgent(Agent{TenantID: "minh-an", AgentKey: "co-van-minh-an", DisplayName: "Cố vấn Minh An"})
	if err != nil {
		t.Fatal(err)
	}
	live, err := s.PutKGEntity(KGEntity{TenantID: "minh-an", Name: "Kho song", Kind: "place", AgentID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	otherLive, err := s.PutKGEntity(KGEntity{TenantID: "minh-an", Name: "Dai ly song", Kind: "org", AgentID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	liveRel, err := s.PutKGRelation(KGRelation{TenantID: "minh-an", FromID: live.ID, ToID: otherLive.ID, Rel: "song"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := s.CreateSession(Session{AgentID: a.ID, TenantID: "minh-an", Label: "song"})
	if err != nil {
		t.Fatal(err)
	}
	mem, err := s.PutMemory(Memory{SessionID: sess.ID, TenantID: "minh-an", Body: "MEMLIVE", Kind: KindDurable})
	if err != nil {
		t.Fatal(err)
	}
	imported := KGEntity{
		ID: "gc-kg:bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb1", TenantID: "minh-an",
		Name: "Kho Cần Thơ", Kind: "place", Body: "KEEPMARKER",
		Source:    "goclaw:bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb1",
		ValidFrom: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
		CreatedAt: time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
	}
	if _, err := s.InsertImportedKGEntity(imported); err != nil {
		t.Fatal(err)
	}
	otherTenant := imported
	otherTenant.ID = "gc-kg:bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbb2"
	otherTenant.TenantID = "phap-che"
	otherTenant.Name = "Điều khoản mẫu"
	if _, err := s.InsertImportedKGEntity(otherTenant); err != nil {
		t.Fatal(err)
	}
	db, pg := s.SQL()
	if pg {
		t.Fatal("sqlite helper opened postgres")
	}
	if _, err := db.Exec(`INSERT INTO kg_entities(id, tenant_id, name, kind, body, valid_from, created_at, agent_id, provenance) VALUES('GC-kept','minh-an','GC kept','org','GCKEPT','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z','','posted')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO memories(id, session_id, kind, body, created_at, tenant_id, customer_key) VALUES('gc-sess:minh-an',?,'durable','MEMLIVEPREFIX','2026-01-01T00:00:00Z','minh-an','')`, sess.ID); err != nil {
		t.Fatal(err)
	}
	var customerKeyCols int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('kg_entities') WHERE name='customer_key'`).Scan(&customerKeyCols); err != nil {
		t.Fatal(err)
	}
	if customerKeyCols != 1 {
		t.Fatalf("customer_key columns %d", customerKeyCols)
	}
	var importedKey string
	if err := db.QueryRow(`SELECT customer_key FROM kg_entities WHERE id=?`, imported.ID).Scan(&importedKey); err != nil || importedKey != "" {
		t.Fatalf("imported customer_key %q %v", importedKey, err)
	}

	ent, rel, err := s.DeleteImportedKG("minh-an")
	if err != nil {
		t.Fatal(err)
	}
	if ent != 1 || rel != 0 {
		t.Fatalf("deleted entities=%d relations=%d", ent, rel)
	}
	if _, err := s.GetKGEntity(imported.ID, KGScope{}); err != ErrNotFound {
		t.Fatal("imported row still present")
	}
	if _, err := s.GetKGEntity(otherTenant.ID, KGScope{}); err != nil {
		t.Fatal("other tenant gc row was deleted")
	}
	if _, err := s.GetKGEntity(live.ID, KGScope{}); err != nil {
		t.Fatal("live entity was deleted")
	}
	if _, err := s.GetKGEntity("GC-kept", KGScope{}); err != nil {
		t.Fatal("case-different id was deleted")
	}
	gotRel, err := s.ListKGRelations(live.ID, KGScope{})
	if err != nil || len(gotRel) != 1 || gotRel[0].ID != liveRel.ID || gotRel[0].ValidUntil != nil {
		t.Fatalf("live relation %+v %v", gotRel, err)
	}
	gotMem, err := s.GetMemory(mem.ID)
	if err != nil || gotMem.Body != "MEMLIVE" {
		t.Fatal("live memory changed")
	}
	var prefixBody string
	if err := db.QueryRow(`SELECT body FROM memories WHERE id='gc-sess:minh-an'`).Scan(&prefixBody); err != nil || prefixBody != "MEMLIVEPREFIX" {
		t.Fatalf("gc- memory %q %v", prefixBody, err)
	}
}

func openImportSQLite(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := OpenSQLite(filepath.Join(t.TempDir(), "kg.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
