// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
)

const minhAnPhrase = "Hợp đồng thuê nhà tại Đà Nẵng"

type fakeConvert struct {
	mu       sync.Mutex
	calls    int
	hang     bool
	class    string
	markdown string
	engine   string
	lang     string
	pages    int
}

func (f *fakeConvert) Convert(ctx context.Context, req ConvertRequest) (ConvertResult, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.hang {
		<-ctx.Done()
		return ConvertResult{}, ctx.Err()
	}
	if f.class != "" {
		return ConvertResult{ErrorClass: f.class}, nil
	}
	md := f.markdown
	if md == "" {
		body, err := os.ReadFile(req.AbsPath)
		if err != nil {
			return ConvertResult{ErrorClass: ClassInternal}, nil
		}
		if bytes.Contains(body, []byte(minhAnPhrase)) || imageRequest(req) {
			md = "# Minh An\n\n" + minhAnPhrase + "\n"
		} else {
			md = "# Minh An\n\n"
		}
	}
	engine := f.engine
	if engine == "" {
		engine = ocrEngineTesseract
	}
	lang := f.lang
	if lang == "" {
		lang = ocrLangVieEng
	}
	pages := f.pages
	if pages == 0 {
		pages = 1
	}
	return ConvertResult{Markdown: md, PageCount: pages, OCREngine: engine, OCRLang: lang}, nil
}

func (f *fakeConvert) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func imageRequest(req ConvertRequest) bool {
	switch strings.ToLower(filepath.Ext(req.RawPath)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return true
	}
	return strings.HasPrefix(strings.ToLower(req.MIME), "image/")
}

func testWorker(t *testing.T, st store.StoreIface, root string, conv Converter) *Worker {
	t.Helper()
	t.Setenv("GOSO_VAULT_DOCLING", "1")
	t.Setenv("GOSO_VAULT_INGEST_QUOTA_BYTES", "16777216")
	t.Setenv("GOSO_WORKSPACE", "")
	w := NewConvertWorker(st, root)
	w.Converter = conv
	w.PageTimeout = 40 * time.Millisecond
	w.MaxAttempts = 2
	return w
}

func seedRaw(t *testing.T, w *Worker, tenant, name string, body []byte) *store.IngestJob {
	t.Helper()
	res, err := w.Vault.SaveUpload(tenant, name, bytes.NewReader(body))
	if err != nil || res == nil || res.Job == nil || res.Duplicate {
		t.Fatalf("seed %s: %#v %v", name, res, err)
	}
	return res.Job
}

