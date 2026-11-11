// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestContactsDurableFlagDefaultsOff(t *testing.T) {
	for _, v := range []string{"1", "true", "yes", "on", "TRUE"} {
		t.Run("on_"+v, func(t *testing.T) {
			t.Setenv("GOSO_CONTACTS_DURABLE", v)
			if !ContactsDurableEnabled() {
				t.Fatal("expected on")
			}
		})
	}
	for _, v := range []string{"", "0", "false", "off", "no", "NO"} {
		t.Run("off_"+v, func(t *testing.T) {
			t.Setenv("GOSO_CONTACTS_DURABLE", v)
			if ContactsDurableEnabled() {
				t.Fatal("expected off")
			}
		})
	}
}

func TestContactsFlagOffMatchesMemory(t *testing.T) {
	for _, v := range []string{"", "0", "false", "off", "no"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv("GOSO_CONTACTS_DURABLE", v)
			path := filepath.Join(t.TempDir(), "off.db")
			st, err := store.OpenSQLite(path)
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			bound := NewContacts()
			bound.BindStore(st)
			plain := NewContacts()
			boundJSON, boundN, boundOther := contactScript(t, bound)
			plainJSON, plainN, plainOther := contactScript(t, plain)
			if boundN != 200 || plainN != 200 {
				t.Fatalf("cap bound %d plain %d", boundN, plainN)
			}
			if boundJSON != plainJSON {
				t.Fatalf("flag-off list diverged\nbound %s\nplain %s", boundJSON, plainJSON)
			}
			if boundOther != 1 || plainOther != 1 {
				t.Fatalf("other tenant bound %d plain %d", boundOther, plainOther)
			}
			n, err := st.CountContacts("minh-an")
			if err != nil || n != 0 {
				t.Fatalf("flag off wrote %d %v", n, err)
			}
			if err := st.Close(); err != nil {
				t.Fatal(err)
			}
			re, err := store.OpenSQLite(path)
			if err != nil {
				t.Fatal(err)
			}
			defer re.Close()
			if n, err := re.CountContacts("minh-an"); err != nil || n != 0 {
				t.Fatalf("reopen wrote %d %v", n, err)
			}
		})
	}
}

