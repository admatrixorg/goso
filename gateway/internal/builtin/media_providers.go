// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Provider wire formats used by the media chains: Gemini's native
// generateContent (+ File API for large media), OpenAI-compatible chat
// completions with multimodal parts, /audio/transcriptions and
// /images/generations.

// geminiPollInterval is how often an uploaded Gemini file is polled until
// ACTIVE. Tests shorten it.
var geminiPollInterval = 2 * time.Second

// geminiInlineLimit is geminiInlineBytes; tests lower it to exercise the
// File API.
var geminiInlineLimit = geminiInlineBytes

type genReply struct {
	text   string
	images [][]byte
	refs   []string // image URLs to fetch (compat replies)
}

// ---- Gemini native ----

func geminiEndpoint(t *mediaTarget, model string) string {
	return strings.TrimRight(t.base, "/") + "/models/" + url.PathEscape(geminiModel(model)) + ":generateContent"
}

func geminiGenerate(ctx context.Context, t *mediaTarget, model string, parts []map[string]any, genCfg map[string]any) (*genReply, error) {
	if genCfg == nil {
		genCfg = map[string]any{}
	}
	if v, ok := t.params["temperature"].(float64); ok {
		genCfg["temperature"] = v
	}
	if n := t.paramInt("max_tokens", 0); n > 0 {
		genCfg["maxOutputTokens"] = n
	}
	body := map[string]any{"contents": []map[string]any{{"role": "user", "parts": parts}}}
	if len(genCfg) > 0 {
		body["generationConfig"] = genCfg
	}
	raw, _ := json.Marshal(body)
	var out struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text       string `json:"text"`
					Thought    bool   `json:"thought"`
					InlineData *struct {
						MimeType string `json:"mimeType"`
						Data     string `json:"data"`
					} `json:"inlineData"`
				} `json:"parts"`
			} `json:"content"`
			FinishReason string `json:"finishReason"`
		} `json:"candidates"`
		PromptFeedback *struct {
			BlockReason string `json:"blockReason"`
		} `json:"promptFeedback"`
	}
	if _, err := providerJSON(ctx, t, http.MethodPost, geminiEndpoint(t, model), nil, raw, maxProviderJSON, &out); err != nil {
		return nil, err
	}
	if len(out.Candidates) == 0 {
		if out.PromptFeedback != nil && out.PromptFeedback.BlockReason != "" {
			return nil, permanent("blocked: %s", out.PromptFeedback.BlockReason)
		}
		return nil, errors.New("empty response")
	}
	r := &genReply{}
	var sb strings.Builder
	for _, p := range out.Candidates[0].Content.Parts {
		if p.Thought {
			continue
		}
		if p.Text != "" {
			sb.WriteString(p.Text)
		}
		if p.InlineData != nil && p.InlineData.Data != "" {
			b, err := base64.StdEncoding.DecodeString(p.InlineData.Data)
			if err != nil {
				return nil, errors.New("bad base64 in response")
			}
			r.images = append(r.images, b)
		}
	}
	r.text = strings.TrimSpace(sb.String())
	if r.text == "" && len(r.images) == 0 {
		if fr := out.Candidates[0].FinishReason; fr != "" && fr != "STOP" {
			return nil, permanent("no output (finish reason %s)", fr)
		}
		return nil, errors.New("empty response")
	}
	return r, nil
}

// geminiMediaPart returns an inline part for small media, else uploads it
// through the File API and returns a fileData part plus a cleanup.
func geminiMediaPart(ctx context.Context, t *mediaTarget, mime string, data []byte) (map[string]any, func(), error) {
	if len(data) <= geminiInlineLimit {
		return map[string]any{"inlineData": map[string]any{"mimeType": mime, "data": base64.StdEncoding.EncodeToString(data)}}, func() {}, nil
	}
	name, uri, err := geminiUpload(ctx, t, mime, data)
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() {
		dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		_, _ = providerJSON(dctx, t, http.MethodDelete, strings.TrimRight(t.base, "/")+"/"+name, nil, nil, 1<<16, nil)
	}
	return map[string]any{"fileData": map[string]any{"mimeType": mime, "fileUri": uri}}, cleanup, nil
}

