// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/skill"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/tts"
	"github.com/mqglobal/goso/gateway/internal/vault"
)

// Media tools: read_image, read_document, read_audio (stt alias),
// create_image (image_gen alias) and tts. Every input is a path inside the
// calling tenant's workspace or an http(s) URL checked by the SSRF guard.
// Every output file lands in that tenant's workspace under generated/.
const (
	ToolReadImage    = "read_image"
	ToolReadDocument = "read_document"
	ToolReadAudio    = "read_audio"
	ToolSTT          = "stt"
	ToolCreateImage  = "create_image"

	// MaxImageInBytes caps one image read by read_image.
	MaxImageInBytes = 10 << 20
	// MaxAudioInBytes caps one clip sent to speech-to-text (OpenAI's limit).
	MaxAudioInBytes = 25 << 20
	// MaxImageOutBytes caps one generated image.
	MaxImageOutBytes = 20 << 20
	// DefaultDocChars is the read_document text cap when max_chars is unset.
	DefaultDocChars = 50000
	// MaxDocChars is the largest max_chars a caller may ask for.
	MaxDocChars = 200000

	maxPromptChars   = 4000
	maxProviderJSON  = 32 << 20
	mediaCallTimeout = 120 * time.Second
	mediaFetchLimit  = 30 * time.Second
	generatedDir     = "generated"
)

var (
	// TTSService is the gateway's speech config (the same instance as
	// /api/tts). Nil → env-only tts.New().
	TTSService *tts.Service
	// ProviderStore lets GOSO_VISION_PROVIDER name a saved llm_providers row.
	// Nil → only env-registry providers resolve.
	ProviderStore store.StoreIface
	// MediaHTTPClient is the base client for media provider calls and URL
	// fetches. Tests may set Transport. GuardClient is always applied on a
	// per-call clone.
	MediaHTTPClient *http.Client
)

var mediaCatalog = []Spec{
	{
		Name:             ToolReadImage,
		Description:      "Describe or answer a question about an image (workspace path or http(s) URL) with a vision model: the tenant's provider chain, else GOSO_VISION_PROVIDER / GOSO_VISION_BASE_URL. Max 10MiB; png, jpeg, gif, webp.",
		RequiresApproval: false,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"prompt":{"type":"string","description":"What to look for, e.g. 'Read the price on this label'."},"path":{"type":"string","description":"Image file in the workspace."},"url":{"type":"string","description":"Public http(s) image URL."}}}`),
	},
	{
		Name:             ToolReadDocument,
		Description:      "Extract text from a workspace file: .txt/.md in process; PDF and scanned images through the tenant's document provider chain (Gemini) when set, else the Docling sidecar (GOSO_VAULT_DOCLING=1), which is also the fallback; DOCX, XLSX, PPTX through Docling. Returns a truncated flag. Read-only.",
		RequiresApproval: false,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Document file in the workspace."},"prompt":{"type":"string","description":"Optional instruction for the document model, default: extract all text as Markdown."},"max_chars":{"type":"integer","description":"Text cap, default 50000, max 200000."}},"required":["path"]}`),
	},
	readVideoSpec,
	{
		Name:             ToolReadAudio,
		Description:      "Transcribe a voice message or audio file (workspace path or http(s) URL) with the tenant's audio provider chain, else an OpenAI-compatible /audio/transcriptions endpoint (GOSO_STT_URL). Language hint defaults to vi. Max 25MiB (50MiB through Gemini); ogg, mp3, m4a, wav, webm, flac.",
		RequiresApproval: false,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"url":{"type":"string"},"language":{"type":"string","description":"ISO-639-1 hint, default vi."},"hint":{"type":"string","description":"Optional names or terms to spell correctly."}}}`),
	},
	{
		Name:             ToolSTT,
		Description:      "Alias of read_audio: speech-to-text for a workspace audio file or URL.",
		RequiresApproval: false,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"url":{"type":"string"},"language":{"type":"string"},"hint":{"type":"string"}}}`),
	},
	{
		Name:             ToolCreateImage,
		Description:      "Generate an image from a text prompt with the tenant's image provider chain (chat models that return images, e.g. Gemini image models), else an OpenAI-compatible /images/generations endpoint (GOSO_IMAGE_URL). Saves it under generated/images in the workspace and returns the path for send_file.",
		RequiresApproval: false,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"prompt":{"type":"string"},"aspect_ratio":{"type":"string","description":"e.g. 1:1, 16:9, 9:16 (chain providers)."},"size":{"type":"string","description":"WxH, e.g. 1024x1024 (/images/generations)."},"filename":{"type":"string","description":"Short name without extension."}},"required":["prompt"]}`),
	},
	{
		Name:             ToolImageGen,
		Description:      "Legacy alias of create_image. Approval-gated.",
		RequiresApproval: true,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"prompt":{"type":"string"},"aspect_ratio":{"type":"string"},"size":{"type":"string"},"filename":{"type":"string"}},"required":["prompt"]}`),
	},
	{
		Name:             ToolTTS,
		Description:      "Text-to-speech with the TTS settings (openai or elevenlabs). Saves an mp3 under generated/audio in the workspace and returns the path for send_file.",
		RequiresApproval: false,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"},"voice":{"type":"string"},"model":{"type":"string"},"filename":{"type":"string"}},"required":["text"]}`),
	},
}

