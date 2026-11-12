// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mqglobal/goso/gateway/internal/store"
)

const spec154Query = "mã MA-WALNUT-77 làm bằng gỗ gì"

func TestRecallForOlderThanScanCap(t *testing.T) {
	t.Setenv("GOSO_MEMORY_RECALL", "1")
	t.Setenv("GOSO_DATABASE_URL", "")
	stores := map[string]store.StoreIface{"memory": store.New()}
	sq, err := store.OpenSQLite(filepath.Join(t.TempDir(), "cap.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sq.Close() })
	stores["sqlite"] = sq
	if dsn := spec154DSN(t, "goso_spec154_cap_for", false); dsn != "" {
		pg, err := store.OpenPostgres(dsn)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = pg.Close() })
		stores["postgres"] = pg
	} else if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
		t.Fatal("GOSO_TEST_POSTGRES_DSN is required in CI")
	}
	for name, st := range stores {
		t.Run(name, func(t *testing.T) {
			recallForOlderThanCap(t, st)
		})
	}
}

func TestRecallFor50kUnder50ms(t *testing.T) {
	t.Setenv("GOSO_MEMORY_RECALL", "1")
	t.Setenv("GOSO_DATABASE_URL", "")
	dsn := spec154DSN(t, "goso_spec154_50k_for", true)
	if dsn == "" {
		return
	}
	st, err := store.OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ag, err := st.CreateAgent(store.Agent{AgentKey: "minh-an-sales", DisplayName: "Cố vấn Minh An"})
	if err != nil {
		t.Fatal(err)
	}
	needleSess, err := st.CreateSession(store.Session{AgentID: ag.ID, Label: "cu"})
	if err != nil {
		t.Fatal(err)
	}
	fillerSess, err := st.CreateSession(store.Session{AgentID: ag.ID, Label: "moi"})
	if err != nil {
		t.Fatal(err)
	}
	ask, err := st.CreateSession(store.Session{AgentID: ag.ID, Label: "hoi"})
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	const needle = "Ghế mẫu Minh An mã MA-WALNUT-77 làm bằng gỗ sồi."
	if _, err := db.Exec(`INSERT INTO memories(id, session_id, kind, body, created_at, tenant_id, customer_key) VALUES($1,$2,$3,$4,$5,$6,$7)`,
		"spec154-needle", needleSess.ID, store.KindEpisodic, needle, "2000-01-01T00:00:00Z", needleSess.TenantID, ""); err != nil {
		t.Fatal(err)
	}
	fillers := []string{
		"Công ty mẫu Minh An gửi báo giá dạng PDF có VAT.",
		"Chiết khấu nội bộ Minh An đã chốt tám phần trăm.",
		"Kho mẫu Minh An đặt tại khu A và không giao trước chín giờ.",
	}
	const nFill = 49999
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	const batch = 400
	for seq := 0; seq < nFill; {
		end := seq + batch
		if end > nFill {
			end = nFill
		}
		var b strings.Builder
		args := make([]any, 0, (end-seq)*7)
		b.WriteString(`INSERT INTO memories(id, session_id, kind, body, created_at, tenant_id, customer_key) VALUES`)
		for i := seq; i < end; i++ {
			if i > seq {
				b.WriteByte(',')
			}
			base := (i - seq) * 7
			fmt.Fprintf(&b, "($%d,$%d,$%d,$%d,$%d,$%d,$%d)", base+1, base+2, base+3, base+4, base+5, base+6, base+7)
			args = append(args, fmt.Sprintf("spec154-f-%d", i), fillerSess.ID, store.KindEpisodic, fillers[i%len(fillers)], "2026-06-01T00:00:00Z", fillerSess.TenantID, "")
		}
		if _, err := tx.Exec(b.String(), args...); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		seq = end
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ANALYZE memories`); err != nil {
		t.Fatal(err)
	}
	q := store.RecallQuery{TenantID: needleSess.TenantID, AgentID: ag.ID, ExcludeSession: ask.ID, Query: spec154Query}
	start := time.Now()
	note, n := RecallFor(st, q)
	elapsed := time.Since(start)
	if n != 1 || !strings.Contains(note, "MA-WALNUT-77") || !strings.Contains(note, "gỗ sồi") {
		t.Fatalf("RecallFor n=%d note=%q", n, note)
	}
	fmt.Printf("SPEC154 recall_for_50k rows=%d elapsed=%s\n", nFill+1, elapsed)
	if elapsed >= 50*time.Millisecond {
		t.Fatalf("RecallFor %s is not under 50ms", elapsed)
	}
}

func recallForOlderThanCap(t *testing.T, st store.StoreIface) {
	t.Helper()
	ag, err := st.CreateAgent(store.Agent{AgentKey: "minh-an-sales", DisplayName: "Cố vấn Minh An"})
	if err != nil {
		t.Fatal(err)
	}
	oldSess, err := st.CreateSession(store.Session{AgentID: ag.ID, Label: "cu"})
	if err != nil {
		t.Fatal(err)
	}
	newSess, err := st.CreateSession(store.Session{AgentID: ag.ID, Label: "moi"})
	if err != nil {
		t.Fatal(err)
	}
	ask, err := st.CreateSession(store.Session{AgentID: ag.ID, Label: "hoi"})
	if err != nil {
		t.Fatal(err)
	}
	const needle = "Ghế mẫu Minh An mã MA-WALNUT-77 làm bằng gỗ sồi."
	if _, err := st.PutMemory(store.Memory{SessionID: oldSess.ID, TenantID: oldSess.TenantID, Kind: store.KindEpisodic, Body: needle}); err != nil {
		t.Fatal(err)
	}
	const cap = 500
	for i := 0; i < cap; i++ {
		body := "Công ty mẫu Minh An gửi báo giá dạng PDF có VAT."
		if _, err := st.PutMemory(store.Memory{SessionID: newSess.ID, TenantID: newSess.TenantID, Kind: store.KindEpisodic, Body: body}); err != nil {
			t.Fatal(err)
		}
	}
	q := store.RecallQuery{TenantID: oldSess.TenantID, AgentID: ag.ID, ExcludeSession: ask.ID, Query: spec154Query}
	note, n := RecallFor(st, q)
	if n < 1 || !strings.Contains(note, "MA-WALNUT-77") || !strings.Contains(note, "gỗ sồi") {
		t.Fatalf("RecallFor n=%d note=%q", n, note)
	}
}

// spec154DSN returns a schema-scoped DSN, or "" when Postgres is not configured
// outside CI. require in CI when must is true.
func spec154DSN(t *testing.T, schema string, must bool) string {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("GOSO_TEST_POSTGRES_DSN"))
	if base == "" {
		base = strings.TrimSpace(os.Getenv("GOSO_TEST_DATABASE_URL"))
	}
	if base == "" {
		if must && (os.Getenv("CI") == "true" || os.Getenv("CI") == "1") {
			t.Fatal("GOSO_TEST_POSTGRES_DSN is required in CI")
		}
		if must {
			t.Skip("GOSO_TEST_POSTGRES_DSN unset")
		}
		return ""
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
	t.Cleanup(func() {
		c, err := sql.Open("pgx", base)
		if err != nil {
			return
		}
		defer c.Close()
		_, _ = c.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
	})
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("options", "-csearch_path="+schema)
	u.RawQuery = q.Encode()
	return u.String()
}
