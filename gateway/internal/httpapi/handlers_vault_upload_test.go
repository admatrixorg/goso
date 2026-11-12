// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/apikey"
	"github.com/mqglobal/goso/gateway/internal/auditlog"
	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/vault"
)

func TestCriterion1_UploadRouteHasNoV1Alias(t *testing.T) {
	t.Setenv("GOSO_VAULT_UPLOAD", "")
	t.Setenv("GOSO_VAULT_DIR", t.TempDir())
	t.Setenv("GOSO_ADMIN_TOKEN", "")
	h := Router(store.New(), "test")

	api := httptest.NewRecorder()
	h.ServeHTTP(api, httptest.NewRequest(http.MethodPost, "/api/vault/upload", strings.NewReader("small")))
	if api.Code != http.StatusNotFound || !strings.Contains(api.Body.String(), `"not found"`) {
		t.Fatalf("flag off upload %d %s", api.Code, api.Body.String())
	}
	ingest := httptest.NewRecorder()
	h.ServeHTTP(ingest, httptest.NewRequest(http.MethodGet, "/api/vault/ingest/job-1", nil))
	if ingest.Code != http.StatusNotFound || !strings.Contains(ingest.Body.String(), `"not found"`) {
		t.Fatalf("flag off ingest %d %s", ingest.Code, ingest.Body.String())
	}
	for _, path := range []string{"/v1/vault/upload", "/v1/vault/ingest/job-1"} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("small"))
		if strings.Contains(path, "ingest") {
			req.Method = http.MethodGet
		}
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound || strings.Contains(rr.Body.String(), `"not found"`) {
			t.Fatalf("%s must not be registered, got %d %s", path, rr.Code, rr.Body.String())
		}
	}
}

func TestVaultUpload_FlagOffTwoMiBDoesNotWrite(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_UPLOAD", "")
	t.Setenv("GOSO_VAULT_DIR", root)
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-170")
	t.Setenv("GOSO_WORKSPACE", "")
	h := security.LimitAPI(Router(store.New(), "test"))
	body := bytes.Repeat([]byte("a"), 2<<20)
	rr := postUpload(t, h, "admin-170", "", "minh-an.txt", body, "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status %d %s", rr.Code, rr.Body.String())
	}
	if n := countRaw(t, root); n != 0 {
		t.Fatalf("raw files %d", n)
	}
}

func TestVaultUpload_TwentyMiBAndOtherRoutesStayCapped(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_UPLOAD", "1")
	t.Setenv("GOSO_VAULT_INGEST_QUOTA_BYTES", "67108864")
	t.Setenv("GOSO_VAULT_DIR", root)
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-170")
	t.Setenv("GOSO_WORKSPACE", "")
	t.Setenv("GOSO_VAULT_INGEST_MAX_BYTES", "")
	h := security.LimitAPI(Router(store.New(), "test"))

	body := bytes.Repeat([]byte("a"), 20<<20)
	rr := postUpload(t, h, "admin-170", "", "minh-an.txt", body, "")
	if rr.Code != http.StatusCreated {
		t.Fatalf("20 MiB %d %s", rr.Code, rr.Body.String())
	}
	var resp uploadResp
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	if resp.Job.Status != "uploaded" || resp.Job.Bytes != int64(len(body)) || resp.Job.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("job %+v", resp.Job)
	}
	raw := filepath.Join(root, "t", "default", filepath.FromSlash(resp.Job.RawPath))
	info, err := os.Stat(raw)
	if err != nil || info.Size() != int64(len(body)) {
		t.Fatalf("raw file %v %v", info, err)
	}

	over := bytes.Repeat([]byte("b"), security.MaxAPIBody+64)
	docs := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/vault/docs", bytes.NewReader(over))
	req.Header.Set("Authorization", "Bearer admin-170")
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(docs, req)
	if docs.Code == http.StatusCreated || docs.Code == http.StatusOK {
		t.Fatalf("other route accepted a body over 1 MiB: %d %s", docs.Code, docs.Body.String())
	}
}