func init() {
	catalog = append(catalog, mediaCatalog...)
}

func mediaDoubleOn() bool { return MediaInvoke != nil && MediaEnvAllowed() }

// mediaToolConfigured is env/config readiness, independent of the UI flag.
func mediaToolConfigured(name string) bool {
	switch name {
	case ToolReadImage:
		return visionConfigured()
	case ToolReadDocument:
		return workspaceConfigured()
	case ToolReadAudio, ToolSTT:
		return sttConfigured()
	case ToolCreateImage:
		return workspaceConfigured() && imageGenConfigured()
	case ToolImageGen:
		return mediaDoubleOn() || (workspaceConfigured() && imageGenConfigured())
	case ToolTTS:
		return mediaDoubleOn() || (workspaceConfigured() && ttsService().Configured())
	case ToolReadVideo:
		return false // provider chain only: see ConfiguredForTenant
	}
	return false
}

// invokeMediaTool runs a media tool. Tools that call a paid provider also
// need the UI flag (like web_search). read_document does not: it is local,
// and it calls a provider only when the tenant set a document chain.
func invokeMediaTool(ctx context.Context, name string, args map[string]any, uiEnabled bool) (*connector.InvokeResult, error) {
	if (name == ToolImageGen || name == ToolTTS) && mediaDoubleOn() {
		return invokeMedia(ctx, name, args)
	}
	if name == ToolReadDocument {
		return readDocument(ctx, args)
	}
	if !uiEnabled {
		return mediaNotConfigured(name, "tool is disabled; enable it in Tools"), nil
	}
	switch name {
	case ToolReadImage:
		return readImage(ctx, args)
	case ToolReadVideo:
		return readVideo(ctx, args)
	case ToolReadAudio, ToolSTT:
		return readAudio(ctx, name, args)
	case ToolCreateImage, ToolImageGen:
		return createImage(ctx, name, args)
	case ToolTTS:
		return speak(ctx, args)
	}
	return notConfigured(name), nil
}

func mediaNotConfigured(name, hint string) *connector.InvokeResult {
	res := notConfigured(name)
	if hint != "" {
		res.Content = map[string]any{"error": "not_configured", "hint": hint}
	}
	return res
}

func mediaOK(name string, content map[string]any) *connector.InvokeResult {
	return &connector.InvokeResult{Tool: name, Connector: ConnectorName, Status: "ok", Content: content}
}

func envTrim(k string) string { return strings.TrimSpace(os.Getenv(k)) }

func ttsService() *tts.Service {
	if TTSService != nil {
		return TTSService
	}
	return tts.New()
}

