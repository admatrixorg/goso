// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestContactsMigrationTwiceSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contacts.db")
	s := openContactSQLite(t, path)
	seeds := minhAnSeeds("minh-an", 1)
	if _, err := s.BulkUpsertContacts(seeds); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openContactSQLite(t, path)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openContactSQLite(t, path)
	defer s.Close()
	n, err := s.CountContacts("minh-an")
	if err != nil || n != 1 {
		t.Fatalf("count after second migrate %d %v", n, err)
	}
	var tables int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='contacts'`).Scan(&tables); err != nil || tables != 1 {
		t.Fatalf("contacts tables %d %v", tables, err)
	}
	var idents int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM contact_identifiers WHERE tenant_id=?`, "minh-an").Scan(&idents); err != nil || idents != 1 {
		t.Fatalf("identifiers %d %v", idents, err)
	}
}

func TestContactsMigrationFaultRollsBackSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fault.db")
	contactsMigrateFault = func(step string) error {
		if step == "contacts" {
			return errors.New("injected")
		}
		return nil
	}
	t.Cleanup(func() { contactsMigrateFault = nil })
	if _, err := OpenSQLite(path); err == nil {
		t.Fatal("faulted migration reported success")
	}
	contactsMigrateFault = nil
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var n int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='contacts'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("rolled-back contacts table is still present")
	}
	s := openContactSQLite(t, path)
	defer s.Close()
	if _, err := s.BulkUpsertContacts(minhAnSeeds("minh-an", 1)); err != nil {
		t.Fatal(err)
	}
	if n, err := s.CountContacts("minh-an"); err != nil || n != 1 {
		t.Fatalf("replay %d %v", n, err)
	}
}

func TestContactsMigrationLeavesMemoryVaultKG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keep.db")
	s := openContactSQLite(t, path)
	stmts := []string{
		`INSERT INTO memories(id, session_id, kind, body, created_at, tenant_id, customer_key) VALUES('m1','sess','episodic','minh-an-note','2026-01-01T00:00:00Z','minh-an','ck')`,
		`INSERT INTO vault_docs(id, title, path, sha256, mtime, body, tenant_id, owner_explicit) VALUES('v1','Minh An','minh-an.md','aa','2026-01-01T00:00:00Z','keep-note','minh-an',1)`,
		`INSERT INTO kg_entities(id, tenant_id, name, kind, body, valid_from, created_at, agent_id, provenance) VALUES('e1','minh-an','Minh An','person','note','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z','','')`,
		`INSERT INTO kg_relations(id, tenant_id, from_id, to_id, rel, body, valid_from, provenance) VALUES('r1','minh-an','e1','e1','same','note','2026-01-01T00:00:00Z','')`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openContactSQLite(t, path)
	defer s.Close()
	var body string
	if err := s.db.QueryRow(`SELECT body FROM memories WHERE id='m1'`).Scan(&body); err != nil || body != "minh-an-note" {
		t.Fatalf("memory %q %v", body, err)
	}
	if err := s.db.QueryRow(`SELECT body FROM vault_docs WHERE id='v1'`).Scan(&body); err != nil || body != "keep-note" {
		t.Fatalf("vault %q %v", body, err)
	}
	if err := s.db.QueryRow(`SELECT body FROM kg_entities WHERE id='e1'`).Scan(&body); err != nil || body != "note" {
		t.Fatalf("kg entity %q %v", body, err)
	}
	if err := s.db.QueryRow(`SELECT body FROM kg_relations WHERE id='r1'`).Scan(&body); err != nil || body != "note" {
		t.Fatalf("kg relation %q %v", body, err)
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM contacts`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("contacts side effect %d %v", n, err)
	}
}

func TestContactsBulkUpsertIdempotentAndUncapped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bulk.db")
	s := openContactSQLite(t, path)
	defer s.Close()
	exerciseContactBulk(t, s)
}

func TestContactsTenantIsolationStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "iso.db")
	s := openContactSQLite(t, path)
	defer s.Close()
	exerciseContactIsolation(t, s)
}

func TestContactsBulkUpsertLogIsCountOnly(t *testing.T) {
	prev := contactLogf
	t.Cleanup(func() { contactLogf = prev })
	var mu sync.Mutex
	var lines []string
	contactLogf = func(format string, args ...any) {
		mu.Lock()
		lines = append(lines, fmt.Sprintf(format, args...))
		mu.Unlock()
	}
	path := filepath.Join(t.TempDir(), "log.db")
	s := openContactSQLite(t, path)
	defer s.Close()
	dest := "ext-minh-an-log-1"
	display := "Minh An"
	if _, err := s.BulkUpsertContacts([]ContactSeed{{
		TenantID: "minh-an", Channel: "telegram", Dest: dest, Display: display, Kind: "user", Count: 2,
	}}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.ListContacts("minh-an", "", "", "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("list %d %v", len(rows), err)
	}
	joined := strings.Join(lines, "\n")
	for _, leak := range []string{dest, display, rows[0].ID, "minh-an", "telegram"} {
		if strings.Contains(joined, leak) {
			t.Fatalf("log contains %q in %q", leak, joined)
		}
	}
	if !strings.Contains(joined, "count=1") {
		t.Fatalf("log %q", joined)
	}
}

func TestContactsMigrationTwicePostgres(t *testing.T) {
	base := postgresTestDSN()
	if base == "" {
		if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
			t.Fatal("GOSO_TEST_POSTGRES_DSN is required in CI")
		}
		t.Skip("GOSO_TEST_POSTGRES_DSN unset")
	}
	dsn := prepareSchema(t, base, "goso_contacts_mig", false)
	s := openContactPostgres(t, dsn)
	if _, err := s.BulkUpsertContacts(minhAnSeeds("minh-an", 3)); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openContactPostgres(t, dsn)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openContactPostgres(t, dsn)
	defer s.Close()
	if n, err := s.CountContacts("minh-an"); err != nil || n != 3 {
		t.Fatalf("count %d %v", n, err)
	}
	var tables int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name='contacts'`).Scan(&tables); err != nil || tables != 1 {
		t.Fatalf("tables %d %v", tables, err)
	}
}