func TestVaultUpload_AuthTenantBeforeBody(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_UPLOAD", "1")
	t.Setenv("GOSO_VAULT_INGEST_QUOTA_BYTES", "67108864")
	t.Setenv("GOSO_VAULT_DIR", root)
	t.Setenv("GOSO_WORKSPACE", "")
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-170")
	t.Setenv("GOSO_VIEW_TOKEN", "view-170")
	h := Router(store.New(), "test")

	for _, tc := range []struct {
		name  string
		token string
		want  int
	}{
		{"missing", "", http.StatusUnauthorized},
		{"wrong", "nope", http.StatusUnauthorized},
		{"view", "view-170", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/vault/upload", boomBody{t: t})
			req.ContentLength = 20 << 20
			req.Header.Set("Content-Type", "multipart/form-data; boundary=minhan")
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status %d %s", rr.Code, rr.Body.String())
			}
			if strings.Contains(rr.Body.String(), "minhan") && tc.name != "missing" {
				t.Fatalf("error leaked a name: %s", rr.Body.String())
			}
		})
	}
	if n := countRaw(t, root); n != 0 {
		t.Fatalf("raw files %d", n)
	}

	prev := apikey.Default()
	reg := apikey.New()
	apikey.SetDefault(reg)
	t.Cleanup(func() { apikey.SetDefault(prev) })
	readKey, err := reg.Create(apikey.Input{Name: "minh-an-read", Scopes: []string{"read"}})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/vault/upload", boomBody{t: t})
	req.ContentLength = 20 << 20
	req.Header.Set("Authorization", "Bearer "+readKey.Secret)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=minhan")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("read key %d %s", rr.Code, rr.Body.String())
	}

	writeKey, err := reg.Create(apikey.Input{Name: "minh-an-write", Scopes: []string{"write"}})
	if err != nil {
		t.Fatal(err)
	}
	small := []byte("%PDF-1.4\n% Minh An write\n")
	rr = postUpload(t, h, writeKey.Secret, "", "hop-dong.pdf", small, "")
	if rr.Code != http.StatusCreated {
		t.Fatalf("write key %d %s", rr.Code, rr.Body.String())
	}
	rr = postUpload(t, h, "admin-170", "", "admin.pdf", []byte("%PDF-1.4\n% Minh An admin\n"), "")
	if rr.Code != http.StatusCreated {
		t.Fatalf("admin %d %s", rr.Code, rr.Body.String())
	}
}