// ---- loading inputs ----

type mediaInput struct {
	data   []byte
	name   string // basename used for extension hints
	source string // workspace-relative path or the URL
}

var errNoMediaRef = errors.New("path or url is required")

// loadMedia reads one input: a jailed workspace path (no symlinks, regular
// file) or an http(s) URL through CheckURL + GuardClient. Both are capped.
func loadMedia(ctx context.Context, name string, args map[string]any, limit int64) (*mediaInput, *connector.InvokeResult) {
	if p := pathArg(args); p != "" {
		if !workspaceConfigured() {
			return nil, mediaNotConfigured(name, "GOSO_WORKSPACE is not set")
		}
		abs, rel, err := jailPath(ctx, p)
		if err != nil {
			return nil, mapFSErr(name, err)
		}
		f, st, err := openRegular(abs, os.O_RDONLY)
		if err != nil {
			return nil, mapFSErr(name, err)
		}
		defer f.Close()
		if st.Size() > limit {
			return nil, toolErr(name, "error", "too large")
		}
		data, err := io.ReadAll(io.LimitReader(f, limit+1))
		if err != nil {
			return nil, toolErr(name, "error", "read failed")
		}
		if int64(len(data)) > limit {
			return nil, toolErr(name, "error", "too large")
		}
		return &mediaInput{data: data, name: filepath.Base(rel), source: rel}, nil
	}
	raw := urlArg(args)
	if raw == "" {
		return nil, toolErr(name, "error", errNoMediaRef.Error())
	}
	data, err := fetchMediaURL(ctx, raw, limit)
	if err != nil {
		return nil, toolErr(name, "error", err.Error())
	}
	u, _ := url.Parse(raw)
	base := ""
	if u != nil {
		base = filepath.Base(u.Path)
	}
	return &mediaInput{data: data, name: base, source: raw}, nil
}

func mediaClient(timeout time.Duration) *http.Client {
	c := &http.Client{Timeout: timeout}
	if MediaHTTPClient != nil {
		c.Transport = MediaHTTPClient.Transport
		if MediaHTTPClient.Timeout > 0 {
			c.Timeout = MediaHTTPClient.Timeout
		}
	}
	security.GuardClient(c)
	return c
}

func fetchMediaURL(ctx context.Context, raw string, limit int64) ([]byte, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("url must be http or https")
	}
	if err := security.CheckURL(raw); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, errors.New("invalid url")
	}
	resp, err := mediaClient(mediaFetchLimit).Do(req)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "ssrf") {
			return nil, errors.New("ssrf: blocked")
		}
		return nil, errors.New("fetch failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch failed: http %d", resp.StatusCode)
	}
	if resp.ContentLength > limit {
		return nil, errors.New("too large")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, errors.New("fetch failed")
	}
	if int64(len(data)) > limit {
		return nil, errors.New("too large")
	}
	return data, nil
}

// sniffImage trusts bytes, never the extension or Content-Type.
func sniffImage(b []byte) (mime, ext string, ok bool) {
	switch http.DetectContentType(b) {
	case "image/png":
		return "image/png", ".png", true
	case "image/jpeg":
		return "image/jpeg", ".jpg", true
	case "image/gif":
		return "image/gif", ".gif", true
	case "image/webp":
		return "image/webp", ".webp", true
	}
	return "", "", false
}

