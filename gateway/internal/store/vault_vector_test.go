// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
	"os"
	"testing"
)

// openVectorStore is a Postgres store with vault_chunks, or a skip. CI runs
// pgvector/pgvector:pg16, so there a missing index is a failure.
func openVectorStore(t *testing.T, schema string) *PostgresStore {
	t.Helper()
	dsn := postgresTestDSN()
	if dsn == "" {
		t.Skip("GOSO_TEST_POSTGRES_DSN unset")
	}
	pg, err := OpenPostgres(prepareSchema(t, dsn, schema, false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pg.Close() })
	if !pg.VaultVectorReady() {
		if os.Getenv("CI") == "true" || os.Getenv("CI") == "1" {
			t.Fatal("CI postgres has no pgvector")
		}
		t.Skip("pgvector not available")
	}
	return pg
}

func TestVaultVector_SearchScopedAndCurrent(t *testing.T) {
	s := openVectorStore(t, "vault_vec_search")
	ids := vaultAccessFixture(t, s)
	vec := map[string][]float32{
		"chung":         {1, 0, 0},
		"rieng-x":       {0.9, 0.1, 0},
		"rieng-y":       {0.95, 0.05, 0},
		"nhom-ban-hang": {0, 1, 0},
		"ca-nhan-x":     {0, 0, 1},
		"khach-minh-an": {0.5, 0.5, 0},
		"khach-khac":    {0.6, 0.4, 0},
	}
	for title, v := range vec {
		d, err := s.GetVaultDoc(ids[title])
		if err != nil {
			t.Fatal(err)
		}
		if err := s.ReplaceVaultChunks("minh-an", d.ID, d.SHA256, "fake-embed", 3, []VaultChunk{{Index: 0, Body: d.Title, Embedding: v}}); err != nil {
			t.Fatal(err)
		}
	}
	// Another tenant cannot write chunks for this doc.
	if err := s.ReplaceVaultChunks("tenant-khac", ids["chung"], "chung", "fake-embed", 3, []VaultChunk{{Embedding: []float32{1, 0, 0}}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant chunk write: %v", err)
	}
	docs, embedded, err := s.VaultEmbedCounts("minh-an", "fake-embed", 3)
	if err != nil || docs != 7 || embedded != 7 {
		t.Fatalf("counts docs=%d embedded=%d err=%v", docs, embedded, err)
	}
	titles := func(v VaultViewer) map[string]bool {
		hits, err := s.SearchVaultVector("minh-an", v, "fake-embed", []float32{1, 0, 0}, 20)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, h := range hits {
			d, _ := s.GetVaultDoc(h.DocID)
			out[d.Title] = true
		}
		return out
	}
	x := titles(VaultViewer{AgentID: "agent-x"})
	if !x["chung"] || !x["rieng-x"] || x["rieng-y"] || x["nhom-ban-hang"] {
		t.Fatalf("agent x sees %v", x)
	}
	ch := titles(VaultViewer{AgentID: "agent-x", Channel: true, CustomerKey: "zalo:minh-an"})
	if ch["ca-nhan-x"] || ch["khach-khac"] || !ch["khach-minh-an"] {
		t.Fatalf("channel sees %v", ch)
	}
	if got := titles(VaultViewer{AgentID: "agent-x"}); len(got) == 0 {
		t.Fatal("no hits")
	}
	if other, err := s.SearchVaultVector("tenant-khac", OperatorView, "fake-embed", []float32{1, 0, 0}, 20); err != nil || len(other) != 0 {
		t.Fatalf("other tenant hits %v %v", other, err)
	}

	// A body edit makes chunks stale: no vector hit, back in the backlog.
	d, _ := s.GetVaultDoc(ids["chung"])
	d.Body, d.SHA256 = "bang gia moi", "chung-v2"
	if _, err := s.PutVaultDoc(*d); err != nil {
		t.Fatal(err)
	}
	if titles(OperatorView)["chung"] {
		t.Fatal("stale chunk served")
	}
	back, err := s.VaultEmbedBacklog("minh-an", "fake-embed", 3, "", 10)
	if err != nil || len(back) != 1 || back[0].ID != ids["chung"] {
		t.Fatalf("backlog %v %v", back, err)
	}
	// Delete drops the chunks.
	if err := s.DeleteVaultDoc(ids["rieng-x"]); err != nil {
		t.Fatal(err)
	}
	if _, embedded, _ := s.VaultEmbedCounts("minh-an", "fake-embed", 3); embedded != 5 {
		t.Fatalf("embedded after delete+edit = %d", embedded)
	}
}
