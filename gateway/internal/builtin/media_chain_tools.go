// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/llm"
)

// Chain-backed paths of the media tools. Each runs the caller tenant's
// provider chain (media_chain.go); the env paths in media_tools.go remain
// the fallback when the tenant has no settings for the tool.

const (
	// ToolReadVideo describes or answers a question about a video.
	ToolReadVideo = "read_video"
	// MaxVideoInBytes caps one video read by read_video.
	MaxVideoInBytes = 100 << 20
	// MaxDocumentInBytes caps a document sent to a chain provider.
	MaxDocumentInBytes = 20 << 20
	// MaxAudioChainBytes caps audio sent to a chain provider. The
	// /audio/transcriptions path keeps MaxAudioInBytes.
	MaxAudioChainBytes = 50 << 20

	defaultVideoPrompt = "Describe this video: the scenes, the people and actions, any on-screen text (transcribed exactly) and what is said."
	defaultDocPrompt   = "Extract all the text of this document as Markdown. Keep headings, lists and tables. Do not summarize or add commentary."
	defaultImagePrompt = "Describe this image in detail. Transcribe any visible text exactly."
)

var aspectRe = regexp.MustCompile(`^[1-9][0-9]?:[1-9][0-9]?$`)

var readVideoSpec = Spec{
	Name:             ToolReadVideo,
	Description:      "Describe or answer a question about a video (workspace path or http(s) URL) with the tenant's video provider chain (Gemini). Max 100MiB; mp4, mov, webm, avi, mpeg, 3gp, flv.",
	RequiresApproval: false,
	InputSchema:      []byte(`{"type":"object","properties":{"prompt":{"type":"string","description":"What to look for, e.g. 'Summarize the product demo'."},"path":{"type":"string","description":"Video file in the workspace."},"url":{"type":"string","description":"Public http(s) video URL."}}}`),
}

func promptArg(name string, args map[string]any, def string) (string, *connector.InvokeResult) {
	p, _ := stringArg(args, "prompt", "question")
	p = strings.TrimSpace(p)
	if p == "" {
		p = def
	}
	if utf8.RuneCountInString(p) > maxPromptChars {
		return "", toolErr(name, "error", "prompt too long")
	}
	return p, nil
}

func noChainHint(tool string) string {
	return "add providers in the tool settings: PUT /api/tools/builtin/" + tool + "/settings"
}

func withAttempts(out map[string]any, content map[string]any) map[string]any {
	for _, k := range []string{"provider", "model", "fallback_from"} {
		if v, ok := out[k]; ok {
			content[k] = v
		}
	}
	return content
}

// ---- read_image ----

func readImageChain(ctx context.Context, chain []MediaProviderEntry, args map[string]any) (*connector.InvokeResult, error) {
	in, fail := loadMedia(ctx, ToolReadImage, args, MaxImageInBytes)
	if fail != nil {
		return fail, nil
	}
	mime, _, ok := sniffImage(in.data)
	if !ok {
		return toolErr(ToolReadImage, "error", "unsupported image type"), nil
	}
	prompt, bad := promptArg(ToolReadImage, args, defaultImagePrompt)
	if bad != nil {
		return bad, nil
	}
	out, attempts, err := runMediaChain(ctx, chain, func(ctx context.Context, t *mediaTarget) (map[string]any, error) {
		text, err := describeMedia(ctx, t, prompt, mime, in.data, "image.bin")
		if err != nil {
			return nil, err
		}
		return map[string]any{"text": text}, nil
	})
	if err != nil {
		return chainFailed(ToolReadImage, "vision call", attempts, err), nil
	}
	return mediaOK(ToolReadImage, withAttempts(out, map[string]any{
		"text": out["text"], "source": in.source, "mime": mime, "bytes": len(in.data),
	})), nil
}

