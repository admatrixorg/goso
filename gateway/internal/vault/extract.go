// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"syscall"
	"unicode/utf8"

	"github.com/mqglobal/goso/gateway/internal/security"
)

// ExtractConverter replaces the Docling sidecar for ExtractText. Tests set a
// fake; production leaves it nil and uses HTTPConverter at GOSO_VAULT_DOCLING_URL
// when GOSO_VAULT_DOCLING is on.
var ExtractConverter Converter

// Extracted is the text of one file read for a tool call. Nothing is stored:
// no ingest job, no sources/ page. ErrorClass uses the vault Class* values.
type Extracted struct {
	Markdown   string
	PageCount  int
	OCR        bool
	MIME       string
	ErrorClass string
}

// ExtractText turns one file's bytes into text with the same rules as the
// ingest worker. .txt/.md are normalized in process. PDF, DOCX, XLSX, PPTX
// and images go to the Docling sidecar, which reads only t/<tenant>/raw, so
// the bytes are staged there under a dot name and removed after the call.
// The extension must agree with the sniffed bytes.
func ExtractText(ctx context.Context, tenant, name string, data []byte) Extracted {
	if int64(len(data)) > IngestMaxBytes() {
		return Extracted{ErrorClass: ClassTooLarge}
	}
	ext := clientExt(name)
	if ext == "" || unsupportedExt(ext) {
		return Extracted{ErrorClass: ClassUnsupported}
	}
	if ext == ".txt" || ext == ".md" {
		if !utf8.Valid(data) {
			return Extracted{ErrorClass: ClassUnsupported}
		}
		text, err := normalizeText(data)
		if err != nil {
			return Extracted{ErrorClass: ClassUnsupported}
		}
		if !hasLetter(text) {
			return Extracted{ErrorClass: ClassEmpty}
		}
		mime := "text/plain"
		if ext == ".md" {
			mime = "text/markdown"
		}
		return Extracted{Markdown: text, MIME: mime}
	}
	if !isConvertible("", "x"+ext) {
		return Extracted{ErrorClass: ClassUnsupported}
	}
	conv := ExtractConverter
	if conv == nil {
		if !DoclingEnabled() || DoclingURL() == "" {
			return Extracted{ErrorClass: ClassNotConfigured}
		}
		conv = HTTPConverter{URL: DoclingURL(), PageBudget: PageBudget()}
	}
	svc := New(nil, "")
	dir, err := svc.tenantDir(tenant)
	if err != nil {
		return Extracted{ErrorClass: ClassInternal}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Extracted{ErrorClass: ClassInternal}
	}
	if err := security.ConfineTo(svc.Root, dir); err != nil {
		return Extracted{ErrorClass: ClassInternal}
	}
	rawDir := filepath.Join(dir, "raw")
	if err := rejectSymlinks(dir, rawDir); err != nil {
		return Extracted{ErrorClass: ClassInternal}
	}
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		return Extracted{ErrorClass: ClassInternal}
	}
	staged, err := stageExtract(rawDir, ext, data)
	if err != nil {
		return Extracted{ErrorClass: ClassInternal}
	}
	defer removeRegular(staged)
	kind, err := sniffFile(staged)
	if err != nil {
		return Extracted{ErrorClass: ClassInternal}
	}
	storedExt, ok := extAgrees(ext, kind)
	if !ok {
		return Extracted{ErrorClass: ClassUnsupported}
	}
	mime := mimeForExt(storedExt, kind)
	w := &Worker{Converter: conv, PageTimeout: PageTimeout(), MaxAttempts: PageAttempts()}
	res, err := w.invoke(ctx, ConvertRequest{
		Tenant:      tenant,
		RawPath:     "raw/" + filepath.Base(staged),
		MIME:        mime,
		AbsPath:     staged,
		PageTimeout: w.PageTimeout,
		MaxAttempts: w.MaxAttempts,
	})
	if err != nil {
		return Extracted{ErrorClass: ClassInternal}
	}
	if res.ErrorClass != "" {
		if !knownClass(res.ErrorClass) {
			return Extracted{ErrorClass: ClassInternal}
		}
		return Extracted{ErrorClass: res.ErrorClass}
	}
	if !hasLetter(res.Markdown) {
		return Extracted{ErrorClass: ClassEmpty}
	}
	pages := res.PageCount
	if pages < 1 {
		pages = 1
	}
	return Extracted{Markdown: res.Markdown, PageCount: pages, OCR: res.OCREngine != "", MIME: mime}
}

func stageExtract(rawDir, ext string, data []byte) (string, error) {
	var rnd [8]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return "", err
	}
	path := filepath.Join(rawDir, ".read-"+hex.EncodeToString(rnd[:])+ext)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		removeRegular(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		removeRegular(path)
		return "", err
	}
	return path, nil
}
