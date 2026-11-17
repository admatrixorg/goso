// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif" // decoders for inline downscaling
	"image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mqglobal/goso/gateway/internal/builtin"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/pipeline"
	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Inbound media. Each adapter turns the files of one customer message into
// Attachments (pure parsing, no network) and gives a MediaFetcher that knows
// the platform's download call. withInboundMedia then downloads each file
// with a size cap and a timeout, saves it in the agent tenant's workspace
// (GOSO_WORKSPACE/<tenant>/.uploads), transcribes voice when speech-to-text
// is configured, and returns the turn text the agent reads:
//
//	<media:image path=".uploads/photo-….jpg">
//	<media:voice path=".uploads/voice-….ogg">
//	<transcript>…</transcript>
//	<media:document name="bao-gia.pdf" path=".uploads/bao-gia-….pdf">
//	[File: bao-gia.pdf — use read_document to read it]
//
//	<customer text or caption>
//
// Images also go to the model as image input for this turn when its
// provider takes images (pipeline.WithTurnImages); otherwise the agent can
// open the saved path with read_image. Platform file URLs can carry the bot
// token (Telegram) or need it as a header (Slack, WhatsApp, Lark): they are
// built inside the fetcher only and never logged, stored or shown.

// Attachment kinds.
const (
	MediaImage    = "image"
	MediaDocument = "document"
	MediaAudio    = "audio"
	MediaVoice    = "voice"
	MediaVideo    = "video"
	// MediaSticker is a sticker the platform serves as an image file.
	MediaSticker = "sticker"
)

const (
	// MaxInboundImageBytes caps one received image (read_image's cap).
	MaxInboundImageBytes = 10 << 20
	// MaxInboundFileBytes caps any other received file (the Telegram Bot
	// API download limit, and GoClaw's default).
	MaxInboundFileBytes = 20 << 20
	// MaxInboundAttachments is how many files of one message are handled.
	MaxInboundAttachments = 10
	// MaxInlineImages is how many images of one message go to the model as
	// image input; the rest are reachable by path.
	MaxInlineImages = 5
	// MediaDownloadTimeout bounds one file download.
	MediaDownloadTimeout = 60 * time.Second
	// maxInlineImageBytes keeps one inline image under the strictest
	// provider limit (5 MB of base64).
	maxInlineImageBytes = 3_700_000
	// maxInlineImageSide is the longest side an inline image is scaled to.
	maxInlineImageSide = 1600
	// maxDecodePixels refuses to decode larger images (decompression bombs).
	maxDecodePixels = 40_000_000
	// maxInlineDocChars caps a text document copied into the turn.
	maxInlineDocChars = 50_000
)

// Attachment is one file of an inbound message.
type Attachment struct {
	Kind string
	// Name is the file name the customer's app gave, if any.
	Name string
	// MIME and Size are what the platform claims; the bytes decide.
	MIME string
	Size int64
	// Ref is the platform's handle: a file id, a resource key, or a public
	// URL. It is passed to the adapter's fetcher only.
	Ref string
	// Note replaces the download: the agent reads it as is (a sticker).
	Note string
}

// MediaFetcher downloads one attachment, at most limit bytes.
type MediaFetcher func(ctx context.Context, a Attachment, limit int64) ([]byte, error)

var (
	errMediaTooLarge = errors.New("too large")
	errMediaFetch    = errors.New("download failed")
	errMediaNoRef    = errors.New("no file reference")
)

// mediaKindFromMIME classifies a platform content type.
func mediaKindFromMIME(mime string) string {
	mime = strings.ToLower(strings.TrimSpace(mime))
	switch {
	case strings.HasPrefix(mime, "image/"):
		return MediaImage
	case strings.HasPrefix(mime, "audio/"):
		return MediaAudio
	case strings.HasPrefix(mime, "video/"):
		return MediaVideo
	}
	return MediaDocument
}

func mediaLimit(kind string) int64 {
	if kind == MediaImage || kind == MediaSticker {
		return MaxInboundImageBytes
	}
	return MaxInboundFileBytes
}