func TestVaultUpload_MinhAnFilesDuplicateAndQuota(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_UPLOAD", "1")
	t.Setenv("GOSO_VAULT_INGEST_QUOTA_BYTES", "1048576")
	t.Setenv("GOSO_VAULT_DIR", root)
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-170")
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_WORKSPACE", "")
	al := auditlog.New(128)
	st := store.New()
	h := NewRouter(Options{Store: st, Version: "test", Audit: al})

	var maxChunk, calls, total int
	vault.UploadChunkHook = func(n int) {
		calls++
		total += n
		if n > maxChunk {
			maxChunk = n
		}
	}
	t.Cleanup(func() { vault.UploadChunkHook = nil })

	text := bytes.Repeat([]byte("Minh An\n"), 8000)
	cases := []struct {
		name string
		file string
		body []byte
	}{
		{"pdf", "hop-dong.pdf", []byte("%PDF-1.4\n% Minh An hop dong\n")},
		{"docx", "hop-dong.docx", officeZip(t, "word/document.xml", "<w>Minh An</w>")},
		{"xlsx", "bang.xlsx", officeZip(t, "xl/workbook.xml", "<x>Minh An</x>")},
		{"pptx", "trinh.pptx", officeZip(t, "ppt/presentation.xml", "<p>Minh An</p>")},
		{"png", "anh.png", append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, []byte("Minh An")...)},
		{"jpeg", "anh.jpg", append([]byte{0xff, 0xd8, 0xff}, []byte("Minh An")...)},
		{"gif", "anh.gif", append([]byte("GIF89a"), []byte("Minh An")...)},
		{"webp", "anh.webp", append([]byte("RIFF\x00\x00\x00\x00WEBP"), []byte("Minh An")...)},
		{"txt", "ghi-chu.txt", text},
		{"md", "ghi-chu.md", []byte("# Minh An\n\nHop dong\n")},
	}
	var first uploadResp
	for _, tc := range cases {
		rr := postUpload(t, h, "admin-170", "minh-an", tc.file, tc.body, "")
		if rr.Code != http.StatusCreated {
			t.Fatalf("%s %d %s", tc.name, rr.Code, rr.Body.String())
		}
		var resp uploadResp
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(tc.body)
		if resp.Job.Status != "uploaded" || resp.Job.TenantID != "minh-an" || resp.Job.SHA256 != hex.EncodeToString(sum[:]) || resp.Job.Bytes != int64(len(tc.body)) || resp.Job.OriginalFilename != tc.file {
			t.Fatalf("%s job %+v", tc.name, resp.Job)
		}
		raw := filepath.Join(root, "t", "minh-an", filepath.FromSlash(resp.Job.RawPath))
		got, err := os.ReadFile(raw)
		if err != nil || !bytes.Equal(got, tc.body) {
			t.Fatalf("%s raw %v", tc.name, err)
		}
		if _, err := st.FindVaultDocByPath("minh-an", resp.Job.RawPath); err == nil {
			t.Fatalf("%s created a vault_docs row", tc.name)
		}
		if tc.name == "pdf" {
			first = resp
		}
	}
	if calls < 2 || maxChunk > 32<<10 || total < len(text) {
		t.Fatalf("stream calls=%d max=%d total=%d", calls, maxChunk, total)
	}
	for _, rel := range []string{
		"raw", "sources/inbox", "sources/projects", "sources/areas", "sources/resources", "sources/archive", "wiki/topics",
		"wiki/schema.md", "wiki/index.md", "wiki/log.md",
	} {
		if _, err := os.Stat(filepath.Join(root, "t", "minh-an", filepath.FromSlash(rel))); err != nil {
			t.Fatalf("tree %s: %v", rel, err)
		}
	}

	again := postUpload(t, h, "admin-170", "minh-an", "hop-dong.pdf", cases[0].body, "")
	if again.Code != http.StatusOK {
		t.Fatalf("duplicate %d %s", again.Code, again.Body.String())
	}
	var dup uploadResp
	if err := json.Unmarshal(again.Body.Bytes(), &dup); err != nil {
		t.Fatal(err)
	}
	if !dup.Duplicate || dup.Job.Status != "duplicate" || dup.ExistingJobID != first.Job.ID {
		t.Fatalf("dup %+v", dup)
	}
	if n := countRaw(t, filepath.Join(root, "t", "minh-an")); n != len(cases) {
		t.Fatalf("raw count %d", n)
	}

	other := postUpload(t, h, "admin-170", "minh-an-legal", "hop-dong.pdf", cases[0].body, "")
	if other.Code != http.StatusCreated {
		t.Fatalf("other tenant %d %s", other.Code, other.Body.String())
	}
	var legal uploadResp
	if err := json.Unmarshal(other.Body.Bytes(), &legal); err != nil {
		t.Fatal(err)
	}
	if legal.Job.TenantID != "minh-an-legal" || legal.Job.ID == first.Job.ID {
		t.Fatalf("legal %+v", legal.Job)
	}
	cross := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/vault/ingest/"+first.Job.ID, nil)
	req.Header.Set("Authorization", "Bearer admin-170")
	req.Header.Set("X-Goso-Tenant", "minh-an-legal")
	h.ServeHTTP(cross, req)
	if cross.Code != http.StatusNotFound || strings.Contains(cross.Body.String(), first.Job.SHA256) {
		t.Fatalf("cross tenant %d %s", cross.Code, cross.Body.String())
	}
	same := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/vault/ingest/"+first.Job.ID, nil)
	req.Header.Set("Authorization", "Bearer admin-170")
	req.Header.Set("X-Goso-Tenant", "minh-an")
	h.ServeHTTP(same, req)
	if same.Code != http.StatusOK || !strings.Contains(same.Body.String(), first.Job.ID) {
		t.Fatalf("same tenant get %d %s", same.Code, same.Body.String())
	}

	fake := postUpload(t, h, "admin-170", "minh-an", "SECRETNAME.pdf", []byte("Minh An khong phai pdf"), "application/pdf")
	if fake.Code != http.StatusBadRequest || !strings.Contains(fake.Body.String(), "type not allowed") || strings.Contains(fake.Body.String(), "SECRETNAME") {
		t.Fatalf("fake pdf %d %s", fake.Code, fake.Body.String())
	}
	exe := postUpload(t, h, "admin-170", "minh-an", "tool.exe", []byte("MZ Minh An"), "")
	if exe.Code != http.StatusBadRequest || !strings.Contains(exe.Body.String(), "type not allowed") {
		t.Fatalf("exe %d %s", exe.Code, exe.Body.String())
	}
	doc := postUpload(t, h, "admin-170", "minh-an", "cu.doc", []byte("Minh An doc"), "")
	if doc.Code != http.StatusBadRequest || !strings.Contains(doc.Body.String(), "type not supported yet") {
		t.Fatalf("doc %d %s", doc.Code, doc.Body.String())
	}

	page := al.Query(auditlog.Query{Action: "upload", Entity: "vault_ingest"})
	if page.Total < 1 {
		t.Fatal("missing audit")
	}
	rawAudit, _ := json.Marshal(page.Records[0].After)
	if bytes.Contains(rawAudit, []byte("Minh An hop dong")) || bytes.Contains(rawAudit, []byte("SECRETNAME")) {
		t.Fatalf("audit leaked a body or name: %s", rawAudit)
	}

	before := countRaw(t, filepath.Join(root, "t", "minh-an"))
	t.Setenv("GOSO_VAULT_INGEST_QUOTA_BYTES", "1")
	over := postUpload(t, h, "admin-170", "minh-an", "nua.pdf", []byte("%PDF-1.4\n% qua han muc Minh An\n"), "")
	if over.Code != http.StatusConflict || !strings.Contains(over.Body.String(), "quota exceeded") {
		t.Fatalf("quota %d %s", over.Code, over.Body.String())
	}
	if countRaw(t, filepath.Join(root, "t", "minh-an")) != before {
		t.Fatal("quota wrote a raw file")
	}
}

