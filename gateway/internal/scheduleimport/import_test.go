// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package scheduleimport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/cron"
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
			opt := Options{SourceTenant: "minh-an", DestTenant: "minh-an", DryRun: true, Log: bufLog{logBuf}}
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
			if minh.CronJobs.Input != 4 || minh.CronJobs.New != 2 || minh.CronJobs.NotImported != 2 || minh.CronJobs.Written != 0 {
				t.Fatalf("dry cron %+v", minh.CronJobs)
			}
			if minh.ChannelInstances.Input != 2 || minh.ChannelInstances.New != 1 || minh.ChannelInstances.NotImported != 1 || minh.ChannelInstances.Written != 0 {
				t.Fatalf("dry channel %+v", minh.ChannelInstances)
			}
			if other.CronJobs.Held != 1 || other.CronJobs.Written != 0 || other.ChannelInstances.Held != 1 || other.ChannelInstances.Written != 0 {
				t.Fatalf("other cron %+v channel %+v", other.CronJobs, other.ChannelInstances)
			}
			assertNeedsCredential(t, rep)
			assertNoSecret(t, logBuf.String()+mustJSON(t, rep))
			if !hasDetail(rep, "schedule-unsupported") || !hasDetail(rep, "agent-other-tenant") || !hasDetail(rep, "unknown-channel") {
				t.Fatalf("drops %+v", rep.NotImported)
			}
			for _, d := range rep.NotImported {
				if d.Reason != ReasonNeedsDecision {
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
			if minh.CronJobs.New != 2 || minh.CronJobs.Written != 2 || minh.ChannelInstances.New != 1 || minh.ChannelInstances.Written != 1 {
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
			if minh.CronJobs.New != 0 || minh.CronJobs.Skipped != 2 || minh.CronJobs.Written != 0 {
				t.Fatalf("rerun cron %+v", minh.CronJobs)
			}
			if minh.ChannelInstances.New != 0 || minh.ChannelInstances.Skipped != 1 || minh.ChannelInstances.Written != 0 {
				t.Fatalf("rerun channel %+v", minh.ChannelInstances)
			}
			assertImportedDisabled(t, st, w)
			if len(st.ListCronJobs()) != 3 {
				t.Fatalf("cron count %d", len(st.ListCronJobs()))
			}
		})
	}
}

