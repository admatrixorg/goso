// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package tenant

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestLoadMemoryStoreIsUnchanged(t *testing.T) {
	reg, err := Load(store.New())
	if err != nil || reg != nil {
		t.Fatalf("memory load reg=%v err=%v", reg, err)
	}
	reg, err = Load(nil)
	if err != nil || reg != nil {
		t.Fatalf("nil load reg=%v err=%v", reg, err)
	}
}

func TestRegistryPersistRestartSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tenants.db")
	exerciseRegistryRestart(t, func() *store.SQLiteStore {
		s, err := store.OpenSQLite(path)
		if err != nil {
			t.Fatal(err)
		}
		return s
	})
}

func TestRegistryPersistRestartPostgres(t *testing.T) {
	dsn := tenantSchemaDSN(t, "goso_treg_restart")
	if dsn == "" {
		return
	}
	exerciseRegistryRestart(t, func() *store.SQLiteStore {
		s, err := store.OpenPostgres(dsn)
		if err != nil {
			t.Fatal(err)
		}
		return s.SQLiteStore
	})
}

func TestRegistryPersistWriteFailureSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tenants.db")
	s, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	exerciseRegistryWriteFailure(t, s, func() *store.SQLiteStore {
		re, err := store.OpenSQLite(path)
		if err != nil {
			t.Fatal(err)
		}
		return re
	})
}

func TestRegistryPersistWriteFailurePostgres(t *testing.T) {
	dsn := tenantSchemaDSN(t, "goso_treg_fail")
	if dsn == "" {
		return
	}
	s, err := store.OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	exerciseRegistryWriteFailure(t, s.SQLiteStore, func() *store.SQLiteStore {
		re, err := store.OpenPostgres(dsn)
		if err != nil {
			t.Fatal(err)
		}
		return re.SQLiteStore
	})
}

func TestRegistryEmptyTableRecreatesMaster(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.db")
	s, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := Load(s)
	if err != nil || reg == nil {
		t.Fatalf("load %v %v", reg, err)
	}
	first := reg.Master().CreatedAt
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	s, err = store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	reg, err = Load(s)
	if err != nil {
		t.Fatal(err)
	}
	list := reg.List("")
	if len(list) != 1 || !list[0].Master || list[0].ID != Default || list[0].Status != StatusActive {
		t.Fatalf("empty %#v", list)
	}
	if reg.Master().CreatedAt.Equal(first) {
		t.Fatal("empty table persisted master")
	}
}

