// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/skill"
	"github.com/mqglobal/goso/gateway/internal/vault"
)

var (
	fakePNG = append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), make([]byte, 64)...)
	fakeOGG = append([]byte("OggS\x00\x02"), make([]byte, 64)...)
	fakeMP3 = append([]byte("ID3\x04\x00"), make([]byte, 64)...)
)

const testKey = "sk-test-0123456789abcdef"

// clearMediaEnv blanks every env var the media tools read and allows
// loopback httptest servers (GOSO_SSRF=0).
func clearMediaEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"GOSO_VISION_PROVIDER", "GOSO_VISION_MODEL", "GOSO_VISION_BASE_URL", "GOSO_VISION_API_KEY", "GOSO_VISION_TYPE",
		"GOSO_STT_URL", "GOSO_STT_API_KEY", "GOSO_STT_MODEL", "GOSO_STT_LANGUAGE",
		"GOSO_IMAGE_URL", "GOSO_IMAGE_API_KEY", "GOSO_IMAGE_MODEL", "GOSO_IMAGE_SIZE",
		"GOSO_TTS_PROVIDER", "GOSO_TTS_API_KEY", "GOSO_TTS_VOICE", "GOSO_TTS_MODEL", "GOSO_TTS_LANGUAGE",
		"GOSO_TTS_REGION", "GOSO_TTS_ENDPOINT", "GOSO_TTS_AUTO_APPLY", "GOSO_TTS_MAX_CHARS",
		"GOSO_TTS_TIMEOUT_MS", "GOSO_TTS_ENABLED",
		"GOSO_VAULT_DOCLING", "GOSO_VAULT_DOCLING_URL", "GOSO_MEDIA",
	} {
		t.Setenv(k, "")
	}
	t.Setenv("GOSO_SSRF", "0")
	t.Setenv("GOSO_ENV", "demo")
	prevTTS, prevStore, prevClient := TTSService, ProviderStore, MediaHTTPClient
	TTSService, ProviderStore, MediaHTTPClient = nil, nil, nil
	t.Cleanup(func() { TTSService, ProviderStore, MediaHTTPClient = prevTTS, prevStore, prevClient })
}

func mediaCall(t *testing.T, ctx context.Context, name string, args map[string]any) (string, map[string]any) {
	t.Helper()
	res, err := Invoke(ctx, name, args, true)
	if err != nil || res == nil {
		t.Fatalf("%s: %v %+v", name, err, res)
	}
	return res.Status, contentMap(res)
}

func contentMap(res *connector.InvokeResult) map[string]any {
	m, _ := res.Content.(map[string]any)
	return m
}

