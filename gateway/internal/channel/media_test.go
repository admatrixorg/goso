// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/pipeline"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// fakeBotToken is a made-up bot token, assembled at run time so secret
// scanners do not flag the fixture.
var fakeBotToken = strings.Join([]string{"4242", "demo-fixture-not-a-token"}, ":")

// mediaEnv clears the media settings and allows loopback test servers.
func mediaEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"GOSO_WORKSPACE", "GOSO_STT_URL", "GOSO_STT_API_KEY", "GOSO_STT_MODEL", "GOSO_STT_LANGUAGE"} {
		t.Setenv(k, "")
	}
	t.Setenv("GOSO_SSRF", "0")
	t.Setenv("GOSO_ENV", "demo")
}

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x += 7 {
		img.Set(x, 0, color.RGBA{R: 200, A: 255})
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// captureLog collects log output for the rest of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	var mu sync.Mutex
	prev := log.Writer()
	log.SetOutput(writerFunc(func(p []byte) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		return buf.Write(p)
	}))
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

// savedFiles lists the files under root, relative to it.
func savedFiles(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(root, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	return out
}

// ---- attachment parsing, one adapter at a time ----

func TestTelegramAttachmentsParsing(t *testing.T) {
	var upd TelegramUpdate
	raw := `{"update_id":7,"message":{"message_id":3,"chat":{"id":9001},"from":{"id":9001},
	  "caption":"Ảnh đơn hàng của Minh An",
	  "photo":[{"file_id":"ph_small","width":90,"height":60,"file_size":900},{"file_id":"ph_big","width":1280,"height":853,"file_size":99000},{"file_id":"ph_mid","width":320,"height":213}],
	  "document":{"file_id":"doc_1","file_name":"bao-gia.pdf","mime_type":"application/pdf","file_size":2048},
	  "voice":{"file_id":"voice_1","mime_type":"audio/ogg","file_size":4000},
	  "video_note":{"file_id":"vn_1","file_size":5000},
	  "sticker":{"file_id":"st_1"}}}`
	if err := json.Unmarshal([]byte(raw), &upd); err != nil {
		t.Fatal(err)
	}
	got := telegramAttachments(upd)
	want := []Attachment{
		{Kind: MediaImage, Size: 99000, Ref: "ph_big"},
		{Kind: MediaDocument, Name: "bao-gia.pdf", MIME: "application/pdf", Size: 2048, Ref: "doc_1"},
		{Kind: MediaVoice, MIME: "audio/ogg", Size: 4000, Ref: "voice_1"},
		{Kind: MediaVideo, Size: 5000, Ref: "vn_1"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("[%d] got %+v want %+v", i, got[i], want[i])
		}
	}
	// An audio file sent as a document is audio.
	upd.Message.Photo, upd.Message.Voice, upd.Message.VideoNote = nil, nil, nil
	upd.Message.Document = &TelegramFile{FileID: "doc_2", FileName: "ghi-am.mp3", MIMEType: "audio/mpeg"}
	if got := telegramAttachments(upd); len(got) != 1 || got[0].Kind != MediaAudio {
		t.Fatalf("audio document %+v", got)
	}
}

func TestSlackAttachmentsParsing(t *testing.T) {
	files := []SlackFile{
		{ID: "F1", Name: "menu.png", Mimetype: "image/png", Size: 10, URLPrivate: "https://files.slack.com/files-pri/T1-F1/menu.png", URLPrivateDownload: "https://files.slack.com/files-pri/T1-F1/download/menu.png"},
		{ID: "F2", Name: "notes.txt", Mimetype: "text/plain", URLPrivate: "https://files.slack.com/files-pri/T1-F2/notes.txt"},
		{ID: "F3", Name: "clip.mp4", Mimetype: "video/mp4", URLPrivate: "https://files.slack.com/x/clip.mp4"},
		{ID: "F4", Name: "gone"},
	}
	got := slackAttachments(files)
	if len(got) != 3 || got[0].Kind != MediaImage || !strings.HasSuffix(got[0].Ref, "/download/menu.png") ||
		got[1].Kind != MediaDocument || got[2].Kind != MediaVideo {
		t.Fatalf("got %+v", got)
	}
	for raw, ok := range map[string]bool{
		"https://files.slack.com/a":        true,
		"https://x.slack-edge.com/a":       true,
		"https://files.slack-files.com/a":  true,
		"http://files.slack.com/a":         false,
		"https://evil.example/a":           false,
		"https://slack.com.evil.example/a": false,
		"https://notslack.com/a":           false,
	} {
		if slackFileHost(raw) != ok {
			t.Fatalf("slackFileHost(%q) = %v", raw, !ok)
		}
	}
}

func TestDiscordAttachmentsParsing(t *testing.T) {
	got := discordAttachments([]DiscordAttachment{
		{ID: "1", Filename: "anh.jpg", ContentType: "image/jpeg", Size: 5, URL: "https://cdn.discordapp.com/attachments/1/2/anh.jpg"},
		{ID: "2", Filename: "voice-message.ogg", ContentType: "audio/ogg", URL: "https://cdn.discordapp.com/attachments/1/3/voice-message.ogg"},
		{ID: "3", Filename: "hop-dong.pdf", ContentType: "application/pdf", URL: "https://cdn.discordapp.com/attachments/1/4/hop-dong.pdf"},
	})
	if len(got) != 3 || got[0].Kind != MediaImage || got[1].Kind != MediaAudio || got[2].Kind != MediaDocument || got[2].Name != "hop-dong.pdf" {
		t.Fatalf("got %+v", got)
	}
}

func TestWhatsAppAttachmentsParsing(t *testing.T) {
	var upd WhatsAppUpdate
	raw := `{"object":"whatsapp_business_account","entry":[{"changes":[{"value":{"metadata":{"phone_number_id":"pn_demo"},
	  "messages":[{"from":"84900000001","type":"audio","audio":{"id":"media_v1","mime_type":"audio/ogg; codecs=opus","voice":true}}]}}]}]}`
	if err := json.Unmarshal([]byte(raw), &upd); err != nil {
		t.Fatal(err)
	}
	m := upd.Entry[0].Changes[0].Value.Messages[0]
	a, ok := whatsappAttachment(MediaAudio, m.Audio)
	if !ok || a.Kind != MediaVoice || a.Ref != "media_v1" {
		t.Fatalf("voice %+v", a)
	}
	if _, ok := whatsappAttachment(MediaImage, nil); ok {
		t.Fatal("nil media")
	}
	d, _ := whatsappAttachment(MediaDocument, &WhatsAppMedia{ID: "media_d1", MIMEType: "application/pdf", Filename: "bao-gia.pdf", Caption: "gửi anh"})
	if d.Kind != MediaDocument || d.Name != "bao-gia.pdf" {
		t.Fatalf("doc %+v", d)
	}
}

func TestZaloOAAttachmentsParsing(t *testing.T) {
	var upd ZaloOAUpdate
	raw := `{"event_name":"user_send_image","sender":{"id":"u_demo"},"message":{"text":"","attachments":[
	  {"type":"image","payload":{"url":"https://photo.example/anh.jpg","thumbnail":"https://photo.example/t.jpg"}},
	  {"type":"file","payload":{"url":"https://file.example/bao-gia.pdf","name":"bao-gia.pdf","size":"2048"}},
	  {"type":"audio","payload":{"url":"https://file.example/voice.aac"}},
	  {"type":"gif","payload":{"thumbnail":"https://photo.example/g.gif"}},
	  {"type":"sticker","payload":{"id":"st1","url":"https://sticker.example/s.webp"}},
	  {"type":"location","payload":{"coordinates":{}}}]}}`
	if err := json.Unmarshal([]byte(raw), &upd); err != nil {
		t.Fatal(err)
	}
	got := zaloOAAttachments(upd.Message.Attachments)
	if len(got) != 5 {
		t.Fatalf("got %+v", got)
	}
	if got[0].Kind != MediaImage || got[1].Kind != MediaDocument || got[1].Size != 2048 || got[1].Name != "bao-gia.pdf" ||
		got[2].Kind != MediaVoice || got[3].Kind != MediaImage || got[3].Ref != "https://photo.example/g.gif" ||
		got[4].Kind != MediaSticker || got[4].Note != "[sticker]" || got[4].Ref != "" {
		t.Fatalf("got %+v", got)
	}
}

func TestZaloBotImageEventParsing(t *testing.T) {
	evs := decodeZaloBotEvents([]byte(`{"ok":true,"result":{"event_name":"message.image.received","message":{
	  "from":{"id":"u_demo","display_name":"Minh An"},"chat":{"id":"c_demo","chat_type":"PRIVATE"},
	  "photo_url":"https://photo.example/zb.jpg","caption":"mẫu này còn không","message_id":"m1"}}}`))
	if len(evs) != 1 {
		t.Fatalf("events %d", len(evs))
	}
	atts := zaloBotAttachments(evs[0])
	if len(atts) != 1 || atts[0].Kind != MediaImage || atts[0].Ref != "https://photo.example/zb.jpg" {
		t.Fatalf("atts %+v", atts)
	}
	if zaloBotText(evs[0]) != "mẫu này còn không" {
		t.Fatalf("text %q", zaloBotText(evs[0]))
	}
	text := zaloBotEvent{EventName: zaloBotTextEvent}
	if zaloBotAttachments(text) != nil {
		t.Fatal("text event has no attachments")
	}
}

func TestZCRMBridgeAttachmentsParsing(t *testing.T) {
	ev := func(kind, content string) ZCRMEvent {
		var e ZCRMEvent
		e.Data.ContentType, e.Data.Content = kind, content
		return e
	}
	photo := ev("photo", `{"title":"","description":"Minh An gửi ảnh","href":"https://zalo.example/p.jpg","thumb":"https://zalo.example/t.jpg"}`)
	if a := bridgeAttachments(photo); len(a) != 1 || a[0].Kind != MediaImage || a[0].Ref != "https://zalo.example/p.jpg" {
		t.Fatalf("photo %+v", a)
	}
	if bridgeText(photo) != "Minh An gửi ảnh" {
		t.Fatalf("caption %q", bridgeText(photo))
	}
	file := ev("file", `{"title":"bao-gia.pdf","href":"https://zalo.example/f","params":{"fileSize":"4096","fileExt":"pdf"}}`)
	if a := bridgeAttachments(file); len(a) != 1 || a[0].Kind != MediaDocument || a[0].Name != "bao-gia.pdf" || a[0].Size != 4096 {
		t.Fatalf("file %+v", a)
	}
	voice := ev("voice", "https://zalo.example/v.m4a")
	if a := bridgeAttachments(voice); len(a) != 1 || a[0].Kind != MediaVoice || bridgeText(voice) != "" {
		t.Fatalf("voice %+v %q", a, bridgeText(voice))
	}
	sticker := ev("sticker", `{"id":1}`)
	if a := bridgeAttachments(sticker); len(a) != 1 || a[0].Note != "[sticker]" {
		t.Fatalf("sticker %+v", a)
	}
	// A kind with no file, or a media message with no link, is a note.
	if bridgeText(ev("location", `{}`)) != "[location]" || bridgeAttachments(ev("location", `{}`)) != nil {
		t.Fatal("location")
	}
	if bridgeText(ev("photo", `{"title":"x"}`)) != "[photo]" || bridgeAttachments(ev("photo", `{"title":"x"}`)) != nil {
		t.Fatal("photo without link")
	}
	if bridgeText(ev("text", " xin chào ")) != "xin chào" {
		t.Fatal("text")
	}
}

func TestLarkContentParsing(t *testing.T) {
	cases := []struct {
		typ, content, text string
		kinds              []string
		ok                 bool
	}{
		{"text", `{"text":"xin chào"}`, "xin chào", nil, true},
		{"image", `{"image_key":"img_demo_1"}`, "", []string{MediaImage}, true},
		{"file", `{"file_key":"file_demo_1","file_name":"bao-gia.pdf"}`, "", []string{MediaDocument}, true},
		{"audio", `{"file_key":"file_demo_2","duration":3000}`, "", []string{MediaVoice}, true},
		{"media", `{"file_key":"file_demo_3","image_key":"img_thumb","file_name":"clip.mp4"}`, "", []string{MediaVideo}, true},
		{"sticker", `{"file_key":"st_demo"}`, "", []string{MediaSticker}, true},
		{"post", `{"title":"Đơn hàng","content":[[{"tag":"text","text":"Minh An gửi"},{"tag":"img","image_key":"img_p1"}],[{"tag":"text","text":"cảm ơn"}]]}`, "Đơn hàng\nMinh An gửi\ncảm ơn", []string{MediaImage}, true},
		{"post", `{"zh_cn":{"title":"","content":[[{"tag":"img","image_key":"img_p2"}]]}}`, "", []string{MediaImage}, true},
		{"image", `{}`, "", nil, false},
		{"share_chat", `{"chat_id":"oc_x"}`, "", nil, false},
	}
	for _, c := range cases {
		text, atts, ok := larkContent(c.typ, c.content)
		if ok != c.ok || text != c.text || len(atts) != len(c.kinds) {
			t.Fatalf("%s %s: %q %+v %v", c.typ, c.content, text, atts, ok)
		}
		for i, k := range c.kinds {
			if atts[i].Kind != k {
				t.Fatalf("%s: kind %s", c.typ, atts[i].Kind)
			}
		}
	}
	l := &Lark{}
	msg, ok := l.parseLarkEvent([]byte(`{"header":{"event_type":"im.message.receive_v1"},"event":{"sender":{"sender_id":{"open_id":"ou_demo"},"sender_type":"user"},
	  "message":{"message_id":"om_demo","chat_id":"oc_demo","chat_type":"p2p","message_type":"image","content":"{\"image_key\":\"img_demo_9\"}"}}}`))
	if !ok || msg.Text != "" || len(msg.Attachments) != 1 || msg.Attachments[0].Ref != "img_demo_9" {
		t.Fatalf("image event %+v %v", msg, ok)
	}
}

// ---- download guard ----

func TestFetchURLSizeCap(t *testing.T) {
	mediaEnv(t)
	big := bytes.Repeat([]byte("a"), 4096)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chunked" {
			// No Content-Length: the cap applies while reading.
			w.(http.Flusher).Flush()
		}
		_, _ = w.Write(big)
	}))
	defer srv.Close()
	ctx := context.Background()
	for _, p := range []string{"/sized", "/chunked"} {
		if _, err := fetchURL(ctx, nil, srv.URL+p, 1024, "", ""); err != errMediaTooLarge {
			t.Fatalf("%s: err %v", p, err)
		}
		if b, err := fetchURL(ctx, nil, srv.URL+p, 8192, "", ""); err != nil || len(b) != len(big) {
			t.Fatalf("%s: %d %v", p, len(b), err)
		}
	}
	if _, err := fetchURL(ctx, nil, "file:///etc/passwd", 1024, "", ""); err == nil {
		t.Fatal("file scheme fetched")
	}
}