var geminiFileName = regexp.MustCompile(`^files/[a-z0-9-]{1,64}$`)

type geminiFile struct {
	Name     string `json:"name"`
	URI      string `json:"uri"`
	State    string `json:"state"`
	MimeType string `json:"mimeType"`
}

// geminiUpload runs the resumable File API upload and waits until the file
// is ACTIVE. The upload URL must stay on the provider's host.
func geminiUpload(ctx context.Context, t *mediaTarget, mime string, data []byte) (string, string, error) {
	base, err := url.Parse(t.base)
	if err != nil || base.Host == "" {
		return "", "", permanent("invalid provider base URL")
	}
	start := *base
	start.Path = "/upload" + strings.TrimRight(base.Path, "/") + "/files"
	start.RawQuery = ""
	meta, _ := json.Marshal(map[string]any{"file": map[string]any{"display_name": "goso-media"}})
	hdr, err := providerJSON(ctx, t, http.MethodPost, start.String(), map[string]string{
		"X-Goog-Upload-Protocol":              "resumable",
		"X-Goog-Upload-Command":               "start",
		"X-Goog-Upload-Header-Content-Length": strconv.Itoa(len(data)),
		"X-Goog-Upload-Header-Content-Type":   mime,
	}, meta, 1<<16, nil)
	if err != nil {
		return "", "", fmt.Errorf("upload start: %w", err)
	}
	up := strings.TrimSpace(hdr.Get("X-Goog-Upload-URL"))
	uu, err := url.Parse(up)
	if up == "" || err != nil || !strings.EqualFold(uu.Host, base.Host) || uu.Scheme != base.Scheme {
		return "", "", errors.New("upload start: bad upload URL")
	}
	var fin struct {
		File geminiFile `json:"file"`
	}
	if _, err := providerJSON(ctx, t, http.MethodPost, up, map[string]string{
		"Content-Type":          mime,
		"X-Goog-Upload-Command": "upload, finalize",
		"X-Goog-Upload-Offset":  "0",
	}, data, 1<<20, &fin); err != nil {
		return "", "", fmt.Errorf("upload: %w", err)
	}
	f := fin.File
	if !geminiFileName.MatchString(f.Name) || f.URI == "" {
		return "", "", errors.New("upload: bad file reply")
	}
	for f.State == "PROCESSING" || f.State == "STATE_UNSPECIFIED" {
		select {
		case <-ctx.Done():
			return "", "", errors.New("timed out waiting for file processing")
		case <-time.After(geminiPollInterval):
		}
		var cur geminiFile
		if _, err := providerJSON(ctx, t, http.MethodGet, strings.TrimRight(t.base, "/")+"/"+f.Name, nil, nil, 1<<20, &cur); err != nil {
			return "", "", fmt.Errorf("file status: %w", err)
		}
		f.State = cur.State
		if cur.URI != "" {
			f.URI = cur.URI
		}
	}
	if f.State != "" && f.State != "ACTIVE" {
		return "", "", permanent("file processing failed (%s)", f.State)
	}
	return f.Name, f.URI, nil
}

// ---- OpenAI-compatible chat ----