func writeWS(t *testing.T, ws, rel string, data []byte) {
	t.Helper()
	p := filepath.Join(ws, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMedia_NotConfiguredWithoutEnv(t *testing.T) {
	clearMediaEnv(t)
	t.Setenv("GOSO_WORKSPACE", "")
	for _, n := range []string{ToolReadImage, ToolReadDocument, ToolReadAudio, ToolSTT, ToolCreateImage, ToolImageGen, ToolTTS} {
		if Configured(n) {
			t.Fatalf("%s configured with empty env", n)
		}
		res, err := Invoke(context.Background(), n, map[string]any{"path": "a.png", "prompt": "x", "text": "x"}, true)
		if err != nil || res.Status != "not_configured" {
			t.Fatalf("%s %v %+v", n, err, res)
		}
	}
}

func TestMedia_ConfiguredFromEnv(t *testing.T) {
	clearMediaEnv(t)
	useWorkspace(t, t.TempDir())
	t.Setenv("GOSO_VISION_BASE_URL", "http://127.0.0.1:1/v1")
	t.Setenv("GOSO_STT_URL", "http://127.0.0.1:1")
	t.Setenv("GOSO_IMAGE_URL", "http://127.0.0.1:1")
	t.Setenv("GOSO_TTS_PROVIDER", "openai")
	t.Setenv("GOSO_TTS_API_KEY", testKey)
	for _, n := range []string{ToolReadImage, ToolReadDocument, ToolReadAudio, ToolSTT, ToolCreateImage, ToolImageGen, ToolTTS} {
		if !Configured(n) {
			t.Fatalf("%s not configured", n)
		}
	}
}

func TestMedia_UIFlagOffNoNetwork(t *testing.T) {
	clearMediaEnv(t)
	ws := useWorkspace(t, t.TempDir())
	writeWS(t, ws, "a.png", fakePNG)
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
	}))
	defer srv.Close()
	t.Setenv("GOSO_VISION_BASE_URL", srv.URL)
	t.Setenv("GOSO_STT_URL", srv.URL)
	t.Setenv("GOSO_IMAGE_URL", srv.URL)
	for _, n := range []string{ToolReadImage, ToolReadAudio, ToolSTT, ToolCreateImage, ToolTTS} {
		res, err := Invoke(context.Background(), n, map[string]any{"path": "a.png", "prompt": "x", "text": "x"}, false)
		if err != nil || res.Status != "not_configured" {
			t.Fatalf("%s %v %+v", n, err, res)
		}
		if contentMap(res)["hint"] == nil {
			t.Fatalf("%s missing hint %+v", n, res.Content)
		}
	}
	if hits != 0 {
		t.Fatalf("network hit %d", hits)
	}
}

func TestReadImage_VisionFake(t *testing.T) {
	clearMediaEnv(t)
	base := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", base)
	ctx := skill.WithTenant(context.Background(), "minh-an")
	writeWS(t, filepath.Join(base, "minh-an"), "photos/menu.png", fakePNG)
	var got map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		auth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Thực đơn: phở 45.000đ"}}]}`))
	}))
	defer srv.Close()
	t.Setenv("GOSO_VISION_BASE_URL", srv.URL+"/v1")
	t.Setenv("GOSO_VISION_API_KEY", testKey)
	t.Setenv("GOSO_VISION_MODEL", "vision-test")

	status, c := mediaCall(t, ctx, ToolReadImage, map[string]any{"path": "photos/menu.png", "prompt": "Đọc giá món"})
	if status != "ok" || c["text"] != "Thực đơn: phở 45.000đ" || c["mime"] != "image/png" || c["source"] != "photos/menu.png" {
		t.Fatalf("%s %+v", status, c)
	}
	if auth != "Bearer "+testKey || got["model"] != "vision-test" {
		t.Fatalf("auth/model %v %v", auth != "", got["model"])
	}
	raw, _ := json.Marshal(got["messages"])
	if !strings.Contains(string(raw), "data:image/png;base64,"+base64.StdEncoding.EncodeToString(fakePNG)) || !strings.Contains(string(raw), "Đọc giá món") {
		t.Fatalf("messages %s", raw)
	}

	// Another tenant cannot see minh-an's file.
	other := skill.WithTenant(context.Background(), "demo-shop")
	if status, c := mediaCall(t, other, ToolReadImage, map[string]any{"path": "photos/menu.png"}); status != "not_found" {
		t.Fatalf("cross-tenant %s %+v", status, c)
	}
	if status, c := mediaCall(t, ctx, ToolReadImage, map[string]any{"path": "../demo-shop/x.png"}); status != "error" || c["error"] != "path escape" {
		t.Fatalf("escape %s %+v", status, c)
	}
	// Bytes decide the type, not the extension.
	writeWS(t, filepath.Join(base, "minh-an"), "fake.png", []byte("<html>not an image</html>"))
	if status, c := mediaCall(t, ctx, ToolReadImage, map[string]any{"path": "fake.png"}); status != "error" || !strings.Contains(c["error"].(string), "unsupported") {
		t.Fatalf("sniff %s %+v", status, c)
	}
	if status, c := mediaCall(t, ctx, ToolReadImage, map[string]any{}); status != "error" {
		t.Fatalf("no ref %s %+v", status, c)
	}
}

