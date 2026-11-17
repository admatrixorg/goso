// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import "encoding/base64"

// ImagePart is one inline image for a vision call. MIME is image/png,
// image/jpeg, image/gif or image/webp; the caller sniffs and caps Data.
type ImagePart struct {
	MIME string
	Data []byte
}

// ImageAccepter is implemented by a provider outside this package that
// encodes Message.Images (tests, wrappers).
type ImageAccepter interface {
	AcceptsImages() bool
}

// SupportsImages reports whether p encodes Message.Images on the wire.
// Echo and other providers would silently drop the image.
func SupportsImages(p Provider) bool {
	switch v := p.(type) {
	case *OpenAI, *Anthropic:
		return true
	case ImageAccepter:
		return v.AcceptsImages()
	}
	return false
}

// openaiContentParts is the Chat Completions multi-part user content:
// one text part, then one image_url data URL per image.
func openaiContentParts(text string, images []ImagePart) []map[string]any {
	parts := make([]map[string]any, 0, len(images)+1)
	if text != "" {
		parts = append(parts, map[string]any{"type": "text", "text": text})
	}
	for _, img := range images {
		parts = append(parts, map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url": "data:" + img.MIME + ";base64," + base64.StdEncoding.EncodeToString(img.Data),
			},
		})
	}
	return parts
}

// anthropicContentParts is the Messages API user content: base64 image
// blocks first (Anthropic's recommended order), then the text block.
func anthropicContentParts(text string, images []ImagePart) []map[string]any {
	parts := make([]map[string]any, 0, len(images)+1)
	for _, img := range images {
		parts = append(parts, map[string]any{
			"type": "image",
			"source": map[string]any{
				"type":       "base64",
				"media_type": img.MIME,
				"data":       base64.StdEncoding.EncodeToString(img.Data),
			},
		})
	}
	if text != "" {
		parts = append(parts, map[string]any{"type": "text", "text": text})
	}
	return parts
}