// describeMedia sends prompt + one media item and returns the model's text.
func describeMedia(ctx context.Context, t *mediaTarget, prompt, mime string, data []byte, filename string) (string, error) {
	kind := strings.SplitN(mime, "/", 2)[0]
	switch t.api {
	case apiGemini:
		part, cleanup, err := geminiMediaPart(ctx, t, mime, data)
		if err != nil {
			return "", err
		}
		defer cleanup()
		r, err := geminiGenerate(ctx, t, t.model, []map[string]any{{"text": prompt}, part}, nil)
		if err != nil {
			return "", err
		}
		if r.text == "" {
			return "", permanent("no text in response")
		}
		return r.text, nil
	case apiOpenAI:
		if len(data) > compatInlineBytes {
			return "", permanent("media too large for this provider (max %d MiB inline)", compatInlineBytes>>20)
		}
		var part map[string]any
		switch kind {
		case "image":
			part = map[string]any{"type": "image_url", "image_url": map[string]any{"url": dataURL(mime, data)}}
		case "video":
			part = map[string]any{"type": "video_url", "video_url": map[string]any{"url": dataURL(mime, data)}}
		case "audio":
			part = map[string]any{"type": "input_audio", "input_audio": map[string]any{"data": base64.StdEncoding.EncodeToString(data), "format": audioFormat(mime)}}
		default:
			part = map[string]any{"type": "file", "file": map[string]any{"filename": filename, "file_data": dataURL(mime, data)}}
		}
		r, err := compatChat(ctx, t, t.model, prompt, []map[string]any{part}, nil)
		if err != nil {
			return "", err
		}
		if r.text == "" {
			return "", permanent("no text in response")
		}
		return r.text, nil
	case apiAnthropic:
		if kind != "image" {
			return "", permanent("anthropic provider cannot read %s here; use a Gemini or OpenAI-compatible provider", kind)
		}
		return t.llm.Chat(ctx, []llm.Message{{Role: "user", Content: prompt, Images: []llm.ImagePart{{MIME: mime, Data: data}}}})
	}
	return "", permanent("unsupported provider api")
}

func audioFormat(mime string) string {
	switch mime {
	case "audio/wav":
		return "wav"
	case "audio/mpeg":
		return "mp3"
	case "audio/ogg":
		return "ogg"
	case "audio/flac":
		return "flac"
	case "audio/webm":
		return "webm"
	case "audio/mp4":
		return "m4a"
	}
	return "mp3"
}

// ---- read_document ----

// docChainMIME is the MIME of a document a chain provider reads directly:
// PDF or an image (scans). Other formats use local extraction.
func docChainMIME(b []byte) string {
	if bytes.HasPrefix(b, []byte("%PDF-")) {
		return "application/pdf"
	}
	if m, _, ok := sniffImage(b); ok {
		return m
	}
	return ""
}

func readDocumentChain(ctx context.Context, chain []MediaProviderEntry, rel, mime string, data []byte, args map[string]any) (string, map[string]any, []chainAttempt, error) {
	prompt, bad := promptArg(ToolReadDocument, args, defaultDocPrompt)
	if bad != nil {
		return "", nil, nil, permanent("prompt too long")
	}
	name := rel
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	out, attempts, err := runMediaChain(ctx, chain, func(ctx context.Context, t *mediaTarget) (map[string]any, error) {
		text, err := describeMedia(ctx, t, prompt, mime, data, name)
		if err != nil {
			return nil, err
		}
		return map[string]any{"text": text}, nil
	})
	if err != nil {
		return "", nil, attempts, err
	}
	text, _ := out["text"].(string)
	return text, out, attempts, nil
}

// ---- read_video ----