func TestFetchURLBlockedBySSRFGuard(t *testing.T) {
	mediaEnv(t)
	t.Setenv("GOSO_SSRF", "1")
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { atomic.AddInt32(&hits, 1) }))
	defer srv.Close()
	if _, err := fetchURL(context.Background(), nil, srv.URL+"/x", 1024, "", ""); err == nil || hits != 0 {
		t.Fatalf("loopback fetched: %v hits=%d", err, hits)
	}
}

func TestFetchURLDropsAuthOnRedirect(t *testing.T) {
	mediaEnv(t)
	var finalAuth atomic.Value
	finalAuth.Store("unset")
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/final", http.StatusFound)
	})
	mux.HandleFunc("/final", func(w http.ResponseWriter, r *http.Request) {
		finalAuth.Store(r.Header.Get("Authorization"))
		_, _ = w.Write([]byte("ok"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	b, err := fetchURL(context.Background(), nil, srv.URL+"/start", 1024, "Authorization", "Bearer "+fakeBotToken)
	if err != nil || string(b) != "ok" {
		t.Fatalf("%q %v", b, err)
	}
	if got := finalAuth.Load().(string); got != "" {
		t.Fatalf("auth header followed the redirect: %q", got)
	}
}

// telegramFileServer serves getFile and the file endpoint for fakeBotToken.
func telegramFileServer(t *testing.T, files map[string][]byte) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/bot"+fakeBotToken+"/getFile":
			id := r.URL.Query().Get("file_id")
			if _, ok := files[id]; !ok && id != "missing" {
				_, _ = w.Write([]byte(`{"ok":false}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": map[string]any{"file_path": "docs/" + id}})
		case strings.HasPrefix(r.URL.Path, "/file/bot"+fakeBotToken+"/docs/"):
			b, ok := files[strings.TrimPrefix(r.URL.Path, "/file/bot"+fakeBotToken+"/docs/")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(b)
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestTelegramFetchNeverLeaksToken(t *testing.T) {
	mediaEnv(t)
	logs := captureLog(t)
	srv := telegramFileServer(t, map[string][]byte{"doc_ok": []byte("hello")})
	defer srv.Close()
	tg := &Telegram{BotToken: fakeBotToken, APIBase: srv.URL}
	b, err := tg.fetchFile(context.Background(), Attachment{Ref: "doc_ok"}, 1024)
	if err != nil || string(b) != "hello" {
		t.Fatalf("%q %v", b, err)
	}
	_, err = tg.fetchFile(context.Background(), Attachment{Ref: "missing"}, 1024)
	if err == nil || strings.Contains(err.Error(), fakeBotToken) || strings.Contains(err.Error(), srv.URL) {
		t.Fatalf("error leaks: %v", err)
	}
	agent := &store.Agent{ID: "a1", TenantID: "minh-an"}
	_, text := withInboundMedia(context.Background(), agent, "telegram", "", []Attachment{{Kind: MediaDocument, Name: "x.pdf", Ref: "missing"}}, tg.fetchFile)
	if !strings.Contains(text, "[sent media (document) — skipped: download failed: http 404]") {
		t.Fatalf("note %q", text)
	}
	for _, out := range []string{text, logs.String()} {
		if strings.Contains(out, fakeBotToken) || strings.Contains(out, "/file/bot") {
			t.Fatalf("token leaked: %q", out)
		}
	}
}

func TestTelegramFetchRejectsBadFilePath(t *testing.T) {
	mediaEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"../../etc/passwd"}}`))
	}))
	defer srv.Close()
	tg := &Telegram{BotToken: fakeBotToken, APIBase: srv.URL}
	if _, err := tg.fetchFile(context.Background(), Attachment{Ref: "x"}, 1024); err != errMediaNoRef {
		t.Fatalf("err %v", err)
	}
}