func rawBytes(t *testing.T, w *Worker, job *store.IngestJob) []byte {
	t.Helper()
	abs, err := w.Vault.namespaced(job.TenantID, job.RawPath)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestConvert_MinhAnFixturesAndText(t *testing.T) {
	for _, name := range []string{"memory", "sqlite", "postgres"} {
		t.Run(name, func(t *testing.T) {
			st := openNamedVaultStore(t, name)
			if st == nil {
				t.Skip("postgres DSN unset")
			}
			root := t.TempDir()
			fake := &fakeConvert{}
			w := testWorker(t, st, root, fake)
			phrase := minhAnPhrase
			png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 13, 'I', 'H', 'D', 'R'}
			if bytes.Contains(png, []byte(phrase)) {
				t.Fatal("scan fixture contains the phrase as text")
			}
			cases := []struct {
				file string
				body []byte
				ocr  bool
			}{
				{"hop-dong.pdf", pdfWithPhrase(phrase), true},
				{"hop-dong.docx", officeZip(t, "word/document.xml", "<w:t>"+phrase+"</w:t>"), true},
				{"bang.xlsx", officeZip(t, "xl/workbook.xml", "<x>"+phrase+"</x>"), true},
				{"trinh.pptx", officeZip(t, "ppt/presentation.xml", "<a:t>"+phrase+"</a:t>"), true},
				{"scan.png", png, true},
			}
			var jobs []*store.IngestJob
			for _, tc := range cases {
				if tc.file != "scan.png" && !bytes.Contains(tc.body, []byte(phrase)) {
					t.Fatalf("%s fixture missing phrase", tc.file)
				}
				jobs = append(jobs, seedRaw(t, w, "minh-an", tc.file, tc.body))
			}
			textRaw := []byte("Minh An\r\ne\u0301\r\n")
			txt := seedRaw(t, w, "minh-an", "ghi-chu.txt", textRaw)
			md := seedRaw(t, w, "minh-an", "ghi-chu.md", []byte("# Minh An\r\n\r\n"+phrase+"\r\n"))
			before := map[string][]byte{}
			for _, job := range append(jobs, txt, md) {
				before[job.ID] = rawBytes(t, w, job)
			}
			callsBeforeText := fake.callCount()
			if err := w.Step(context.Background()); err != nil {
				t.Fatal(err)
			}
			if fake.callCount() != len(cases) {
				t.Fatalf("sidecar calls %d want %d", fake.callCount(), len(cases))
			}
			if callsBeforeText != 0 {
				t.Fatal("calls counted before step")
			}
			for i, job := range jobs {
				got, err := st.GetIngestJob(job.ID)
				if err != nil || got.Status != store.IngestConverted || got.ErrorClass != "" {
					t.Fatalf("%s job %#v %v", cases[i].file, got, err)
				}
				if got.OCREngine != ocrEngineTesseract || got.OCRLang != ocrLangVieEng || got.PageCount < 1 || got.ConvertedAt.IsZero() {
					t.Fatalf("%s ocr %#v", cases[i].file, got)
				}
				if len(got.SourcePaths) != 1 || !strings.HasPrefix(got.SourcePaths[0], "sources/inbox/") {
					t.Fatalf("%s paths %#v", cases[i].file, got.SourcePaths)
				}
				doc, err := st.FindVaultDocByPath("minh-an", got.SourcePaths[0])
				if err != nil || !strings.Contains(doc.Body, phrase) {
					t.Fatalf("%s markdown %#v %v", cases[i].file, doc, err)
				}
				raw, _ := json.Marshal(got)
				if bytes.Contains(raw, []byte(phrase)) {
					t.Fatalf("%s job stored the document text", cases[i].file)
				}
				if !bytes.Equal(rawBytes(t, w, got), before[job.ID]) {
					t.Fatalf("%s raw changed", cases[i].file)
				}
			}
			for _, job := range []*store.IngestJob{txt, md} {
				got, err := st.GetIngestJob(job.ID)
				if err != nil || got.Status != store.IngestConverted || got.OCREngine != "" || got.OCRLang != "" {
					t.Fatalf("text job %#v %v", got, err)
				}
				doc, err := st.FindVaultDocByPath("minh-an", got.SourcePaths[0])
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(doc.Body, "\r") {
					t.Fatalf("CR remained: %q", doc.Body)
				}
				if !bytes.Equal(rawBytes(t, w, got), before[job.ID]) {
					t.Fatal("text raw changed")
				}
			}
			txtDoc, err := st.FindVaultDocByPath("minh-an", "sources/inbox/"+safeJobFile(txt.ID)+".md")
			if err != nil || txtDoc == nil || txtDoc.Body != "Minh An\né\n" {
				body := ""
				if txtDoc != nil {
					body = txtDoc.Body
				}
				t.Fatalf("normalized %q %v", body, err)
			}
			if stageAfterConverted(store.IngestConverted) != store.IngestConverted {
				t.Fatal("later specs ran")
			}
		})
	}
}

