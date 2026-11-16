// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/embed"
	"github.com/mqglobal/goso/gateway/internal/store"
)

const accessTenant = "minh-an"

type accessFixture struct {
	st       store.StoreIface
	svc      *Service
	x, y, z  *store.Agent
	team     *store.Team
	foreignA *store.Agent
	foreignT *store.Team
}

func newAccessFixture(t *testing.T, st store.StoreIface) *accessFixture {
	t.Helper()
	f := &accessFixture{st: st, svc: New(st, t.TempDir())}
	mk := func(key, tenant string) *store.Agent {
		a, err := st.CreateAgent(store.Agent{AgentKey: key, DisplayName: "Minh An " + key, TenantID: tenant})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	f.x = mk("minh-an-x", accessTenant)
	f.y = mk("minh-an-y", accessTenant)
	f.z = mk("minh-an-z", accessTenant)
	f.foreignA = mk("khac-a", "tenant-khac")
	var err error
	if f.team, err = st.CreateTeam(store.Team{Name: "Ban hang", LeadAgentID: f.y.ID, TenantID: accessTenant}); err != nil {
		t.Fatal(err)
	}
	if f.foreignT, err = st.CreateTeam(store.Team{Name: "Khac", LeadAgentID: f.foreignA.ID, TenantID: "tenant-khac"}); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *accessFixture) put(t *testing.T, title, body string, acc Access) *store.VaultDoc {
	t.Helper()
	acc, err := ResolveAccess(f.st, accessTenant, acc)
	if err != nil {
		t.Fatal(err)
	}
	d, err := f.svc.PutTenantAs(title, body, accessTenant, acc)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (f *accessFixture) found(t *testing.T, q string, v store.VaultViewer, id string) bool {
	t.Helper()
	hits, err := f.svc.SearchAs(accessTenant, q, v)
	if err != nil {
		t.Fatal(err)
	}
	inSearch := false
	for _, h := range hits {
		if h.ID == id {
			inSearch = true
		}
	}
	_, getErr := f.svc.GetAs(accessTenant, id, v)
	if inSearch != (getErr == nil) {
		t.Fatalf("search=%v but get err=%v", inSearch, getErr)
	}
	return inSearch
}

func accessStores(t *testing.T) map[string]store.StoreIface {
	sq, err := store.OpenSQLite(filepath.Join(t.TempDir(), "access.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sq.Close() })
	return map[string]store.StoreIface{"memory": store.New(), "sqlite": sq}
}

func TestVaultScope_AgentDocHiddenFromOtherAgent(t *testing.T) {
	for name, st := range accessStores(t) {
		t.Run(name, func(t *testing.T) {
			f := newAccessFixture(t, st)
			own := f.put(t, "Bang gia rieng X", "bang gia dai ly rieng cua X", Access{AgentID: f.x.AgentKey})
			if own.Scope != store.VaultScopeAgent || own.AgentID != f.x.ID {
				t.Fatalf("stored scope %q agent %q", own.Scope, own.AgentID)
			}
			team := f.put(t, "Bang gia nhom", "bang gia dai ly cua nhom ban hang", Access{TeamID: f.team.ID})
			shared := f.put(t, "Bang gia chung", "bang gia dai ly chung", Access{})
			vx := ViewerForAgent(st, accessTenant, f.x.ID, false, "")
			vy := ViewerForAgent(st, accessTenant, f.y.AgentKey, false, "")
			vz := ViewerForAgent(st, accessTenant, f.z.ID, false, "")
			if !f.found(t, "bang gia", vx, own.ID) || f.found(t, "bang gia", vy, own.ID) || f.found(t, "bang gia", vz, own.ID) {
				t.Fatal("agent doc of X leaked or missing")
			}
			if !f.found(t, "bang gia", vy, team.ID) || f.found(t, "bang gia", vx, team.ID) {
				t.Fatal("team doc visibility wrong")
			}
			for _, v := range []store.VaultViewer{vx, vy, vz, store.OperatorView} {
				if !f.found(t, "bang gia", v, shared.ID) {
					t.Fatal("tenant doc hidden")
				}
			}
			if !f.found(t, "bang gia", store.OperatorView, own.ID) {
				t.Fatal("operator view lost agent doc")
			}
			// The same title for another agent is a separate doc, not an overwrite.
			ownY := f.put(t, "Bang gia rieng X", "ban cua Y", Access{AgentID: f.y.ID})
			if ownY.ID == own.ID {
				t.Fatal("agent Y overwrote agent X's doc")
			}
		})
	}
}

func TestVaultScope_ChannelCustomerAndPersonal(t *testing.T) {
	for name, st := range accessStores(t) {
		t.Run(name, func(t *testing.T) {
			f := newAccessFixture(t, st)
			shared := f.put(t, "Chinh sach doi tra", "chinh sach doi tra 7 ngay", Access{})
			mine := f.put(t, "Ghi chu khach Minh An", "chinh sach doi tra rieng Minh An", Access{CustomerKey: "zalo:minh-an"})
			other := f.put(t, "Ghi chu khach khac", "chinh sach doi tra rieng khach khac", Access{CustomerKey: "zalo:khach-khac"})
			personal := f.put(t, "Nhap cua X", "chinh sach doi tra nhap cua X", Access{Scope: "personal", AgentID: f.x.ID})
			ch := ViewerForAgent(st, accessTenant, f.x.ID, true, "zalo:minh-an")
			if !f.found(t, "chinh sach", ch, shared.ID) || !f.found(t, "chinh sach", ch, mine.ID) {
				t.Fatal("channel lost shared or own-customer doc")
			}
			if f.found(t, "chinh sach", ch, other.ID) {
				t.Fatal("channel saw another customer's doc")
			}
			if f.found(t, "chinh sach", ch, personal.ID) {
				t.Fatal("channel saw a personal doc")
			}
			// Outside a channel the owner reads its personal doc.
			if !f.found(t, "chinh sach", ViewerForAgent(st, accessTenant, f.x.ID, false, ""), personal.ID) {
				t.Fatal("owner lost personal doc")
			}
		})
	}
}

func TestResolveAccess_RejectsForeignOwners(t *testing.T) {
	f := newAccessFixture(t, store.New())
	for _, a := range []Access{
		{AgentID: f.foreignA.ID},
		{AgentID: f.foreignA.AgentKey},
		{TeamID: f.foreignT.ID},
		{AgentID: "khong-co"},
	} {
		if _, err := ResolveAccess(f.st, accessTenant, a); !errors.Is(err, ErrAccessOwner) {
			t.Fatalf("%+v: %v", a, err)
		}
	}
	if _, err := ResolveAccess(f.st, accessTenant, Access{Scope: "galaxy"}); !errors.Is(err, store.ErrVaultScope) {
		t.Fatalf("bad scope: %v", err)
	}
	got, err := ResolveAccess(f.st, accessTenant, Access{AgentID: f.x.AgentKey})
	if err != nil || got.Scope != store.VaultScopeAgent || got.AgentID != f.x.ID {
		t.Fatalf("agent key resolve %+v %v", got, err)
	}
	if v := ViewerForAgent(f.st, accessTenant, f.foreignA.ID, false, ""); v.AgentID != "" {
		t.Fatal("foreign agent got an agent view")
	}
}

func TestChunk(t *testing.T) {
	if Chunk("   ") != nil {
		t.Fatal("blank body chunked")
	}
	short := Chunk("bang gia")
	if len(short) != 1 || short[0] != "bang gia" {
		t.Fatalf("short %v", short)
	}
	long := strings.Repeat("Đoạn văn mẫu về bảng giá Minh An.\n", 200)
	parts := Chunk(long)
	if len(parts) < 2 {
		t.Fatalf("long body gave %d chunks", len(parts))
	}
	for _, p := range parts {
		if n := len([]rune(p)); n > chunkRunes {
			t.Fatalf("chunk of %d runes", n)
		}
	}
	if huge := Chunk(strings.Repeat("x", chunkRunes*maxChunks*2)); len(huge) != maxChunks {
		t.Fatalf("cap: %d", len(huge))
	}
}

// topicEmbedder maps text to a 4-d vector by topic words, so a query and a
// doc with no word in common can still be near each other.
func topicEmbedder(t *testing.T) *httptest.Server {
	topics := [][]string{
		{"giao hang", "van chuyen", "ship"},
		{"bang gia", "chiet khau"},
		{"bao hanh", "doi tra"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		type item struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}
		data := []item{}
		for i, in := range req.Input {
			v := []float32{0, 0, 0, 0.01}
			low := strings.ToLower(in)
			for d, words := range topics {
				for _, w := range words {
					if strings.Contains(low, w) {
						v[d]++
					}
				}
			}
			data = append(data, item{Index: i, Embedding: v})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func openVectorPostgres(t *testing.T, schema string) store.StoreIface {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("GOSO_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("GOSO_TEST_POSTGRES_DSN unset")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	for _, q := range []string{`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`, `CREATE SCHEMA ` + schema} {
		if _, err := admin.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		if db, err := sql.Open("pgx", dsn); err == nil {
			_, _ = db.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
			_ = db.Close()
		}
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("options", "-csearch_path="+schema)
	u.RawQuery = q.Encode()
	st, err := store.OpenPostgres(u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if vi, ok := store.StoreIface(st).(store.VaultVectorIndex); !ok || !vi.VaultVectorReady() {
		if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
			t.Fatal("CI postgres has no pgvector")
		}
		t.Skip("pgvector not available")
	}
	return st
}

func TestHybridSearch_VectorFindsParaphraseWithinScope(t *testing.T) {
	st := openVectorPostgres(t, "vault_hybrid")
	srv := topicEmbedder(t)
	c := embed.New(embed.Config{URL: srv.URL + "/v1", Model: "fake-embed", RPS: 1000})
	ix := NewIndexer(st, c)
	if ix == nil {
		t.Fatal("no indexer on pgvector store")
	}
	f := newAccessFixture(t, st)
	ship := f.put(t, "Giao hang noi thanh", "Giao hang noi thanh mien phi trong 2 gio.", Access{})
	shipX := f.put(t, "Ship rieng X", "Ship tinh le cua X mat 3 ngay.", Access{AgentID: f.x.ID})
	f.put(t, "Bao hanh", "Bao hanh 12 thang.", Access{})

	// Keyword-only before the indexer is installed: no word in common.
	if hits, _ := f.svc.SearchAs(accessTenant, "van chuyen", store.OperatorView); len(hits) != 0 {
		t.Fatalf("keyword search matched a paraphrase: %v", hits)
	}
	SetIndexer(ix)
	t.Cleanup(func() { SetIndexer(nil) })
	if Status(accessTenant).Embedded != 0 {
		t.Fatal("embedded before backfill")
	}
	res, err := ix.Backfill(context.Background(), accessTenant, 10)
	if err != nil || res.Embedded != 3 || res.Remain {
		t.Fatalf("backfill %+v %v", res, err)
	}
	if again, _ := ix.Backfill(context.Background(), accessTenant, 10); again.Embedded != 0 {
		t.Fatalf("second backfill re-embedded %+v", again)
	}
	stat := Status(accessTenant)
	if stat.State != EmbedReady || stat.Docs != 3 || stat.Embedded != 3 || stat.Dim != 4 {
		t.Fatalf("status %+v", stat)
	}
	match := func(v store.VaultViewer) map[string]string {
		hits, err := f.svc.SearchAs(accessTenant, "van chuyen", v)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]string{}
		for _, h := range hits {
			out[h.ID] = h.Match
		}
		return out
	}
	y := match(ViewerForAgent(st, accessTenant, f.y.ID, false, ""))
	if y[ship.ID] != MatchVector {
		t.Fatalf("vector hit missing for agent Y: %v", y)
	}
	if _, leaked := y[shipX.ID]; leaked {
		t.Fatal("vector search leaked agent X's doc to Y")
	}
	if x := match(ViewerForAgent(st, accessTenant, f.x.ID, false, "")); x[shipX.ID] != MatchVector {
		t.Fatalf("agent X lost its own doc: %v", x)
	}
	// Another tenant gets nothing from this tenant's chunks.
	if hits, _ := f.svc.SearchAs("tenant-khac", "van chuyen", store.OperatorView); len(hits) != 0 {
		t.Fatalf("cross-tenant vector hits %v", hits)
	}
	// An embeddings outage falls back to keyword search.
	srv.Close()
	hits, err := f.svc.SearchAs(accessTenant, "giao hang", store.OperatorView)
	if err != nil || len(hits) != 1 || hits[0].ID != ship.ID {
		t.Fatalf("fallback %v %v", hits, err)
	}
}
