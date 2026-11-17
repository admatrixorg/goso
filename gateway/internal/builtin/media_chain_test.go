// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/skill"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/vault"
)

// chainFixture is a memory store wired as ProviderStore, a workspace and a
// tenant context. Keys are built at runtime.
type chainFixture struct {
	st  *store.Store
	ctx context.Context
	ws  string
}

func newChainFixture(t *testing.T, tenant string) *chainFixture {
	t.Helper()
	clearMediaEnv(t)
	t.Setenv("GOSO_MASTER_KEY", strings.Repeat("ab", 32))
	base := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", base)
	st := store.New()
	ProviderStore = st
	return &chainFixture{st: st, ctx: skill.WithTenant(context.Background(), tenant), ws: filepath.Join(base, tenant)}
}

func (f *chainFixture) provider(t *testing.T, name, tenant, base, model string) {
	t.Helper()
	if _, err := f.st.CreateLLMProvider(store.LLMProvider{Name: name, TenantID: tenant, Type: llm.TypeOpenAICompat, BaseURL: base, Model: model, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := secrets.Put(f.st, llm.APIKeySecretName(name), []byte(testKey)); err != nil {
		t.Fatal(err)
	}
}

func (f *chainFixture) chain(t *testing.T, tenant, tool, settings string) {
	t.Helper()
	if _, err := f.st.PutToolSettings(store.ToolSettings{TenantID: tenant, Tool: tool, Settings: json.RawMessage(settings)}); err != nil {
		t.Fatal(err)
	}
}

func mustNoKey(t *testing.T, v any) {
	t.Helper()
	raw, _ := json.Marshal(v)
	if strings.Contains(string(raw), testKey) {
		t.Fatalf("key leaked: %s", raw)
	}
}

type geminiCall struct {
	path, key string
	body      map[string]any
}

func geminiServer(t *testing.T, reply string) (*httptest.Server, *[]geminiCall) {
	t.Helper()
	var mu sync.Mutex
	calls := &[]geminiCall{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		mu.Lock()
		*calls = append(*calls, geminiCall{path: r.URL.Path, key: r.Header.Get("x-goog-api-key"), body: m})
		mu.Unlock()
		if r.Header.Get("Authorization") != "" {
			http.Error(w, "gemini uses x-goog-api-key", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	return srv, calls
}

func geminiText(s string) string {
	b, _ := json.Marshal(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{map[string]any{"text": s}}}, "finishReason": "STOP"}}})
	return string(b)
}

func TestMediaChain_ReadImageFallsBackAndRedacts(t *testing.T) {
	f := newChainFixture(t, "shop-a")
	writeWS(t, f.ws, "photos/label.png", fakePNG)
	var compatHits int
	compat := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		compatHits++
		if r.URL.Path != "/api/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer "+testKey {
			t.Errorf("compat %s %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"upstream down for ` + testKey + `"}`))
	}))
	defer compat.Close()
	gem, calls := geminiServer(t, geminiText("Price: 45.00"))
	f.provider(t, "router-a", "shop-a", compat.URL+"/api/v1", "router-default")
	f.provider(t, "gem-a", "shop-a", gem.URL+"/v1beta/openai", "gemini-2.5-flash")
	f.chain(t, "shop-a", ToolReadImage, `{"providers":[
		{"provider":"router-a","model":"google/gemini-2.5-flash","max_retries":2,"timeout":30},
		{"provider":"disabled-one","enabled":false},
		{"provider":"gem-a","model":"google/gemini-2.5-flash","params":{"api":"gemini"}}]}`)

	res, err := Invoke(f.ctx, ToolReadImage, map[string]any{"path": "photos/label.png", "prompt": "Read the price"}, true)
	if err != nil || res.Status != "ok" {
		t.Fatalf("%v %+v", err, res)
	}
	c := contentMap(res)
	if c["text"] != "Price: 45.00" || c["provider"] != "gem-a" || c["model"] != "google/gemini-2.5-flash" {
		t.Fatalf("%+v", c)
	}
	if compatHits != 2 {
		t.Fatalf("502 must be retried max_retries=2 times, got %d", compatHits)
	}
	att, _ := c["fallback_from"].([]chainAttempt)
	if len(att) != 1 || att[0].Provider != "router-a" || !strings.Contains(att[0].Error, "http 502") {
		t.Fatalf("attempts %+v", c["fallback_from"])
	}
	mustNoKey(t, c)
	if len(*calls) != 1 {
		t.Fatalf("gemini calls %d", len(*calls))
	}
	call := (*calls)[0]
	if call.path != "/v1beta/models/gemini-2.5-flash:generateContent" || call.key != testKey {
		t.Fatalf("gemini call %s", call.path)
	}
	parts := call.body["contents"].([]any)[0].(map[string]any)["parts"].([]any)
	inline := parts[1].(map[string]any)["inlineData"].(map[string]any)
	if parts[0].(map[string]any)["text"] != "Read the price" || inline["mimeType"] != "image/png" || inline["data"] != base64.StdEncoding.EncodeToString(fakePNG) {
		t.Fatalf("parts %+v", parts)
	}
}

