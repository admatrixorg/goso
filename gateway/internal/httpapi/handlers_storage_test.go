// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func storageServer(t *testing.T) (string, *eventstore.Store, http.Handler) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", dir)
	t.Setenv("GOSO_STORAGE_MAX_BYTES", "1048576")
	st := store.New()
	ev := eventstore.New(64)
	h := NewRouter(Options{Store: st, Version: "t", Events: ev})
	return dir, ev, h
}

func TestStorage_ListEmptyNotConfiguredAndV1(t *testing.T) {
	t.Setenv("GOSO_WORKSPACE", "")
	h := NewRouter(Options{Store: store.New(), Version: "t"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/storage", nil))
	if w.Code != 200 {
		t.Fatalf("GET %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"configured":false`) && !strings.Contains(w.Body.String(), `"configured": false`) {
		t.Fatalf("not_configured %s", w.Body.String())
	}
	assertSameGET(t, h, "/api/storage", "/v1/storage")
}

func TestStorage_ListPreviewDownloadDeleteNoSecrets(t *testing.T) {
	dir, ev, h := storageServer(t)
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello workspace"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET=super-secret-value"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "secrets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secrets", "token.txt"), []byte("sk-live-abcdefghijk"), 0o644); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/storage", nil))
	if w.Code != 200 {
		t.Fatalf("list %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "super-secret-value") || strings.Contains(body, "sk-live-") || strings.Contains(body, ".env") || strings.Contains(body, `"secrets"`) {
		t.Fatalf("secret leaked %s", body)
	}
	if !strings.Contains(body, "readme.txt") {
		t.Fatalf("missing file %s", body)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/storage?path=..%2Fetc", nil))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "path escape") {
		t.Fatalf("jail %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/storage/preview?path=readme.txt", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "hello workspace") {
		t.Fatalf("preview %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/storage/preview?path=.env", nil))
	if w.Code != 403 {
		t.Fatalf("preview env %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "super-secret-value") {
		t.Fatalf("preview leaked %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/storage/download?path=readme.txt", nil))
	if w.Code != 200 || w.Body.String() != "hello workspace" {
		t.Fatalf("download %d %s", w.Code, w.Body.String())
	}

	if err := os.WriteFile(filepath.Join(dir, "leaky.txt"), []byte("token sk-live-abcdefghijk"), 0o644); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/storage/download?path=leaky.txt", nil))
	if w.Code != 403 || strings.Contains(w.Body.String(), "sk-live-") {
		t.Fatalf("download leaky %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/storage/download?path=.env", nil))
	if w.Code != 403 || strings.Contains(w.Body.String(), "super-secret-value") {
		t.Fatalf("download env %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/storage/delete", bytes.NewBufferString(`{"path":"readme.txt"}`)))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "confirm is required") {
		t.Fatalf("confirm %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/storage/delete", bytes.NewBufferString(`{"path":"readme.txt","confirm":"nope"}`)))
	if w.Code != 400 || !strings.Contains(w.Body.String(), "confirm does not match") {
		t.Fatalf("mismatch %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/storage/delete", bytes.NewBufferString(`{"path":"readme.txt","confirm":"readme.txt"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("delete %d %s", w.Code, w.Body.String())
	}
	found := false
	for _, e := range ev.Filter("", "storage", 64) {
		if e.Connector == "storage" && e.Tool == "delete" {
			found = true
			if strings.Contains(e.Summary, "super-secret") {
				t.Fatalf("audit %s", e.Summary)
			}
		}
	}
	if !found {
		t.Fatal("audit missing")
	}
}

func TestStorage_UploadTypeAndV1(t *testing.T) {
	_, _, h := storageServer(t)
	w := httptest.NewRecorder()
	body, ctype := multipartFile(t, "notes.md", "ok notes", "")
	req := httptest.NewRequest(http.MethodPost, "/api/storage/upload", body)
	req.Header.Set("Content-Type", ctype)
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("upload %d %s", w.Code, w.Body.String())
	}
	var ent map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &ent); err != nil {
		t.Fatal(err)
	}
	if ent["name"] != "notes.md" {
		t.Fatalf("ent %v", ent)
	}

	w = httptest.NewRecorder()
	body, ctype = multipartFile(t, "evil.sh", "echo hi", "")
	req = httptest.NewRequest(http.MethodPost, "/api/storage/upload", body)
	req.Header.Set("Content-Type", ctype)
	h.ServeHTTP(w, req)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "type not allowed") {
		t.Fatalf("sh %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	body, ctype = multipartFile(t, "key.pem", "-----BEGIN OPENSSH PRIVATE KEY-----\n", "")
	req = httptest.NewRequest(http.MethodPost, "/api/storage/upload", body)
	req.Header.Set("Content-Type", ctype)
	h.ServeHTTP(w, req)
	if w.Code == 201 || strings.Contains(w.Body.String(), "BEGIN OPENSSH") {
		t.Fatalf("pem %d %s", w.Code, w.Body.String())
	}

	assertSameGET(t, h, "/api/storage", "/v1/storage")
}

func multipartFile(t *testing.T, name, content, path string) (*bytes.Buffer, string) {
	t.Helper()
	buf := &bytes.Buffer{}
	mw := multipart.NewWriter(buf)
	if path != "" {
		if err := mw.WriteField("path", path); err != nil {
			t.Fatal(err)
		}
	}
	fw, err := mw.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(fw, content); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf, mw.FormDataContentType()
}