func TestImportedChannelsAndCronsStayDisabled(t *testing.T) {
	st := store.New()
	_ = seed(t, st)
	ex := readFixture(t, "testdata/minh-an.json")
	if _, err := Import(st, ex, Options{SourceTenant: "minh-an", DestTenant: "minh-an"}); err != nil {
		t.Fatal(err)
	}
	fired := 0
	cron.Tick(context.Background(), st, time.Now().UTC().Add(48*time.Hour), func(_ context.Context, sessionID, message string) error {
		fired++
		for _, j := range st.ListCronJobs() {
			if j.SessionID == sessionID && strings.Contains(message, "Minh An") {
				t.Errorf("imported job fired: %s", j.ID)
			}
		}
		return nil
	})
	for _, j := range st.ListCronJobs() {
		if strings.Contains(j.Message, "Minh An") && j.Enabled {
			t.Fatalf("imported cron enabled: %+v", j)
		}
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
	_ = fired
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
	rep, err := Import(st, ex, Options{SourceTenant: "minh-an", DestTenant: "minh-an"})
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
	if !hasDetail(rep, "name-taken") {
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
	ex := &Export{CronJobs: []CronRow{
		{ID: "cron-hour", TenantID: "minh-an", Enabled: boolPtr(true), ScheduleKind: "interval", IntervalMS: int64Ptr(3600000), Payload: json.RawMessage(`{"text":"Nhắc Minh An uống nước."}`)},
		{ID: "cron-hour", TenantID: "minh-an", Enabled: boolPtr(true), ScheduleKind: "every", IntervalMS: int64Ptr(7200000), Payload: json.RawMessage(`{"text":"Nhắc Minh An lần hai."}`)},
	}}
	rep, err := Import(st, ex, Options{SourceTenant: "minh-an", DestTenant: "minh-an"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Tenants) != 1 {
		t.Fatalf("tenants %+v", rep.Tenants)
	}
	minh := rep.Tenants[0]
	balance(t, minh.CronJobs)
	if minh.CronJobs.New != 1 || minh.CronJobs.Skipped != 1 || minh.CronJobs.Written != 1 {
		t.Fatalf("%+v", minh.CronJobs)
	}
	n := 0
	for _, j := range st.ListCronJobs() {
		if strings.Contains(j.Message, "uống nước") {
			n++
			if j.Spec != "every:1h" || j.Enabled {
				t.Fatalf("%+v", j)
			}
		}
		if strings.Contains(j.Message, "lần hai") {
			t.Fatal("duplicate inserted")
		}
	}
	if n != 1 {
		t.Fatalf("jobs %d", n)
	}
}

func TestOtherTenantImportDoesNotTouchMinhAn(t *testing.T) {
	st := store.New()
	w := seed(t, st)
	ex := readFixture(t, "testdata/minh-an.json")
	if _, err := Import(st, ex, Options{SourceTenant: "minh-an", DestTenant: "minh-an"}); err != nil {
		t.Fatal(err)
	}
	minhJobs := len(st.ListCronJobs())
	if _, err := Import(st, ex, Options{SourceTenant: "other-co", DestTenant: "other-co"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := st.GetChannelConfig("other-co/zalo-personal/other-telegram")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled || cfg.DMPolicy != "allowlist" {
		t.Fatalf("%+v", cfg)
	}
	found := false
	for _, j := range st.ListCronJobs() {
		if j.Message == "Việc của tenant khác." {
			found = true
			if j.Enabled {
				t.Fatal("other cron enabled")
			}
			sess, err := st.GetSession(j.SessionID)
			if err != nil || sess.TenantID != "other-co" {
				t.Fatalf("session %+v %v", sess, err)
			}
		}
	}
	if !found {
		t.Fatal("other cron missing")
	}
	if _, err := st.GetChannelConfig("minh-an/telegram/minh-an-telegram"); err != nil {
		t.Fatal(err)
	}
	kept, _ := st.GetCronJob(w.kept.ID)
	if kept.Message != "giữ nguyên" || !kept.Enabled {
		t.Fatalf("kept changed %+v", kept)
	}
	if len(st.ListCronJobs()) != minhJobs+1 {
		t.Fatalf("count %d want %d", len(st.ListCronJobs()), minhJobs+1)
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
	rep, err := Import(st, ex, Options{SourceTenant: "minh-an", DestTenant: "minh-an"})
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
	var week, hour bool
	for _, j := range st.ListCronJobs() {
		if j.ID == w.kept.ID {
			continue
		}
		if j.Enabled {
			t.Fatalf("enabled job %+v", j)
		}
		sess, err := st.GetSession(j.SessionID)
		if err != nil || sess.TenantID != "minh-an" {
			t.Fatalf("job tenant %+v %v", sess, err)
		}
		switch j.Message {
		case "Nhắc Minh An xem lịch tuần.":
			week = true
			if j.Spec != "0 9 * * 1" || j.SessionID != w.sess.ID {
				t.Fatalf("%+v", j)
			}
		case "Nhắc Minh An uống nước.":
			hour = true
			if j.Spec != "every:1h" {
				t.Fatalf("%+v", j)
			}
		default:
			t.Fatalf("unexpected job %+v", j)
		}
	}
	if !week || !hour {
		t.Fatal("missing imported cron")
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
	if c.Input != c.New+c.Skipped+c.NotImported+c.Held {
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

func boolPtr(v bool) *bool { return &v }

func int64Ptr(v int64) *int64 { return &v }

type bufLog struct{ b *bytes.Buffer }

func (l bufLog) Printf(format string, args ...any) {
	fmt.Fprintf(l.b, format+"\n", args...)
}