// sniffAudio accepts the containers OpenAI-compatible STT takes. The upload
// filename extension is chosen from the bytes because servers key on it.
func sniffAudio(b []byte) (mime, ext string, ok bool) {
	switch {
	case len(b) >= 4 && string(b[:4]) == "OggS":
		return "audio/ogg", ".ogg", true
	case len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WAVE":
		return "audio/wav", ".wav", true
	case len(b) >= 4 && string(b[:4]) == "fLaC":
		return "audio/flac", ".flac", true
	case len(b) >= 8 && string(b[4:8]) == "ftyp":
		return "audio/mp4", ".m4a", true
	case len(b) >= 4 && bytes.Equal(b[:4], []byte{0x1a, 0x45, 0xdf, 0xa3}):
		return "audio/webm", ".webm", true
	case len(b) >= 3 && string(b[:3]) == "ID3":
		return "audio/mpeg", ".mp3", true
	case len(b) >= 2 && b[0] == 0xff && b[1]&0xe0 == 0xe0 && b[1]&0x06 != 0:
		// MPEG audio frame sync with a non-zero layer (ADTS AAC has layer 00).
		return "audio/mpeg", ".mp3", true
	}
	return "", "", false
}

// ---- read_image ----

func visionConfigured() bool {
	return envTrim("GOSO_VISION_BASE_URL") != "" || envTrim("GOSO_VISION_PROVIDER") != ""
}

// visionProvider builds the vision model. GOSO_VISION_BASE_URL (+ _API_KEY,
// _TYPE) is a direct endpoint; else GOSO_VISION_PROVIDER names an env or
// saved provider. GOSO_VISION_MODEL overrides the model either way.
func visionProvider() (llm.Provider, string) {
	model := envTrim("GOSO_VISION_MODEL")
	if base := envTrim("GOSO_VISION_BASE_URL"); base != "" {
		typ := envTrim("GOSO_VISION_TYPE")
		if typ == "" {
			typ = llm.TypeOpenAICompat
		}
		p, err := llm.Build("vision", typ, base, model, envTrim("GOSO_VISION_API_KEY"))
		if err != nil {
			return nil, "GOSO_VISION_TYPE must be openai-compat or anthropic"
		}
		if !llm.SupportsImages(p) {
			return nil, "vision provider type does not accept images"
		}
		if o, ok := p.(*llm.OpenAI); ok && envTrim("GOSO_VISION_API_KEY") == "" {
			o.AllowEmptyKey = true // self-hosted OpenAI-compatible endpoints (vLLM, Ollama)
		}
		return p, ""
	}
	name := envTrim("GOSO_VISION_PROVIDER")
	if name == "" {
		return nil, "set GOSO_VISION_PROVIDER or GOSO_VISION_BASE_URL"
	}
	p, err := llm.Resolve(ProviderStore, name, model, nil)
	if err != nil {
		return nil, "GOSO_VISION_PROVIDER not found"
	}
	if !llm.SupportsImages(p) {
		return nil, "GOSO_VISION_PROVIDER does not accept images"
	}
	return p, ""
}

func readImage(ctx context.Context, args map[string]any) (*connector.InvokeResult, error) {
	if chain := tenantChain(ctx, ToolReadImage); len(chain) > 0 {
		return readImageChain(ctx, chain, args)
	}
	if !visionConfigured() {
		return mediaNotConfigured(ToolReadImage, "set GOSO_VISION_PROVIDER or GOSO_VISION_BASE_URL"), nil
	}
	p, why := visionProvider()
	if p == nil {
		return mediaNotConfigured(ToolReadImage, why), nil
	}
	in, fail := loadMedia(ctx, ToolReadImage, args, MaxImageInBytes)
	if fail != nil {
		return fail, nil
	}
	mime, _, ok := sniffImage(in.data)
	if !ok {
		return toolErr(ToolReadImage, "error", "unsupported image type"), nil
	}
	prompt, _ := stringArg(args, "prompt", "question")
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		prompt = defaultImagePrompt
	}
	if utf8.RuneCountInString(prompt) > maxPromptChars {
		return toolErr(ToolReadImage, "error", "prompt too long"), nil
	}
	callCtx, cancel := context.WithTimeout(ctx, mediaCallTimeout)
	defer cancel()
	text, err := p.Chat(callCtx, []llm.Message{{Role: "user", Content: prompt, Images: []llm.ImagePart{{MIME: mime, Data: in.data}}}})
	if err != nil {
		return toolErr(ToolReadImage, "error", "vision call failed: "+tts.Redact(err.Error())), nil
	}
	return mediaOK(ToolReadImage, map[string]any{
		"text":   text,
		"source": in.source,
		"mime":   mime,
		"bytes":  len(in.data),
	}), nil
}