func TestReadImage_URLAndLimits(t *testing.T) {
	clearMediaEnv(t)
	useWorkspace(t, t.TempDir())
	big := append(append([]byte{}, fakePNG...), make([]byte, MaxImageInBytes)...)
	img := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a.png":
			_, _ = w.Write(fakePNG)
		case "/big.png":
			_, _ = w.Write(big)
		default:
			http.NotFound(w, r)
		}
	}))
	defer img.Close()
	vision := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer vision.Close()
	t.Setenv("GOSO_VISION_BASE_URL", vision.URL)
	if status, c := mediaCall(t, context.Background(), ToolReadImage, map[string]any{"url": img.URL + "/a.png"}); status != "ok" || c["text"] != "ok" {
		t.Fatalf("url %s %+v", status, c)
	}
	if status, c := mediaCall(t, context.Background(), ToolReadImage, map[string]any{"url": img.URL + "/big.png"}); status != "error" || c["error"] != "too large" {
		t.Fatalf("big %s %+v", status, c)
	}
	if status, _ := mediaCall(t, context.Background(), ToolReadImage, map[string]any{"url": "file:///etc/passwd"}); status != "error" {
		t.Fatal("file scheme must fail")
	}
}

func TestReadImage_SSRFBlocksLoopback(t *testing.T) {
	clearMediaEnv(t)
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write(fakePNG)
	}))
	defer srv.Close()
	t.Setenv("GOSO_VISION_BASE_URL", "https://vision.example.test/v1")
	t.Setenv("GOSO_SSRF", "1")
	status, c := mediaCall(t, context.Background(), ToolReadImage, map[string]any{"url": srv.URL + "/a.png"})
	if status != "error" || hits != 0 {
		t.Fatalf("%s %+v hits=%d", status, c, hits)
	}
}

func TestReadImage_ProviderErrorRedacted(t *testing.T) {
	clearMediaEnv(t)
	ws := useWorkspace(t, t.TempDir())
	writeWS(t, ws, "a.png", fakePNG)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad key ` + testKey + `"}`))
	}))
	defer srv.Close()
	t.Setenv("GOSO_VISION_BASE_URL", srv.URL)
	t.Setenv("GOSO_VISION_API_KEY", testKey)
	status, c := mediaCall(t, context.Background(), ToolReadImage, map[string]any{"path": "a.png"})
	raw, _ := json.Marshal(c)
	if status != "error" || strings.Contains(string(raw), testKey) {
		t.Fatalf("%s %s", status, raw)
	}
}

type fakeDocConv struct{ res vault.ConvertResult }

func (f fakeDocConv) Convert(ctx context.Context, req vault.ConvertRequest) (vault.ConvertResult, error) {
	return f.res, nil
}