func TestVaultUpload_QuotaRequiredFailClosed(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_UPLOAD", "1")
	t.Setenv("GOSO_VAULT_DIR", root)
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-170")
	t.Setenv("GOSO_WORKSPACE", "")
	for _, v := range []string{"", "0", "-5", "nope"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv("GOSO_VAULT_INGEST_QUOTA_BYTES", v)
			h := Router(store.New(), "test")
			req := httptest.NewRequest(http.MethodPost, "/api/vault/upload", boomBody{t: t})
			req.ContentLength = 20 << 20
			req.Header.Set("Authorization", "Bearer admin-170")
			req.Header.Set("Content-Type", "multipart/form-data; boundary=minhan")
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusServiceUnavailable || !strings.Contains(rr.Body.String(), "quota_required") {
				t.Fatalf("upload %d %s", rr.Code, rr.Body.String())
			}
			hz := httptest.NewRecorder()
			h.ServeHTTP(hz, httptest.NewRequest(http.MethodGet, "/healthz", nil))
			if !strings.Contains(hz.Body.String(), `"quota_required":true`) {
				t.Fatalf("healthz %s", hz.Body.String())
			}
			vh := httptest.NewRecorder()
			reqH := httptest.NewRequest(http.MethodGet, "/api/vault/health", nil)
			reqH.Header.Set("Authorization", "Bearer admin-170")
			h.ServeHTTP(vh, reqH)
			if !strings.Contains(vh.Body.String(), `"quota_required":true`) {
				t.Fatalf("vault health %s", vh.Body.String())
			}
			cfg := httptest.NewRecorder()
			h.ServeHTTP(cfg, httptest.NewRequest(http.MethodGet, "/api/config", nil))
			if !strings.Contains(cfg.Body.String(), `"quota_required"`) {
				t.Fatalf("config %s", cfg.Body.String())
			}
			if n := countRaw(t, root); n != 0 {
				t.Fatalf("raw files %d", n)
			}
		})
	}
}