func TestSlackFetchSendsTokenOnlyToSlack(t *testing.T) {
	mediaEnv(t)
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { atomic.AddInt32(&hits, 1) }))
	defer srv.Close()
	s := &Slack{BotToken: fakeBotToken}
	_, err := s.fetchFile(context.Background(), Attachment{Ref: srv.URL + "/files-pri/x"}, 1024)
	if err == nil || hits != 0 {
		t.Fatalf("non-Slack host fetched: %v hits=%d", err, hits)
	}
}

func TestWhatsAppFetchMedia(t *testing.T) {
	mediaEnv(t)
	var srv *httptest.Server
	var authOK int32
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer "+fakeBotToken {
			atomic.AddInt32(&authOK, 1)
		}
		switch r.URL.Path {
		case "/media_demo_1":
			_ = json.NewEncoder(w).Encode(map[string]any{"url": srv.URL + "/dl/media_demo_1", "file_size": 5})
		case "/media_big":
			_ = json.NewEncoder(w).Encode(map[string]any{"url": srv.URL + "/dl/big", "file_size": 1 << 30})
		case "/dl/media_demo_1":
			_, _ = w.Write([]byte("hello"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	wa := &WhatsApp{AccessToken: fakeBotToken, APIBase: srv.URL}
	b, err := wa.fetchMedia(context.Background(), Attachment{Ref: "media_demo_1"}, 1024)
	if err != nil || string(b) != "hello" || authOK != 2 {
		t.Fatalf("%q %v auth=%d", b, err, authOK)
	}
	if _, err := wa.fetchMedia(context.Background(), Attachment{Ref: "media_big"}, 1024); err != errMediaTooLarge {
		t.Fatalf("big: %v", err)
	}
	if _, err := wa.fetchMedia(context.Background(), Attachment{Ref: "../x"}, 1024); err == nil {
		t.Fatal("path in media id")
	}
}

func TestLarkFetchResource(t *testing.T) {
	mediaEnv(t)
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.RequestURI(), r.Header.Get("Authorization")
		_, _ = w.Write([]byte("img"))
	}))
	defer srv.Close()
	l := &Lark{}
	l.base, l.token, l.tokenExp = srv.URL, fakeBotToken, time.Now().Add(time.Hour)
	b, err := l.mediaFetcher("om_demo")(context.Background(), Attachment{Kind: MediaImage, Ref: "img_demo_1"}, 1024)
	if err != nil || string(b) != "img" {
		t.Fatalf("%q %v", b, err)
	}
	if gotPath != "/open-apis/im/v1/messages/om_demo/resources/img_demo_1?type=image" || gotAuth != "Bearer "+fakeBotToken {
		t.Fatalf("%s %s", gotPath, gotAuth)
	}
}