// ---- read_document ----

func readDocument(ctx context.Context, args map[string]any) (*connector.InvokeResult, error) {
	if !workspaceConfigured() {
		return notConfigured(ToolReadDocument), nil
	}
	abs, rel, err := jailPath(ctx, pathArg(args))
	if err != nil {
		return mapFSErr(ToolReadDocument, err), nil
	}
	f, st, err := openRegular(abs, os.O_RDONLY)
	if err != nil {
		return mapFSErr(ToolReadDocument, err), nil
	}
	defer f.Close()
	chain := tenantChain(ctx, ToolReadDocument)
	limit := vault.IngestMaxBytes()
	if len(chain) > 0 && limit < MaxDocumentInBytes {
		limit = MaxDocumentInBytes
	}
	if st.Size() > limit {
		return toolErr(ToolReadDocument, "error", "too large"), nil
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return toolErr(ToolReadDocument, "error", "read failed"), nil
	}
	maxChars := intArg(args, "max_chars", DefaultDocChars)
	if maxChars < 1 {
		maxChars = DefaultDocChars
	}
	if maxChars > MaxDocChars {
		maxChars = MaxDocChars
	}
	// PDFs and scans go to the tenant's document chain first; local
	// extraction (Docling) is the fallback when every provider fails.
	var llmErr error
	var attempts []chainAttempt
	if mime := docChainMIME(data); len(chain) > 0 && mime != "" && len(data) <= MaxDocumentInBytes {
		text, out, att, err := readDocumentChain(ctx, chain, rel, mime, data, args)
		if err == nil {
			clipped, total, truncated := clipRunes(text, maxChars)
			return mediaOK(ToolReadDocument, withAttempts(out, map[string]any{
				"path": rel, "mime": mime, "chars": total, "truncated": truncated, "text": clipped,
			})), nil
		}
		llmErr, attempts = err, att
	}
	tenant := store.NormalizeTenant(skill.TenantFrom(ctx))
	ex := vault.ExtractText(ctx, tenant, rel, data)
	if ex.ErrorClass != "" && llmErr != nil {
		return chainFailed(ToolReadDocument, "document read", attempts, llmErr), nil
	}
	switch ex.ErrorClass {
	case "":
	case vault.ClassNotConfigured:
		hint := "PDF/Office/image files need the Docling sidecar (GOSO_VAULT_DOCLING=1, GOSO_VAULT_DOCLING_URL)"
		if docChainMIME(data) != "" {
			hint += " or a document provider chain (" + noChainHint(ToolReadDocument) + ")"
		}
		return mediaNotConfigured(ToolReadDocument, hint), nil
	case vault.ClassUnsupported:
		return toolErr(ToolReadDocument, "error", "unsupported file type"), nil
	case vault.ClassTooLarge:
		return toolErr(ToolReadDocument, "error", "too large"), nil
	case vault.ClassEmpty:
		return toolErr(ToolReadDocument, "error", "no text found"), nil
	case vault.ClassTimeout:
		return toolErr(ToolReadDocument, "error", "conversion timed out"), nil
	case vault.ClassOCRFailed:
		return toolErr(ToolReadDocument, "error", "ocr failed"), nil
	default:
		return toolErr(ToolReadDocument, "error", "extract failed"), nil
	}
	text, total, truncated := clipRunes(ex.Markdown, maxChars)
	content := map[string]any{
		"path":      rel,
		"mime":      ex.MIME,
		"pages":     ex.PageCount,
		"ocr":       ex.OCR,
		"chars":     total,
		"truncated": truncated,
		"text":      text,
	}
	if llmErr != nil {
		content["fallback"] = "local"
		content["llm_error"] = llmErr.Error()
		content["attempts"] = attempts
	}
	return mediaOK(ToolReadDocument, content), nil
}

