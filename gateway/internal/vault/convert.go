// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Conversion error classes. The job row stores the class, never the document text.
const (
	ClassTimeout       = "timeout"
	ClassOCRFailed     = "ocr_failed"
	ClassUnsupported   = "unsupported"
	ClassEmpty         = "empty_conversion"
	ClassTooLarge      = "too_large"
	ClassNotConfigured = "not_configured"
	ClassInternal      = "internal"
)

const (
	ocrEngineTesseract = "tesseract"
	ocrLangVieEng      = "vie+eng"
)

// ConvertRequest asks a converter for markdown. AbsPath is for an in-process
// fake that reads the fixture. The HTTP sidecar receives only the tenant and
// the logical raw path, and reads that file from its own read-only mount.
type ConvertRequest struct {
	Tenant      string
	RawPath     string
	MIME        string
	AbsPath     string
	PageTimeout time.Duration
	MaxAttempts int
}

// ConvertResult is markdown plus the OCR facts the job row is allowed to store.
// Markdown is not written to the job table.
type ConvertResult struct {
	Markdown   string
	PageCount  int
	OCREngine  string
	OCRLang    string
	ErrorClass string
}

// Converter turns one raw file into markdown. The gateway writes the result.
type Converter interface {
	Convert(ctx context.Context, req ConvertRequest) (ConvertResult, error)
}

// PageTimeoutOwner means the converter already enforces the per-page timeout
// and the attempt cap. The Docling sidecar does. A test fake does not, so the
// worker applies a short deadline and retries.
type PageTimeoutOwner interface {
	EnforcesPageTimeout() bool
}

// Worker claims uploaded jobs, calls the sidecar for binary files, normalizes
// text in process, and writes sources/inbox only through PutTenantAt.
type Worker struct {
	Vault       *Service
	Converter   Converter
	PageTimeout time.Duration
	MaxAttempts int
	PageBudget  int
	Poll        time.Duration
}

// NewConvertWorker builds the in-process queue. Conversion itself is the
// sidecar HTTP call. Text files are normalized here and do not use OCR.
func NewConvertWorker(st store.StoreIface, root string) *Worker {
	budget := PageBudget()
	return &Worker{
		Vault:       New(st, root),
		Converter:   HTTPConverter{URL: DoclingURL(), PageBudget: budget},
		PageTimeout: PageTimeout(),
		MaxAttempts: PageAttempts(),
		PageBudget:  budget,
		Poll:        2 * time.Second,
	}
}

// Loop reaps jobs left in converting by a previous process, then polls.
// A cancelled context stops the loop. It does not stop the HTTP server.
func (w *Worker) Loop(ctx context.Context) {
	if w == nil {
		return
	}
	w.ReapConverting()
	poll := w.Poll
	if poll <= 0 {
		poll = 2 * time.Second
	}
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			_ = w.Step(ctx)
			timer.Reset(poll)
		}
	}
}

// ReapConverting marks leftover converting jobs failed so a restart cannot
// leave them stuck. Raw files are not removed.
func (w *Worker) ReapConverting() {
	if w == nil || w.Vault == nil {
		return
	}
	jobs, err := w.Vault.Store.ListIngestJobsByStatus(store.IngestConverting)
	if err != nil {
		log.Printf("vault convert: reap list failed")
		return
	}
	for _, job := range jobs {
		if job == nil {
			continue
		}
		job.Status = store.IngestFailed
		job.ErrorClass = ClassInternal
		job.UpdatedAt = time.Now().UTC()
		if _, err := w.Vault.Store.PutIngestJob(*job); err != nil {
			log.Printf("vault convert: reap save failed")
		}
	}
}

// Step converts every uploaded job when the flag is on. The flag off is a no-op
// and does not call the converter. A converter or store error is recorded on
// the job. Step itself does not return that error to the HTTP server.
func (w *Worker) Step(ctx context.Context) error {
	if w == nil || w.Vault == nil || !DoclingEnabled() {
		return nil
	}
	jobs, err := w.Vault.Store.ListIngestJobsByStatus(store.IngestUploaded)
	if err != nil {
		log.Printf("vault convert: list failed")
		return nil
	}
	for _, job := range jobs {
		if ctx.Err() != nil || !DoclingEnabled() {
			return nil
		}
		if job == nil {
			continue
		}
		w.convertJob(ctx, job.ID)
	}
	return nil
}