// ---- what the turn receives ----

func TestInboundMediaSizeCapSkipsDownload(t *testing.T) {
	mediaEnv(t)
	called := false
	fetch := func(context.Context, Attachment, int64) ([]byte, error) { called = true; return nil, nil }
	_, text := withInboundMedia(context.Background(), nil, "telegram", "xem file", []Attachment{{Kind: MediaDocument, Size: MaxInboundFileBytes + 1, Ref: "x"}}, fetch)
	if called || !strings.Contains(text, "skipped: too large (max 20 MB)") || !strings.HasSuffix(text, "\n\nxem file") {
		t.Fatalf("called=%v text=%q", called, text)
	}
}

func TestInboundMediaImageInlineAndSaved(t *testing.T) {
	mediaEnv(t)
	ws := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	pic := testPNG(t, 4, 4)
	fetch := func(context.Context, Attachment, int64) ([]byte, error) { return pic, nil }
	agent := &store.Agent{ID: "a1", TenantID: "minh-an"}
	ctx, text := withInboundMedia(context.Background(), agent, "zalo-oa", "", []Attachment{{Kind: MediaImage, Ref: "https://photo.example/a.jpg"}}, fetch)
	imgs := pipeline.TurnImagesFrom(ctx)
	if len(imgs) != 1 || imgs[0].MIME != "image/png" {
		t.Fatalf("images %+v", imgs)
	}
	if !strings.HasPrefix(text, `<media:image path=".uploads/`) || strings.Contains(text, "photo.example") {
		t.Fatalf("text %q", text)
	}
	files := savedFiles(t, filepath.Join(ws, "minh-an"))
	if len(files) != 1 || !strings.HasPrefix(files[0], ".uploads/") || !strings.HasSuffix(files[0], ".png") {
		t.Fatalf("files %v", files)
	}
	// Another tenant's workspace stays empty.
	if other := savedFiles(t, filepath.Join(ws, "default")); len(other) != 0 {
		t.Fatalf("leaked into default: %v", other)
	}
}

