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

func TestRawDestKeepsSenderAndThread(t *testing.T) {
	got := rawDests(SharedSender, "")
	if len(got) != 1 || got[0] != SharedSender {
		t.Fatalf("%v", got)
	}
	both := rawDests(SharedSender, SharedThread)
	if len(both) != 2 || both[0] != SharedSender || both[1] != SharedThread {
		t.Fatalf("%v", both)
	}
	ch, ok := gosoChannel("zalo_personal")
	if !ok || ch != "zalo-personal" {
		t.Fatalf("rename %q %v", ch, ok)
	}
	if _, ok := gosoChannel("zcrm"); ok {
		t.Fatal("unknown channel accepted")
	}
	key, ok := customerKey("telegram", "4242", "")
	if !ok || key != "telegram:4242" {
		t.Fatalf("key %q", key)
	}
	key, ok = customerKey("telegram", "", "user-1")
	if !ok || key != "goclaw:user-1" {
		t.Fatalf("fallback %q", key)
	}
	if _, ok := customerKey("", "", ""); ok {
		t.Fatal("empty customer key")
	}
}

func denySet() map[string]struct{} {
	return map[string]struct{}{"blocked-tenant": {}}
}

func minhOpts(t *testing.T, dry bool) Options {
	t.Helper()
	return Options{
		Tenant: "minh-an", SourceTenant: "minh-an", Deny: denySet(), DryRun: dry,
		CustomerMap: filepath.Join(t.TempDir(), "customers.tsv"),
	}
}