func TestMediaChain_OtherTenantProviderRejected(t *testing.T) {
	f := newChainFixture(t, "shop-a")
	writeWS(t, f.ws, "a.png", fakePNG)
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer srv.Close()
	f.provider(t, "gem-b", "shop-b", srv.URL+"/v1", "m")
	f.chain(t, "shop-a", ToolReadImage, `{"providers":[{"provider":"gem-b"}]}`)
	status, c := mediaCall(t, f.ctx, ToolReadImage, map[string]any{"path": "a.png"})
	if status != "error" || hit || !strings.Contains(c["error"].(string), "not found for this tenant") {
		t.Fatalf("%s %+v hit=%v", status, c, hit)
	}
	// Tenant b's own chain is not visible to tenant a either: a has no
	// chain for read_video.
	f.chain(t, "shop-b", ToolReadVideo, `{"providers":[{"provider":"gem-b"}]}`)
	if status, _ := mediaCall(t, f.ctx, ToolReadVideo, map[string]any{"path": "a.png"}); status != "not_configured" {
		t.Fatalf("video for shop-a %s", status)
	}
	if !ConfiguredForTenant("shop-b", ToolReadVideo) || ConfiguredForTenant("shop-a", ToolReadVideo) {
		t.Fatal("ConfiguredForTenant must follow the tenant chain")
	}
}