func TestInboundMediaNoWorkspace(t *testing.T) {
	mediaEnv(t)
	doc := []byte("%PDF-1.4 demo")
	fetch := func(context.Context, Attachment, int64) ([]byte, error) { return doc, nil }
	ctx, text := withInboundMedia(context.Background(), nil, "discord", "", []Attachment{{Kind: MediaDocument, Name: "bao-gia.pdf", Ref: "x"}}, fetch)
	if len(pipeline.TurnImagesFrom(ctx)) != 0 || !strings.Contains(text, "[File: bao-gia.pdf — not saved: workspace is not configured]") {
		t.Fatalf("text %q", text)
	}
	// An image still reaches the model inline without a workspace.
	pic := testPNG(t, 2, 2)
	ctx, text = withInboundMedia(context.Background(), nil, "discord", "", []Attachment{{Kind: MediaImage, Ref: "x"}}, func(context.Context, Attachment, int64) ([]byte, error) { return pic, nil })
	if len(pipeline.TurnImagesFrom(ctx)) != 1 || text != "<media:image>" {
		t.Fatalf("image %q", text)
	}
}

func TestInboundMediaTextDocumentInlined(t *testing.T) {
	mediaEnv(t)
	t.Setenv("GOSO_WORKSPACE", t.TempDir())
	fetch := func(context.Context, Attachment, int64) ([]byte, error) {
		return []byte("Tên,Số lượng\nMinh An,2\n"), nil
	}
	_, text := withInboundMedia(context.Background(), &store.Agent{TenantID: "minh-an"}, "slack", "", []Attachment{{Kind: MediaDocument, Name: `don"hang.csv`, Ref: "x"}}, fetch)
	if !strings.Contains(text, `<file name="don_hang.csv">`) || !strings.Contains(text, "Minh An,2") || !strings.Contains(text, `path=".uploads/`) {
		t.Fatalf("text %q", text)
	}
}