func TestFixtureFileMatchesGenerator(t *testing.T) {
	if FixtureLive < 201 || FixtureRows != FixtureLive+FixtureMerged {
		t.Fatalf("counts live %d rows %d", FixtureLive, FixtureRows)
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
	if len(file.Contacts) != FixtureRows {
		t.Fatalf("rows %d", len(file.Contacts))
	}
	shared, threads, merged, groups := 0, 0, 0, 0
	for i, c := range file.Contacts {
		if c.TenantID != "minh-an" || !strings.HasPrefix(c.DisplayName, "Minh An") {
			t.Fatalf("row %d identity", i)
		}
		if c.SenderID == SharedSender && c.ThreadID == "" {
			shared++
		}
		if c.ThreadID != "" {
			threads++
		}
		if c.MergedID != "" {
			merged++
			if c.MergedID == c.ID {
				t.Fatal("merged row points at itself")
			}
		}
		if c.ContactType == "group" {
			groups++
		}
	}
	if shared != 1 || threads != FixtureThreads || merged != 1 || groups != 1 {
		t.Fatalf("shared %d threads %d merged %d groups %d", shared, threads, merged, groups)
	}
}

func TestReadExportShape(t *testing.T) {
	if _, err := ReadExport(strings.NewReader(`{"contacts":[]}`)); err != nil {
		t.Fatal(err)
	}
	old, err := ReadExport(strings.NewReader(`{"contacts":[]}`))
	if err != nil || old == nil || len(old.rows) != 1 {
		t.Fatal("old contacts key was treated as the channel_contacts wrapper")
	}
	ex, err := ReadExport(strings.NewReader(`{"channel_contacts":null}`))
	if err != nil || ex == nil || len(ex.rows) != 0 {
		t.Fatalf("null %+v %v", ex, err)
	}
	if _, err := ReadExport(strings.NewReader("{\"channel_contacts\":[]}\n{\"channel_contacts\":[]}")); err == nil {
		t.Fatal("trailing value accepted")
	}
	body := "{\"tenant_id\":\"minh-an\",\"channel_type\":\"telegram\",\"sender_id\":\"ext-minh-an-j1\",\"display_name\":\"Minh An J\",\"contact_type\":\"user\",\"first_seen_at\":\"2026-01-15T08:00:00Z\",\"last_seen_at\":\"2026-01-15T09:00:00Z\"}\n\n{\"channel_type\":1}\n"
	ex, err = ReadExport(strings.NewReader(body))
	if err != nil || len(ex.rows) != 2 {
		t.Fatalf("jsonl %+v %v", ex, err)
	}
	prev := maxExportBytes
	maxExportBytes = 8
	t.Cleanup(func() { maxExportBytes = prev })
	if _, err := ReadExport(strings.NewReader(`{"channel_contacts":[]}`)); !errors.Is(err, ErrExportTooLarge) {
		t.Fatalf("too large %v", err)
	}
}

func TestImportMinhAnSQLite(t *testing.T) {
	s := openSQLite(t)
	exerciseMinhAn(t, s)
}

func TestImportMinhAnPostgres(t *testing.T) {
	s := openImportPostgres(t, "contactsimport172")
	if s == nil {
		return
	}
	exerciseMinhAn(t, s)
}

func TestBatch3300SQLite(t *testing.T) {
	s := openSQLite(t)
	exerciseBatch(t, s, 3300)
}

func TestBatch3300Postgres(t *testing.T) {
	s := openImportPostgres(t, "contactsimport172b")
	if s == nil {
		return
	}
	exerciseBatch(t, s, 3300)
}

func exerciseMinhAn(t *testing.T, s *store.SQLiteStore) {
	t.Helper()
	assertSideTables(t, s)
	if _, err := s.BulkUpsertContacts([]store.ContactSeed{{
		TenantID: "tenant-bee", Channel: "telegram", Dest: "ext-bee-keep",
		Display: "Bee Keep", Kind: "user", AgentID: "bee-agent", Count: 3,
		FirstSeen: mustStamp(t, "2026-01-15T08:00:00Z"), LastSeen: mustStamp(t, "2026-01-15T09:00:00Z"),
	}}); err != nil {
		t.Fatal(err)
	}
	ex, err := ReadExport(bytes.NewReader(FixtureJSON()))
	if err != nil {
		t.Fatal(err)
	}
	spy := &spyStore{SQLiteStore: s}
	rep, err := Import(spy, ex, minhOpts(t, true))
	if err != nil {
		t.Fatal(err)
	}
	if rep.New != FixtureLive || rep.Updated != 0 || rep.Skipped != FixtureMerged || rep.Invalid != 0 || !rep.DryRun {
		t.Fatalf("dry-run %+v", rep)
	}
	assertReasons(t, rep, map[string]int{"merged": 1})
	assertNoLeak(t, rep)
	if spy.calls != 0 || spy.lists != 1 {
		t.Fatalf("dry-run calls %d lists %d", spy.calls, spy.lists)
	}
	if n := mustCount(t, s, "minh-an"); n != 0 {
		t.Fatalf("dry-run wrote %d", n)
	}

	rep, err = Import(spy, ex, minhOpts(t, false))
	if err != nil {
		t.Fatal(err)
	}
	if rep.New != FixtureLive || rep.Skipped != FixtureMerged || rep.Invalid != 0 || rep.DryRun {
		t.Fatalf("apply %+v", rep)
	}
	if spy.calls != 1 || spy.last != storedCount(MinhAnFile()) || spy.lists != 3 {
		t.Fatalf("upsert calls %d last %d lists %d", spy.calls, spy.last, spy.lists)
	}
	if n := mustCount(t, s, "minh-an"); n != storedCount(MinhAnFile()) {
		t.Fatalf("count %d", n)
	}
	rows, err := s.ListContacts("minh-an", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	byDest := map[string]store.ContactRow{}
	groups := 0
	for _, row := range rows {
		if row.TenantID != "minh-an" {
			t.Fatalf("tenant %s", row.TenantID)
		}
		byDest[row.Dest] = row
		if row.Kind == "group" {
			groups++
		}
		if row.Count != 1 {
			t.Fatalf("count %d", row.Count)
		}
	}
	person := SharedSender
	thread := SharedThread
	if _, ok := byDest[person]; !ok {
		t.Fatal("missing person")
	}
	if _, ok := byDest[thread]; !ok {
		t.Fatal("thread collapsed or missing")
	}
	if person == thread {
		t.Fatal("same dest")
	}
	if _, ok := byDest[MergedSender]; ok {
		t.Fatal("merged row was inserted")
	}
	if groups != 2 {
		t.Fatalf("groups %d", groups)
	}
	wantFirst := mustStamp(t, "2026-01-15T08:00:00Z")
	if !byDest[person].FirstSeen.Equal(wantFirst) || !byDest[person].LastSeen.Equal(wantFirst.Add(time.Hour)) {
		t.Fatalf("times %s %s", byDest[person].FirstSeen, byDest[person].LastSeen)
	}
	if n := mustCount(t, s, "tenant-bee"); n != 1 {
		t.Fatalf("bee %d", n)
	}
	bee, err := s.ListContacts("tenant-bee", "", "", "")
	if err != nil || len(bee) != 1 || bee[0].Display != "Bee Keep" {
		t.Fatalf("bee %#v %v", bee, err)
	}
	if _, err := s.GetContact(bee[0].ID, "minh-an"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross get %v", err)
	}
	assertSideTables(t, s)

	rep, err = Import(spy, ex, minhOpts(t, false))
	if err != nil {
		t.Fatal(err)
	}
	if rep.New != 0 || rep.Updated != 0 || rep.Skipped != FixtureRows || rep.Invalid != 0 || spy.calls != 1 {
		t.Fatalf("rerun %+v calls %d", rep, spy.calls)
	}
	if n := mustCount(t, s, "minh-an"); n != storedCount(MinhAnFile()) {
		t.Fatalf("rerun count %d", n)
	}

	// An observed count and agent stay when the file has no seen_count.
	bumped := byDest[person]
	if _, err := s.BulkUpsertContacts([]store.ContactSeed{{
		TenantID: "minh-an", Channel: bumped.Channel, Dest: bumped.Dest, Display: bumped.Display,
		Kind: bumped.Kind, AgentID: "minh-an-sales", Count: 7,
		FirstSeen: bumped.FirstSeen, LastSeen: bumped.LastSeen,
	}}); err != nil {
		t.Fatal(err)
	}
	calls := spy.calls
	rep, err = Import(spy, ex, minhOpts(t, false))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Updated != 0 || rep.New != 0 || spy.calls != calls {
		t.Fatalf("preserve %+v calls %d", rep, spy.calls)
	}
	kept, err := s.ListContacts("minh-an", SharedSender, "", "")
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, row := range kept {
		if row.Dest == person {
			found++
			if row.Count != 7 || row.AgentID != "minh-an-sales" {
				t.Fatalf("preserved %#v", row)
			}
		}
	}
	if found != 1 {
		t.Fatalf("person rows %d", found)
	}

	renamed := MinhAnFile().Contacts[0]
	renamed.DisplayName = "Minh An Renamed"
	renamed.LastSeenAt = "2026-01-15T10:00:00Z"
	one, err := ReadExport(bytes.NewReader(mustExport(t, File{Contacts: []Contact{renamed}})))
	if err != nil {
		t.Fatal(err)
	}
	rep, err = Import(spy, one, minhOpts(t, true))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Updated != 0 || rep.New != 0 || rep.Skipped != 1 || spy.calls != calls {
		t.Fatalf("rename dry-run %+v", rep)
	}
	assertReasons(t, rep, map[string]int{"native-exists": 1})
	assertNoLeak(t, rep)
	rep, err = Import(spy, one, minhOpts(t, false))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Updated != 0 || rep.New != 0 || rep.Skipped != 1 || spy.calls != calls {
		t.Fatalf("rename %+v calls %d", rep, spy.calls)
	}
	after, err := s.ListContacts("minh-an", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range after {
		if row.Dest != person {
			continue
		}
		if row.Display != "Minh An 0000" || row.Count != 7 || row.AgentID != "minh-an-sales" || row.ID != bumped.ID {
			t.Fatalf("native contact changed %#v", row)
		}
		if !row.FirstSeen.Equal(wantFirst) || !row.LastSeen.Equal(wantFirst.Add(time.Hour)) {
			t.Fatalf("native times %s %s", row.FirstSeen, row.LastSeen)
		}
	}
	if n := mustCount(t, s, "minh-an"); n != storedCount(MinhAnFile()) {
		t.Fatalf("after rename %d", n)
	}
	assertMarkerAbsent(t, s, "DROPME")
	assertSideTables(t, s)
}

func TestEdgesAndSourceTenantSQLite(t *testing.T) {
	s := openSQLite(t)
	exerciseEdges(t, s)
}

func TestEdgesAndSourceTenantPostgres(t *testing.T) {
	s := openImportPostgres(t, "contactsimport172e")
	if s == nil {
		return
	}
	exerciseEdges(t, s)
}

func exerciseEdges(t *testing.T, s *store.SQLiteStore) {
	t.Helper()
	rows := []any{
		edge("ext-inherit-0001", "Minh An Inherit"),
		func() map[string]any {
			m := edge("ext-minh-an-extra", "Minh An Extra")
			m["avatar_url"] = "https://example.invalid/DROPME-avatar"
			m["user_id"] = "DROPME-user"
			m["channel_instance"] = "DROPME-instance"
			m["metadata"] = map[string]any{"note": "DROPME-note"}
			m["username"] = "DROPME-username"
			return m
		}(),
		func() map[string]any {
			m := edge("ext-kind-group", "Minh An Peer")
			m["contact_type"] = "user"
			m["peer_kind"] = "group"
			return m
		}(),
		func() map[string]any {
			m := edge("ext-username-0001", "")
			m["username"] = "Minh An User"
			return m
		}(),
		func() map[string]any {
			m := edge("ext-blank-name-0001", "")
			delete(m, "display_name")
			return m
		}(),
		func() map[string]any {
			m := edge("ext-bot-0001", "Minh An Bot")
			m["contact_type"] = "bot"
			return m
		}(),
		func() map[string]any {
			m := edge("", "Minh An Empty")
			return m
		}(),
		edge(SharedSender, "Minh An Dup"),
		func() map[string]any {
			m := edge("ext-bee-foreign", "Bee Foreign")
			m["tenant_id"] = "tenant-bee"
			return m
		}(),
		func() map[string]any {
			m := edge("ext-control-0001", "Minh An\nBad")
			return m
		}(),
		func() map[string]any {
			m := edge(strings.Repeat("d", maxSenderLen+1), "Minh An Long")
			return m
		}(),
		func() map[string]any {
			m := edge("ext-time-0001", "Minh An Time")
			delete(m, "last_seen_at")
			return m
		}(),
		func() map[string]any {
			m := edge("ext-self-0001", "Minh An Self")
			m["id"] = "same-id"
			m["merged_id"] = "same-id"
			return m
		}(),
		json.RawMessage(`{"channel_type":1}`),
	}
	// The duplicate uses the fixture sender with an empty thread. Import the
	// fixture first so the second copy is a duplicate inside this file only
	// when the first copy is also in the file. Build the file to contain both.
	fileRows := []any{edge(SharedSender, "Minh An 0000")}
	fileRows = append(fileRows, rows...)
	raw := mustRaw(t, map[string]any{"channel_contacts": fileRows})
	ex, err := ReadExport(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Import(&spyStore{SQLiteStore: s}, ex, minhOpts(t, false))
	if err != nil {
		t.Fatal(err)
	}
	// valid: first shared sender, inherit, extra, peer group, username, blank = 6
	// invalid: bot, empty sender, duplicate, foreign, control, sender-length, time, merged-self, row = 9
	if rep.New != 6 || rep.Invalid != 9 || rep.Skipped != 0 {
		t.Fatalf("%+v", rep)
	}
	assertReasons(t, rep, map[string]int{
		"control":        1,
		"duplicate":      1,
		"empty-sender":   1,
		"foreign-tenant": 1,
		"kind":           1,
		"merged-self":    1,
		"row":            1,
		"sender-length":  1,
		"time":           1,
	})
	assertNoLeak(t, rep)
	blankDest := "ext-blank-name-0001"
	rowsOut, err := s.ListContacts("minh-an", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	var blank, peer store.ContactRow
	for _, row := range rowsOut {
		if strings.Contains(row.Display, "DROPME") || strings.Contains(row.Dest, "DROPME") || strings.Contains(row.AgentID, "DROPME") {
			t.Fatal("dropped field was stored")
		}
		if row.Dest == blankDest {
			blank = row
		}
		if row.Dest == "ext-kind-group" {
			peer = row
		}
	}
	if blank.Display != "telegram:"+blankDest {
		t.Fatalf("blank %#v", blank)
	}
	if peer.Kind != "group" {
		t.Fatalf("peer %#v", peer)
	}
	user := "ext-username-0001"
	for _, row := range rowsOut {
		if row.Dest == user && row.Display != "Minh An User" {
			t.Fatalf("username %#v", row)
		}
	}
	assertMarkerAbsent(t, s, "DROPME")
	if n := mustCount(t, s, "default"); n != 0 {
		t.Fatalf("default %d", n)
	}
	if n := mustCount(t, s, "tenant-bee"); n != 0 {
		t.Fatalf("bee %d", n)
	}

	const src = "11111111-1111-4111-8111-111111111111"
	const other = "22222222-2222-4222-8222-222222222222"
	srcRows := []any{
		func() map[string]any {
			m := edge("ext-src-0001", "Minh An Source")
			m["tenant_id"] = src
			return m
		}(),
		func() map[string]any {
			m := edge("ext-src-0002", "Minh An Other")
			m["tenant_id"] = other
			return m
		}(),
	}
	ex, err = ReadExport(bytes.NewReader(mustRaw(t, map[string]any{"channel_contacts": srcRows})))
	if err != nil {
		t.Fatal(err)
	}
	before := mustCount(t, s, "minh-an")
	rep, err = Import(&spyStore{SQLiteStore: s}, ex, Options{
		Tenant: "minh-an", SourceTenant: src, Deny: denySet(),
		CustomerMap: filepath.Join(t.TempDir(), "customers.tsv"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.New != 1 || rep.Invalid != 1 {
		t.Fatalf("source %+v", rep)
	}
	assertReasons(t, rep, map[string]int{"foreign-tenant": 1})
	if mustCount(t, s, "minh-an") != before+1 {
		t.Fatal("source row missing")
	}
	if mustCount(t, s, src) != 0 || mustCount(t, s, other) != 0 {
		t.Fatal("source uuid was stored as a tenant")
	}
	listed, err := s.ListContacts("minh-an", "ext-src-0002", "", "")
	if err != nil || len(listed) != 0 {
		t.Fatalf("other source visible %#v %v", listed, err)
	}
}

func TestObserveImportedSender(t *testing.T) {
	s := openSQLite(t)
	row := person("4242", "", "Minh An 4242", "user", "user", "", time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC))
	row.ChannelType = "zalo_personal"
	row.UserID = "user-4242"
	ex, err := ReadExport(bytes.NewReader(mustExport(t, File{Contacts: []Contact{row}})))
	if err != nil {
		t.Fatal(err)
	}
	mapPath := filepath.Join(t.TempDir(), "customers.tsv")
	opt := minhOpts(t, false)
	opt.CustomerMap = mapPath
	if _, err := Import(&spyStore{SQLiteStore: s}, ex, opt); err != nil {
		t.Fatal(err)
	}
	again := opt
	if _, err := Import(&spyStore{SQLiteStore: s}, ex, again); err != nil {
		t.Fatal(err)
	}
	rows, err := s.ListContacts("minh-an", "", "", "")
	if err != nil || len(rows) != 1 || rows[0].Channel != "zalo-personal" || rows[0].Dest != "4242" {
		t.Fatalf("%#v %v", rows, err)
	}
	got, err := s.ObserveContact(store.ContactSighting{
		Channel: "zalo-personal", Dest: "4242", Kind: "user", TenantID: "minh-an",
		At: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != rows[0].ID || got.Dest != "4242" {
		t.Fatalf("observe %#v imported %#v", got, rows[0])
	}
	if n := mustCount(t, s, "minh-an"); n != 1 {
		t.Fatalf("second contact %d", n)
	}
	first, err := os.ReadFile(mapPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Import(&spyStore{SQLiteStore: s}, ex, opt); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(mapPath)
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(mapPath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("map mode %v %v", info, err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("map changed %q %q", first, second)
	}
	text := string(first)
	if !strings.Contains(text, "zalo-personal\t4242\tuser-4242\tzalo-personal:4242\n") {
		t.Fatalf("map %q", text)
	}
	for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		parts := strings.Split(line, "\t")
		if len(parts) != 4 || parts[3] == "" {
			t.Fatalf("empty key %q", line)
		}
	}
}

func TestNativeSkipAndRollback(t *testing.T) {
	s := openSQLite(t)
	real := store.ContactSeed{
		TenantID: "minh-an", Channel: "telegram", Dest: "real-minh-an", Display: "Minh An Real",
		Kind: "user", Count: 4, AgentID: "kept-agent",
		FirstSeen: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		LastSeen:  time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	}
	if _, err := s.BulkUpsertContacts([]store.ContactSeed{real}); err != nil {
		t.Fatal(err)
	}
	fresh := person("new-minh-an", "", "Minh An New", "user", "user", "", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	clash := person("real-minh-an", "", "Minh An Replaced", "user", "user", "", time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC))
	ex, err := ReadExport(bytes.NewReader(mustExport(t, File{Contacts: []Contact{clash, fresh}})))
	if err != nil {
		t.Fatal(err)
	}
	rep, err := Import(&spyStore{SQLiteStore: s}, ex, minhOpts(t, false))
	if err != nil {
		t.Fatal(err)
	}
	if rep.New != 1 || rep.Updated != 0 || rep.Skipped != 1 {
		t.Fatalf("%+v", rep)
	}
	assertReasons(t, rep, map[string]int{"native-exists": 1})
	rows, err := s.ListContacts("minh-an", "", "", "")
	if err != nil || len(rows) != 2 {
		t.Fatalf("%#v %v", rows, err)
	}
	for _, row := range rows {
		if row.Dest == "real-minh-an" && (row.Display != "Minh An Real" || row.Count != 4 || row.AgentID != "kept-agent") {
			t.Fatalf("real changed %#v", row)
		}
	}
	rolled, err := s.RollbackContactImport("minh-an", true, true)
	if err != nil || rolled.Deleted != 1 || rolled.Active != 0 {
		t.Fatalf("rollback %+v %v", rolled, err)
	}
	left, err := s.ListContacts("minh-an", "", "", "")
	if err != nil || len(left) != 1 || left[0].Dest != "real-minh-an" || left[0].Display != "Minh An Real" {
		t.Fatalf("after rollback %#v %v", left, err)
	}
}

func TestRollbackActivitySQLite(t *testing.T) {
	s := openSQLite(t)
	exerciseRollback(t, s)
}

func TestRollbackActivityPostgres(t *testing.T) {
	s := openImportPostgres(t, "contactsimport172r")
	if s == nil {
		return
	}
	exerciseRollback(t, s)
}

func exerciseRollback(t *testing.T, s *store.SQLiteStore) {
	t.Helper()
	at := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	quiet := person("quiet-minh-an", "", "Minh An Quiet", "user", "user", "", at)
	active := person("active-minh-an", "", "Minh An Active", "user", "user", "", at)
	ex, err := ReadExport(bytes.NewReader(mustExport(t, File{Contacts: []Contact{quiet, active}})))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Import(&spyStore{SQLiteStore: s}, ex, minhOpts(t, false)); err != nil {
		t.Fatal(err)
	}
	bee := person("bee-keep", "", "Bee Keep", "user", "user", "", at)
	bee.TenantID = "tenant-bee"
	beeEx, err := ReadExport(bytes.NewReader(mustExport(t, File{Contacts: []Contact{bee}})))
	if err != nil {
		t.Fatal(err)
	}
	beeOpt := Options{
		Tenant: "tenant-bee", SourceTenant: "tenant-bee", Deny: denySet(),
		CustomerMap: filepath.Join(t.TempDir(), "bee.tsv"),
	}
	if _, err := Import(&spyStore{SQLiteStore: s}, beeEx, beeOpt); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ObserveContact(store.ContactSighting{
		Channel: "telegram", Dest: "active-minh-an", Kind: "user", TenantID: "minh-an",
		At: at.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	preview, err := s.RollbackContactImport("minh-an", false, false)
	if err != nil || preview.Deleted != 0 || preview.Candidates != 2 || preview.Active != 1 {
		t.Fatalf("preview %+v %v", preview, err)
	}
	if mustCount(t, s, "minh-an") != 2 || mustCount(t, s, "tenant-bee") != 1 {
		t.Fatal("preview wrote")
	}
	refused, err := s.RollbackContactImport("minh-an", true, false)
	if !errors.Is(err, store.ErrContactRollbackConfirm) || refused.Deleted != 0 || refused.Active != 1 {
		t.Fatalf("refused %+v %v", refused, err)
	}
	if strings.Contains(err.Error(), "active-minh-an") || strings.Contains(err.Error(), "Minh An") {
		t.Fatal("rollback error leaked a contact")
	}
	if mustCount(t, s, "minh-an") != 2 || mustCount(t, s, "tenant-bee") != 1 {
		t.Fatal("refused rollback deleted a row")
	}
	done, err := s.RollbackContactImport("minh-an", true, true)
	if err != nil || done.Deleted != 2 {
		t.Fatalf("confirm %+v %v", done, err)
	}
	if mustCount(t, s, "minh-an") != 0 || mustCount(t, s, "tenant-bee") != 1 {
		t.Fatal("rollback crossed tenants")
	}
	left, err := s.ListContacts("tenant-bee", "", "", "")
	if err != nil || len(left) != 1 || left[0].Dest != "bee-keep" || left[0].Display != "Bee Keep" {
		t.Fatalf("bee %#v %v", left, err)
	}
	pair := []Contact{
		person("merge-a", "", "Minh An A", "user", "user", "", at),
		person("merge-b", "", "Minh An B", "user", "user", "", at),
	}
	mergeEx, err := ReadExport(bytes.NewReader(mustExport(t, File{Contacts: pair})))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Import(&spyStore{SQLiteStore: s}, mergeEx, minhOpts(t, false)); err != nil {
		t.Fatal(err)
	}
	rows, err := s.ListContacts("minh-an", "", "", "")
	if err != nil || len(rows) != 2 {
		t.Fatalf("merge setup %#v %v", rows, err)
	}
	var target, source store.ContactRow
	for _, row := range rows {
		if row.Dest == "merge-a" {
			target = row
		}
		if row.Dest == "merge-b" {
			source = row
		}
	}
	if _, err := s.MergeContacts(target.ID, source.ID, "minh-an", source.ID); err != nil {
		t.Fatal(err)
	}
	refused, err = s.RollbackContactImport("minh-an", true, false)
	if !errors.Is(err, store.ErrContactRollbackConfirm) || refused.Deleted != 0 || refused.Active < 1 {
		t.Fatalf("merge refused %+v %v", refused, err)
	}
	if n := mustCount(t, s, "tenant-bee"); n != 1 {
		t.Fatalf("bee after merge rollback %d", n)
	}
	if _, err := s.RollbackContactImport("minh-an", true, true); err != nil {
		t.Fatal(err)
	}
	if mustCount(t, s, "minh-an") != 0 || mustCount(t, s, "tenant-bee") != 1 {
		t.Fatal("merge rollback crossed tenants")
	}
}

func exerciseBatch(t *testing.T, s *store.SQLiteStore, n int) {
	t.Helper()
	rows := make([]Contact, 0, n)
	base := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		rows = append(rows, person(fmt.Sprintf("ext-batch-%04d", i), "", fmt.Sprintf("Minh An Batch %04d", i), "user", "user", "", base))
	}
	ex, err := ReadExport(bytes.NewReader(mustExport(t, File{Contacts: rows})))
	if err != nil {
		t.Fatal(err)
	}
	spy := &spyStore{SQLiteStore: s}
	rep, err := Import(spy, ex, minhOpts(t, true))
	if err != nil {
		t.Fatal(err)
	}
	if rep.New != n || spy.calls != 0 || spy.lists != 1 {
		t.Fatalf("dry-run %+v calls %d lists %d", rep, spy.calls, spy.lists)
	}
	if mustCount(t, s, "minh-an") != 0 {
		t.Fatal("dry-run wrote")
	}
	rep, err = Import(spy, ex, minhOpts(t, false))
	if err != nil {
		t.Fatal(err)
	}
	if rep.New != n || spy.calls != 1 || spy.last != n || spy.lists != 3 {
		t.Fatalf("apply %+v calls %d last %d lists %d", rep, spy.calls, spy.last, spy.lists)
	}
	if mustCount(t, s, "minh-an") != n {
		t.Fatalf("count %d", mustCount(t, s, "minh-an"))
	}
	rep, err = Import(spy, ex, minhOpts(t, false))
	if err != nil {
		t.Fatal(err)
	}
	if rep.Skipped != n || rep.New != 0 || rep.Updated != 0 || spy.calls != 1 || spy.lists != 4 {
		t.Fatalf("rerun %+v calls %d lists %d", rep, spy.calls, spy.lists)
	}
	assertSideTables(t, s)
}

func edge(sender, display string) map[string]any {
	m := map[string]any{
		"tenant_id":     "minh-an",
		"channel_type":  "telegram",
		"sender_id":     sender,
		"display_name":  display,
		"contact_type":  "user",
		"peer_kind":     "user",
		"first_seen_at": "2026-01-15T08:00:00Z",
		"last_seen_at":  "2026-01-15T09:00:00Z",
	}
	return m
}

func storedCount(file File) int {
	type pair struct{ ch, dest string }
	seen := map[pair]struct{}{}
	for _, c := range file.Contacts {
		if strings.TrimSpace(c.MergedID) != "" {
			continue
		}
		ch, ok := gosoChannel(c.ChannelType)
		if !ok {
			continue
		}
		for _, dest := range rawDests(c.SenderID, c.ThreadID) {
			seen[pair{ch, dest}] = struct{}{}
		}
	}
	return len(seen)
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
	for _, m := range []string{"Minh An", "ext-", "grp-", "DROPME", "Bee", "telegram", "zalo", "minh-an", "bee-agent", "thread-"} {
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

func openSQLite(t *testing.T) *store.SQLiteStore {
	t.Helper()
	s, err := store.OpenSQLite(filepath.Join(t.TempDir(), "contacts.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

type spyStore struct {
	*store.SQLiteStore
	calls int
	last  int
	lists int
}

func (s *spyStore) ListContacts(tenant, q, channel, kind string) ([]store.ContactRow, error) {
	s.lists++
	return s.SQLiteStore.ListContacts(tenant, q, channel, kind)
}

func (s *spyStore) BulkUpsertContacts(seeds []store.ContactSeed) (store.ContactBulkResult, error) {
	s.calls++
	s.last = len(seeds)
	return s.SQLiteStore.BulkUpsertContacts(seeds)
}

func openImportPostgres(t *testing.T, schema string) *store.SQLiteStore {
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