func TestMediaChain_ReadDocumentGeminiAndLocalFallback(t *testing.T) {
	f := newChainFixture(t, "shop-a")
	t.Setenv("GOSO_VAULT_DIR", t.TempDir())
	pdf := []byte("%PDF-1.7 quote for a demo shop")
	writeWS(t, f.ws, "docs/quote.pdf", pdf)
	writeWS(t, f.ws, "docs/notes.md", []byte("# Notes\nplain text stays local"))
	gem, calls := geminiServer(t, geminiText("# Quote\nTotal 1.200.000"))
	f.provider(t, "gem-a", "shop-a", gem.URL, "gemini-2.5-flash")
	f.chain(t, "shop-a", ToolReadDocument, `{"providers":[{"provider":"gem-a","params":{"api":"gemini"}}]}`)

	// No UI flag needed; the tenant chain is the opt-in.
	res, err := Invoke(f.ctx, ToolReadDocument, map[string]any{"path": "docs/quote.pdf"}, false)
	if err != nil || res.Status != "ok" {
		t.Fatalf("%v %+v", err, res)
	}
	c := contentMap(res)
	if c["text"] != "# Quote\nTotal 1.200.000" || c["mime"] != "application/pdf" || c["provider"] != "gem-a" {
		t.Fatalf("%+v", c)
	}
	call := (*calls)[0]
	if call.path != "/v1beta/models/gemini-2.5-flash:generateContent" {
		t.Fatalf("path %s", call.path)
	}
	parts := call.body["contents"].([]any)[0].(map[string]any)["parts"].([]any)
	if parts[1].(map[string]any)["inlineData"].(map[string]any)["mimeType"] != "application/pdf" {
		t.Fatalf("parts %+v", parts)
	}

	// Text files never reach the provider.
	_, c = mediaCall(t, f.ctx, ToolReadDocument, map[string]any{"path": "docs/notes.md"})
	if !strings.Contains(c["text"].(string), "stays local") || len(*calls) != 1 {
		t.Fatalf("md %+v calls=%d", c, len(*calls))
	}

	// Provider down: local extraction is the fallback.
	gem.Close()
	prev := vault.ExtractConverter
	t.Cleanup(func() { vault.ExtractConverter = prev })
	vault.ExtractConverter = fakeDocConv{res: vault.ConvertResult{Markdown: "local text", PageCount: 1}}
	status, c := mediaCall(t, f.ctx, ToolReadDocument, map[string]any{"path": "docs/quote.pdf"})
	if status != "ok" || c["text"] != "local text" || c["fallback"] != "local" || c["llm_error"] == "" {
		t.Fatalf("fallback %s %+v", status, c)
	}
	// Both down: the chain error is reported with its attempts.
	vault.ExtractConverter = nil
	status, c = mediaCall(t, f.ctx, ToolReadDocument, map[string]any{"path": "docs/quote.pdf"})
	if status != "error" || c["attempts"] == nil || !strings.Contains(c["error"].(string), "document read failed") {
		t.Fatalf("both down %s %+v", status, c)
	}
	mustNoKey(t, c)
}