// downloadMedia runs req and reads at most limit bytes. Errors are generic:
// a transport error names the URL (and so a token in it), so it is dropped.
func downloadMedia(client *http.Client, req *http.Request, limit int64) ([]byte, error) {
	if client == nil {
		client = &http.Client{Timeout: MediaDownloadTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(req.Context().Err(), context.DeadlineExceeded) {
			return nil, errors.New("download timed out")
		}
		return nil, errMediaFetch
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed: http %d", resp.StatusCode)
	}
	if resp.ContentLength > limit {
		return nil, errMediaTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, errMediaFetch
	}
	if int64(len(data)) > limit {
		return nil, errMediaTooLarge
	}
	return data, nil
}

// mediaClient is base (or a default) with the download timeout and the SSRF
// redirect check. base's Transport is kept (tests).
func mediaClient(base *http.Client) *http.Client {
	c := &http.Client{Timeout: MediaDownloadTimeout}
	if base != nil {
		c.Transport = base.Transport
		c.CheckRedirect = base.CheckRedirect
	}
	security.GuardClient(c)
	return c
}

// fetchURL downloads a URL the message itself carried (a Zalo CDN link, a
// Discord attachment). Only http(s), and the SSRF guard applies. header is
// an optional extra header (Slack's bearer token): it is dropped on any
// redirect.
func fetchURL(ctx context.Context, base *http.Client, raw string, limit int64, header, value string) ([]byte, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errMediaNoRef
	}
	if err := security.CheckURL(u.String()); err != nil {
		return nil, errors.New("download blocked")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errMediaNoRef
	}
	c := mediaClient(base)
	if header != "" && value != "" {
		req.Header.Set(header, value)
		guard := c.CheckRedirect
		c.CheckRedirect = func(r *http.Request, via []*http.Request) error {
			r.Header.Del(header)
			return guard(r, via)
		}
	}
	return downloadMedia(c, req, limit)
}

// inboundResult is what one message's files add to the turn.
type inboundResult struct {
	Note   string
	Images []llm.ImagePart
}

// withInboundMedia downloads, saves and describes atts for agent's tenant.
// It returns the turn context (carrying inline images) and the turn text.
// Nothing here fails the turn: a file that cannot be fetched becomes a note.
func withInboundMedia(ctx context.Context, agent *store.Agent, channelName, text string, atts []Attachment, fetch MediaFetcher) (context.Context, string) {
	if len(atts) == 0 {
		return ctx, text
	}
	tenant := store.DefaultTenant
	if agent != nil {
		tenant = store.NormalizeTenant(agent.TenantID)
	}
	res := prepareInbound(ctx, channelName, tenant, atts, fetch)
	ctx = pipeline.WithTurnImages(ctx, res.Images)
	return ctx, joinMediaText(res.Note, text)
}

func joinMediaText(note, text string) string {
	note, text = strings.TrimSpace(note), strings.TrimSpace(text)
	switch {
	case note == "":
		return text
	case text == "":
		return note
	}
	return note + "\n\n" + text
}

func prepareInbound(ctx context.Context, channelName, tenant string, atts []Attachment, fetch MediaFetcher) inboundResult {
	var res inboundResult
	var notes []string
	for i, a := range atts {
		if i >= MaxInboundAttachments {
			notes = append(notes, fmt.Sprintf("[%d more attachments not processed]", len(atts)-i))
			break
		}
		notes = append(notes, describeAttachment(ctx, channelName, tenant, a, fetch, &res))
	}
	res.Note = strings.Join(notes, "\n")
	return res
}