func TestContactsMigrationFaultRollsBackPostgres(t *testing.T) {
	base := postgresTestDSN()
	if base == "" {
		if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
			t.Fatal("GOSO_TEST_POSTGRES_DSN is required in CI")
		}
		t.Skip("GOSO_TEST_POSTGRES_DSN unset")
	}
	dsn := prepareSchema(t, base, "goso_contacts_fault", false)
	contactsMigrateFault = func(step string) error {
		if step == "contacts" {
			return errors.New("injected")
		}
		return nil
	}
	t.Cleanup(func() { contactsMigrateFault = nil })
	if _, err := OpenPostgres(dsn); err == nil {
		t.Fatal("faulted migration reported success")
	}
	contactsMigrateFault = nil
	check, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var n int
	if err := check.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name='contacts'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("rolled-back contacts table is still present")
	}
	s := openContactPostgres(t, dsn)
	defer s.Close()
	if _, err := s.BulkUpsertContacts(minhAnSeeds("minh-an", 1)); err != nil {
		t.Fatal(err)
	}
	if n, err := s.CountContacts("minh-an"); err != nil || n != 1 {
		t.Fatalf("replay %d %v", n, err)
	}
}

func TestContactsBulkAndIsolationPostgres(t *testing.T) {
	base := postgresTestDSN()
	if base == "" {
		if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
			t.Fatal("GOSO_TEST_POSTGRES_DSN is required in CI")
		}
		t.Skip("GOSO_TEST_POSTGRES_DSN unset")
	}
	dsn := prepareSchema(t, base, "goso_contacts_data", false)
	s := openContactPostgres(t, dsn)
	exerciseContactBulk(t, s)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	dsn = prepareSchema(t, base, "goso_contacts_iso", false)
	s = openContactPostgres(t, dsn)
	defer s.Close()
	exerciseContactIsolation(t, s)
}