func TestConvert_EmptyKeepsRawAndShortFileSurvives(t *testing.T) {
	root := t.TempDir()
	fake := &fakeConvert{markdown: " \n\t...\n"}
	w := testWorker(t, store.New(), root, fake)
	job := seedRaw(t, w, "minh-an", "trong.pdf", pdfWithPhrase(minhAnPhrase))
	before := rawBytes(t, w, job)
	if err := w.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := w.Vault.Store.GetIngestJob(job.ID)
	if err != nil || got.Status != store.IngestFailed || got.ErrorClass != ClassEmpty || len(got.SourcePaths) != 0 {
		t.Fatalf("empty %#v %v", got, err)
	}
	if !bytes.Equal(rawBytes(t, w, got), before) {
		t.Fatal("raw removed or changed")
	}
	if _, err := os.Stat(filepath.Join(root, "t", "minh-an", filepath.FromSlash(got.RawPath))); err != nil {
		t.Fatal(err)
	}
	short := seedRaw(t, w, "minh-an", "ngan.txt", []byte("Ơ\n"))
	digits := seedRaw(t, w, "minh-an", "so.txt", []byte("123\n"))
	fake.markdown = ""
	if err := w.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err = w.Vault.Store.GetIngestJob(short.ID)
	if err != nil || got.Status != store.IngestConverted {
		t.Fatalf("short %#v %v", got, err)
	}
	doc, err := w.Vault.Store.FindVaultDocByPath("minh-an", got.SourcePaths[0])
	if err != nil || doc == nil || doc.Body != "Ơ\n" {
		body := ""
		if doc != nil {
			body = doc.Body
		}
		t.Fatalf("short body %q %v", body, err)
	}
	got, err = w.Vault.Store.GetIngestJob(digits.ID)
	if err != nil || got.Status != store.IngestFailed || got.ErrorClass != ClassEmpty {
		t.Fatalf("digits %#v %v", got, err)
	}
}

func TestConvert_FlagOffDoesNotCallSidecar(t *testing.T) {
	root := t.TempDir()
	fake := &fakeConvert{}
	w := testWorker(t, store.New(), root, fake)
	t.Setenv("GOSO_VAULT_DOCLING", "")
	job := seedRaw(t, w, "minh-an", "hop-dong.pdf", pdfWithPhrase(minhAnPhrase))
	if err := w.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := w.Vault.Store.GetIngestJob(job.ID)
	if err != nil || got.Status != store.IngestUploaded || fake.callCount() != 0 {
		t.Fatalf("flag off %#v calls=%d %v", got, fake.callCount(), err)
	}
}

func TestConvert_SidecarMissingFailsClosed(t *testing.T) {
	root := t.TempDir()
	closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closedURL := closed.URL
	closed.Close()
	w := testWorker(t, store.New(), root, HTTPConverter{URL: closedURL, PageBudget: 1})
	job := seedRaw(t, w, "minh-an", "hop-dong.pdf", pdfWithPhrase(minhAnPhrase))
	before := rawBytes(t, w, job)
	if err := w.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := w.Vault.Store.GetIngestJob(job.ID)
	if err != nil || got.Status != store.IngestFailed || got.ErrorClass != ClassNotConfigured {
		t.Fatalf("down %#v %v", got, err)
	}
	if !bytes.Equal(rawBytes(t, w, got), before) {
		t.Fatal("raw changed")
	}
	empty := testWorker(t, store.New(), t.TempDir(), HTTPConverter{URL: ""})
	other := seedRaw(t, empty, "minh-an", "hop-dong.pdf", pdfWithPhrase(minhAnPhrase))
	if err := empty.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err = empty.Vault.Store.GetIngestJob(other.ID)
	if err != nil || got.ErrorClass != ClassNotConfigured || got.Status != store.IngestFailed {
		t.Fatalf("empty url %#v %v", got, err)
	}
}

func TestConvert_HungPageTimesOut(t *testing.T) {
	root := t.TempDir()
	fake := &fakeConvert{hang: true}
	w := testWorker(t, store.New(), root, fake)
	w.PageTimeout = 30 * time.Millisecond
	w.MaxAttempts = 2
	job := seedRaw(t, w, "minh-an", "treo.pdf", pdfWithPhrase(minhAnPhrase))
	start := time.Now()
	if err := w.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("job stayed in converting for %s", time.Since(start))
	}
	got, err := w.Vault.Store.GetIngestJob(job.ID)
	if err != nil || got.Status != store.IngestFailed || got.ErrorClass != ClassTimeout {
		t.Fatalf("timeout %#v %v", got, err)
	}
	if fake.callCount() != 2 {
		t.Fatalf("attempts %d", fake.callCount())
	}
}