func describeAttachment(ctx context.Context, channelName, tenant string, a Attachment, fetch MediaFetcher, res *inboundResult) string {
	kind := a.Kind
	if kind == "" {
		kind = mediaKindFromMIME(a.MIME)
	}
	if a.Note != "" {
		return a.Note
	}
	limit := mediaLimit(kind)
	if a.Size > limit {
		return skippedNote(kind, fmt.Sprintf("too large (max %d MB)", limit>>20))
	}
	if fetch == nil || strings.TrimSpace(a.Ref) == "" {
		return skippedNote(kind, "download not available")
	}
	dctx, cancel := context.WithTimeout(ctx, MediaDownloadTimeout)
	data, err := fetch(dctx, a, limit)
	cancel()
	if err != nil {
		reason := err.Error()
		if errors.Is(err, errMediaTooLarge) {
			reason = fmt.Sprintf("too large (max %d MB)", limit>>20)
		}
		log.Printf("goso channel %s: inbound %s skipped: %s", channelName, kind, reason)
		return skippedNote(kind, reason)
	}
	if len(data) == 0 {
		return skippedNote(kind, "empty file")
	}

	imgMIME, imgExt, isImage := builtin.SniffImage(data)
	if kind == MediaSticker && !isImage {
		return "[sticker]"
	}
	if (kind == MediaDocument || kind == MediaSticker) && isImage {
		kind = MediaImage
	}
	if kind == MediaImage && !isImage {
		kind = MediaDocument
	}
	name := cleanAttr(a.Name)

	switch kind {
	case MediaImage:
		if int64(len(data)) > MaxInboundImageBytes {
			return skippedNote(kind, fmt.Sprintf("too large (max %d MB)", MaxInboundImageBytes>>20))
		}
		rel := saveInbound(ctx, channelName, tenant, a.Name, imgExt, "photo", data)
		inline := false
		if len(res.Images) < MaxInlineImages {
			if part, ok := inlineImage(data, imgMIME); ok {
				res.Images = append(res.Images, part)
				inline = true
			}
		}
		if rel == "" && !inline {
			return skippedNote(kind, "not saved: workspace is not configured")
		}
		return mediaTag(kind, "", rel)
	case MediaVoice, MediaAudio:
		ext := filepath.Ext(a.Name)
		if _, aext, ok := builtin.SniffAudio(data); ok {
			ext = aext
		}
		rel := saveInbound(ctx, channelName, tenant, a.Name, ext, kind, data)
		tag := mediaTag(kind, name, rel)
		what := "Voice message"
		if kind == MediaAudio {
			what = "Audio"
		}
		if !builtin.STTReady() {
			return tag + "\n[" + what + " not transcribed: speech-to-text is not configured]"
		}
		sctx, cancel := context.WithTimeout(ctx, MediaDownloadTimeout)
		text, err := builtin.Transcribe(sctx, data, "")
		cancel()
		if err != nil {
			log.Printf("goso channel %s: inbound %s not transcribed: %v", channelName, kind, err)
			return tag + "\n[" + what + " not transcribed: transcription failed]"
		}
		if strings.TrimSpace(text) == "" {
			return tag + "\n[" + what + ": no speech recognized]"
		}
		return tag + "\n<transcript>" + escapeMediaText(text) + "</transcript>"
	case MediaVideo:
		rel := saveInbound(ctx, channelName, tenant, a.Name, filepath.Ext(a.Name), "video", data)
		return mediaTag(kind, name, rel) + "\n[Video received: its content cannot be watched; ask the customer what it shows if it matters]"
	default:
		rel := saveInbound(ctx, channelName, tenant, a.Name, filepath.Ext(a.Name), "file", data)
		label := name
		if label == "" {
			label = "file"
		}
		tag := mediaTag(MediaDocument, name, rel)
		if body, ok := inlineText(a.Name, data); ok {
			return tag + "\n<file name=\"" + label + "\">\n" + body + "\n</file>"
		}
		if rel == "" {
			return tag + "\n[File: " + label + " — not saved: workspace is not configured]"
		}
		return tag + "\n[File: " + label + " — use read_document to read it]"
	}
}

func skippedNote(kind, reason string) string {
	return "[sent media (" + kind + ") — skipped: " + reason + "]"
}

func mediaTag(kind, name, rel string) string {
	var b strings.Builder
	b.WriteString("<media:" + kind)
	if name != "" {
		b.WriteString(` name="` + name + `"`)
	}
	if rel != "" {
		b.WriteString(` path="` + rel + `"`)
	}
	b.WriteString(">")
	return b.String()
}