func dataURL(mime string, data []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func compatChat(ctx context.Context, t *mediaTarget, model, prompt string, parts []map[string]any, extra map[string]any) (*genReply, error) {
	content := make([]map[string]any, 0, len(parts)+1)
	if prompt != "" {
		content = append(content, map[string]any{"type": "text", "text": prompt})
	}
	content = append(content, parts...)
	body := map[string]any{
		"model":    model,
		"messages": []map[string]any{{"role": "user", "content": content}},
	}
	if v, ok := t.params["temperature"].(float64); ok {
		body["temperature"] = v
	}
	if n := t.paramInt("max_tokens", 0); n > 0 {
		body["max_tokens"] = n
	}
	for k, v := range extra {
		body[k] = v
	}
	raw, _ := json.Marshal(body)
	var out struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
				Images  []struct {
					ImageURL struct {
						URL string `json:"url"`
					} `json:"image_url"`
				} `json:"images"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if _, err := providerJSON(ctx, t, http.MethodPost, compatURL(t.base, "chat/completions"), nil, raw, maxProviderJSON, &out); err != nil {
		return nil, err
	}
	if out.Error != nil && out.Error.Message != "" && len(out.Choices) == 0 {
		msg := out.Error.Message
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return nil, errors.New(msg)
	}
	if len(out.Choices) == 0 {
		return nil, errors.New("empty response")
	}
	m := out.Choices[0].Message
	r := &genReply{}
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		r.text = strings.TrimSpace(s)
	} else {
		var ps []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			ImageURL struct {
				URL string `json:"url"`
			} `json:"image_url"`
		}
		if err := json.Unmarshal(m.Content, &ps); err == nil {
			var sb strings.Builder
			for _, p := range ps {
				switch {
				case p.Type == "text":
					sb.WriteString(p.Text)
				case p.ImageURL.URL != "":
					r.refs = append(r.refs, p.ImageURL.URL)
				}
			}
			r.text = strings.TrimSpace(sb.String())
		}
	}
	for _, im := range m.Images {
		if im.ImageURL.URL != "" {
			r.refs = append(r.refs, im.ImageURL.URL)
		}
	}
	if r.text == "" && len(r.refs) == 0 {
		return nil, errors.New("empty response")
	}
	return r, nil
}

// decodeImageRef turns a data: URL or an http(s) URL into image bytes.
func decodeImageRef(ctx context.Context, ref string) ([]byte, error) {
	if strings.HasPrefix(ref, "data:") {
		i := strings.Index(ref, ",")
		if i < 0 || !strings.Contains(ref[:i], ";base64") {
			return nil, errors.New("bad image data URL")
		}
		b, err := base64.StdEncoding.DecodeString(ref[i+1:])
		if err != nil {
			return nil, errors.New("bad image base64")
		}
		return b, nil
	}
	return fetchMediaURL(ctx, ref, MaxImageOutBytes)
}

// ---- /images/generations ----

func imagesGenerate(ctx context.Context, endpoint, key string, t *mediaTarget, model, prompt, size string) ([]byte, error) {
	payload, _ := json.Marshal(map[string]any{"model": model, "prompt": prompt, "n": 1, "size": size})
	var out struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
			URL     string `json:"url"`
		} `json:"data"`
	}
	var err error
	if t != nil {
		_, err = providerJSON(ctx, t, http.MethodPost, endpoint, nil, payload, maxProviderJSON, &out)
	} else {
		err = postProvider(ctx, endpoint, key, "application/json", payload, maxProviderJSON, &out)
	}
	if err != nil {
		return nil, err
	}
	if len(out.Data) == 0 {
		return nil, errors.New("empty response")
	}
	switch {
	case out.Data[0].B64JSON != "":
		b, err := base64.StdEncoding.DecodeString(out.Data[0].B64JSON)
		if err != nil {
			return nil, errors.New("bad base64")
		}
		return b, nil
	case out.Data[0].URL != "":
		b, err := fetchMediaURL(ctx, out.Data[0].URL, MaxImageOutBytes)
		if err != nil {
			return nil, fmt.Errorf("image download failed: %w", err)
		}
		return b, nil
	}
	return nil, errors.New("no image")
}

// ---- /audio/transcriptions ----

func transcriptionBody(model, lang, hint, mime, ext string, data []byte) ([]byte, string, error) {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="file"; filename="audio`+ext+`"`)
	h.Set("Content-Type", mime)
	part, err := mw.CreatePart(h)
	if err != nil {
		return nil, "", err
	}
	_, _ = part.Write(data)
	_ = mw.WriteField("model", model)
	if lang != "" {
		_ = mw.WriteField("language", lang)
	}
	_ = mw.WriteField("response_format", "json")
	if hint != "" {
		_ = mw.WriteField("prompt", hint)
	}
	if err := mw.Close(); err != nil {
		return nil, "", err
	}
	return body.Bytes(), mw.FormDataContentType(), nil
}