func TestConvert_LargeMarkdownSplitsOnHeadings(t *testing.T) {
	var b strings.Builder
	b.WriteString("# Minh An\n\n")
	for b.Len() < security.MaxAPIBody+80 {
		b.WriteString("ế")
	}
	b.WriteString("\n\n## Tiếp theo\n\n" + minhAnPhrase + "\n")
	body := b.String()
	parts, err := splitMarkdown(body, security.MaxAPIBody)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) < 2 || strings.Join(parts, "") != body {
		t.Fatalf("parts %d join", len(parts))
	}
	for _, part := range parts {
		if len(part) > security.MaxAPIBody || !utf8.ValidString(part) {
			t.Fatalf("part len %d valid %v", len(part), utf8.ValidString(part))
		}
	}
	root := t.TempDir()
	fake := &fakeConvert{markdown: body}
	w := testWorker(t, store.New(), root, fake)
	job := seedRaw(t, w, "minh-an", "dai.pdf", pdfWithPhrase(minhAnPhrase))
	if err := w.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := w.Vault.Store.GetIngestJob(job.ID)
	if err != nil || got.Status != store.IngestConverted || len(got.SourcePaths) != len(parts) {
		t.Fatalf("split job %#v %v", got, err)
	}
	var joined strings.Builder
	for i, path := range got.SourcePaths {
		if !strings.Contains(path, "-part") {
			t.Fatalf("path %s", path)
		}
		doc, err := w.Vault.Store.FindVaultDocByPath("minh-an", path)
		if err != nil || len(doc.Body) > security.MaxAPIBody {
			t.Fatalf("part %d %#v %v", i, doc, err)
		}
		joined.WriteString(doc.Body)
	}
	if joined.String() != body || !strings.Contains(joined.String(), minhAnPhrase) {
		t.Fatal("rejoined markdown mismatch")
	}
}

func TestConvert_WrongEngineAndClasses(t *testing.T) {
	root := t.TempDir()
	fake := &fakeConvert{engine: "easyocr", lang: "vie+eng"}
	w := testWorker(t, store.New(), root, fake)
	job := seedRaw(t, w, "minh-an", "hop-dong.pdf", pdfWithPhrase(minhAnPhrase))
	if err := w.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := w.Vault.Store.GetIngestJob(job.ID)
	if err != nil || got.Status != store.IngestFailed || got.ErrorClass != ClassInternal {
		t.Fatalf("easyocr %#v %v", got, err)
	}
	for _, class := range []string{ClassOCRFailed, ClassUnsupported, ClassTooLarge} {
		fake.class = class
		fake.engine = ""
		next := seedRaw(t, w, "minh-an", class+".pdf", pdfWithPhrase(minhAnPhrase+class))
		if err := w.Step(context.Background()); err != nil {
			t.Fatal(err)
		}
		got, err = w.Vault.Store.GetIngestJob(next.ID)
		if err != nil || got.ErrorClass != class || got.Status != store.IngestFailed {
			t.Fatalf("%s %#v %v", class, got, err)
		}
	}
}

func TestConvert_QuotaBlocksSources(t *testing.T) {
	root := t.TempDir()
	fake := &fakeConvert{}
	w := testWorker(t, store.New(), root, fake)
	body := pdfWithPhrase(minhAnPhrase)
	t.Setenv("GOSO_VAULT_INGEST_QUOTA_BYTES", "1048576")
	job := seedRaw(t, w, "minh-an", "hop-dong.pdf", body)
	t.Setenv("GOSO_VAULT_INGEST_QUOTA_BYTES", "1")
	if err := w.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := w.Vault.Store.GetIngestJob(job.ID)
	if err != nil || got.Status != store.IngestFailed || got.ErrorClass != ClassTooLarge {
		t.Fatalf("quota %#v %v", got, err)
	}
	matches, _ := filepath.Glob(filepath.Join(root, "t", "minh-an", "sources", "inbox", "*.md"))
	if len(matches) != 0 {
		t.Fatalf("wrote sources %v", matches)
	}
}