func TestContactsFlagOnWithoutStoreKeepsCap(t *testing.T) {
	t.Setenv("GOSO_CONTACTS_DURABLE", "1")
	c := NewContacts()
	for i := 0; i < 201; i++ {
		c.Observe(Sighting{
			Channel: "telegram", Dest: fmt.Sprintf("ma-%04d", i), Kind: "user",
			TenantID: "minh-an", At: time.Unix(int64(1000+i), 0).UTC(),
		})
	}
	rows, err := c.List("minh-an", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(rows); n != 200 {
		t.Fatalf("unbound durable flag list %d", n)
	}
}

func TestContactsDurableSurvivesReopen(t *testing.T) {
	t.Setenv("GOSO_CONTACTS_DURABLE", "1")
	path := filepath.Join(t.TempDir(), "on.db")
	st, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	c := NewContacts()
	c.BindStore(st)
	const n = 205
	var firstID string
	for i := 0; i < n; i++ {
		row := c.Observe(Sighting{
			Channel: "telegram", Dest: fmt.Sprintf("ext-minh-an-%04d", i), Kind: "user",
			TenantID: "minh-an", AgentID: "ag-minh-an", At: time.Unix(int64(1000+i), 0).UTC(),
		})
		if i == 0 {
			firstID = row.ID
		}
		if row.ID == "" {
			t.Fatalf("empty id at %d", i)
		}
	}
	// A second sighting of the same external id increments; it does not insert.
	again := c.Observe(Sighting{
		Channel: "telegram", Dest: "ext-minh-an-0000", Kind: "user",
		TenantID: "minh-an", At: time.Unix(2000, 0).UTC(),
	})
	if again.ID != firstID || again.Count != 2 {
		t.Fatalf("increment %#v", again)
	}
	list, err := c.List("minh-an", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != n {
		t.Fatalf("list %d", len(list))
	}
	if list[len(list)-1].Dest != "ext-minh-an-0001" && !listHas(list, "ext-minh-an-0000") {
		t.Fatal("missing oldest")
	}
	if !listHas(list, "ext-minh-an-0000") || !listHas(list, fmt.Sprintf("ext-minh-an-%04d", n-1)) {
		t.Fatal("list dropped a row")
	}
	other := c.Observe(Sighting{Channel: "telegram", Dest: "ext-minh-an-0000", Kind: "user", TenantID: "other-co", At: time.Unix(5, 0).UTC()})
	listed, err := c.List("minh-an", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != n {
		t.Fatal("other tenant changed minh-an list")
	}
	if _, err := c.Get(other.ID, "minh-an"); !errors.Is(err, ErrContactNotFound) {
		t.Fatalf("cross get %v", err)
	}
	if _, err := c.Merge(firstID, other.ID, "minh-an", other.ID); !errors.Is(err, ErrContactNotFound) {
		t.Fatalf("cross merge %v", err)
	}

	group := c.Observe(Sighting{Channel: "telegram", Dest: "-42", Kind: "group", SenderID: "sender-minh-an", TenantID: "minh-an", At: time.Unix(8, 0).UTC()})
	if group.Dest != "-42" || group.Kind != "group" {
		t.Fatalf("group %#v", group)
	}
	users, err := c.List("minh-an", "sender-minh-an", "", "user")
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].Dest != "sender-minh-an" {
		t.Fatalf("sender split %#v", users)
	}

	a := c.Observe(Sighting{Channel: "discord", Dest: "ext-a", Kind: "user", TenantID: "minh-an", At: time.Unix(20, 0).UTC()})
	b := c.Observe(Sighting{Channel: "slack", Dest: "ext-b", Kind: "user", TenantID: "minh-an", At: time.Unix(21, 0).UTC()})
	merged, err := c.Merge(a.ID, b.ID, "minh-an", b.ID+">"+a.ID)
	if err != nil || len(merged.Identifiers) != 2 || !merged.CanUndo {
		t.Fatalf("merge %#v %v", merged, err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st, err = store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	re := NewContacts()
	re.BindStore(st)
	list, err = re.List("minh-an", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !listHas(list, "ext-minh-an-0000") || !listHas(list, fmt.Sprintf("ext-minh-an-%04d", n-1)) {
		t.Fatalf("reopen dropped rows %d", len(list))
	}
	got, err := re.Get(a.ID, "minh-an")
	if err != nil || len(got.Identifiers) != 2 || !got.CanUndo {
		t.Fatalf("reopen merge %#v %v", got, err)
	}
	if _, err := re.Get(b.ID, "minh-an"); !errors.Is(err, ErrContactNotFound) {
		t.Fatalf("merged source visible %v", err)
	}
	undone, err := re.Undo(a.ID, "minh-an", b.ID)
	if err != nil || undone.CanUndo {
		t.Fatalf("undo %#v %v", undone, err)
	}
	src, err := re.Get(b.ID, "minh-an")
	if err != nil || src.Dest != "ext-b" {
		t.Fatalf("restored %#v %v", src, err)
	}
	if nOther, err := st.CountContacts("other-co"); err != nil || nOther != 1 {
		t.Fatalf("other count %d %v", nOther, err)
	}
}

func contactScript(t *testing.T, c *Contacts) (string, int, int) {
	t.Helper()
	c.Observe(Sighting{Channel: "telegram", Dest: "-9", Kind: "group", SenderID: "42", TenantID: "minh-an", At: time.Unix(2, 0).UTC()})
	c.Observe(Sighting{Channel: "discord", Dest: "room", Kind: "user", TenantID: "minh-an", At: time.Unix(3, 0).UTC()})
	a := c.Observe(Sighting{Channel: "telegram", Dest: "111", Kind: "user", TenantID: "minh-an", At: time.Unix(10, 0).UTC()})
	b := c.Observe(Sighting{Channel: "discord", Dest: "222", Kind: "user", TenantID: "minh-an", At: time.Unix(11, 0).UTC()})
	if _, err := c.Merge(a.ID, b.ID, "minh-an", ""); !errors.Is(err, ErrContactConfirmRequired) {
		t.Fatalf("confirm %v", err)
	}
	merged, err := c.Merge(a.ID, b.ID, "minh-an", b.ID+">"+a.ID)
	if err != nil || len(merged.Identifiers) != 2 {
		t.Fatalf("merge %#v %v", merged, err)
	}
	if _, err := c.Undo(a.ID, "minh-an", b.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 201; i++ {
		c.Observe(Sighting{
			Channel: "telegram", Dest: fmt.Sprintf("ma-%04d", i), Kind: "user",
			TenantID: "minh-an", At: time.Unix(int64(1000+i), 0).UTC(),
		})
	}
	c.Observe(Sighting{Channel: "telegram", Dest: "ma-other", Kind: "user", TenantID: "other-co", At: time.Unix(50, 0).UTC()})
	list, err := c.List("minh-an", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	other, err := c.List("other-co", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw), len(list), len(other)
}

func TestContactsObserveFailureLogsWithoutPII(t *testing.T) {
	t.Setenv("GOSO_CONTACTS_DURABLE", "1")
	prevLog := contactFaultLogf
	prevCall := observeContactStore
	t.Cleanup(func() {
		contactFaultLogf = prevLog
		observeContactStore = prevCall
	})
	var lines []string
	contactFaultLogf = func(format string, args ...any) {
		lines = append(lines, fmt.Sprintf(format, args...))
	}
	path := filepath.Join(t.TempDir(), "obs.db")
	st, err := store.OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	c := NewContacts()
	c.BindStore(st)
	dest := "ext-minh-an-7f3a"
	sender := "0900000000"
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("panic %v", rec)
			}
		}()
		c.Observe(Sighting{
			Channel: "telegram", Dest: dest, Kind: "user", SenderID: sender,
			TenantID: "minh-an", AgentID: "ag-minh-an", At: time.Unix(3, 0).UTC(),
		})
	}()
	if len(lines) == 0 {
		t.Fatal("observe failure was not logged")
	}
	assertContactFaultLog(t, lines, "observe")

	lines = nil
	observeContactStore = func(db *store.SQLiteStore, in store.ContactSighting) (store.ContactRow, error) {
		return store.ContactRow{}, fmt.Errorf("phone=0900000000 dest=%s name=Minh An id=ct_secret tenant=%s body=hello", in.Dest, in.TenantID)
	}
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("panic %v", rec)
			}
		}()
		c.Observe(Sighting{
			Channel: "telegram", Dest: dest, Kind: "group", SenderID: sender,
			TenantID: "minh-an", AgentID: "ag-minh-an", At: time.Unix(4, 0).UTC(),
		})
	}()
	if len(lines) == 0 {
		t.Fatal("injected observe failure was not logged")
	}
	joined := assertContactFaultLog(t, lines, "observe")
	for _, leak := range []string{"0900000000", dest, "Minh An", "ct_secret", "minh-an", "ag-minh-an", "hello", "telegram"} {
		if strings.Contains(joined, leak) {
			t.Fatalf("log contains %q in %q", leak, joined)
		}
	}
}

func assertContactFaultLog(t *testing.T, lines []string, op string) string {
	t.Helper()
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "goso contacts: "+op+" failed class=") {
		t.Fatalf("log %q", joined)
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "goso contacts: ") || !strings.Contains(line, " failed class=") {
			t.Fatalf("unexpected log %q", line)
		}
		class := line[strings.LastIndex(line, "class=")+len("class="):]
		switch class {
		case "closed", "conflict", "schema", "driver", "store":
		default:
			t.Fatalf("class %q in %q", class, line)
		}
	}
	return joined
}

func listHas(list []PublicContact, dest string) bool {
	for _, row := range list {
		if row.Dest == dest {
			return true
		}
		for _, id := range row.Identifiers {
			if id.Dest == dest {
				return true
			}
		}
	}
	return false
}