func (w *Worker) convertJob(ctx context.Context, id string) {
	job, err := w.Vault.Store.ClaimIngestJob(id)
	if err != nil {
		return
	}
	done := false
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("vault convert: panic")
			done = false
		}
		if done {
			return
		}
		current, err := w.Vault.Store.GetIngestJob(id)
		if err != nil || current == nil || current.Status != store.IngestConverting {
			return
		}
		if err := w.failJob(current, ClassInternal); err != nil {
			log.Printf("vault convert: save failed job=%s", id)
		}
	}()
	class := w.convertBody(ctx, job)
	if class != "" {
		if err := w.failJob(job, class); err != nil {
			log.Printf("vault convert: save failed job=%s", id)
			return
		}
		log.Printf("vault convert: job=%s class=%s", id, class)
		done = true
		return
	}
	job.Status = stageAfterConverted(store.IngestConverted)
	job.ErrorClass = ""
	if job.ConvertedAt.IsZero() {
		job.ConvertedAt = time.Now().UTC()
	}
	if err := w.saveJob(job); err != nil {
		log.Printf("vault convert: save failed job=%s", id)
		return
	}
	done = true
}

func (w *Worker) convertBody(ctx context.Context, job *store.IngestJob) string {
	if job == nil {
		return ClassInternal
	}
	logical := strings.TrimSpace(job.RawPath)
	if !strings.HasPrefix(filepath.ToSlash(logical), "raw/") {
		return ClassInternal
	}
	abs, err := w.Vault.namespaced(job.TenantID, logical)
	if err != nil {
		return ClassInternal
	}
	raw, err := readLimited(abs, IngestMaxBytes()+1)
	if err != nil {
		return ClassInternal
	}
	if int64(len(raw)) > IngestMaxBytes() {
		return ClassTooLarge
	}
	if !isConvertible(job.MIME, logical) {
		return ClassUnsupported
	}
	var markdown string
	if isTextMIME(job.MIME, logical) {
		text, err := normalizeText(raw)
		if err != nil {
			return ClassUnsupported
		}
		markdown = text
		job.OCREngine = ""
		job.OCRLang = ""
		job.PageCount = 0
	} else {
		res, err := w.invoke(ctx, ConvertRequest{
			Tenant:      job.TenantID,
			RawPath:     logical,
			MIME:        job.MIME,
			AbsPath:     abs,
			PageTimeout: w.PageTimeout,
			MaxAttempts: w.MaxAttempts,
		})
		if err != nil {
			return ClassInternal
		}
		if res.ErrorClass != "" {
			if !knownClass(res.ErrorClass) {
				return ClassInternal
			}
			return res.ErrorClass
		}
		if res.OCREngine != "" && res.OCREngine != ocrEngineTesseract {
			return ClassInternal
		}
		if res.OCRLang != "" && res.OCRLang != ocrLangVieEng {
			return ClassInternal
		}
		markdown = res.Markdown
		job.OCREngine = ocrEngineTesseract
		job.OCRLang = ocrLangVieEng
		job.PageCount = res.PageCount
		if job.PageCount < 1 {
			job.PageCount = 1
		}
	}
	if !hasLetter(markdown) {
		return ClassEmpty
	}
	parts, err := splitMarkdown(markdown, security.MaxAPIBody)
	if err != nil {
		return ClassTooLarge
	}
	paths := sourcePaths(job.ID, len(parts))
	var total int64
	for _, part := range parts {
		total += int64(len(part))
	}
	over, err := w.overQuota(job.TenantID, total)
	if err != nil {
		return ClassInternal
	}
	if over {
		return ClassTooLarge
	}
	var wrote []string
	for i, part := range parts {
		if _, err := w.Vault.PutTenantAt(job.TenantID, paths[i], []byte(part)); err != nil {
			w.removeWritten(job.TenantID, wrote)
			return ClassInternal
		}
		wrote = append(wrote, paths[i])
	}
	after, err := os.ReadFile(abs)
	if err != nil || !bytes.Equal(after, raw) {
		log.Printf("vault convert: raw changed job=%s", job.ID)
		w.removeWritten(job.TenantID, wrote)
		return ClassInternal
	}
	job.SourcePaths = paths
	job.ConvertedAt = time.Now().UTC()
	return ""
}

func (w *Worker) invoke(ctx context.Context, req ConvertRequest) (ConvertResult, error) {
	if w.Converter == nil {
		return ConvertResult{ErrorClass: ClassNotConfigured}, nil
	}
	if owner, ok := w.Converter.(PageTimeoutOwner); ok && owner.EnforcesPageTimeout() {
		res, err := w.Converter.Convert(ctx, req)
		return mapConvertError(res, err)
	}
	attempts := req.MaxAttempts
	if attempts < 1 {
		attempts = DefaultPageAttempts
	}
	timeout := req.PageTimeout
	if timeout <= 0 {
		timeout = DefaultPageTimeout
	}
	var last ConvertResult
	for i := 0; i < attempts; i++ {
		callCtx, cancel := context.WithTimeout(ctx, timeout)
		res, err := w.Converter.Convert(callCtx, req)
		cancel()
		if err == nil && res.ErrorClass == "" {
			return res, nil
		}
		if err == nil && res.ErrorClass != "" && res.ErrorClass != ClassTimeout {
			return res, nil
		}
		last = res
		if err == nil && res.ErrorClass == ClassTimeout {
			continue
		}
		if errors.Is(err, context.DeadlineExceeded) {
			continue
		}
		return mapConvertError(res, err)
	}
	if last.ErrorClass == "" {
		last.ErrorClass = ClassTimeout
	}
	return last, nil
}

