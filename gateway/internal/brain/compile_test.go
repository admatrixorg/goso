package brain

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestCompileTenantFallbackWithoutJSON(t *testing.T) {
	dir := t.TempDir()
	st, closer, err := store.Open(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer closer()
	if _, err := st.PutVaultDoc(store.VaultDoc{Title: "Quy trinh ban hang", Path: "sales.md", Body: "Buoc 1: chao khach. Buoc 2: bao gia.", TenantID: "digi"}); err != nil {
		t.Fatal(err)
	}
	n, err := CompileTenant(context.Background(), st, llm.Echo{}, "digi")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("pages %d", n)
	}
	hits, err := Search(st, "digi", "bao gia", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("hits %d", len(hits))
	}
}

func TestSlugify(t *testing.T) {
	if slugify("Quy trinh Ban hang") == "" {
		t.Fatal("empty slug")
	}
}