func intArg(args map[string]any, key string, def int) int {
	if args == nil {
		return def
	}
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n)
		}
	}
	return def
}

func clipRunes(s string, max int) (string, int, bool) {
	total := utf8.RuneCountInString(s)
	if total <= max {
		return s, total, false
	}
	n := 0
	for i := range s {
		if n == max {
			return s[:i], total, true
		}
		n++
	}
	return s, total, false
}

// ---- read_audio / stt ----

func sttConfigured() bool { return envTrim("GOSO_STT_URL") != "" }

func sttEndpoint() string {
	base := strings.TrimRight(envTrim("GOSO_STT_URL"), "/")
	if strings.HasSuffix(base, "/audio/transcriptions") {
		return base
	}
	if strings.HasSuffix(base, "/v1") {
		return base + "/audio/transcriptions"
	}
	return base + "/v1/audio/transcriptions"
}

func readAudio(ctx context.Context, name string, args map[string]any) (*connector.InvokeResult, error) {
	chain := tenantChain(ctx, name)
	if len(chain) == 0 && !sttConfigured() {
		return mediaNotConfigured(name, "set GOSO_STT_URL (and GOSO_STT_API_KEY), or "+noChainHint(ToolReadAudio)), nil
	}
	lang, _ := stringArg(args, "language")
	hint, _ := stringArg(args, "hint")
	if len(chain) > 0 {
		lang, err := audioLang(lang)
		if err != nil {
			return toolErr(name, "error", err.Error()), nil
		}
		hint = strings.TrimSpace(hint)
		if utf8.RuneCountInString(hint) > 1000 {
			return toolErr(name, "error", "hint too long"), nil
		}
		return readAudioChain(ctx, name, chain, args, lang, hint)
	}
	in, fail := loadMedia(ctx, name, args, MaxAudioInBytes)
	if fail != nil {
		return fail, nil
	}
	tr, err := transcribe(ctx, in.data, lang, hint)
	if err != nil {
		return toolErr(name, "error", err.Error()), nil
	}
	return mediaOK(name, map[string]any{
		"text":     tr.text,
		"language": tr.lang,
		"model":    tr.model,
		"source":   in.source,
		"mime":     tr.mime,
		"bytes":    len(in.data),
	}), nil
}

type transcript struct {
	text, lang, model, mime string
}

var langCode = regexp.MustCompile(`^[a-z]{2,3}$`)

// audioLang normalises a language hint: "" → GOSO_STT_LANGUAGE, else vi.
func audioLang(lang string) (string, error) {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "" {
		lang = strings.ToLower(envTrim("GOSO_STT_LANGUAGE"))
	}
	if lang == "" {
		lang = "vi"
	}
	if !langCode.MatchString(lang) {
		return "", errors.New("language must be an ISO-639-1 code")
	}
	return lang, nil
}