func TestVaultUpload_FileCap413(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_UPLOAD", "1")
	t.Setenv("GOSO_VAULT_INGEST_QUOTA_BYTES", "1048576")
	t.Setenv("GOSO_VAULT_INGEST_MAX_BYTES", "64")
	t.Setenv("GOSO_VAULT_DIR", root)
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-170")
	t.Setenv("GOSO_WORKSPACE", "")
	h := security.LimitAPI(Router(store.New(), "test"))
	body := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("M"), 80)...)
	rr := postUpload(t, h, "admin-170", "", "lon.pdf", body, "")
	if rr.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rr.Body.String(), "file too large") {
		t.Fatalf("cap %d %s", rr.Code, rr.Body.String())
	}
	if n := countRaw(t, root); n != 0 {
		t.Fatalf("over-cap raw files %d", n)
	}
}

func TestVaultUpload_FlagOffLeavesPutDocs(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_UPLOAD", "")
	t.Setenv("GOSO_VAULT_DIR", root)
	t.Setenv("GOSO_WORKSPACE", "")
	h := Router(store.New(), "test")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/vault/docs", strings.NewReader(`{"title":"Minh An","body":"giu nguyen"}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("put %d %s", rr.Code, rr.Body.String())
	}
	if n := countRaw(t, root); n != 0 {
		t.Fatalf("raw files %d", n)
	}
	hz := httptest.NewRecorder()
	h.ServeHTTP(hz, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if strings.Contains(hz.Body.String(), "quota_required") {
		t.Fatalf("flag off healthz changed: %s", hz.Body.String())
	}
}

func TestVaultUpload_WriteKeyCannotSwitchTenant(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GOSO_VAULT_UPLOAD", "1")
	t.Setenv("GOSO_VAULT_INGEST_QUOTA_BYTES", "1048576")
	t.Setenv("GOSO_VAULT_DIR", root)
	t.Setenv("GOSO_ADMIN_TOKEN", "admin-170")
	t.Setenv("GOSO_MULTI_TENANT", "1")
	t.Setenv("GOSO_WORKSPACE", "")
	prev := apikey.Default()
	reg := apikey.New()
	apikey.SetDefault(reg)
	t.Cleanup(func() { apikey.SetDefault(prev) })
	key, err := reg.Create(apikey.Input{Name: "minh-an-key", Scopes: []string{"write"}})
	if err != nil {
		t.Fatal(err)
	}
	h := Router(store.New(), "test")
	rr := postUpload(t, h, key.Secret, "minh-an", "hop-dong.pdf", []byte("%PDF-1.4\n% Minh An key\n"), "")
	if rr.Code != http.StatusCreated {
		t.Fatalf("status %d %s", rr.Code, rr.Body.String())
	}
	var resp uploadResp
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Job.TenantID != "default" {
		t.Fatalf("api key changed tenant: %+v", resp.Job)
	}
	if _, err := os.Stat(filepath.Join(root, "t", "minh-an")); !os.IsNotExist(err) {
		t.Fatalf("header tenant dir: %v", err)
	}
}

type uploadResp struct {
	Duplicate     bool   `json:"duplicate"`
	ExistingJobID string `json:"existing_job_id"`
	Error         string `json:"error"`
	Job           struct {
		ID               string `json:"id"`
		TenantID         string `json:"tenant_id"`
		Status           string `json:"status"`
		SHA256           string `json:"sha256"`
		Bytes            int64  `json:"bytes"`
		OriginalFilename string `json:"original_filename"`
		RawPath          string `json:"raw_path"`
	} `json:"job"`
}

type boomBody struct{ t *testing.T }

func (b boomBody) Read([]byte) (int, error) {
	b.t.Fatal("request body was read")
	return 0, io.EOF
}

func (b boomBody) Close() error { return nil }

func postUpload(t *testing.T, h http.Handler, token, tenant, filename string, body []byte, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	var part io.Writer
	var err error
	if contentType == "" {
		part, err = w.CreateFormFile("file", filename)
	} else {
		hdr := textproto.MIMEHeader{}
		hdr.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
		hdr.Set("Content-Type", contentType)
		part, err = w.CreatePart(hdr)
	}
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/vault/upload", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if tenant != "" {
		req.Header.Set("X-Goso-Tenant", tenant)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func officeZip(t *testing.T, name, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
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

func countRaw(t *testing.T, root string) int {
	t.Helper()
	n := 0
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".partial-") {
			return nil
		}
		rel := filepath.ToSlash(path)
		if strings.Contains(rel, "/raw/") {
			n++
		}
		return nil
	})
	return n
}
