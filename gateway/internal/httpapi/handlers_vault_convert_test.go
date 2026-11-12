// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/vault"
)

type phraseConvert struct{}

func (phraseConvert) Convert(context.Context, vault.ConvertRequest) (vault.ConvertResult, error) {
	return vault.ConvertResult{
		Markdown:  "# Minh An\n\nHợp đồng thuê nhà tại Đà Nẵng\n",
		PageCount: 1,
		OCREngine: "tesseract",
		OCRLang:   "vie+eng",
	}, nil
}

func TestVaultIngest_ConvertedFieldsStayInTenant(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_UPLOAD", "1")
	t.Setenv("GOSO_VAULT_DOCLING", "1")
	t.Setenv("GOSO_VAULT_INGEST_QUOTA_BYTES", "1048576")
	t.Setenv("GOSO_VAULT_DIR", root)
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-171")
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_WORKSPACE", "")
	st := store.New()
	h := NewRouter(Options{Store: st, Version: "test"})
	rr := postUpload(t, h, "admin-171", "minh-an", "hop-dong.pdf", []byte("%PDF-1.4\n% Minh An\n"), "")
	if rr.Code != http.StatusCreated {
		t.Fatalf("upload %d %s", rr.Code, rr.Body.String())
	}
	var created uploadResp
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	w := vault.NewConvertWorker(st, root)
	w.Converter = phraseConvert{}
	if err := w.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	cross := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/vault/ingest/"+created.Job.ID, nil)
	req.Header.Set("Authorization", "Bearer admin-171")
	req.Header.Set("X-Goso-Tenant", "minh-an-legal")
	h.ServeHTTP(cross, req)
	if cross.Code != http.StatusNotFound || strings.Contains(cross.Body.String(), "sources/inbox") || strings.Contains(cross.Body.String(), "Đà Nẵng") {
		t.Fatalf("cross tenant %d %s", cross.Code, cross.Body.String())
	}
	same := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/vault/ingest/"+created.Job.ID, nil)
	req.Header.Set("Authorization", "Bearer admin-171")
	req.Header.Set("X-Goso-Tenant", "minh-an")
	h.ServeHTTP(same, req)
	if same.Code != http.StatusOK {
		t.Fatalf("get %d %s", same.Code, same.Body.String())
	}
	if strings.Contains(same.Body.String(), "Đà Nẵng") {
		t.Fatalf("job json stored the document: %s", same.Body.String())
	}
	var body struct {
		Job struct {
			Status      string   `json:"status"`
			SourcePaths []string `json:"source_paths"`
			OCREngine   string   `json:"ocr_engine"`
			OCRLang     string   `json:"ocr_lang"`
			PageCount   int      `json:"page_count"`
			ConvertedAt string   `json:"converted_at"`
		} `json:"job"`
	}
	if err := json.Unmarshal(same.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Job.Status != "converted" || body.Job.OCREngine != "tesseract" || body.Job.OCRLang != "vie+eng" || body.Job.PageCount != 1 || body.Job.ConvertedAt == "" || len(body.Job.SourcePaths) != 1 {
		t.Fatalf("view %+v", body.Job)
	}
	if _, err := os.Stat(filepath.Join(root, "t", "minh-an", filepath.FromSlash(body.Job.SourcePaths[0]))); err != nil {
		t.Fatal(err)
	}
	hz := httptest.NewRecorder()
	h.ServeHTTP(hz, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if hz.Code != http.StatusOK {
		t.Fatalf("healthz %d %s", hz.Code, hz.Body.String())
	}
}

func TestVaultIngest_SidecarDownDoesNotFailHealth(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_UPLOAD", "1")
	t.Setenv("GOSO_VAULT_DOCLING", "1")
	t.Setenv("GOSO_VAULT_INGEST_QUOTA_BYTES", "1048576")
	t.Setenv("GOSO_VAULT_DIR", root)
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-171")
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_WORKSPACE", "")
	st := store.New()
	h := NewRouter(Options{Store: st, Version: "test"})
	rr := postUpload(t, h, "admin-171", "minh-an", "hop-dong.pdf", []byte("%PDF-1.4\n% Minh An\n"), "")
	if rr.Code != http.StatusCreated {
		t.Fatalf("upload %d %s", rr.Code, rr.Body.String())
	}
	var created uploadResp
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	w := vault.NewConvertWorker(st, root)
	w.Converter = vault.HTTPConverter{URL: "", PageBudget: 1}
	if err := w.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	same := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/vault/ingest/"+created.Job.ID, nil)
	req.Header.Set("Authorization", "Bearer admin-171")
	req.Header.Set("X-Goso-Tenant", "minh-an")
	h.ServeHTTP(same, req)
	if same.Code != http.StatusOK || !strings.Contains(same.Body.String(), `"not_configured"`) {
		t.Fatalf("get %d %s", same.Code, same.Body.String())
	}
	hz := httptest.NewRecorder()
	h.ServeHTTP(hz, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if hz.Code != http.StatusOK || strings.Contains(hz.Body.String(), "not_configured") {
		t.Fatalf("healthz %d %s", hz.Code, hz.Body.String())
	}
}