// transcribe sends one clip to the GOSO_STT_URL endpoint. Errors are short
// and never carry the key or the endpoint.
func transcribe(ctx context.Context, data []byte, lang, hint string) (transcript, error) {
	mime, ext, ok := sniffAudio(data)
	if !ok {
		return transcript{}, errors.New("unsupported audio type")
	}
	lang, err := audioLang(lang)
	if err != nil {
		return transcript{}, err
	}
	hint = strings.TrimSpace(hint)
	if utf8.RuneCountInString(hint) > 1000 {
		return transcript{}, errors.New("hint too long")
	}
	model := envTrim("GOSO_STT_MODEL")
	if model == "" {
		model = "whisper-1"
	}
	body, ct, err := transcriptionBody(model, lang, hint, mime, ext, data)
	if err != nil {
		return transcript{}, errors.New("encode failed")
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := postProvider(ctx, sttEndpoint(), envTrim("GOSO_STT_API_KEY"), ct, body, 1<<20, &out); err != nil {
		return transcript{}, errors.New("transcription failed: " + err.Error())
	}
	return transcript{text: strings.TrimSpace(out.Text), lang: lang, model: model, mime: mime}, nil
}

// postProvider POSTs to a provider endpoint through the SSRF guard and decodes
// a JSON body capped at limit. Errors never carry the key.
func postProvider(ctx context.Context, endpoint, key, contentType string, payload []byte, limit int64, out any) error {
	if err := security.CheckURL(endpoint); err != nil {
		return errors.New("endpoint blocked")
	}
	callCtx, cancel := context.WithTimeout(ctx, mediaCallTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return errors.New("invalid endpoint")
	}
	req.Header.Set("Content-Type", contentType)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := mediaClient(mediaCallTimeout).Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("request failed")
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return errors.New("read failed")
	}
	if int64(len(b)) > limit {
		return errors.New("response too large")
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return errors.New("unauthorized")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail := strings.TrimSpace(tts.Redact(string(b)))
		if len(detail) > 300 {
			detail = detail[:300]
		}
		if detail == "" {
			return fmt.Errorf("http %d", resp.StatusCode)
		}
		return fmt.Errorf("http %d: %s", resp.StatusCode, detail)
	}
	if err := json.Unmarshal(b, out); err != nil {
		return errors.New("bad provider response")
	}
	return nil
}

// ---- create_image ----

func imageGenConfigured() bool { return envTrim("GOSO_IMAGE_URL") != "" }

var sizeRe = regexp.MustCompile(`^[1-9][0-9]{2,3}x[1-9][0-9]{2,3}$`)

func imagesEndpoint() string {
	base := strings.TrimRight(envTrim("GOSO_IMAGE_URL"), "/")
	if strings.HasSuffix(base, "/images/generations") {
		return base
	}
	if strings.HasSuffix(base, "/v1") {
		return base + "/images/generations"
	}
	return base + "/v1/images/generations"
}

func createImage(ctx context.Context, name string, args map[string]any) (*connector.InvokeResult, error) {
	if !workspaceConfigured() {
		return mediaNotConfigured(name, "GOSO_WORKSPACE is not set"), nil
	}
	chain := tenantChain(ctx, name)
	if len(chain) == 0 && !imageGenConfigured() {
		return mediaNotConfigured(name, "set GOSO_IMAGE_URL (and GOSO_IMAGE_API_KEY), or "+noChainHint(ToolCreateImage)), nil
	}
	prompt, _ := stringArg(args, "prompt")
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return toolErr(name, "error", "prompt is required"), nil
	}
	if utf8.RuneCountInString(prompt) > maxPromptChars {
		return toolErr(name, "error", "prompt too long"), nil
	}
	size, _ := stringArg(args, "size")
	size = strings.TrimSpace(size)
	if size != "" && !sizeRe.MatchString(size) {
		return toolErr(name, "error", "size must look like 1024x1024"), nil
	}
	if len(chain) > 0 {
		return createImageChain(ctx, name, chain, args, prompt, size)
	}
	if size == "" {
		size = envTrim("GOSO_IMAGE_SIZE")
	}
	if size == "" {
		size = "1024x1024"
	}
	if !sizeRe.MatchString(size) {
		return toolErr(name, "error", "size must look like 1024x1024"), nil
	}
	model := envTrim("GOSO_IMAGE_MODEL")
	if model == "" {
		model = "gpt-image-1"
	}
	img, err := imagesGenerate(ctx, imagesEndpoint(), envTrim("GOSO_IMAGE_API_KEY"), nil, model, prompt, size)
	if err != nil {
		return toolErr(name, "error", "image generation failed: "+err.Error()), nil
	}
	if len(img) > MaxImageOutBytes {
		return toolErr(name, "error", "too large"), nil
	}
	mime, ext, ok := sniffImage(img)
	if !ok {
		return toolErr(name, "error", "provider did not return an image"), nil
	}
	fn, _ := stringArg(args, "filename")
	rel, err := saveGenerated(ctx, "images", fn, "image", ext, img)
	if err != nil {
		return mapFSErr(name, err), nil
	}
	return mediaOK(name, map[string]any{
		"path":  rel,
		"bytes": len(img),
		"mime":  mime,
		"model": model,
	}), nil
}

