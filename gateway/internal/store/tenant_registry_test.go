// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTenantRegistryMigrationTwiceSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tenants.db")
	s := openTenantRegistrySQLite(t, path)
	if err := s.PutTenantRegistry(minhAnTenantRow()); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTenantRegistrySQLite(t, path)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTenantRegistrySQLite(t, path)
	defer s.Close()
	got := listMinhAn(t, s)
	if got.Name != "Minh An" || len(got.Members) != 2 {
		t.Fatalf("after second migrate %#v", got)
	}
	var tables int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='tenant_registry'`).Scan(&tables); err != nil || tables != 1 {
		t.Fatalf("tables %d %v", tables, err)
	}
}

func TestTenantRegistryMigrationFaultRollsBackSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fault.db")
	tenantRegistryMigrateFault = func(step string) error {
		if step == "tenant_registry_members" {
			return errors.New("injected")
		}
		return nil
	}
	t.Cleanup(func() { tenantRegistryMigrateFault = nil })
	if _, err := OpenSQLite(path); err == nil {
		t.Fatal("faulted migration reported success")
	}
	tenantRegistryMigrateFault = nil
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	for _, name := range []string{"tenant_registry", "tenant_registry_members"} {
		var n int
		if err := raw.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("rolled-back table %s is still present", name)
		}
	}
	s := openTenantRegistrySQLite(t, path)
	defer s.Close()
	if err := s.PutTenantRegistry(minhAnTenantRow()); err != nil {
		t.Fatal(err)
	}
	if got := listMinhAn(t, s); got.Status != "active" {
		t.Fatalf("replay %#v", got)
	}
}

func TestTenantRegistryMigrationLeavesMemoryVaultContacts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keep.db")
	s := openTenantRegistrySQLite(t, path)
	var memDDL string
	if err := s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='memories'`).Scan(&memDDL); err != nil {
		t.Fatal(err)
	}
	agent, err := s.CreateAgent(Agent{AgentKey: "minh-an-sales", DisplayName: "Cố vấn Minh An", TenantID: "minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := s.CreateSession(Session{AgentID: agent.ID, TenantID: "minh-an", Label: "phap-che"})
	if err != nil {
		t.Fatal(err)
	}
	mem, err := s.PutMemory(Memory{SessionID: sess.ID, Kind: KindEpisodic, Body: "minh-an-note", TenantID: "minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := s.PutVaultDoc(VaultDoc{
		Title: "Minh An", Path: "minh-an.md", Body: "keep-note", TenantID: "minh-an",
		SHA256: "aa", OwnerExplicit: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.BulkUpsertContacts([]ContactSeed{{
		TenantID: "minh-an", Channel: "telegram", Dest: "ext-minh-an-1", Display: "Minh An", Kind: "user", Count: 1,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTenantRegistrySQLite(t, path)
	defer s.Close()
	var memDDL2 string
	if err := s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='memories'`).Scan(&memDDL2); err != nil {
		t.Fatal(err)
	}
	if memDDL2 != memDDL {
		t.Fatalf("memories ddl changed\nbefore %s\nafter %s", memDDL, memDDL2)
	}
	got, err := s.GetMemory(mem.ID)
	if err != nil || got.Body != "minh-an-note" || got.TenantID != "minh-an" {
		t.Fatalf("memory %#v %v", got, err)
	}
	vd, err := s.GetVaultDoc(doc.ID)
	if err != nil || vd.Body != "keep-note" || vd.TenantID != "minh-an" {
		t.Fatalf("vault %#v %v", vd, err)
	}
	n, err := s.CountContacts("minh-an")
	if err != nil || n != 1 {
		t.Fatalf("contacts %d %v", n, err)
	}
	var tenants int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM tenant_registry`).Scan(&tenants); err != nil || tenants != 0 {
		t.Fatalf("tenant side effect %d %v", tenants, err)
	}
}

func TestTenantRegistryRoundTripAndReplaceSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "round.db")
	s := openTenantRegistrySQLite(t, path)
	defer s.Close()
	exerciseTenantRegistryRoundTrip(t, s)
}

func TestTenantRegistryPutErrorHidesLabels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "closed.db")
	s := openTenantRegistrySQLite(t, path)
	row := minhAnTenantRow()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	err := s.PutTenantRegistry(row)
	if err == nil {
		t.Fatal("expected write failure")
	}
	msg := err.Error()
	for _, leak := range []string{"covan.phaply@minh-an.test", "troly@minh-an.test", "Minh An", "minh-an", "Pháp"} {
		if strings.Contains(msg, leak) {
			t.Fatalf("error contains %q: %s", leak, msg)
		}
	}
	if msg != "tenant registry save failed" {
		t.Fatalf("error %q", msg)
	}
	var fault *TenantRegistryFault
	if !errors.As(err, &fault) || fault.Class != "closed" && fault.Class != "driver" {
		t.Fatalf("fault %#v", fault)
	}
}

func TestTenantRegistryMigrationTwicePostgres(t *testing.T) {
	base := postgresTestDSN()
	if base == "" {
		if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
			t.Fatal("GOSO_TEST_POSTGRES_DSN is required in CI")
		}
		t.Skip("GOSO_TEST_POSTGRES_DSN unset")
	}
	dsn := prepareSchema(t, base, "goso_treg_mig", false)
	s := openTenantRegistryPostgres(t, dsn)
	if err := s.PutTenantRegistry(minhAnTenantRow()); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTenantRegistryPostgres(t, dsn)
	defer s.Close()
	if got := listMinhAn(t, s); len(got.Members) != 2 || got.Members[0].Subject != "covan.phaply@minh-an.test" {
		t.Fatalf("postgres %#v", got)
	}
	var tables int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name='tenant_registry'`).Scan(&tables); err != nil || tables != 1 {
		t.Fatalf("tables %d %v", tables, err)
	}
}

func TestTenantRegistryMigrationFaultRollsBackPostgres(t *testing.T) {
	base := postgresTestDSN()
	if base == "" {
		if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
			t.Fatal("GOSO_TEST_POSTGRES_DSN is required in CI")
		}
		t.Skip("GOSO_TEST_POSTGRES_DSN unset")
	}
	dsn := prepareSchema(t, base, "goso_treg_fault", false)
	tenantRegistryMigrateFault = func(step string) error {
		if step == "tenant_registry_members" {
			return errors.New("injected")
		}
		return nil
	}
	t.Cleanup(func() { tenantRegistryMigrateFault = nil })
	if _, err := OpenPostgres(dsn); err == nil {
		t.Fatal("faulted migration reported success")
	}
	tenantRegistryMigrateFault = nil
	check, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	for _, name := range []string{"tenant_registry", "tenant_registry_members"} {
		var n int
		q := `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=$1`
		if err := check.QueryRow(q, name).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("rolled-back table %s is still present", name)
		}
	}
	s := openTenantRegistryPostgres(t, dsn)
	defer s.Close()
	if err := s.PutTenantRegistry(minhAnTenantRow()); err != nil {
		t.Fatal(err)
	}
}

func TestTenantRegistryRoundTripPostgres(t *testing.T) {
	base := postgresTestDSN()
	if base == "" {
		if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
			t.Fatal("GOSO_TEST_POSTGRES_DSN is required in CI")
		}
		t.Skip("GOSO_TEST_POSTGRES_DSN unset")
	}
	dsn := prepareSchema(t, base, "goso_treg_data", false)
	s := openTenantRegistryPostgres(t, dsn)
	defer s.Close()
	exerciseTenantRegistryRoundTrip(t, s)
}

func TestTenantRegistryNeighborsPostgres(t *testing.T) {
	base := postgresTestDSN()
	if base == "" {
		if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
			t.Fatal("GOSO_TEST_POSTGRES_DSN is required in CI")
		}
		t.Skip("GOSO_TEST_POSTGRES_DSN unset")
	}
	dsn := prepareSchema(t, base, "goso_treg_keep", false)
	s := openTenantRegistryPostgres(t, dsn)
	agent, err := s.CreateAgent(Agent{AgentKey: "minh-an-sales", DisplayName: "Cố vấn Minh An", TenantID: "minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := s.CreateSession(Session{AgentID: agent.ID, TenantID: "minh-an", Label: "phap-che"})
	if err != nil {
		t.Fatal(err)
	}
	mem, err := s.PutMemory(Memory{SessionID: sess.ID, Kind: KindEpisodic, Body: "minh-an-note", TenantID: "minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := s.PutVaultDoc(VaultDoc{
		Title: "Minh An", Path: "minh-an.md", Body: "keep-note", TenantID: "minh-an",
		SHA256: "aa", OwnerExplicit: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.BulkUpsertContacts([]ContactSeed{{
		TenantID: "minh-an", Channel: "telegram", Dest: "ext-minh-an-1", Display: "Minh An", Kind: "user", Count: 1,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = openTenantRegistryPostgres(t, dsn)
	defer s.Close()
	got, err := s.GetMemory(mem.ID)
	if err != nil || got.Body != "minh-an-note" {
		t.Fatalf("memory %#v %v", got, err)
	}
	vd, err := s.GetVaultDoc(doc.ID)
	if err != nil || vd.Body != "keep-note" {
		t.Fatalf("vault %#v %v", vd, err)
	}
	n, err := s.CountContacts("minh-an")
	if err != nil || n != 1 {
		t.Fatalf("contacts %d %v", n, err)
	}
}

func exerciseTenantRegistryRoundTrip(t *testing.T, s *SQLiteStore) {
	t.Helper()
	row := minhAnTenantRow()
	if err := s.PutTenantRegistry(row); err != nil {
		t.Fatal(err)
	}
	other := TenantRegistryRow{
		ID: "phap-che", Name: "Pháp chế", Status: "deactivated", Master: false, CreatedAt: row.CreatedAt,
		Members: []TenantRegistryMember{},
	}
	if err := s.PutTenantRegistry(other); err != nil {
		t.Fatal(err)
	}
	got := listMinhAn(t, s)
	if !got.CreatedAt.Equal(row.CreatedAt) || got.Members[1].Role != "member" {
		t.Fatalf("roundtrip %#v", got)
	}
	if !got.Members[0].CreatedAt.Equal(row.Members[0].CreatedAt) || got.Members[0].ID != "tm_1" {
		t.Fatalf("member %#v", got.Members[0])
	}
	row.Members = []TenantRegistryMember{row.Members[0]}
	row.Members[0].Role = "admin"
	if err := s.PutTenantRegistry(row); err != nil {
		t.Fatal(err)
	}
	got = listMinhAn(t, s)
	if len(got.Members) != 1 || got.Members[0].Role != "admin" || got.Members[0].Subject != "covan.phaply@minh-an.test" {
		t.Fatalf("replaced %#v", got.Members)
	}
	rows, err := s.ListTenantRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows %d", len(rows))
	}
}

func minhAnTenantRow() TenantRegistryRow {
	at := time.Date(2026, 11, 2, 9, 30, 0, 0, time.UTC)
	return TenantRegistryRow{
		ID: "minh-an", Name: "Minh An", Status: "active", Master: false, CreatedAt: at,
		Members: []TenantRegistryMember{
			{ID: "tm_1", Subject: "covan.phaply@minh-an.test", Role: "owner", CreatedAt: at},
			{ID: "tm_2", Subject: "troly@minh-an.test", Role: "member", CreatedAt: at.Add(time.Minute)},
		},
	}
}

func listMinhAn(t *testing.T, s *SQLiteStore) TenantRegistryRow {
	t.Helper()
	rows, err := s.ListTenantRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ID == "minh-an" {
			return row
		}
	}
	t.Fatal("minh-an missing")
	return TenantRegistryRow{}
}

func openTenantRegistrySQLite(t *testing.T, path string) *SQLiteStore {
	t.Helper()
	s, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func openTenantRegistryPostgres(t *testing.T, dsn string) *SQLiteStore {
	t.Helper()
	s, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	return s.SQLiteStore
}