func mapConvertError(res ConvertResult, err error) (ConvertResult, error) {
	if err == nil {
		return res, nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return ConvertResult{ErrorClass: ClassTimeout}, nil
	}
	if unreachableConvert(err) {
		return ConvertResult{ErrorClass: ClassNotConfigured}, nil
	}
	return ConvertResult{ErrorClass: ClassInternal}, nil
}

func (w *Worker) failJob(job *store.IngestJob, class string) error {
	job.Status = store.IngestFailed
	job.ErrorClass = class
	job.SourcePaths = nil
	job.ConvertedAt = time.Time{}
	return w.saveJob(job)
}

func (w *Worker) saveJob(job *store.IngestJob) error {
	job.UpdatedAt = time.Now().UTC()
	_, err := w.Vault.Store.PutIngestJob(*job)
	return err
}

func (w *Worker) overQuota(tenant string, add int64) (bool, error) {
	quota, ok := QuotaBytes()
	if !ok || add < 0 {
		return false, nil
	}
	dir, err := w.Vault.tenantDir(tenant)
	if err != nil {
		return false, err
	}
	used, err := tenantDiskUsage(dir, "")
	if err != nil {
		return false, err
	}
	return used+add > quota, nil
}

func (w *Worker) removeWritten(tenant string, paths []string) {
	for _, p := range paths {
		doc, err := w.Vault.Store.FindVaultDocByPath(tenant, p)
		if err == nil && doc != nil {
			_ = w.Vault.Store.DeleteVaultDoc(doc.ID)
		}
		abs, err := w.Vault.namespaced(tenant, p)
		if err == nil {
			removeRegular(abs)
		}
	}
}

func readLimited(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("path escape")
	}
	return io.ReadAll(io.LimitReader(f, limit))
}

func isTextMIME(mime, rawPath string) bool {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "text/plain", "text/markdown":
		return true
	}
	switch strings.ToLower(filepath.Ext(rawPath)) {
	case ".txt", ".md":
		return true
	}
	return false
}

func isConvertible(mime, rawPath string) bool {
	if isTextMIME(mime, rawPath) {
		return true
	}
	switch strings.ToLower(filepath.Ext(rawPath)) {
	case ".pdf", ".docx", ".xlsx", ".pptx", ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return true
	}
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "application/pdf",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.openxmlformats-officedocument.presentationml.presentation",
		"image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	}
	return false
}

func normalizeText(b []byte) (string, error) {
	if bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}) {
		b = b[3:]
	}
	if !utf8.Valid(b) {
		return "", errors.New("not utf-8")
	}
	s := norm.NFC.String(string(b))
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s, nil
}

// hasLetter is the empty-conversion check. Whitespace, digits, and punctuation
// are empty. A short file that contains one letter is not.
func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func knownClass(class string) bool {
	switch class {
	case ClassTimeout, ClassOCRFailed, ClassUnsupported, ClassEmpty, ClassTooLarge, ClassNotConfigured, ClassInternal:
		return true
	default:
		return false
	}
}

func sourcePaths(jobID string, n int) []string {
	id := safeJobFile(jobID)
	if n <= 1 {
		return []string{"sources/inbox/" + id + ".md"}
	}
	out := make([]string, n)
	for i := range out {
		out[i] = "sources/inbox/" + id + "-part" + twoDigits(i+1) + ".md"
	}
	return out
}

func safeJobFile(id string) string {
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "job"
	}
	return b.String()
}

func twoDigits(n int) string {
	if n < 0 {
		n = 0
	}
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// stageAfterConverted is the no-op hook for SPEC 173, 172, and 174.
// Those flags do not exist yet, so a converted job stays converted.
func stageAfterConverted(status string) string {
	if status != store.IngestConverted {
		return status
	}
	if namingEnabled() {
		return "naming"
	}
	if classifyEnabled() {
		return "classifying"
	}
	if wikiEnabled() {
		return "wiki_updating"
	}
	return store.IngestConverted
}

func namingEnabled() bool   { return false }
func classifyEnabled() bool { return false }
func wikiEnabled() bool     { return false }