// ---- tts ----

func speak(ctx context.Context, args map[string]any) (*connector.InvokeResult, error) {
	if !workspaceConfigured() {
		return mediaNotConfigured(ToolTTS, "GOSO_WORKSPACE is not set"), nil
	}
	svc := ttsService()
	if !svc.Configured() {
		return mediaNotConfigured(ToolTTS, "set the TTS provider (openai or elevenlabs) and key in TTS settings"), nil
	}
	text, _ := stringArg(args, "text")
	voice, _ := stringArg(args, "voice")
	model, _ := stringArg(args, "model")
	callCtx, cancel := context.WithTimeout(ctx, mediaCallTimeout)
	defer cancel()
	audio, err := svc.Synthesize(callCtx, text, tts.SynthOptions{Voice: voice, Model: model})
	if err != nil {
		switch {
		case errors.Is(err, tts.ErrNotConfigured), errors.Is(err, tts.ErrDisabled), errors.Is(err, tts.ErrUnsupported):
			return mediaNotConfigured(ToolTTS, err.Error()), nil
		case errors.Is(err, tts.ErrEmptyText), errors.Is(err, tts.ErrTooLong), errors.Is(err, tts.ErrVoiceRequired):
			return toolErr(ToolTTS, "error", err.Error()), nil
		default:
			return toolErr(ToolTTS, "error", "speech failed: "+tts.Redact(err.Error())), nil
		}
	}
	fn, _ := stringArg(args, "filename")
	rel, err := saveGenerated(ctx, "audio", fn, "speech", audio.Ext, audio.Data)
	if err != nil {
		return mapFSErr(ToolTTS, err), nil
	}
	return mediaOK(ToolTTS, map[string]any{
		"path":     rel,
		"bytes":    len(audio.Data),
		"mime":     audio.MIME,
		"provider": audio.Provider,
	}), nil
}

// ---- output files ----

var slugDrop = regexp.MustCompile(`[^a-z0-9_-]+`)

func slugName(s, def string) string {
	s = slugDrop.ReplaceAllString(strings.ToLower(strings.TrimSpace(s)), "-")
	s = strings.Trim(s, "-_")
	if len(s) > 40 {
		s = strings.Trim(s[:40], "-_")
	}
	if s == "" {
		return def
	}
	return s
}

// saveGenerated writes a new file at generated/<kind>/<slug>-<time>-<rand><ext>
// inside the tenant workspace. It never overwrites (O_EXCL) or follows links.
func saveGenerated(ctx context.Context, kind, filename, def, ext string, data []byte) (string, error) {
	return saveUnder(ctx, generatedDir+"/"+kind, filename, def, ext, data)
}

// saveUnder writes a new file at <dir>/<slug>-<time>-<rand><ext> inside the
// tenant workspace, with saveGenerated's guarantees.
func saveUnder(ctx context.Context, dir, filename, def, ext string, data []byte) (string, error) {
	root, err := workspaceAbs(ctx)
	if err != nil {
		return "", err
	}
	var rnd [3]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return "", err
	}
	name := slugName(filename, def) + "-" + time.Now().UTC().Format("20060102-150405") + "-" + hex.EncodeToString(rnd[:]) + ext
	abs, rel, err := jailPath(ctx, dir+"/"+name)
	if err != nil {
		return "", err
	}
	parent := filepath.Dir(abs)
	if err := confineUnder(root, parent); err != nil {
		return "", err
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	if ev, err := filepath.EvalSymlinks(parent); err != nil {
		return "", errPathEscape
	} else if err := confineUnder(root, ev); err != nil {
		return "", err
	}
	f, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		if isLoop(err) {
			return "", errPathEscape
		}
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(abs)
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return rel, nil
}
