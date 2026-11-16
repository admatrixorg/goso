// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeExtractConv struct {
	seen []ConvertRequest
	res  ConvertResult
}

func (f *fakeExtractConv) Convert(ctx context.Context, req ConvertRequest) (ConvertResult, error) {
	f.seen = append(f.seen, req)
	if _, err := os.Stat(req.AbsPath); err != nil {
		return ConvertResult{ErrorClass: ClassInternal}, nil
	}
	return f.res, nil
}

func TestExtractText_TextInProcess(t *testing.T) {
	t.Setenv("GOSO_VAULT_DIR", t.TempDir())
	t.Setenv("GOSO_VAULT_DOCLING_URL", "")
	got := ExtractText(context.Background(), "acme", "note.md", []byte("\xEF\xBB\xBFXin chào Minh An\r\n"))
	if got.ErrorClass != "" || got.Markdown != "Xin chào Minh An\n" || got.MIME != "text/markdown" {
		t.Fatalf("%+v", got)
	}
	if got := ExtractText(context.Background(), "acme", "bad.txt", []byte{0xff, 0xfe, 0x00}); got.ErrorClass != ClassUnsupported {
		t.Fatalf("invalid utf8 %+v", got)
	}
	if got := ExtractText(context.Background(), "acme", "song.mp3", []byte("ID3")); got.ErrorClass != ClassUnsupported {
		t.Fatalf("audio %+v", got)
	}
}

func TestExtractText_BinaryNeedsDocling(t *testing.T) {
	t.Setenv("GOSO_VAULT_DIR", t.TempDir())
	t.Setenv("GOSO_VAULT_DOCLING_URL", "")
	prev := ExtractConverter
	ExtractConverter = nil
	t.Cleanup(func() { ExtractConverter = prev })
	got := ExtractText(context.Background(), "acme", "a.pdf", []byte("%PDF-1.4 x"))
	if got.ErrorClass != ClassNotConfigured {
		t.Fatalf("%+v", got)
	}
}

func TestExtractText_PDFStagedAndRemoved(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_DIR", root)
	fake := &fakeExtractConv{res: ConvertResult{Markdown: "# Báo giá\nMinh An", PageCount: 3, OCREngine: "tesseract", OCRLang: "vie+eng"}}
	prev := ExtractConverter
	ExtractConverter = fake
	t.Cleanup(func() { ExtractConverter = prev })
	got := ExtractText(context.Background(), "acme", "quote.pdf", []byte("%PDF-1.4 body"))
	if got.ErrorClass != "" || got.PageCount != 3 || !got.OCR || !strings.Contains(got.Markdown, "Minh An") || got.MIME != "application/pdf" {
		t.Fatalf("%+v", got)
	}
	if len(fake.seen) != 1 || !strings.HasPrefix(fake.seen[0].RawPath, "raw/.read-") || fake.seen[0].Tenant != "acme" {
		t.Fatalf("req %+v", fake.seen)
	}
	ents, _ := os.ReadDir(filepath.Join(root, "t", "acme", "raw"))
	if len(ents) != 0 {
		t.Fatalf("staged file left behind: %v", ents)
	}
	// Extension must agree with the bytes.
	if got := ExtractText(context.Background(), "acme", "fake.pdf", []byte("plain text")); got.ErrorClass != ClassUnsupported {
		t.Fatalf("mismatch %+v", got)
	}
}

func TestExtractText_TooLarge(t *testing.T) {
	t.Setenv("GOSO_VAULT_DIR", t.TempDir())
	t.Setenv("GOSO_VAULT_INGEST_MAX_BYTES", "4")
	if got := ExtractText(context.Background(), "acme", "a.txt", []byte("hello")); got.ErrorClass != ClassTooLarge {
		t.Fatalf("%+v", got)
	}
}

func TestExtractText_HTTPSidecarFake(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_DIR", root)
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		_, _ = w.Write([]byte(`{"markdown":"Hợp đồng","page_count":2,"ocr_engine":"tesseract","ocr_lang":"vie+eng"}`))
	}))
	defer srv.Close()
	t.Setenv("GOSO_VAULT_DOCLING_URL", srv.URL)
	t.Setenv("GOSO_VAULT_DOCLING", "")
	prev := ExtractConverter
	ExtractConverter = nil
	t.Cleanup(func() { ExtractConverter = prev })
	if got := ExtractText(context.Background(), "acme", "c.pdf", []byte("%PDF-1.7")); got.ErrorClass != ClassNotConfigured {
		t.Fatalf("docling flag off must be not_configured: %+v", got)
	}
	t.Setenv("GOSO_VAULT_DOCLING", "1")
	got := ExtractText(context.Background(), "acme", "c.pdf", []byte("%PDF-1.7"))
	if got.ErrorClass != "" || got.Markdown != "Hợp đồng" || got.PageCount != 2 {
		t.Fatalf("%+v", got)
	}
	if body["tenant"] != "acme" || !strings.HasPrefix(body["raw_path"].(string), "raw/.read-") {
		t.Fatalf("payload %+v", body)
	}
}