func TestConvert_RetryAfterFailureDoesNotRewriteRaw(t *testing.T) {
	root := t.TempDir()
	fake := &fakeConvert{markdown: " \n"}
	w := testWorker(t, store.New(), root, fake)
	body := pdfWithPhrase(minhAnPhrase)
	job := seedRaw(t, w, "minh-an", "hop-dong.pdf", body)
	if err := w.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	fake.markdown = ""
	again, err := w.Vault.SaveUpload("minh-an", "hop-dong.pdf", bytes.NewReader(body))
	if err != nil || again.Duplicate || again.Job.ID == job.ID || again.Job.Status != store.IngestUploaded {
		t.Fatalf("retry %#v %v", again, err)
	}
	if err := w.Step(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := w.Vault.Store.GetIngestJob(again.Job.ID)
	if err != nil || got.Status != store.IngestConverted {
		t.Fatalf("retried %#v %v", got, err)
	}
	n := 0
	_ = filepath.WalkDir(filepath.Join(root, "t", "minh-an", "raw"), func(path string, d os.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() && !strings.HasPrefix(d.Name(), ".partial-") {
			n++
		}
		return nil
	})
	if n != 1 {
		t.Fatalf("raw files %d", n)
	}
	third, err := w.Vault.SaveUpload("minh-an", "hop-dong.pdf", bytes.NewReader(body))
	if err != nil || !third.Duplicate || third.ExistingID != got.ID {
		t.Fatalf("dedupe after convert %#v %v", third, err)
	}
}

func TestConvert_ReapDoesNotLeaveConverting(t *testing.T) {
	root := t.TempDir()
	w := testWorker(t, store.New(), root, &fakeConvert{})
	job, err := w.Vault.Store.PutIngestJob(store.IngestJob{
		TenantID: "minh-an", SHA256: "abc", OriginalFilename: "a.pdf", MIME: "application/pdf",
		Bytes: 3, RawPath: "raw/abc.pdf", Status: store.IngestConverting,
	})
	if err != nil {
		t.Fatal(err)
	}
	w.ReapConverting()
	got, err := w.Vault.Store.GetIngestJob(job.ID)
	if err != nil || got.Status != store.IngestFailed || got.ErrorClass != ClassInternal {
		t.Fatalf("reap %#v %v", got, err)
	}
}

func TestConvert_HTTPClientOmitsFileBytes(t *testing.T) {
	var seen []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"markdown":"# Minh An\n","page_count":1,"ocr_engine":"tesseract","ocr_lang":"vie+eng"}`)
	}))
	defer srv.Close()
	res, err := (HTTPConverter{URL: srv.URL, PageBudget: 1}).Convert(context.Background(), ConvertRequest{
		Tenant: "minh-an", RawPath: "raw/abc.pdf", MIME: "application/pdf", AbsPath: "/tmp/secret-minh-an.pdf",
		PageTimeout: time.Second, MaxAttempts: 2,
	})
	if err != nil || res.Markdown == "" || res.OCREngine != ocrEngineTesseract {
		t.Fatalf("client %#v %v", res, err)
	}
	if bytes.Contains(seen, []byte("secret-minh-an")) || bytes.Contains(seen, []byte(minhAnPhrase)) {
		t.Fatalf("request leaked a path or body: %s", seen)
	}
	var payload map[string]any
	if err := json.Unmarshal(seen, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["tenant"] != "minh-an" || payload["raw_path"] != "raw/abc.pdf" {
		t.Fatalf("payload %#v", payload)
	}
	if _, ok := payload["abs_path"]; ok {
		t.Fatal("abs path was sent")
	}
}

func TestConvert_HTTPBodyCapIsTooLarge(t *testing.T) {
	prev := maxConvertResponse
	maxConvertResponse = 8
	t.Cleanup(func() { maxConvertResponse = prev })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"markdown":"# Minh An hop dong dai","page_count":1,"ocr_engine":"tesseract","ocr_lang":"vie+eng"}`)
	}))
	defer srv.Close()
	res, err := (HTTPConverter{URL: srv.URL, PageBudget: 1}).Convert(context.Background(), ConvertRequest{
		Tenant: "minh-an", RawPath: "raw/abc.pdf", MIME: "application/pdf", PageTimeout: time.Second, MaxAttempts: 1,
	})
	if err != nil || res.ErrorClass != ClassTooLarge || res.Markdown != "" {
		t.Fatalf("cap %#v %v", res, err)
	}
}