func TestMediaChain_ReadVideoFileAPI(t *testing.T) {
	f := newChainFixture(t, "shop-a")
	prevLimit, prevPoll := geminiInlineLimit, geminiPollInterval
	geminiInlineLimit, geminiPollInterval = 16, time.Millisecond
	t.Cleanup(func() { geminiInlineLimit, geminiPollInterval = prevLimit, prevPoll })
	video := append([]byte("\x00\x00\x00\x18ftypisom"), make([]byte, 80)...)
	writeWS(t, f.ws, "clips/demo.mp4", video)

	var mu sync.Mutex
	var steps []string
	var uploaded []byte
	polls := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("x-goog-api-key") != testKey {
			t.Errorf("missing key on %s %s", r.Method, r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/upload/v1beta/files" && r.Header.Get("X-Goog-Upload-Command") == "upload, finalize":
			steps = append(steps, "finalize")
			uploaded = b
			if r.Header.Get("X-Goog-Upload-Offset") != "0" || r.URL.Query().Get("upload_id") != "u1" {
				http.Error(w, "bad finalize", http.StatusBadRequest)
				return
			}
			// PROCESSING first, so the client has to poll once.
			_, _ = w.Write([]byte(`{"file":{"name":"files/vid-1","uri":"` + srv.URL + `/v1beta/files/vid-1","state":"PROCESSING","mimeType":"video/mp4"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/upload/v1beta/files":
			steps = append(steps, "start")
			if r.Header.Get("X-Goog-Upload-Protocol") != "resumable" || r.Header.Get("X-Goog-Upload-Command") != "start" ||
				r.Header.Get("X-Goog-Upload-Header-Content-Type") != "video/mp4" || r.Header.Get("X-Goog-Upload-Header-Content-Length") != "92" {
				t.Errorf("start headers %v", r.Header)
			}
			w.Header().Set("X-Goog-Upload-URL", srv.URL+"/upload/v1beta/files?upload_id=u1")
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1beta/files/vid-1":
			steps = append(steps, "poll")
			polls++
			_, _ = w.Write([]byte(`{"name":"files/vid-1","uri":"` + srv.URL + `/v1beta/files/vid-1","state":"ACTIVE"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1beta/models/gemini-2.5-flash:generateContent":
			steps = append(steps, "generate")
			var m map[string]any
			_ = json.Unmarshal(b, &m)
			parts := m["contents"].([]any)[0].(map[string]any)["parts"].([]any)
			fd, _ := parts[1].(map[string]any)["fileData"].(map[string]any)
			if fd["mimeType"] != "video/mp4" || fd["fileUri"] != srv.URL+"/v1beta/files/vid-1" {
				t.Errorf("fileData %+v", parts)
			}
			_, _ = w.Write([]byte(geminiText("A person unboxes a phone.")))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1beta/files/vid-1":
			steps = append(steps, "delete")
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.String())
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	f.provider(t, "gem-a", "shop-a", srv.URL+"/v1beta", "gemini-2.5-flash")
	f.chain(t, "shop-a", ToolReadVideo, `{"providers":[{"provider":"gem-a","params":{"api":"gemini"}}]}`)

	status, c := mediaCall(t, f.ctx, ToolReadVideo, map[string]any{"path": "clips/demo.mp4"})
	if status != "ok" || c["text"] != "A person unboxes a phone." || c["mime"] != "video/mp4" || c["provider"] != "gem-a" {
		t.Fatalf("%s %+v", status, c)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(steps, ",") != "start,finalize,poll,generate,delete" || string(uploaded) != string(video) || polls != 1 {
		t.Fatalf("steps %v polls %d uploaded %d", steps, polls, len(uploaded))
	}
}

func TestMediaChain_ReadVideoRejectsAudioAndNoChain(t *testing.T) {
	f := newChainFixture(t, "shop-a")
	status, c := mediaCall(t, f.ctx, ToolReadVideo, map[string]any{"path": "x.mp4"})
	if status != "not_configured" || !strings.Contains(c["hint"].(string), "/api/tools/builtin/read_video/settings") {
		t.Fatalf("%s %+v", status, c)
	}
	m4a := append([]byte("\x00\x00\x00\x18ftypM4A "), make([]byte, 40)...)
	if _, _, ok := sniffVideo(m4a); ok {
		t.Fatal("m4a is not video")
	}
	for _, v := range [][]byte{
		append([]byte("\x00\x00\x00\x14ftypqt  "), make([]byte, 8)...),
		{0x1a, 0x45, 0xdf, 0xa3, 0, 0},
		append([]byte("RIFF\x00\x00\x00\x00AVI "), make([]byte, 4)...),
	} {
		if _, _, ok := sniffVideo(v); !ok {
			t.Fatalf("video %q", v[:8])
		}
	}
}

func TestMediaChain_CreateImageOpenRouterChat(t *testing.T) {
	f := newChainFixture(t, "shop-a")
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		reply, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]any{
			"role": "assistant", "content": "Here is your image.",
			"images": []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + base64.StdEncoding.EncodeToString(fakePNG)}}},
		}}}})
		_, _ = w.Write(reply)
	}))
	defer srv.Close()
	f.provider(t, "router-a", "shop-a", srv.URL+"/api/v1", "router-default")
	f.chain(t, "shop-a", ToolCreateImage, `{"providers":[{"provider":"router-a","model":"chat-model","params":{"image_model":"google/gemini-2.5-flash-image"}}]}`)

	status, c := mediaCall(t, f.ctx, ToolCreateImage, map[string]any{"prompt": "a red bicycle", "aspect_ratio": "16:9", "filename": "bike"})
	if status != "ok" || c["mime"] != "image/png" || c["model"] != "google/gemini-2.5-flash-image" || c["provider"] != "router-a" {
		t.Fatalf("%s %+v", status, c)
	}
	rel := c["path"].(string)
	if !strings.HasPrefix(rel, "generated/images/bike-") || !strings.HasSuffix(rel, ".png") {
		t.Fatalf("path %s", rel)
	}
	if b, err := os.ReadFile(filepath.Join(f.ws, filepath.FromSlash(rel))); err != nil || string(b) != string(fakePNG) {
		t.Fatalf("saved %v", err)
	}
	mods, _ := got["modalities"].([]any)
	ic, _ := got["image_config"].(map[string]any)
	if got["model"] != "google/gemini-2.5-flash-image" || len(mods) != 2 || mods[0] != "image" || ic["aspect_ratio"] != "16:9" {
		t.Fatalf("request %+v", got)
	}
	// Image alias uses the same settings.
	if status, _ := mediaCall(t, f.ctx, ToolImageGen, map[string]any{"prompt": "a blue bicycle"}); status != "ok" {
		t.Fatalf("image_gen %s", status)
	}
	if status, c := mediaCall(t, f.ctx, ToolCreateImage, map[string]any{"prompt": "x", "aspect_ratio": "wide"}); status != "error" {
		t.Fatalf("bad aspect %s %+v", status, c)
	}
}

