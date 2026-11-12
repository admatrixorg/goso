// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package contactsimport

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestFixtureFileMatchesGenerator(t *testing.T) {
	if FixtureCount < 201 {
		t.Fatalf("fixture count %d", FixtureCount)
	}
	got, err := os.ReadFile(filepath.Join("testdata", "minh-an-contacts.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := FixtureJSON()
	if !bytes.Equal(got, want) {
		t.Fatalf("testdata/minh-an-contacts.json is not FixtureJSON() (%d vs %d bytes)", len(got), len(want))
	}
	if bytes.Contains(got, []byte("@")) || bytes.Contains(got, []byte("+")) || bytes.Contains(got, []byte("phone")) || bytes.Contains(got, []byte("email")) {
		t.Fatal("fixture contains an email, a plus, or a phone/email field")
	}
	if regexp.MustCompile(`\d{8,}`).Match(got) {
		t.Fatal("fixture contains a long digit run")
	}
	var file File
	if err := json.Unmarshal(got, &file); err != nil {
		t.Fatal(err)
	}
	if len(file.Contacts) != FixtureCount {
		t.Fatalf("contacts %d", len(file.Contacts))
	}
	groups := 0
	for i, c := range file.Contacts {
		if c.TenantID != "minh-an" || !strings.HasPrefix(c.Display, "Minh An ") || c.AgentID != "minh-an-sales" {
			t.Fatalf("row %d identity", i)
		}
		if i < 200 {
			if c.Channel != "telegram" || c.Kind != "user" || c.Dest != fmt.Sprintf("ext-minh-an-%04d", i) {
				t.Fatalf("user row %d", i)
			}
		} else {
			groups++
			if c.Channel != "zalo-oa" || c.Kind != "group" || c.Dest != fmt.Sprintf("grp-minh-an-%04d", i) {
				t.Fatalf("group row %d", i)
			}
		}
	}
	if groups != 10 {
		t.Fatalf("groups %d", groups)
	}
}

func TestReadExportShape(t *testing.T) {
	if _, err := ReadExport(strings.NewReader(`{"vault_documents":[]}`)); err == nil {
		t.Fatal("missing contacts accepted")
	}
	ex, err := ReadExport(strings.NewReader(`{"contacts":null}`))
	if err != nil || ex == nil || len(ex.rows) != 0 {
		t.Fatalf("null contacts %+v %v", ex, err)
	}
	if _, err := ReadExport(strings.NewReader(`{"contacts":[]}{"contacts":[]}`)); err == nil {
		t.Fatal("trailing value accepted")
	}
	prev := maxExportBytes
	maxExportBytes = 8
	t.Cleanup(func() { maxExportBytes = prev })
	if _, err := ReadExport(strings.NewReader(`{"contacts":[]}`)); !errors.Is(err, ErrExportTooLarge) {
		t.Fatalf("too large %v", err)
	}
}

func TestImportMinhAnSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contacts.db")
	s, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	exerciseMinhAn(t, s)
}

func TestImportMinhAnPostgres(t *testing.T) {
	s := openImportPostgres(t)
	if s == nil {
		return
	}
	exerciseMinhAn(t, s)
}

func exerciseMinhAn(t *testing.T, s *store.SQLiteStore) {
	t.Helper()
	assertSideTables(t, s)
	if _, err := s.BulkUpsertContacts([]store.ContactSeed{{
		TenantID:  "tenant-bee",
		Channel:   "telegram",
		Dest:      "ext-bee-keep",
		Display:   "Bee Keep",
		Kind:      "user",
		AgentID:   "bee-agent",
		Count:     3,
		FirstSeen: mustStamp(t, "2026-01-15T08:00:00Z"),
		LastSeen:  mustStamp(t, "2026-01-15T09:00:00Z"),
	}}); err != nil {
		t.Fatal(err)
	}
	raw, validN, invalidN := mixedJSON(t)
	ex, err := ReadExport(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	spy := &spyStore{SQLiteStore: s}
	rep, err := Import(spy, ex, Options{Tenant: "minh-an", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if rep.New != validN || rep.Updated != 0 || rep.Skipped != 0 || rep.Invalid != invalidN || !rep.DryRun {
		t.Fatalf("dry-run %+v want new %d invalid %d", rep, validN, invalidN)
	}
	assertReasons(t, rep, map[string]int{
		"channel-length": 1,
		"control":        1,
		"count":          1,
		"dest-length":    1,
		"duplicate":      1,
		"empty-dest":     1,
		"foreign-tenant": 1,
		"kind":           1,
		"row":            1,
		"tenant":         1,
		"time":           1,
	})
	assertNoLeak(t, rep)
	if spy.calls != 0 {
		t.Fatal("dry-run called upsert")
	}
	if n := mustCount(t, s, "minh-an"); n != 0 {
		t.Fatalf("dry-run wrote %d", n)
	}
	if n := mustCount(t, s, "tenant-bee"); n != 1 {
		t.Fatalf("bee count %d", n)
	}

	rep, err = Import(spy, ex, Options{Tenant: "minh-an", DryRun: false})
	if err != nil {
		t.Fatal(err)
	}
	if rep.New != validN || rep.Updated != 0 || rep.Skipped != 0 || rep.Invalid != invalidN || rep.DryRun {
		t.Fatalf("apply %+v", rep)
	}
	assertNoLeak(t, rep)
	if spy.calls != 1 || spy.last != validN {
		t.Fatalf("upsert calls %d last %d", spy.calls, spy.last)
	}
	if n := mustCount(t, s, "minh-an"); n != validN {
		t.Fatalf("count %d want %d", n, validN)
	}
	if validN < 201 {
		t.Fatalf("imported %d", validN)
	}
	ids := map[string]string{}
	rows, err := s.ListContacts("minh-an", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	groups := 0
	for _, row := range rows {
		if row.TenantID != "minh-an" {
			t.Fatalf("tenant %s", row.TenantID)
		}
		ids[row.Dest] = row.ID
		if row.Kind == "group" {
			groups++
		}
		if strings.Contains(row.Display, "DROPME") || strings.Contains(row.Dest, "DROPME") || strings.Contains(row.AgentID, "DROPME") {
			t.Fatal("unknown field was stored")
		}
	}
	if groups != 11 {
		t.Fatalf("groups %d", groups)
	}
	blank, err := s.ListContacts("minh-an", "ext-blank-name-0001", "", "")
	if err != nil || len(blank) != 1 || blank[0].Display != "telegram:ext-blank-name-0001" {
		t.Fatalf("blank display %#v %v", blank, err)
	}
	zero, err := s.ListContacts("minh-an", "ext-zero-0001", "", "")
	if err != nil || len(zero) != 1 || zero[0].Kind != "user" || zero[0].Count != 1 {
		t.Fatalf("zero count %#v %v", zero, err)
	}
	grp, err := s.ListContacts("minh-an", "-g-minh-an-1", "", "")
	if err != nil || len(grp) != 1 || grp[0].Kind != "group" {
		t.Fatalf("leading minus %#v %v", grp, err)
	}
	if n := mustCount(t, s, "default"); n != 0 {
		t.Fatalf("default tenant %d", n)
	}
	bee, err := s.ListContacts("tenant-bee", "", "", "")
	if err != nil || len(bee) != 1 || bee[0].Display != "Bee Keep" || bee[0].Dest != "ext-bee-keep" || bee[0].Count != 3 {
		t.Fatalf("bee %#v %v", bee, err)
	}
	if _, err := s.GetContact(bee[0].ID, "minh-an"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross get %v", err)
	}
	if leaked, err := s.ListContacts("minh-an", "ext-bee-keep", "", ""); err != nil || len(leaked) != 0 {
		t.Fatalf("minh-an saw bee %#v %v", leaked, err)
	}
	if leaked, err := s.ListContacts("minh-an", "ext-bee-foreign", "", ""); err != nil || len(leaked) != 0 {
		t.Fatalf("foreign dest stored %#v %v", leaked, err)
	}
	if leaked, err := s.ListContacts("tenant-bee", "ext-bee-foreign", "", ""); err != nil || len(leaked) != 0 {
		t.Fatalf("foreign dest written onto bee %#v %v", leaked, err)
	}
	assertMarkerAbsent(t, s, "DROPME")
	assertMarkerAbsent(t, s, "Bee Foreign")
	assertTenants(t, s, []string{"minh-an", "tenant-bee"})

	rep, err = Import(spy, ex, Options{Tenant: "minh-an", DryRun: false})
	if err != nil {
		t.Fatal(err)
	}
	if rep.New != 0 || rep.Updated != 0 || rep.Skipped != validN || rep.Invalid != invalidN {
		t.Fatalf("rerun %+v", rep)
	}
	assertNoLeak(t, rep)
	if spy.calls != 1 {
		t.Fatal("rerun wrote")
	}
	if n := mustCount(t, s, "minh-an"); n != validN {
		t.Fatalf("rerun count %d", n)
	}
	again, err := s.ListContacts("minh-an", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != validN {
		t.Fatalf("rerun list %d", len(again))
	}
	for _, row := range again {
		if ids[row.Dest] != row.ID {
			t.Fatal("rerun changed an id")
		}
	}

	rep, err = Import(spy, ex, Options{Tenant: "minh-an", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Skipped != validN || rep.New != 0 || rep.Updated != 0 || spy.calls != 1 {
		t.Fatalf("second dry-run %+v calls %d", rep, spy.calls)
	}
	if n := mustCount(t, s, "minh-an"); n != validN {
		t.Fatalf("second dry-run count %d", n)
	}

	first := MinhAnFile().Contacts[0]
	renamed := first
	renamed.Display = "Minh An Renamed"
	renameRaw := mustExport(t, File{Contacts: []Contact{renamed}})
	renameEx, err := ReadExport(bytes.NewReader(renameRaw))
	if err != nil {
		t.Fatal(err)
	}
	before := ids[first.Dest]
	rep, err = Import(spy, renameEx, Options{Tenant: "minh-an", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Updated != 1 || rep.New != 0 || rep.Skipped != 0 || rep.Invalid != 0 || spy.calls != 1 {
		t.Fatalf("rename dry-run %+v calls %d", rep, spy.calls)
	}
	assertNoLeak(t, rep)
	held, err := s.ListContacts("minh-an", first.Dest, "", "")
	if err != nil || len(held) != 1 || held[0].Display != first.Display || held[0].ID != before {
		t.Fatalf("dry-run rename wrote %#v %v", held, err)
	}
	rep, err = Import(spy, renameEx, Options{Tenant: "minh-an", DryRun: false})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Updated != 1 || spy.calls != 2 || spy.last != 1 {
		t.Fatalf("rename apply %+v calls %d last %d", rep, spy.calls, spy.last)
	}
	updated, err := s.ListContacts("minh-an", first.Dest, "", "")
	if err != nil || len(updated) != 1 || updated[0].Display != "Minh An Renamed" || updated[0].ID != before {
		t.Fatalf("rename result %#v %v", updated, err)
	}
	if n := mustCount(t, s, "minh-an"); n != validN {
		t.Fatalf("rename count %d", n)
	}
	rep, err = Import(spy, renameEx, Options{Tenant: "minh-an", DryRun: false})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Skipped != 1 || rep.Updated != 0 || rep.New != 0 || spy.calls != 2 {
		t.Fatalf("rename rerun %+v calls %d", rep, spy.calls)
	}
	if n := mustCount(t, s, "tenant-bee"); n != 1 {
		t.Fatalf("bee after rename %d", n)
	}
	assertSideTables(t, s)
	assertNoLeak(t, rep)
}

func mixedJSON(t *testing.T) (raw []byte, validN, invalidN int) {
	t.Helper()
	file := MinhAnFile()
	rows := make([]json.RawMessage, 0, len(file.Contacts)+16)
	for _, c := range file.Contacts {
		rows = append(rows, mustRaw(t, c))
	}
	validN = len(file.Contacts)
	addValid := func(v any) {
		rows = append(rows, mustRaw(t, v))
		validN++
	}
	addInvalid := func(v any) {
		rows = append(rows, mustRaw(t, v))
		invalidN++
	}
	inherit := baseRow("ext-inherit-0001", "Minh An Inherit")
	inherit["tenant_id"] = ""
	addValid(inherit)
	unknown := baseRow("ext-minh-an-extra", "Minh An Extra")
	unknown["phone"] = "DROPME-phone-marker"
	unknown["email"] = "DROPME-email-marker"
	unknown["note"] = "DROPME-note-marker"
	addValid(unknown)
	zero := baseRow("ext-zero-0001", "Minh An Zero")
	zero["seen_count"] = 0
	zero["kind"] = "direct"
	addValid(zero)
	group := baseRow("-g-minh-an-1", "Minh An Group")
	group["kind"] = ""
	addValid(group)
	blank := baseRow("ext-blank-name-0001", "")
	addValid(blank)

	foreign := baseRow("ext-bee-foreign", "Bee Foreign")
	foreign["tenant_id"] = "tenant-bee"
	addInvalid(foreign)
	emptyDest := baseRow("", "Minh An Empty")
	addInvalid(emptyDest)
	badKind := baseRow("ext-kind-0001", "Minh An Kind")
	badKind["kind"] = "bot"
	addInvalid(badKind)
	addInvalid(file.Contacts[0])
	neg := baseRow("ext-neg-0001", "Minh An Neg")
	neg["seen_count"] = -1
	addInvalid(neg)
	missingTime := baseRow("ext-time-0001", "Minh An Time")
	delete(missingTime, "last_seen")
	addInvalid(missingTime)
	longDest := baseRow(strings.Repeat("d", maxDestLen+1), "Minh An Dest")
	addInvalid(longDest)
	longCh := baseRow("ext-channel-0001", "Minh An Channel")
	longCh["channel"] = strings.Repeat("c", maxChannelLen+1)
	addInvalid(longCh)
	ctrl := baseRow("ext-control-0001", "Minh An\nBad")
	addInvalid(ctrl)
	badTenant := baseRow("ext-tenant-0001", "Minh An Tenant")
	badTenant["tenant_id"] = "bad tenant"
	addInvalid(badTenant)
	addInvalid(json.RawMessage(`{"channel":1}`))
	body, err := json.Marshal(struct {
		Contacts []json.RawMessage `json:"contacts"`
	}{Contacts: rows})
	if err != nil {
		t.Fatal(err)
	}
	return body, validN, invalidN
}

func baseRow(dest, display string) map[string]any {
	return map[string]any{
		"tenant_id":  "minh-an",
		"channel":    "telegram",
		"dest":       dest,
		"display":    display,
		"kind":       "user",
		"agent_id":   "minh-an-sales",
		"seen_count": 1,
		"first_seen": "2026-01-15T08:00:00Z",
		"last_seen":  "2026-01-15T09:00:00Z",
	}
}

func assertReasons(t *testing.T, rep Report, want map[string]int) {
	t.Helper()
	if len(rep.Reasons) != len(want) {
		t.Fatalf("reasons %+v", rep.Reasons)
	}
	for _, r := range rep.Reasons {
		if want[r.Code] != r.Count {
			t.Fatalf("reason %s=%d want %d in %+v", r.Code, r.Count, want[r.Code], rep.Reasons)
		}
	}
}

func assertNoLeak(t *testing.T, rep Report) {
	t.Helper()
	b, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, m := range []string{"Minh An", "ext-", "grp-", "DROPME", "Bee", "telegram", "zalo", "minh-an", "bee-agent"} {
		if strings.Contains(s, m) {
			t.Fatalf("report contains %q in %s", m, s)
		}
	}
}

func assertMarkerAbsent(t *testing.T, s *store.SQLiteStore, marker string) {
	t.Helper()
	db, _ := s.SQL()
	for _, q := range []string{
		`SELECT display FROM contacts`,
		`SELECT dest FROM contacts`,
		`SELECT channel FROM contacts`,
		`SELECT agent_id FROM contacts`,
		`SELECT kind FROM contacts`,
		`SELECT dest FROM contact_identifiers`,
		`SELECT channel FROM contact_identifiers`,
	} {
		rows, err := db.Query(q)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var v string
			if err := rows.Scan(&v); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			if strings.Contains(v, marker) {
				rows.Close()
				t.Fatalf("stored column contains %q", marker)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		rows.Close()
	}
}

func assertTenants(t *testing.T, s *store.SQLiteStore, want []string) {
	t.Helper()
	db, _ := s.SQL()
	rows, err := db.Query(`SELECT DISTINCT tenant_id FROM contacts ORDER BY tenant_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		got = append(got, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("tenants %v", got)
	}
}

func assertSideTables(t *testing.T, s *store.SQLiteStore) {
	t.Helper()
	db, _ := s.SQL()
	for _, table := range []string{"memories", "vault_docs", "kg_entities", "kg_relations", "contact_merges"} {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
			t.Fatal(table, err)
		}
		if n != 0 {
			t.Fatalf("%s has %d rows", table, n)
		}
	}
}

func mustCount(t *testing.T, s *store.SQLiteStore, tenant string) int {
	t.Helper()
	n, err := s.CountContacts(tenant)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func mustStamp(t *testing.T, s string) time.Time {
	t.Helper()
	got, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func mustRaw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mustExport(t *testing.T, file File) []byte {
	t.Helper()
	b, err := json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type spyStore struct {
	*store.SQLiteStore
	calls int
	last  int
}

func (s *spyStore) BulkUpsertContacts(seeds []store.ContactSeed) (store.ContactBulkResult, error) {
	s.calls++
	s.last = len(seeds)
	return s.SQLiteStore.BulkUpsertContacts(seeds)
}

func openImportPostgres(t *testing.T) *store.SQLiteStore {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("GOSO_TEST_POSTGRES_DSN"))
	if dsn == "" {
		dsn = strings.TrimSpace(os.Getenv("GOSO_TEST_DATABASE_URL"))
	}
	if dsn == "" {
		if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
			t.Fatal("GOSO_TEST_POSTGRES_DSN is required in CI")
		}
		t.Skip("GOSO_TEST_POSTGRES_DSN unset")
	}
	const schema = "contactsimport172"
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	if err := admin.Ping(); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
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
	return pg.SQLiteStore
}