func TestInboundVoiceWithoutSTT(t *testing.T) {
	mediaEnv(t)
	clip := append([]byte("OggS"), bytes.Repeat([]byte{0}, 60)...)
	fetch := func(context.Context, Attachment, int64) ([]byte, error) { return clip, nil }
	_, text := withInboundMedia(context.Background(), nil, "telegram", "", []Attachment{{Kind: MediaVoice, Ref: "v"}}, fetch)
	if text != "<media:voice>\n[Voice message not transcribed: speech-to-text is not configured]" {
		t.Fatalf("text %q", text)
	}
}

func TestInboundVoiceTranscribed(t *testing.T) {
	mediaEnv(t)
	var gotPath string
	stt := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = io.Copy(io.Discard, r.Body)
		_, _ = w.Write([]byte(`{"text":"Cho mình hỏi giá <b>áo</b> size M"}`))
	}))
	defer stt.Close()
	t.Setenv("GOSO_STT_URL", stt.URL+"/v1")
	clip := append([]byte("OggS"), bytes.Repeat([]byte{0}, 60)...)
	fetch := func(context.Context, Attachment, int64) ([]byte, error) { return clip, nil }
	_, text := withInboundMedia(context.Background(), nil, "whatsapp", "", []Attachment{{Kind: MediaVoice, Ref: "v"}}, fetch)
	if gotPath != "/v1/audio/transcriptions" || text != "<media:voice>\n<transcript>Cho mình hỏi giá ‹b›áo‹/b› size M</transcript>" {
		t.Fatalf("path %s text %q", gotPath, text)
	}
	// A failing endpoint keeps the tag and says so.
	t.Setenv("GOSO_STT_URL", "http://127.0.0.1:1")
	_, text = withInboundMedia(context.Background(), nil, "whatsapp", "", []Attachment{{Kind: MediaVoice, Ref: "v"}}, fetch)
	if text != "<media:voice>\n[Voice message not transcribed: transcription failed]" {
		t.Fatalf("failed stt %q", text)
	}
}