func exerciseContactBulk(t *testing.T, s *SQLiteStore) {
	t.Helper()
	const n = 205
	seeds := minhAnSeeds("minh-an", n)
	first, err := s.BulkUpsertContacts(seeds)
	if err != nil {
		t.Fatal(err)
	}
	if first.Applied != n {
		t.Fatalf("applied %d", first.Applied)
	}
	got, err := s.CountContacts("minh-an")
	if err != nil || got != n {
		t.Fatalf("count %d %v", got, err)
	}
	rows, err := s.ListContacts("minh-an", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != n {
		t.Fatalf("list %d", len(rows))
	}
	ids := map[string]string{}
	seenDest := map[string]struct{}{}
	for _, row := range rows {
		ids[row.Dest] = row.ID
		seenDest[row.Dest] = struct{}{}
		if row.TenantID != "minh-an" {
			t.Fatalf("tenant %s", row.TenantID)
		}
	}
	if _, ok := seenDest["ext-minh-an-0000"]; !ok {
		t.Fatal("missing first")
	}
	if _, ok := seenDest[fmt.Sprintf("ext-minh-an-%04d", n-1)]; !ok {
		t.Fatal("missing last")
	}
	// Same natural key inside one batch collapses to one row.
	dup := []ContactSeed{seeds[0], seeds[0]}
	dup[1].Display = "Minh An updated"
	dup[1].Count = 9
	if _, err := s.BulkUpsertContacts(dup); err != nil {
		t.Fatal(err)
	}
	if got, err := s.CountContacts("minh-an"); err != nil || got != n {
		t.Fatalf("dup count %d %v", got, err)
	}
	again, err := s.BulkUpsertContacts(seeds)
	if err != nil {
		t.Fatal(err)
	}
	if again.Applied != n {
		t.Fatalf("rerun applied %d", again.Applied)
	}
	if got, err := s.CountContacts("minh-an"); err != nil || got != n {
		t.Fatalf("rerun count %d %v", got, err)
	}
	rows, err = s.ListContacts("minh-an", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != n {
		t.Fatalf("rerun list %d", len(rows))
	}
	for _, row := range rows {
		if ids[row.Dest] != row.ID {
			t.Fatalf("id changed for a rerun")
		}
	}
	// The duplicate update wrote count 9, then the full rerun restored the seed count.
	var restored ContactRow
	for _, row := range rows {
		if row.Dest == seeds[0].Dest {
			restored = row
		}
	}
	if restored.Count != 1 || restored.ID == "" {
		t.Fatalf("restored %#v", restored)
	}
}

func exerciseContactIsolation(t *testing.T, s *SQLiteStore) {
	t.Helper()
	a := ContactSeed{TenantID: "minh-an", Channel: "telegram", Dest: "ext-shared", Display: "Minh An", Kind: "user", Count: 2, FirstSeen: time.Unix(10, 0).UTC(), LastSeen: time.Unix(10, 0).UTC()}
	b := a
	b.TenantID = "other-co"
	b.Display = "Other"
	b.Count = 4
	if _, err := s.BulkUpsertContacts([]ContactSeed{a, b}); err != nil {
		t.Fatal(err)
	}
	// Same channel+dest is allowed across tenants.
	if n, err := s.CountContacts("minh-an"); err != nil || n != 1 {
		t.Fatalf("A count %d %v", n, err)
	}
	if n, err := s.CountContacts("other-co"); err != nil || n != 1 {
		t.Fatalf("B count %d %v", n, err)
	}
	rows, err := s.ListContacts("minh-an", "ext-shared", "", "")
	if err != nil || len(rows) != 1 || rows[0].Display != "Minh An" {
		t.Fatalf("A list %#v %v", rows, err)
	}
	other, err := s.ListContacts("other-co", "", "", "")
	if err != nil || len(other) != 1 {
		t.Fatalf("B list %#v %v", other, err)
	}
	gotRow, getErr := s.GetContact(other[0].ID, "minh-an")
	if !errors.Is(getErr, ErrNotFound) || gotRow.ID != "" {
		t.Fatalf("cross get %#v %v", gotRow, getErr)
	}
	if strings.Contains(getErr.Error(), other[0].Dest) || strings.Contains(getErr.Error(), other[0].Display) || strings.Contains(getErr.Error(), other[0].ID) {
		t.Fatalf("error leaks %v", getErr)
	}
	miss, err := s.GetContact("ct_missing", "minh-an")
	if !errors.Is(err, ErrNotFound) || miss.ID != "" {
		t.Fatalf("missing %#v %v", miss, err)
	}
	probed, err := s.ListContacts("minh-an", "Other", "", "")
	if err != nil || len(probed) != 0 {
		t.Fatalf("search inferred other tenant %#v %v", probed, err)
	}
	if _, err := s.MergeContacts(rows[0].ID, other[0].ID, "minh-an", other[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross merge %v", err)
	}
	if n, err := s.CountContacts("other-co"); err != nil || n != 1 {
		t.Fatalf("B count after merge %d %v", n, err)
	}
	if _, err := s.BulkUpsertContacts([]ContactSeed{{TenantID: "minh-an", Channel: "telegram", Dest: "ext-only-a", Kind: "user"}}); err != nil {
		t.Fatal(err)
	}
	if n, err := s.CountContacts("minh-an"); err != nil || n != 2 {
		t.Fatalf("A after extra %d %v", n, err)
	}
	if n, err := s.CountContacts("other-co"); err != nil || n != 1 {
		t.Fatalf("B unchanged %d %v", n, err)
	}
}

func openContactSQLite(t *testing.T, path string) *SQLiteStore {
	t.Helper()
	s, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func openContactPostgres(t *testing.T, dsn string) *SQLiteStore {
	t.Helper()
	s, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s.SQLiteStore
}

func minhAnSeeds(tenant string, n int) []ContactSeed {
	out := make([]ContactSeed, 0, n)
	for i := 0; i < n; i++ {
		at := time.Unix(int64(1000+i), 0).UTC()
		out = append(out, ContactSeed{
			TenantID:  tenant,
			Channel:   "telegram",
			Dest:      fmt.Sprintf("ext-minh-an-%04d", i),
			Display:   fmt.Sprintf("Minh An %04d", i),
			Kind:      "user",
			Count:     1,
			FirstSeen: at,
			LastSeen:  at,
			AgentID:   "ag-minh-an",
		})
	}
	return out
}
