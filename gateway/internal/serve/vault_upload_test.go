// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package serve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

type boomRead struct {
	t *testing.T
}

func (b boomRead) Read(p []byte) (int, error) {
	b.t.Fatal("request body was read")
	return 0, nil
}

func (b boomRead) Close() error { return nil }

// TestVaultUpload_UnauthenticatedBodyNotRead is criterion 3 on the full stack,
// including dev-mode where Require is not installed.
func TestVaultUpload_UnauthenticatedBodyNotRead(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_ENV", "demo")
	t.Setenv("GOSO_VAULT_DIR", root)
	t.Setenv("GOSO_VAULT_UPLOAD", "1")
	t.Setenv("GOSO_VAULT_INGEST_QUOTA_BYTES", "67108864")
	t.Setenv("GOSO_WORKSPACE", "")
	t.Setenv("GOSO_RATE_LIMIT", "0")
	t.Setenv("GOSO_E2E_SCRIPTED", "")

	t.Run("dev mode no token", func(t *testing.T) {
		t.Setenv("GOSO_DEV_MODE", "1")
		t.Setenv("GOSO_ADMIN_TOKEN", "")
		t.Setenv("GOSO_VIEW_TOKEN", "")
		h, status := New(store.New(), "test")
		if !status.DevMode {
			t.Fatal("dev mode")
		}
		req := httptest.NewRequest(http.MethodPost, "/api/vault/upload", boomRead{t})
		req.ContentLength = 20 << 20
		req.Header.Set("Content-Type", "multipart/form-data; boundary=minhan")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status %d %s", rr.Code, rr.Body.String())
		}
		assertNoRaw(t, root)
	})

	t.Run("wrong token", func(t *testing.T) {
		t.Setenv("GOSO_DEV_MODE", "")
		t.Setenv("GOSO_ADMIN_TOKEN", "admin-170")
		t.Setenv("GOSO_VIEW_TOKEN", "view-170")
		h, _ := New(store.New(), "test")
		req := httptest.NewRequest(http.MethodPost, "/api/vault/upload", boomRead{t})
		req.ContentLength = 20 << 20
		req.Header.Set("Content-Type", "multipart/form-data; boundary=minhan")
		req.Header.Set("Authorization", "Bearer not-the-token")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("status %d %s", rr.Code, rr.Body.String())
		}
		assertNoRaw(t, root)
	})

	t.Run("view token", func(t *testing.T) {
		t.Setenv("GOSO_DEV_MODE", "")
		t.Setenv("GOSO_ADMIN_TOKEN", "admin-170")
		t.Setenv("GOSO_VIEW_TOKEN", "view-170")
		h, _ := New(store.New(), "test")
		req := httptest.NewRequest(http.MethodPost, "/api/vault/upload", boomRead{t})
		req.ContentLength = 20 << 20
		req.Header.Set("Authorization", "Bearer view-170")
		req.Header.Set("Content-Type", "multipart/form-data; boundary=minhan")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("status %d %s", rr.Code, rr.Body.String())
		}
		if rr.Body.String() != "{\"error\":\"forbidden\"}\n" {
			t.Fatalf("body %s", rr.Body.String())
		}
		assertNoRaw(t, root)
	})
}

func assertNoRaw(t *testing.T, root string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, "t", "*", "raw", "*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range matches {
		info, err := os.Lstat(m)
		if err != nil {
			continue
		}
		if info.Mode().IsRegular() {
			t.Fatalf("raw file created: %s", m)
		}
	}
}