func TestInboundStickerAndVideoNotes(t *testing.T) {
	mediaEnv(t)
	fetch := func(context.Context, Attachment, int64) ([]byte, error) {
		return []byte("\x00\x00\x00\x18ftypmp42"), nil
	}
	_, text := withInboundMedia(context.Background(), nil, "zalo-oa", "", []Attachment{{Kind: MediaSticker, Note: "[sticker]"}, {Kind: MediaVideo, Ref: "v"}}, fetch)
	if !strings.HasPrefix(text, "[sticker]\n<media:video>\n[Video received:") {
		t.Fatalf("text %q", text)
	}
}

func TestInlineImageDownscalesLargePicture(t *testing.T) {
	part, ok := inlineImage(testPNG(t, 3400, 20), "image/png")
	if !ok || part.MIME != "image/jpeg" {
		t.Fatalf("%v %s", ok, part.MIME)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(part.Data))
	if err != nil || cfg.Width > maxInlineImageSide || cfg.Width < maxInlineImageSide-1 {
		t.Fatalf("%+v %v", cfg, err)
	}
}

// visionProvider records what it was sent and accepts images.
type visionProvider struct {
	mu   sync.Mutex
	msgs []llm.Message
}

func (p *visionProvider) Name() string        { return "vision-fake" }
func (p *visionProvider) AcceptsImages() bool { return true }
func (p *visionProvider) Chat(_ context.Context, msgs []llm.Message) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.msgs = append([]llm.Message(nil), msgs...)
	return "đã xem ảnh", nil
}

