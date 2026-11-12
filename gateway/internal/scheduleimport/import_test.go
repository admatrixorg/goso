// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package scheduleimport

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mqglobal/goso/gateway/internal/store"
)

const secretMarker = "fixture-secret-minh-an-do-not-log"

func TestMinhAnImportDisabledIsolatedAndIdempotent(t *testing.T) {
	ex := readFixture(t, "testdata/minh-an.json")
	jsonl := readFixture(t, "testdata/minh-an.jsonl")
	if len(ex.CronJobs) != len(jsonl.CronJobs) || len(ex.ChannelInstances) != len(jsonl.ChannelInstances) {
		t.Fatalf("json %d/%d jsonl %d/%d", len(ex.CronJobs), len(ex.ChannelInstances), len(jsonl.CronJobs), len(jsonl.ChannelInstances))
	}
	for name, open := range map[string]func(t *testing.T) store.StoreIface{
		"memory": func(t *testing.T) store.StoreIface { return store.New() },
		"sqlite": func(t *testing.T) store.StoreIface {
			s, err := store.OpenSQLite(filepath.Join(t.TempDir(), "goso.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			return s
		},
	} {
		t.Run(name, func(t *testing.T) {
			st := open(t)
			w := seed(t, st)
			logBuf := &bytes.Buffer{}
			opt := Options{SourceTenant: "minh-an", DestTenant: "minh-an", DryRun: true, Deny: testDeny(), Log: bufLog{logBuf}}
			before := snapshot(t, st)
			rep, err := Import(st, ex, opt)
			if err != nil {
				t.Fatal(err)
			}
			if !rep.DryRun {
				t.Fatal("dry-run flag")
			}
			if snapshot(t, st) != before {
				t.Fatal("dry-run wrote")
			}
			minh, other := tenantCounts(t, rep)
			if minh.CronJobs.Input != 4 || minh.CronJobs.New != 0 || minh.CronJobs.NotImported != 4 || minh.CronJobs.Written != 0 {
				t.Fatalf("dry cron %+v", minh.CronJobs)
			}
			if minh.ChannelInstances.Input != 2 || minh.ChannelInstances.New != 1 || minh.ChannelInstances.NotImported != 1 || minh.ChannelInstances.Written != 0 {
				t.Fatalf("dry channel %+v", minh.ChannelInstances)
			}
			if other.CronJobs.NotImported != 1 || other.CronJobs.Written != 0 || other.ChannelInstances.Held != 1 || other.ChannelInstances.Written != 0 {
				t.Fatalf("other cron %+v channel %+v", other.CronJobs, other.ChannelInstances)
			}
			assertNeedsCredential(t, rep)
			assertNoSecret(t, logBuf.String()+mustJSON(t, rep))
			if !hasDetail(rep, ReasonCronNoTenant) || !hasDetail(rep, "unknown-channel") {
				t.Fatalf("drops %+v", rep.NotImported)
			}
			for _, d := range rep.NotImported {
				switch d.Reason {
				case ReasonNeedsDecision, ReasonNativeExists, ReasonCronNoTenant:
				default:
					t.Fatalf("reason %q", d.Reason)
				}
			}

			opt.DryRun = false
			logBuf.Reset()
			rep, err = Import(st, ex, opt)
			if err != nil {
				t.Fatal(err)
			}
			minh, other = tenantCounts(t, rep)
			if minh.CronJobs.New != 0 || minh.CronJobs.Written != 0 || minh.CronJobs.NotImported != 4 || minh.ChannelInstances.New != 1 || minh.ChannelInstances.Written != 1 {
				t.Fatalf("apply cron %+v channel %+v", minh.CronJobs, minh.ChannelInstances)
			}
			if other.CronJobs.Written != 0 || other.ChannelInstances.Written != 0 {
				t.Fatal("other tenant was written")
			}
			assertNoSecret(t, logBuf.String()+mustJSON(t, rep)+snapshot(t, st))
			assertImportedDisabled(t, st, w)
			assertUntouched(t, st, w)

			rep, err = Import(st, ex, opt)
			if err != nil {
				t.Fatal(err)
			}
			minh, _ = tenantCounts(t, rep)
			if minh.CronJobs.New != 0 || minh.CronJobs.NotImported != 4 || minh.CronJobs.Written != 0 {
				t.Fatalf("rerun cron %+v", minh.CronJobs)
			}
			if minh.ChannelInstances.New != 0 || minh.ChannelInstances.Skipped != 1 || minh.ChannelInstances.Written != 0 {
				t.Fatalf("rerun channel %+v", minh.ChannelInstances)
			}
			assertImportedDisabled(t, st, w)
			if len(st.ListCronJobs()) != 1 {
				t.Fatalf("cron count %d", len(st.ListCronJobs()))
			}
		})
	}
}

func TestImportedChannelsAndCronsStayDisabled(t *testing.T) {
	st := store.New()
	_ = seed(t, st)
	ex := readFixture(t, "testdata/minh-an.json")
	if _, err := Import(st, ex, Options{SourceTenant: "minh-an", DestTenant: "minh-an", Deny: testDeny()}); err != nil {
		t.Fatal(err)
	}
	if len(st.ListCronJobs()) != 1 {
		t.Fatalf("cron was imported: %d", len(st.ListCronJobs()))
	}
	cfg, err := st.GetChannelConfig("minh-an/telegram/minh-an-telegram")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled {
		t.Fatal("imported channel enabled")
	}
	if _, err := st.GetChannelConfig("other-co/zalo-personal/other-telegram"); err == nil {
		t.Fatal("other tenant channel was created")
	}
}

func TestAddOnlyDoesNotOverwrite(t *testing.T) {
	st := store.New()
	w := seed(t, st)
	if err := st.PutChannelConfig(store.ChannelConfig{
		Name: "minh-an/telegram/minh-an-telegram", Enabled: true, DMPolicy: "open", AgentID: w.other.ID,
	}); err != nil {
		t.Fatal(err)
	}
	ex := readFixture(t, "testdata/minh-an.json")
	rep, err := Import(st, ex, Options{SourceTenant: "minh-an", DestTenant: "minh-an", Deny: testDeny()})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := st.GetChannelConfig("minh-an/telegram/minh-an-telegram")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Enabled || cfg.DMPolicy != "open" || cfg.AgentID != w.other.ID {
		t.Fatalf("existing channel changed: %+v", cfg)
	}
	if !hasDetail(rep, ReasonNativeExists) {
		t.Fatalf("drops %+v", rep.NotImported)
	}
	kept, err := st.GetCronJob(w.kept.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !kept.Enabled || kept.Message != "giữ nguyên" || kept.Spec != "every:2h" {
		t.Fatalf("existing cron changed: %+v", kept)
	}
}

func TestDuplicateSourceIDSkips(t *testing.T) {
	st := store.New()
	_ = seed(t, st)
	row := ChannelRow{ID: "ch-tg", TenantID: "minh-an", Name: "minh-an-telegram", ChannelType: "telegram", Enabled: boolPtr(true)}
	ex := &Export{ChannelInstances: []ChannelRow{row, row}}
	rep, err := Import(st, ex, Options{SourceTenant: "minh-an", DestTenant: "minh-an", Deny: testDeny()})
	if err != nil {
		t.Fatal(err)
	}
	minh := rep.Tenants[0]
	balance(t, minh.ChannelInstances)
	if minh.ChannelInstances.New != 1 || minh.ChannelInstances.Skipped != 1 || minh.ChannelInstances.Written != 1 {
		t.Fatalf("%+v", minh.ChannelInstances)
	}
	if _, err := st.GetChannelConfig("minh-an/telegram/minh-an-telegram"); err != nil {
		t.Fatal(err)
	}
}

func TestOtherTenantImportDoesNotTouchMinhAn(t *testing.T) {
	st := store.New()
	w := seed(t, st)
	ex := readFixture(t, "testdata/minh-an.json")
	if _, err := Import(st, ex, Options{SourceTenant: "minh-an", DestTenant: "minh-an", Deny: testDeny()}); err != nil {
		t.Fatal(err)
	}
	minhJobs := len(st.ListCronJobs())
	if _, err := Import(st, ex, Options{SourceTenant: "other-co", DestTenant: "other-co", Deny: testDeny()}); err != nil {
		t.Fatal(err)
	}
	cfg, err := st.GetChannelConfig("other-co/zalo-personal/other-telegram")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled || cfg.DMPolicy != "allowlist" {
		t.Fatalf("%+v", cfg)
	}
	if _, err := st.GetChannelConfig("minh-an/telegram/minh-an-telegram"); err != nil {
		t.Fatal(err)
	}
	kept, _ := st.GetCronJob(w.kept.ID)
	if kept.Message != "giữ nguyên" || !kept.Enabled {
		t.Fatalf("kept changed %+v", kept)
	}
	if len(st.ListCronJobs()) != minhJobs {
		t.Fatalf("cron count %d want %d", len(st.ListCronJobs()), minhJobs)
	}
}

func TestSQLiteFileOmitsSecret(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goso.db")
	st, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = seed(t, st)
	ex := readFixture(t, "testdata/minh-an.jsonl")
	rep, err := Import(st, ex, Options{SourceTenant: "minh-an", DestTenant: "minh-an", Deny: testDeny()})
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte(secretMarker)) {
		t.Fatal("sqlite file contains the credential")
	}
	if bytes.Contains([]byte(mustJSON(t, rep)), []byte(secretMarker)) {
		t.Fatal("report contains the credential")
	}
}

type seeded struct {
	sales *store.Agent
	other *store.Agent
	sess  *store.Session
	kept  *store.CronJob
}

func seed(t *testing.T, st store.StoreIface) seeded {
	t.Helper()
	sales, err := st.CreateAgent(store.Agent{AgentKey: "tu-van-sales", DisplayName: "Cố vấn Minh An", TenantID: "minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateAgent(store.Agent{AgentKey: "other-co-sales", DisplayName: "Cố vấn khác", TenantID: "other-co"})
	if err != nil {
		t.Fatal(err)
	}
	sess, err := st.CreateSession(store.Session{AgentID: sales.ID, TenantID: "minh-an", Label: "lich"})
	if err != nil {
		t.Fatal(err)
	}
	otherSess, err := st.CreateSession(store.Session{AgentID: other.ID, TenantID: "other-co", Label: "khac"})
	if err != nil {
		t.Fatal(err)
	}
	kept, err := st.CreateCronJob(store.CronJob{Spec: "every:2h", SessionID: otherSess.ID, Message: "giữ nguyên", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutChannelConfig(store.ChannelConfig{Name: "telegram", Enabled: true, AgentID: other.ID, DMPolicy: "open"}); err != nil {
		t.Fatal(err)
	}
	return seeded{sales: sales, other: other, sess: sess, kept: kept}
}

func assertImportedDisabled(t *testing.T, st store.StoreIface, w seeded) {
	t.Helper()
	for _, j := range st.ListCronJobs() {
		if j.ID != w.kept.ID {
			t.Fatalf("imported cron %+v", j)
		}
	}
	cfg, err := st.GetChannelConfig("minh-an/telegram/minh-an-telegram")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled || cfg.AgentID != w.sales.ID || cfg.DMPolicy != "pairing" || cfg.GroupPolicy != "allowlist" || !cfg.RequireMention {
		t.Fatalf("%+v", cfg)
	}
	if len(cfg.AllowFrom) != 1 || cfg.AllowFrom[0] != "minh-an-user" {
		t.Fatalf("allow %+v", cfg.AllowFrom)
	}
	if _, err := st.GetChannelConfig("minh-an/zcrm/minh-an-zcrm"); err == nil {
		t.Fatal("zcrm was imported")
	}
}

func assertUntouched(t *testing.T, st store.StoreIface, w seeded) {
	t.Helper()
	kept, err := st.GetCronJob(w.kept.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !kept.Enabled || kept.Message != "giữ nguyên" || kept.Spec != "every:2h" {
		t.Fatalf("kept %+v", kept)
	}
	cat, err := st.GetChannelConfig("telegram")
	if err != nil {
		t.Fatal(err)
	}
	if !cat.Enabled || cat.DMPolicy != "open" || cat.AgentID != w.other.ID {
		t.Fatalf("catalog changed %+v", cat)
	}
	if _, err := st.GetChannelConfig("other-co/zalo-personal/other-telegram"); err == nil {
		t.Fatal("held channel was written")
	}
	n := 0
	for _, s := range st.ListSessions() {
		if s.TenantID == "other-co" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("other sessions %d", n)
	}
}

func assertNeedsCredential(t *testing.T, rep *Report) {
	t.Helper()
	if len(rep.NeedsCredential) != 3 {
		t.Fatalf("notes %d", len(rep.NeedsCredential))
	}
	for _, n := range rep.NeedsCredential {
		if !n.NeedsCredential || n.SourceID == "" || n.Name == "" {
			t.Fatalf("%+v", n)
		}
	}
	if _, ok := rep.UnmappedColumns["cron_jobs"]; !ok {
		t.Fatal("missing cron unmapped columns")
	}
	if _, ok := rep.UnmappedColumns["channel_instances"]; !ok {
		t.Fatal("missing channel unmapped columns")
	}
	foundCred := false
	for _, col := range rep.UnmappedColumns["channel_instances"] {
		if col == "credentials" {
			foundCred = true
		}
	}
	if !foundCred {
		t.Fatal("credentials not listed")
	}
}

func tenantCounts(t *testing.T, rep *Report) (TenantCount, TenantCount) {
	t.Helper()
	var minh, other TenantCount
	var sawMinh, sawOther bool
	for _, row := range rep.Tenants {
		balance(t, row.CronJobs)
		balance(t, row.ChannelInstances)
		switch row.TenantID {
		case "minh-an":
			minh = row
			sawMinh = true
			if !row.Selected {
				t.Fatal("minh-an not selected")
			}
		case "other-co":
			other = row
			sawOther = true
			if row.Selected {
				t.Fatal("other-co selected")
			}
		}
	}
	if !sawMinh || !sawOther {
		t.Fatalf("tenants %+v", rep.Tenants)
	}
	return minh, other
}

func balance(t *testing.T, c RowCounts) {
	t.Helper()
	if c.Input != c.New+c.Skipped+c.NotImported+c.Held+c.NativeExists {
		t.Fatalf("unbalanced %+v", c)
	}
}

func hasDetail(rep *Report, detail string) bool {
	for _, d := range rep.NotImported {
		if d.Detail == detail {
			return true
		}
	}
	return false
}

func assertNoSecret(t *testing.T, text string) {
	t.Helper()
	if strings.Contains(text, secretMarker) {
		t.Fatal("secret marker leaked")
	}
}

func snapshot(t *testing.T, st store.StoreIface) string {
	t.Helper()
	var lines []string
	for _, j := range st.ListCronJobs() {
		on := "off"
		if j.Enabled {
			on = "on"
		}
		lines = append(lines, "cron|"+j.ID+"|"+j.Spec+"|"+j.Message+"|"+j.SessionID+"|"+on)
	}
	for _, c := range st.ListChannelConfigs() {
		on := "off"
		if c.Enabled {
			on = "on"
		}
		lines = append(lines, "ch|"+c.Name+"|"+c.AgentID+"|"+c.DMPolicy+"|"+on)
	}
	for _, s := range st.ListSessions() {
		lines = append(lines, "sess|"+s.ID+"|"+s.TenantID)
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func readFixture(t *testing.T, path string) *Export {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ex, err := ReadExport(f)
	if err != nil {
		t.Fatal(err)
	}
	return ex
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func testDeny() map[string]struct{} {
	return map[string]struct{}{"blocked-tenant": {}}
}

func TestDeniedTenantWritesNothing(t *testing.T) {
	st := store.New()
	w := seed(t, st)
	before := snapshot(t, st)
	ex := readFixture(t, "testdata/minh-an.json")
	deny := map[string]struct{}{"blocked-tenant": {}}
	_, err := Import(st, ex, Options{SourceTenant: "blocked-tenant", DestTenant: "minh-an", Deny: deny})
	var denied *DenyError
	if !errors.As(err, &denied) || denied.Count != 1 {
		t.Fatalf("err %v", err)
	}
	if strings.Contains(err.Error(), "blocked-tenant") {
		t.Fatal("error includes the tenant id")
	}
	_, err = Import(st, ex, Options{SourceTenant: "minh-an", DestTenant: "blocked-tenant", Deny: deny})
	if !errors.As(err, &denied) {
		t.Fatalf("dest err %v", err)
	}
	if snapshot(t, st) != before {
		t.Fatal("denied tenant wrote rows")
	}
	kept, _ := st.GetCronJob(w.kept.ID)
	if kept.Message != "giữ nguyên" {
		t.Fatal("native cron changed")
	}
}

func TestDeniedNameStaysOutOfReport(t *testing.T) {
	st := store.New()
	_ = seed(t, st)
	phone := "0901111222"
	zalo := "998877665544"
	ex := &Export{
		CronJobs: []CronRow{{ID: phone, TenantID: "blocked-tenant", UserID: phone}},
		ChannelInstances: []ChannelRow{{
			ID: zalo, TenantID: "blocked-tenant", Name: phone, ChannelType: "telegram",
			Credentials: json.RawMessage(`"` + phone + `"`),
		}},
	}
	logBuf := &bytes.Buffer{}
	rep, err := Import(st, ex, Options{
		SourceTenant: "minh-an", DestTenant: "minh-an", Deny: testDeny(), Log: bufLog{logBuf},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.DeniedTenants != 1 {
		t.Fatalf("denied count %d", rep.DeniedTenants)
	}
	text := mustJSON(t, rep) + logBuf.String()
	if strings.Contains(text, "blocked-tenant") || strings.Contains(text, phone) || strings.Contains(text, zalo) {
		t.Fatal("report or log printed a denied name or customer id")
	}
	if _, err := st.GetChannelConfig("minh-an/telegram/" + phone); err == nil {
		t.Fatal("denied channel was written")
	}
}

func TestRollbackKeepsNativeAndOtherTenant(t *testing.T) {
	st := store.New()
	w := seed(t, st)
	ex := readFixture(t, "testdata/minh-an.json")
	if _, err := Import(st, ex, Options{SourceTenant: "minh-an", DestTenant: "minh-an", Deny: testDeny()}); err != nil {
		t.Fatal(err)
	}
	if _, err := Import(st, ex, Options{SourceTenant: "other-co", DestTenant: "other-co", Deny: testDeny()}); err != nil {
		t.Fatal(err)
	}
	native, err := st.GetChannelConfig("telegram")
	if err != nil || !native.Enabled {
		t.Fatalf("native %+v %v", native, err)
	}
	dry, err := Rollback(st, "minh-an", testDeny(), true)
	if err != nil || !dry.DryRun || dry.Mappings != 1 {
		t.Fatalf("dry %+v %v", dry, err)
	}
	if rows, err := st.ListImportSources("minh-an"); err != nil || len(rows) != 1 {
		t.Fatalf("dry deleted mappings %v %v", rows, err)
	}
	applied, err := Rollback(st, "minh-an", testDeny(), false)
	if err != nil || applied.DryRun || applied.Mappings != 1 {
		t.Fatalf("apply %+v %v", applied, err)
	}
	if rows, err := st.ListImportSources("minh-an"); err != nil || len(rows) != 0 {
		t.Fatalf("minh-an mappings %v %v", rows, err)
	}
	if rows, err := st.ListImportSources("other-co"); err != nil || len(rows) != 1 {
		t.Fatalf("other mappings %v %v", rows, err)
	}
	got, err := st.GetChannelConfig("telegram")
	if err != nil || !got.Enabled || got.AgentID != w.other.ID || got.DMPolicy != "open" {
		t.Fatalf("native changed %+v %v", got, err)
	}
	if _, err := st.GetChannelConfig("minh-an/telegram/minh-an-telegram"); err != nil {
		t.Fatal("rollback deleted a channel row")
	}
	if _, err := st.GetChannelConfig("other-co/zalo-personal/other-telegram"); err != nil {
		t.Fatal("rollback deleted the other tenant channel")
	}
	kept, _ := st.GetCronJob(w.kept.ID)
	if !kept.Enabled || kept.Message != "giữ nguyên" {
		t.Fatalf("native cron %+v", kept)
	}
}

func boolPtr(v bool) *bool { return &v }

func TestPostgresChannelImport(t *testing.T) {
	st := openPostgres(t)
	if st == nil {
		return
	}
	_ = seed(t, st)
	ex := readFixture(t, "testdata/minh-an.json")
	rep, err := Import(st, ex, Options{SourceTenant: "minh-an", DestTenant: "minh-an", Deny: testDeny()})
	if err != nil {
		t.Fatal(err)
	}
	minh, _ := tenantCounts(t, rep)
	if minh.CronJobs.Written != 0 || minh.CronJobs.NotImported != 4 || minh.ChannelInstances.Written != 1 {
		t.Fatalf("cron %+v channel %+v", minh.CronJobs, minh.ChannelInstances)
	}
	cfg, err := st.GetChannelConfig("minh-an/telegram/minh-an-telegram")
	if err != nil || cfg.Enabled {
		t.Fatalf("%+v %v", cfg, err)
	}
	again, err := Import(st, ex, Options{SourceTenant: "minh-an", DestTenant: "minh-an", Deny: testDeny()})
	if err != nil {
		t.Fatal(err)
	}
	minh, _ = tenantCounts(t, again)
	if minh.ChannelInstances.Skipped != 1 || minh.ChannelInstances.Written != 0 {
		t.Fatalf("rerun %+v", minh.ChannelInstances)
	}
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
		t.Skip("postgres DSN is not set")
		return nil
	}
	schema := "sched177"
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

type bufLog struct{ b *bytes.Buffer }

func (l bufLog) Printf(format string, args ...any) {
	fmt.Fprintf(l.b, format+"\n", args...)
}