func TestReadDocument(t *testing.T) {
	clearMediaEnv(t)
	t.Setenv("GOSO_VAULT_DIR", t.TempDir())
	ws := useWorkspace(t, t.TempDir())
	writeWS(t, ws, "notes/khach.md", []byte("# Ghi chú\nMinh An hỏi giá gói Pro."))
	// read_document is local: no UI flag needed.
	res, err := Invoke(context.Background(), ToolReadDocument, map[string]any{"path": "notes/khach.md"}, false)
	if err != nil || res.Status != "ok" {
		t.Fatalf("%v %+v", err, res)
	}
	c := contentMap(res)
	if !strings.Contains(c["text"].(string), "Minh An") || c["truncated"] != false || c["mime"] != "text/markdown" {
		t.Fatalf("%+v", c)
	}
	_, c = mediaCall(t, context.Background(), ToolReadDocument, map[string]any{"path": "notes/khach.md", "max_chars": float64(5)})
	if c["text"] != "# Ghi" || c["truncated"] != true || c["chars"].(int) <= 5 {
		t.Fatalf("truncate %+v", c)
	}

	writeWS(t, ws, "bao-gia.pdf", []byte("%PDF-1.7 demo"))
	prev := vault.ExtractConverter
	t.Cleanup(func() { vault.ExtractConverter = prev })
	vault.ExtractConverter = nil
	status, c := mediaCall(t, context.Background(), ToolReadDocument, map[string]any{"path": "bao-gia.pdf"})
	if status != "not_configured" || c["hint"] == nil {
		t.Fatalf("docling off %s %+v", status, c)
	}
	vault.ExtractConverter = fakeDocConv{res: vault.ConvertResult{Markdown: "Báo giá cho Minh An", PageCount: 4}}
	status, c = mediaCall(t, context.Background(), ToolReadDocument, map[string]any{"path": "bao-gia.pdf"})
	if status != "ok" || c["pages"] != 4 || c["mime"] != "application/pdf" || c["text"] != "Báo giá cho Minh An" {
		t.Fatalf("pdf %s %+v", status, c)
	}
	writeWS(t, ws, "song.mp3", fakeMP3)
	if status, c := mediaCall(t, context.Background(), ToolReadDocument, map[string]any{"path": "song.mp3"}); status != "error" {
		t.Fatalf("mp3 %s %+v", status, c)
	}
	if status, c := mediaCall(t, context.Background(), ToolReadDocument, map[string]any{"path": "../x.md"}); status != "error" || c["error"] != "path escape" {
		t.Fatalf("escape %s %+v", status, c)
	}
}

func TestReadAudio_STTFake(t *testing.T) {
	clearMediaEnv(t)
	ws := useWorkspace(t, t.TempDir())
	writeWS(t, ws, "voice/msg.oga", fakeOGG)
	var fields map[string]string
	var fileName, fileType, auth, path string
	var fileBytes []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		auth = r.Header.Get("Authorization")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			http.Error(w, "bad form", 400)
			return
		}
		fields = map[string]string{}
		for k, v := range r.MultipartForm.Value {
			fields[k] = v[0]
		}
		f, h, err := r.FormFile("file")
		if err == nil {
			fileName, fileType = h.Filename, h.Header.Get("Content-Type")
			fileBytes, _ = io.ReadAll(f)
		}
		_, _ = w.Write([]byte(`{"text":" Xin chào, tôi là Minh An "}`))
	}))
	defer srv.Close()
	t.Setenv("GOSO_STT_URL", srv.URL)
	t.Setenv("GOSO_STT_API_KEY", testKey)

	status, c := mediaCall(t, context.Background(), ToolReadAudio, map[string]any{"path": "voice/msg.oga", "hint": "Minh An"})
	if status != "ok" || c["text"] != "Xin chào, tôi là Minh An" || c["language"] != "vi" || c["model"] != "whisper-1" {
		t.Fatalf("%s %+v", status, c)
	}
	if path != "/v1/audio/transcriptions" || auth != "Bearer "+testKey {
		t.Fatalf("path %q auth set=%v", path, auth != "")
	}
	if fields["model"] != "whisper-1" || fields["language"] != "vi" || fields["prompt"] != "Minh An" || fields["response_format"] != "json" {
		t.Fatalf("fields %+v", fields)
	}
	if fileName != "audio.ogg" || fileType != "audio/ogg" || !bytes.Equal(fileBytes, fakeOGG) {
		t.Fatalf("file %q %q %d", fileName, fileType, len(fileBytes))
	}

	t.Setenv("GOSO_STT_URL", srv.URL+"/v1")
	t.Setenv("GOSO_STT_MODEL", "whisper-large-v3")
	status, c = mediaCall(t, context.Background(), ToolSTT, map[string]any{"path": "voice/msg.oga", "language": "en"})
	if status != "ok" || c["language"] != "en" || fields["model"] != "whisper-large-v3" || path != "/v1/audio/transcriptions" {
		t.Fatalf("stt alias %s %+v %+v", status, c, fields)
	}

	writeWS(t, ws, "voice/bad.ogg", []byte("#!AMR\n not supported"))
	if status, c := mediaCall(t, context.Background(), ToolReadAudio, map[string]any{"path": "voice/bad.ogg"}); status != "error" || !strings.Contains(c["error"].(string), "unsupported") {
		t.Fatalf("amr %s %+v", status, c)
	}
	if status, c := mediaCall(t, context.Background(), ToolReadAudio, map[string]any{"path": "voice/msg.oga", "language": "vi; drop"}); status != "error" {
		t.Fatalf("lang %s %+v", status, c)
	}
}