func TestTelegramPhotoReachesModel(t *testing.T) {
	mediaEnv(t)
	t.Setenv("GOSO_TELEGRAM_WEBHOOK_SECRET", "")
	ws := t.TempDir()
	t.Setenv("GOSO_WORKSPACE", ws)
	pic := testPNG(t, 3, 3)
	srv := telegramFileServer(t, map[string][]byte{"ph_big": pic})
	defer srv.Close()
	prov := &visionProvider{}
	var sent string
	tg := &Telegram{BotToken: fakeBotToken, APIBase: srv.URL, Store: store.New(), LLM: prov,
		Sender: func(_ context.Context, _ int64, text string) error { sent = text; return nil }}
	body := `{"update_id":11,"message":{"message_id":5,"chat":{"id":777},"from":{"id":777},"caption":"Mẫu này còn hàng không?",
	  "photo":[{"file_id":"ph_small","width":10,"height":10},{"file_id":"ph_big","width":300,"height":300}]}}`
	w := httptest.NewRecorder()
	tg.HandleUpdate(w, httptest.NewRequest("POST", "/api/channels/telegram/webhook", strings.NewReader(body)))
	if w.Code != 200 || sent != "đã xem ảnh" {
		t.Fatalf("code %d sent %q", w.Code, sent)
	}
	if len(prov.msgs) == 0 {
		t.Fatal("model not called")
	}
	last := prov.msgs[len(prov.msgs)-1]
	if last.Role != "user" || len(last.Images) != 1 || !bytes.Equal(last.Images[0].Data, pic) {
		t.Fatalf("last %+v", last)
	}
	if !strings.Contains(last.Content, `<media:image path=".uploads/`) || !strings.HasSuffix(last.Content, "Mẫu này còn hàng không?") ||
		strings.Contains(last.Content, fakeBotToken) {
		t.Fatalf("content %q", last.Content)
	}
	if files := savedFiles(t, ws); len(files) != 1 {
		t.Fatalf("files %v", files)
	}
	if _, err := os.Stat(filepath.Join(ws, filepath.FromSlash(savedFiles(t, ws)[0]))); err != nil {
		t.Fatal(err)
	}
}