// sniffVideo trusts bytes only. An ftyp box with an audio-only brand (M4A)
// is rejected.
func sniffVideo(b []byte) (mime, ext string, ok bool) {
	switch {
	case len(b) >= 12 && string(b[4:8]) == "ftyp":
		brand := string(b[8:12])
		switch {
		case brand == "M4A " || brand == "M4B " || brand == "M4P ":
			return "", "", false
		case brand == "qt  ":
			return "video/quicktime", ".mov", true
		case strings.HasPrefix(brand, "3gp") || strings.HasPrefix(brand, "3g2"):
			return "video/3gpp", ".3gp", true
		}
		return "video/mp4", ".mp4", true
	case len(b) >= 4 && bytes.Equal(b[:4], []byte{0x1a, 0x45, 0xdf, 0xa3}):
		return "video/webm", ".webm", true
	case len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "AVI ":
		return "video/x-msvideo", ".avi", true
	case len(b) >= 4 && bytes.Equal(b[:4], []byte{0x00, 0x00, 0x01, 0xba}):
		return "video/mpeg", ".mpg", true
	case len(b) >= 3 && string(b[:3]) == "FLV":
		return "video/x-flv", ".flv", true
	}
	return "", "", false
}

func readVideo(ctx context.Context, args map[string]any) (*connector.InvokeResult, error) {
	chain := tenantChain(ctx, ToolReadVideo)
	if len(chain) == 0 {
		return mediaNotConfigured(ToolReadVideo, noChainHint(ToolReadVideo)), nil
	}
	prompt, bad := promptArg(ToolReadVideo, args, defaultVideoPrompt)
	if bad != nil {
		return bad, nil
	}
	in, fail := loadMedia(ctx, ToolReadVideo, args, MaxVideoInBytes)
	if fail != nil {
		return fail, nil
	}
	mime, _, ok := sniffVideo(in.data)
	if !ok {
		return toolErr(ToolReadVideo, "error", "unsupported video type"), nil
	}
	out, attempts, err := runMediaChain(ctx, chain, func(ctx context.Context, t *mediaTarget) (map[string]any, error) {
		text, err := describeMedia(ctx, t, prompt, mime, in.data, "video.bin")
		if err != nil {
			return nil, err
		}
		return map[string]any{"text": text}, nil
	})
	if err != nil {
		return chainFailed(ToolReadVideo, "video call", attempts, err), nil
	}
	return mediaOK(ToolReadVideo, withAttempts(out, map[string]any{
		"text": out["text"], "source": in.source, "mime": mime, "bytes": len(in.data),
	})), nil
}

// ---- read_audio ----

func wantsTranscriptions(t *mediaTarget) bool {
	if strings.EqualFold(t.param("api"), "transcriptions") {
		return true
	}
	m := strings.ToLower(t.model)
	return strings.Contains(m, "whisper") || strings.Contains(m, "transcribe")
}

func readAudioChain(ctx context.Context, name string, chain []MediaProviderEntry, args map[string]any, lang, hint string) (*connector.InvokeResult, error) {
	in, fail := loadMedia(ctx, name, args, MaxAudioChainBytes)
	if fail != nil {
		return fail, nil
	}
	mime, ext, ok := sniffAudio(in.data)
	if !ok {
		return toolErr(name, "error", "unsupported audio type"), nil
	}
	prompt := "Transcribe this audio exactly, in its original language (expected language code: " + lang + "). Return only the transcript text, with no commentary."
	if hint != "" {
		prompt += " Spell these names and terms correctly: " + hint
	}
	out, attempts, err := runMediaChain(ctx, chain, func(ctx context.Context, t *mediaTarget) (map[string]any, error) {
		if t.api == apiOpenAI && wantsTranscriptions(t) {
			if len(in.data) > MaxAudioInBytes {
				return nil, permanent("audio too large for /audio/transcriptions (max %d MiB)", MaxAudioInBytes>>20)
			}
			body, ct, err := transcriptionBody(t.model, lang, hint, mime, ext, in.data)
			if err != nil {
				return nil, permanent("encode failed")
			}
			var r struct {
				Text string `json:"text"`
			}
			if _, err := providerJSON(ctx, t, http.MethodPost, compatURL(t.base, "audio/transcriptions"), map[string]string{"Content-Type": ct}, body, 1<<20, &r); err != nil {
				return nil, err
			}
			return map[string]any{"text": strings.TrimSpace(r.Text)}, nil
		}
		text, err := describeMedia(ctx, t, prompt, mime, in.data, "audio"+ext)
		if err != nil {
			return nil, err
		}
		return map[string]any{"text": text}, nil
	})
	if err != nil {
		return chainFailed(name, "transcription", attempts, err), nil
	}
	return mediaOK(name, withAttempts(out, map[string]any{
		"text": out["text"], "language": lang, "source": in.source, "mime": mime, "bytes": len(in.data),
	})), nil
}