func TestReadAudio_Unauthorized(t *testing.T) {
	clearMediaEnv(t)
	ws := useWorkspace(t, t.TempDir())
	writeWS(t, ws, "a.ogg", fakeOGG)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid key ` + testKey + `"}`))
	}))
	defer srv.Close()
	t.Setenv("GOSO_STT_URL", srv.URL)
	t.Setenv("GOSO_STT_API_KEY", testKey)
	status, c := mediaCall(t, context.Background(), ToolReadAudio, map[string]any{"path": "a.ogg"})
	raw, _ := json.Marshal(c)
	if status != "error" || !strings.Contains(string(raw), "unauthorized") || strings.Contains(string(raw), testKey) {
		t.Fatalf("%s %s", status, raw)
	}
}

func TestCreateImage_Fake(t *testing.T) {
	clearMediaEnv(t)
	base := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", base)
	ctx := skill.WithTenant(context.Background(), "minh-an")
	var got map[string]any
	var path string
	mode := "b64"
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/files/out.png" {
			_, _ = w.Write(fakePNG)
			return
		}
		path = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		switch mode {
		case "b64":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"b64_json": base64.StdEncoding.EncodeToString(fakePNG)}}})
		case "url":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"url": srvURL + "/files/out.png"}}})
		case "text":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"b64_json": base64.StdEncoding.EncodeToString([]byte("hello"))}}})
		}
	}))
	defer srv.Close()
	srvURL = srv.URL
	t.Setenv("GOSO_IMAGE_URL", srv.URL)
	t.Setenv("GOSO_IMAGE_API_KEY", testKey)

	status, c := mediaCall(t, ctx, ToolCreateImage, map[string]any{"prompt": "Poster khai trương quán cà phê", "filename": "Poster Khai Trương!"})
	if status != "ok" || c["mime"] != "image/png" || c["model"] != "gpt-image-1" {
		t.Fatalf("%s %+v", status, c)
	}
	if path != "/v1/images/generations" || got["size"] != "1024x1024" || got["prompt"] != "Poster khai trương quán cà phê" || got["n"] != float64(1) {
		t.Fatalf("req %s %+v", path, got)
	}
	rel := c["path"].(string)
	if !strings.HasPrefix(rel, "generated/images/poster-khai-tr") || !strings.HasSuffix(rel, ".png") {
		t.Fatalf("path %q", rel)
	}
	onDisk, err := os.ReadFile(filepath.Join(base, "minh-an", filepath.FromSlash(rel)))
	if err != nil || !bytes.Equal(onDisk, fakePNG) {
		t.Fatalf("disk %v", err)
	}
	// send_file can resolve the returned path in the same tenant.
	if res, err := Invoke(ctx, ToolSendFile, map[string]any{"path": rel}, false); err != nil || res.Status != "ok" {
		t.Fatalf("send_file %v %+v", err, res)
	}

	mode = "url"
	status, c = mediaCall(t, ctx, ToolImageGen, map[string]any{"prompt": "logo", "size": "512x512"})
	if status != "ok" || got["size"] != "512x512" || !strings.HasPrefix(c["path"].(string), "generated/images/image-") {
		t.Fatalf("url mode %s %+v", status, c)
	}

	mode = "text"
	if status, c := mediaCall(t, ctx, ToolCreateImage, map[string]any{"prompt": "x"}); status != "error" {
		t.Fatalf("non-image %s %+v", status, c)
	}
	if status, c := mediaCall(t, ctx, ToolCreateImage, map[string]any{"prompt": "x", "size": "../../1"}); status != "error" {
		t.Fatalf("size %s %+v", status, c)
	}
	if status, c := mediaCall(t, ctx, ToolCreateImage, map[string]any{}); status != "error" {
		t.Fatalf("prompt %s %+v", status, c)
	}
}

