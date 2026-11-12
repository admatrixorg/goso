// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package security

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestLimitAPI_VaultUploadExemption is criterion 2 for the body cap:
// the upload route is exempt only while GOSO_VAULT_UPLOAD is on, and every
// other /api or /v1 route stays at 1 MiB.
func TestLimitAPI_VaultUploadExemption(t *testing.T) {
	readAll := func(limit int) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n, err := io.Copy(io.Discard, r.Body)
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) || n > int64(limit) {
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				return
			}
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
		})
	}

	twoMiB := bytes.Repeat([]byte("a"), 2<<20)
	overOne := bytes.Repeat([]byte("b"), MaxAPIBody+8)

	t.Run("flag off cuts upload at 1 MiB", func(t *testing.T) {
		t.Setenv("GOSO_VAULT_UPLOAD", "")
		h := LimitAPI(readAll(MaxAPIBody))
		req := httptest.NewRequest(http.MethodPost, "/api/vault/upload", bytes.NewReader(twoMiB))
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("2 MiB upload with flag off must be cut at 1 MiB, status %d", rr.Code)
		}
	})

	t.Run("flag on lets 2 MiB through this route only", func(t *testing.T) {
		t.Setenv("GOSO_VAULT_UPLOAD", "1")
		h := LimitAPI(readAll(2 << 20))
		req := httptest.NewRequest(http.MethodPost, "/api/vault/upload", bytes.NewReader(twoMiB))
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("flag on must not apply the 1 MiB cap to upload, status %d", rr.Code)
		}
	})

	t.Run("flag on still caps other api routes", func(t *testing.T) {
		t.Setenv("GOSO_VAULT_UPLOAD", "1")
		h := LimitAPI(readAll(MaxAPIBody))
		for _, target := range []struct {
			method string
			path   string
		}{
			{http.MethodPut, "/api/vault/docs"},
			{http.MethodPost, "/api/storage/upload"},
			{http.MethodPost, "/api/agents"},
			{http.MethodPost, "/v1/vault/upload"},
			{http.MethodPost, "/v1/agents"},
			{http.MethodGet, "/api/vault/upload"},
			{http.MethodPost, "/api/vault/upload/"},
		} {
			req := httptest.NewRequest(target.method, target.path, bytes.NewReader(overOne))
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("%s %s stayed open: %d", target.method, target.path, rr.Code)
			}
		}
	})
}