func TestMediaChain_CreateImageGeminiNative(t *testing.T) {
	f := newChainFixture(t, "shop-a")
	reply, _ := json.Marshal(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"parts": []any{
		map[string]any{"text": "done"},
		map[string]any{"inlineData": map[string]any{"mimeType": "image/png", "data": base64.StdEncoding.EncodeToString(fakePNG)}},
	}}}}})
	gem, calls := geminiServer(t, string(reply))
	f.provider(t, "gem-a", "shop-a", gem.URL+"/v1beta/openai", "gemini-2.5-flash")
	f.chain(t, "shop-a", ToolCreateImage, `{"providers":[{"provider":"gem-a","model":"gemini-2.5-flash-image","params":{"api":"gemini","aspect_ratio":"1:1"}}]}`)
	status, c := mediaCall(t, f.ctx, ToolCreateImage, map[string]any{"prompt": "a logo"})
	if status != "ok" || c["model"] != "gemini-2.5-flash-image" {
		t.Fatalf("%s %+v", status, c)
	}
	call := (*calls)[0]
	cfg := call.body["generationConfig"].(map[string]any)
	if call.path != "/v1beta/models/gemini-2.5-flash-image:generateContent" || cfg["imageConfig"].(map[string]any)["aspectRatio"] != "1:1" || len(cfg["responseModalities"].([]any)) != 2 {
		t.Fatalf("call %s %+v", call.path, cfg)
	}
}

func TestMediaChain_ReadAudioTranscriptionsAndGemini(t *testing.T) {
	f := newChainFixture(t, "shop-a")
	writeWS(t, f.ws, "voice/note.ogg", fakeOGG)
	var fields map[string]string
	whisper := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/transcriptions" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
		}
		fields = map[string]string{"model": r.FormValue("model"), "language": r.FormValue("language")}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"model not available"}`))
	}))
	defer whisper.Close()
	gem, calls := geminiServer(t, geminiText("xin chao"))
	f.provider(t, "stt-a", "shop-a", whisper.URL, "whisper-1")
	f.provider(t, "gem-a", "shop-a", gem.URL, "gemini-2.5-flash")
	f.chain(t, "shop-a", ToolReadAudio, `{"providers":[{"provider":"stt-a","max_retries":3},{"provider":"gem-a","params":{"api":"gemini"}}]}`)
	// stt alias reads the read_audio settings.
	status, c := mediaCall(t, f.ctx, ToolSTT, map[string]any{"path": "voice/note.ogg", "language": "vi"})
	if status != "ok" || c["text"] != "xin chao" || c["provider"] != "gem-a" || c["language"] != "vi" {
		t.Fatalf("%s %+v", status, c)
	}
	if fields["model"] != "whisper-1" || fields["language"] != "vi" {
		t.Fatalf("multipart %+v", fields)
	}
	att := c["fallback_from"].([]chainAttempt)
	if len(att) != 1 || !strings.Contains(att[0].Error, "http 400") {
		t.Fatalf("a 400 is not retried and moves on: %+v", att)
	}
	parts := (*calls)[0].body["contents"].([]any)[0].(map[string]any)["parts"].([]any)
	if parts[1].(map[string]any)["inlineData"].(map[string]any)["mimeType"] != "audio/ogg" {
		t.Fatalf("parts %+v", parts)
	}
}

func TestMediaChain_EnvFallbackWithoutTenantSettings(t *testing.T) {
	f := newChainFixture(t, "shop-a")
	writeWS(t, f.ws, "a.png", fakePNG)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"env path"}}]}`))
	}))
	defer srv.Close()
	t.Setenv("GOSO_VISION_BASE_URL", srv.URL+"/v1")
	t.Setenv("GOSO_VISION_API_KEY", testKey)
	status, c := mediaCall(t, f.ctx, ToolReadImage, map[string]any{"path": "a.png"})
	if status != "ok" || c["text"] != "env path" || c["provider"] != nil {
		t.Fatalf("%s %+v", status, c)
	}
}