func TestTTS_SavesAudio(t *testing.T) {
	clearMediaEnv(t)
	ws := useWorkspace(t, t.TempDir())
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		_, _ = w.Write(fakeMP3)
	}))
	defer srv.Close()
	if status, c := mediaCall(t, context.Background(), ToolTTS, map[string]any{"text": "xin chào"}); status != "not_configured" || c["hint"] == nil {
		t.Fatalf("unset %s %+v", status, c)
	}
	t.Setenv("GOSO_TTS_PROVIDER", "openai")
	t.Setenv("GOSO_TTS_API_KEY", testKey)
	t.Setenv("GOSO_TTS_ENDPOINT", srv.URL)
	status, c := mediaCall(t, context.Background(), ToolTTS, map[string]any{"text": "Chào Minh An", "voice": "nova", "filename": "loi chao"})
	if status != "ok" || c["mime"] != "audio/mpeg" || c["provider"] != "openai" {
		t.Fatalf("%s %+v", status, c)
	}
	rel := c["path"].(string)
	if !strings.HasPrefix(rel, "generated/audio/loi-chao-") || !strings.HasSuffix(rel, ".mp3") {
		t.Fatalf("path %q", rel)
	}
	if b, err := os.ReadFile(filepath.Join(ws, filepath.FromSlash(rel))); err != nil || !bytes.Equal(b, fakeMP3) {
		t.Fatalf("disk %v", err)
	}
	if got["input"] != "Chào Minh An" || got["voice"] != "nova" {
		t.Fatalf("payload %+v", got)
	}
	if status, _ := mediaCall(t, context.Background(), ToolTTS, map[string]any{"text": "  "}); status != "error" {
		t.Fatal("empty text must error")
	}
	t.Setenv("GOSO_TTS_PROVIDER", "google")
	if status, _ := mediaCall(t, context.Background(), ToolTTS, map[string]any{"text": "x"}); status != "not_configured" {
		t.Fatal("google has no synthesis path")
	}
}

func TestSaveGenerated_RejectsSymlinkedDir(t *testing.T) {
	clearMediaEnv(t)
	ws := useWorkspace(t, t.TempDir())
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(ws, generatedDir)); err != nil {
		t.Skip("symlink unsupported")
	}
	if _, err := saveGenerated(context.Background(), "images", "x", "image", ".png", fakePNG); err == nil {
		t.Fatal("symlinked generated/ must be rejected")
	}
	if ents, _ := os.ReadDir(outside); len(ents) != 0 {
		t.Fatalf("wrote outside: %v", ents)
	}
}

func TestSniffAudio(t *testing.T) {
	cases := map[string]string{
		"OggS....":                     ".ogg",
		"RIFF\x00\x00\x00\x00WAVEfmt ": ".wav",
		"fLaC....":                     ".flac",
		"\x00\x00\x00\x20ftypM4A ":     ".m4a",
		"\x1a\x45\xdf\xa3....":         ".webm",
		"ID3\x04....":                  ".mp3",
		"\xff\xfb\x90\x00":             ".mp3",
	}
	for in, want := range cases {
		if _, ext, ok := sniffAudio([]byte(in)); !ok || ext != want {
			t.Fatalf("%q → %q %v", in, ext, ok)
		}
	}
	for _, bad := range []string{"#!AMR\n", "\xff\xf1\x50\x80", "<html>", ""} {
		if _, _, ok := sniffAudio([]byte(bad)); ok {
			t.Fatalf("%q accepted", bad)
		}
	}
}