func exerciseRegistryRestart(t *testing.T, open func() *store.SQLiteStore) {
	t.Helper()
	s := open()
	reg, err := Load(s)
	if err != nil || reg == nil {
		t.Fatalf("load %v %v", reg, err)
	}
	stamp := time.Date(2026, 11, 2, 9, 30, 0, 0, time.UTC)
	reg.now = func() time.Time { return stamp }
	if _, err := reg.Create("minh-an", "Minh An"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := reg.AddMember("minh-an", "covan.phaply@minh-an.test", RoleOwner); err != nil {
		t.Fatal(err)
	}
	reg.now = func() time.Time { return stamp.Add(time.Minute) }
	_, member, err := reg.AddMember("minh-an", "troly@minh-an.test", RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := reg.SetMemberRole("minh-an", member.ID, RoleViewer); err != nil {
		t.Fatal(err)
	}
	// A second role change on the owner, then remove the assistant.
	owner, err := reg.Get("minh-an")
	if err != nil {
		t.Fatal(err)
	}
	var ownerID string
	for _, m := range owner.Members {
		if m.Subject == "covan.phaply@minh-an.test" {
			ownerID = m.ID
		}
	}
	if _, _, err := reg.SetMemberRole("minh-an", ownerID, RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if _, _, err := reg.RemoveMember("minh-an", member.ID, "troly@minh-an.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Create("phap-che", "Pháp chế"); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.SetStatus("phap-che", StatusDeactivated, "phap-che"); err != nil {
		t.Fatal(err)
	}
	masterBefore := reg.Master()
	if _, _, err := reg.AddMember(Default, "giamdoc@minh-an.test", RoleViewer); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s = open()
	defer s.Close()
	reg, err = Load(s)
	if err != nil {
		t.Fatal(err)
	}
	list := reg.List("")
	if len(list) != 3 {
		t.Fatalf("list %#v", list)
	}
	got, err := reg.Get("minh-an")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Minh An" || got.Status != StatusActive || got.Master || !got.CreatedAt.Equal(stamp) {
		t.Fatalf("minh-an %#v", got)
	}
	if len(got.Members) != 1 || got.Members[0].Subject != "covan.phaply@minh-an.test" || got.Members[0].Role != RoleAdmin {
		t.Fatalf("members %#v", got.Members)
	}
	if got.Members[0].ID == "" || !got.Members[0].CreatedAt.Equal(stamp) {
		t.Fatalf("member identity %#v", got.Members[0])
	}
	phap, err := reg.Get("phap-che")
	if err != nil || phap.Status != StatusDeactivated || phap.Name != "Pháp chế" || len(phap.Members) != 0 {
		t.Fatalf("phap-che %v %#v", err, phap)
	}
	if reg.Writable("phap-che") {
		t.Fatal("deactivated tenant writable after reload")
	}
	if !reg.Writable("kinh-doanh") {
		t.Fatal("unregistered tenant lost write access")
	}
	if _, err := reg.Get("kinh-doanh"); err != ErrNotFound {
		t.Fatalf("unregistered get %v", err)
	}
	master, err := reg.Get(Default)
	if err != nil || !master.Master || master.Status != StatusActive || master.Name != "Master" {
		t.Fatalf("master %#v %v", master, err)
	}
	if !master.CreatedAt.Equal(masterBefore.CreatedAt) {
		t.Fatalf("master created_at %s want %s", master.CreatedAt, masterBefore.CreatedAt)
	}
	if len(master.Members) != 1 || master.Members[0].Subject != "giamdoc@minh-an.test" || master.Members[0].Role != RoleViewer {
		t.Fatalf("master members %#v", master.Members)
	}
	if _, err := reg.SetStatus(Default, StatusDeactivated, Default); err != ErrMaster {
		t.Fatalf("deactivate master after reload: %v", err)
	}
	if reg.Master().Status != StatusActive {
		t.Fatal("master status changed")
	}
	reg.now = func() time.Time { return stamp.Add(2 * time.Minute) }
	_, extra, err := reg.AddMember("minh-an", "tailieu@minh-an.test", RoleMember)
	if err != nil {
		t.Fatal(err)
	}
	if extra.ID == got.Members[0].ID || extra.ID == "" {
		t.Fatalf("id collision %q", extra.ID)
	}
}

func exerciseRegistryWriteFailure(t *testing.T, s *store.SQLiteStore, reopen func() *store.SQLiteStore) {
	t.Helper()
	reg, err := Load(s)
	if err != nil || reg == nil {
		t.Fatalf("load %v %v", reg, err)
	}
	stamp := time.Date(2026, 11, 2, 9, 30, 0, 0, time.UTC)
	reg.now = func() time.Time { return stamp }
	if _, err := reg.Create("minh-an", "Minh An"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := reg.AddMember("minh-an", "covan.phaply@minh-an.test", RoleOwner); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Create("phap-che", "Pháp chế"); err != nil {
		t.Fatal(err)
	}
	before, err := reg.Get("minh-an")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	prev := tenantRegistryLogf
	var mu sync.Mutex
	var lines []string
	tenantRegistryLogf = func(format string, args ...any) {
		mu.Lock()
		lines = append(lines, fmt.Sprintf(format, args...))
		mu.Unlock()
	}
	t.Cleanup(func() { tenantRegistryLogf = prev })

	if _, err := reg.Create("kinh-doanh", "Kinh doanh"); !errors.Is(err, ErrStore) {
		t.Fatalf("create failure %v", err)
	}
	if _, _, err := reg.AddMember("minh-an", "troly@minh-an.test", RoleMember); !errors.Is(err, ErrStore) {
		t.Fatalf("member failure %v", err)
	}
	if _, err := reg.SetStatus("phap-che", StatusDeactivated, "phap-che"); !errors.Is(err, ErrStore) {
		t.Fatalf("status failure %v", err)
	}
	got, err := reg.Get("minh-an")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Members) != len(before.Members) || got.Members[0].Subject != before.Members[0].Subject {
		t.Fatalf("memory changed %#v", got.Members)
	}
	phap, err := reg.Get("phap-che")
	if err != nil || phap.Status != StatusActive {
		t.Fatalf("status changed %v %#v", err, phap)
	}
	if _, err := reg.Get("kinh-doanh"); err != ErrNotFound {
		t.Fatal("failed create landed in memory")
	}
	if len(reg.List("")) != 3 {
		t.Fatalf("list %#v", reg.List(""))
	}
	joined := strings.Join(lines, "\n")
	for _, leak := range []string{
		"covan.phaply@minh-an.test", "troly@minh-an.test", "kinh-doanh", "phap-che",
		"Minh An", "Pháp", "minh-an", "Kinh doanh",
	} {
		if strings.Contains(joined, leak) {
			t.Fatalf("log contains %q in %q", leak, joined)
		}
	}
	for _, op := range []string{"create", "member_add", "status"} {
		if !strings.Contains(joined, "goso tenant registry: "+op+" failed class=") {
			t.Fatalf("missing %s log in %q", op, joined)
		}
	}
	for _, line := range lines {
		if strings.Contains(line, "database") || strings.Contains(line, "sql:") || strings.Contains(line, "@") {
			t.Fatalf("log line %q", line)
		}
	}

	freshDB := reopen()
	defer freshDB.Close()
	fresh, err := Load(freshDB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fresh.Get("kinh-doanh"); err != ErrNotFound {
		t.Fatal("failed create was committed")
	}
	got, err = fresh.Get("minh-an")
	if err != nil || len(got.Members) != 1 || got.Members[0].Subject != "covan.phaply@minh-an.test" {
		t.Fatalf("db changed %#v %v", got, err)
	}
	phap, err = fresh.Get("phap-che")
	if err != nil || phap.Status != StatusActive {
		t.Fatalf("db status %#v %v", phap, err)
	}
}

func tenantSchemaDSN(t *testing.T, schema string) string {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("GOSO_TEST_POSTGRES_DSN"))
	if base == "" {
		base = strings.TrimSpace(os.Getenv("GOSO_TEST_DATABASE_URL"))
	}
	if base == "" {
		if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
			t.Fatal("GOSO_TEST_POSTGRES_DSN is required in CI")
		}
		t.Skip("GOSO_TEST_POSTGRES_DSN unset")
	}
	db, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("options", "-csearch_path="+schema)
	u.RawQuery = q.Encode()
	return u.String()
}