func TestParseMediaSettings(t *testing.T) {
	ok := `{"providers":[{"provider_id":"0b6c","provider":"gemini","model":"gemini-2.5-flash","enabled":true,"timeout":600,"max_retries":1,"params":{"image_model":"x","max_tokens":2048}}]}`
	s, err := ParseMediaSettings([]byte(ok))
	if err != nil || len(s.Providers) != 1 || !s.Providers[0].On() {
		t.Fatalf("%v %+v", err, s)
	}
	for _, bad := range []string{
		`{"providers":[{"provider":""}]}`,
		`{"providers":[{"provider":"a","timeout":901}]}`,
		`{"providers":[{"provider":"a","max_retries":6}]}`,
		`{"providers":[{"provider":"a","params":{"api_key":"x"}}]}`,
		`{"providers":[{"provider":"a","params":{"Access-Token":"x"}}]}`,
		`{"providers":[{"provider":"a","extra":1}]}`,
		`{"provider":"a"}`,
	} {
		if _, err := ParseMediaSettings([]byte(bad)); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
	if SettingsTool("stt") != ToolReadAudio || SettingsTool("image_gen") != ToolCreateImage || SettingsTool("web_search") != "" {
		t.Fatal("aliases")
	}
}

func TestCompatURLAndGeminiBase(t *testing.T) {
	for in, want := range map[string]string{
		"https://openrouter.ai/api/v1":                             "https://openrouter.ai/api/v1/chat/completions",
		"https://api.openai.com":                                   "https://api.openai.com/v1/chat/completions",
		"https://generativelanguage.googleapis.com/v1beta/openai/": "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions",
		"https://example.test/v1/chat/completions":                 "https://example.test/v1/chat/completions",
		"https://example.test/proxy":                               "https://example.test/proxy/v1/chat/completions",
	} {
		if got := compatURL(in, "chat/completions"); got != want {
			t.Fatalf("%s -> %s", in, got)
		}
	}
	for in, want := range map[string]string{
		"https://generativelanguage.googleapis.com/v1beta/openai": "https://generativelanguage.googleapis.com/v1beta",
		"https://generativelanguage.googleapis.com":               "https://generativelanguage.googleapis.com/v1beta",
		"https://example.test/v1beta/":                            "https://example.test/v1beta",
	} {
		if got := geminiNativeBase(in); got != want {
			t.Fatalf("%s -> %s", in, got)
		}
	}
	if geminiModel("models/gemini-2.5-flash") != "gemini-2.5-flash" || geminiModel("google/gemini-2.5-flash-image") != "gemini-2.5-flash-image" {
		t.Fatal("model prefix")
	}
}