func TestDoclingSidecarPolicy(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	compose, err := os.ReadFile(filepath.Join(root, "compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	block := composeSection(string(compose), "  docling:")
	if block == "" {
		t.Fatal("compose has no docling service")
	}
	for _, want := range []string{
		"profiles:",
		"- docling",
		`user: "10001:10001"`,
		"read_only: true",
		`cpus: "1.0"`,
		"mem_limit: 2g",
		"internal: true",
		"subpath: vault",
		`HF_HUB_OFFLINE: "1"`,
	} {
		if !strings.Contains(block, want) && !strings.Contains(string(compose), want) {
			t.Fatalf("compose missing %s", want)
		}
	}
	if strings.Contains(block, "\n    ports:") || strings.Contains(block, "\n    ports:\n") {
		t.Fatalf("docling publishes a host port:\n%s", block)
	}
	docker, err := os.ReadFile(filepath.Join(root, "sidecar", "docling", "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(docker)
	runtime := text
	if i := strings.LastIndex(text, "\nFROM "); i >= 0 {
		runtime = text[i:]
	}
	for _, banned := range []string{"wget", "curl"} {
		if strings.Contains(strings.ToLower(runtime), banned) {
			t.Fatalf("runtime image mentions %s", banned)
		}
	}
	if !strings.Contains(runtime, "USER 10001:10001") || !strings.Contains(runtime, "HF_HUB_OFFLINE=1") {
		t.Fatal("runtime is not offline and non-root")
	}
	if strings.Contains(runtime, "ENTRYPOINT") && strings.Contains(runtime, "wget") {
		t.Fatal("entrypoint downloads")
	}
	prefetch, err := os.ReadFile(filepath.Join(root, "sidecar", "docling", "prefetch.py"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(prefetch), "with_easyocr=False") || !strings.Contains(string(prefetch), "with_rapidocr=False") {
		t.Fatal("prefetch does not disable easyocr and rapidocr")
	}
	page, err := os.ReadFile(filepath.Join(root, "sidecar", "docling", "goso_docling", "onepage.py"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(page)
	if !strings.Contains(src, "TesseractCliOcrOptions") || !strings.Contains(src, `"vie"`) || !strings.Contains(src, `"eng"`) {
		t.Fatal("onepage is not tesseract vie+eng")
	}
	for _, banned := range []string{"EasyOcr", "RapidOcr", "ocrmac", "easyocr"} {
		if strings.Contains(src, banned) {
			t.Fatalf("onepage mentions %s", banned)
		}
	}
	server, err := os.ReadFile(filepath.Join(root, "sidecar", "docling", "goso_docling", "__main__.py"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(server), "download_models") || strings.Contains(string(server), "wget") || strings.Contains(string(server), "curl") {
		t.Fatal("server can download")
	}
}

func composeSection(file, header string) string {
	i := strings.Index(file, header)
	if i < 0 {
		return ""
	}
	rest := file[i+len(header):]
	if j := strings.Index(rest, "\n  "); j >= 0 {
		// stop at the next top-level service key, which is indented two spaces
		// and is not a list item under this service. The network block is
		// top-level too. Take until the next line that starts a service or a
		// top-level key at column 0.
	}
	end := len(file)
	for _, mark := range []string{"\nnetworks:", "\nvolumes:", "\n  postgres:", "\n  jaeger:", "\n  control-plane:", "\n  gateway:"} {
		if k := strings.Index(file[i+1:], mark); k >= 0 {
			at := i + 1 + k
			if at < end {
				end = at
			}
		}
	}
	return file[i:end]
}

func pdfWithPhrase(phrase string) []byte {
	stream := "BT (" + phrase + ") Tj ET\n"
	return []byte("%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Count 1/Kids[3 0 R]>>endobj\n3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 300 144]/Contents 4 0 R>>endobj\n4 0 obj<</Length " + itoa(len(stream)) + ">>stream\n" + stream + "endstream\nendobj\ntrailer<</Root 1 0 R>>\n%%EOF\n")
}

func officeZip(t *testing.T, name, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, body); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