// saveInbound stores data in the tenant workspace; "" when it cannot.
func saveInbound(ctx context.Context, channelName, tenant, name, ext, def string, data []byte) string {
	if !builtin.WorkspaceReady() {
		return ""
	}
	if strings.TrimSpace(name) == "" {
		name = def
	}
	rel, err := builtin.SaveUpload(ctx, tenant, name, ext, data)
	if err != nil {
		log.Printf("goso channel %s: inbound file not saved: %v", channelName, err)
		return ""
	}
	return rel
}

// cleanAttr makes a customer-given file name safe inside a tag attribute.
func cleanAttr(s string) string {
	s = strings.TrimSpace(filepath.Base(strings.ReplaceAll(s, "\\", "/")))
	if s == "." || s == "/" {
		return ""
	}
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '"' || r == '<' || r == '>' || r == '&':
			return '_'
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, s)
	if utf8.RuneCountInString(s) > 120 {
		s = string([]rune(s)[:120])
	}
	return s
}

func escapeMediaText(s string) string {
	return strings.NewReplacer("<", "‹", ">", "›").Replace(strings.TrimSpace(s))
}

var textDocExt = map[string]bool{
	".txt": true, ".md": true, ".csv": true, ".tsv": true, ".json": true, ".xml": true,
	".yaml": true, ".yml": true, ".log": true, ".html": true, ".htm": true, ".sql": true,
}

// inlineText returns a text document's content for the turn, capped.
func inlineText(name string, data []byte) (string, bool) {
	if !textDocExt[strings.ToLower(filepath.Ext(name))] || !utf8.Valid(data) {
		return "", false
	}
	s := string(data)
	if utf8.RuneCountInString(s) > maxInlineDocChars {
		s = string([]rune(s)[:maxInlineDocChars]) + "\n[truncated]"
	}
	return strings.NewReplacer("</file>", "</ file>").Replace(s), true
}

// inlineImage returns the image as model input: small images as they are,
// larger jpeg/png/gif scaled down and re-encoded as JPEG. ok is false when
// the image cannot be made small enough (webp over the cap, huge images).
func inlineImage(data []byte, mime string) (llm.ImagePart, bool) {
	cfg, _, cfgErr := image.DecodeConfig(bytes.NewReader(data))
	small := int64(len(data)) <= maxInlineImageBytes
	if small && (cfgErr != nil || (cfg.Width <= 2*maxInlineImageSide && cfg.Height <= 2*maxInlineImageSide)) {
		return llm.ImagePart{MIME: mime, Data: data}, true
	}
	if cfgErr != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxDecodePixels {
		return llm.ImagePart{}, false
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return llm.ImagePart{}, false
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, scaleDown(src, maxInlineImageSide), &jpeg.Options{Quality: 85}); err != nil {
		return llm.ImagePart{}, false
	}
	if buf.Len() > maxInlineImageBytes {
		return llm.ImagePart{}, false
	}
	return llm.ImagePart{MIME: "image/jpeg", Data: buf.Bytes()}, true
}

// scaleDown fits src in side×side by averaging source blocks.
func scaleDown(src image.Image, side int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= side && h <= side {
		return src
	}
	nw, nh := side, h*side/w
	if h > w {
		nw, nh = w*side/h, side
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		y0, y1 := b.Min.Y+y*h/nh, b.Min.Y+(y+1)*h/nh
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < nw; x++ {
			x0, x1 := b.Min.X+x*w/nw, b.Min.X+(x+1)*w/nw
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, bl, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					cr, cg, cb, _ := src.At(sx, sy).RGBA()
					r, g, bl, n = r+uint64(cr), g+uint64(cg), bl+uint64(cb), n+1
				}
			}
			i := dst.PixOffset(x, y)
			dst.Pix[i] = uint8(r / n >> 8)
			dst.Pix[i+1] = uint8(g / n >> 8)
			dst.Pix[i+2] = uint8(bl / n >> 8)
			dst.Pix[i+3] = 0xff
		}
	}
	return dst
}