// ---- create_image ----

func wantsImagesAPI(t *mediaTarget, model string) bool {
	if strings.EqualFold(t.param("api"), "images") {
		return true
	}
	m := strings.ToLower(model)
	return strings.HasPrefix(m, "gpt-image") || strings.HasPrefix(m, "dall-e")
}

func createImageChain(ctx context.Context, name string, chain []MediaProviderEntry, args map[string]any, prompt, size string) (*connector.InvokeResult, error) {
	aspect, _ := stringArg(args, "aspect_ratio")
	aspect = strings.TrimSpace(aspect)
	if aspect != "" && !aspectRe.MatchString(aspect) {
		return toolErr(name, "error", "aspect_ratio must look like 16:9"), nil
	}
	var img []byte
	out, attempts, err := runMediaChain(ctx, chain, func(ctx context.Context, t *mediaTarget) (map[string]any, error) {
		model := t.model
		if m := t.param("image_model"); m != "" {
			model = m
		}
		ar := aspect
		if ar == "" {
			ar = t.param("aspect_ratio")
		}
		if ar != "" && !aspectRe.MatchString(ar) {
			ar = ""
		}
		var b []byte
		switch t.api {
		case apiGemini:
			cfg := map[string]any{"responseModalities": []string{"TEXT", "IMAGE"}}
			if ar != "" {
				cfg["imageConfig"] = map[string]any{"aspectRatio": ar}
			}
			r, err := geminiGenerate(ctx, t, model, []map[string]any{{"text": prompt}}, cfg)
			if err != nil {
				return nil, err
			}
			if len(r.images) == 0 {
				return nil, permanent("model returned no image")
			}
			b = r.images[0]
		case apiOpenAI:
			if wantsImagesAPI(t, model) {
				sz := size
				if sz == "" {
					sz = "1024x1024"
				}
				got, err := imagesGenerate(ctx, compatURL(t.base, "images/generations"), "", t, model, prompt, sz)
				if err != nil {
					return nil, err
				}
				b = got
				break
			}
			extra := map[string]any{"modalities": []string{"image", "text"}}
			if ar != "" {
				extra["image_config"] = map[string]any{"aspect_ratio": ar}
			}
			r, err := compatChat(ctx, t, model, prompt, nil, extra)
			if err != nil {
				return nil, err
			}
			if len(r.refs) == 0 {
				return nil, permanent("model returned no image")
			}
			got, err := decodeImageRef(ctx, r.refs[0])
			if err != nil {
				return nil, err
			}
			b = got
		default:
			return nil, permanent("provider cannot generate images; use a Gemini or OpenAI-compatible provider")
		}
		if len(b) > MaxImageOutBytes {
			return nil, permanent("image too large")
		}
		if _, _, ok := sniffImage(b); !ok {
			return nil, permanent("provider did not return an image")
		}
		img = b
		return map[string]any{"image_model": model}, nil
	})
	if err != nil {
		return chainFailed(name, "image generation", attempts, err), nil
	}
	mime, ext, _ := sniffImage(img)
	fn, _ := stringArg(args, "filename")
	rel, err := saveGenerated(ctx, "images", fn, "image", ext, img)
	if err != nil {
		return mapFSErr(name, err), nil
	}
	content := withAttempts(out, map[string]any{"path": rel, "bytes": len(img), "mime": mime})
	if m, ok := out["image_model"].(string); ok && m != "" {
		content["model"] = m
	}
	return mediaOK(name, content), nil
}
