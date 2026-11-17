// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Lark runs one tenant's Feishu/Lark app bot, the "<tenant>/feishu/<name>"
// channels. Like GoClaw it holds Lark's long connection (a websocket the app
// opens to Lark), so it needs no public URL and no change in the Lark
// developer console. The app id is the channel's "app_id" setting and the app
// secret is its stored secret; the "domain" setting picks Lark
// (open.larksuite.com) or Feishu (open.feishu.cn), and is found on its own
// when left empty.
type Lark struct {
	Instance   string
	Store      store.StoreIface
	LLM        llm.Provider
	Meter      *billing.Store
	HTTPClient *http.Client
	// APIBase replaces the domain's API host in tests.
	APIBase string
	// Sender replaces the reply call in tests.
	Sender func(ctx context.Context, msg LarkMessage, text string) error

	// run answers a message; tests make it synchronous.
	run func(func())
	// tryHosts replaces the Lark-then-Feishu hosts in tests.
	tryHosts []string

	mu       sync.Mutex
	stop     context.CancelFunc
	stopped  bool
	seen     map[string]bool
	order    []string
	base     string
	token    string
	tokenExp time.Time
	botID    string
}

// LarkMessage is the part of an im.message.receive_v1 event goso answers.
type LarkMessage struct {
	MessageID string
	ChatID    string
	ChatType  string
	ThreadID  string
	// RootID is the first message of the topic (thread) this one is in.
	RootID    string
	SenderID  string
	Text      string
	Mentioned bool
	// Attachments are the message's image, file, audio or video; each Ref
	// is a resource key of this message.
	Attachments []Attachment
}

// SettingAppID and SettingDomain are a Lark channel's settings.
const (
	SettingAppID  = "app_id"
	SettingDomain = "domain"
	// SettingTopicSessionMode "enabled" gives each topic (thread) of a chat
	// its own session, as GoClaw's topic_session_mode does.
	SettingTopicSessionMode = "topic_session_mode"
)

const (
	larkHost        = "https://open.larksuite.com"
	feishuHost      = "https://open.feishu.cn"
	larkTextEvent   = "im.message.receive_v1"
	larkMaxText     = 4000
	larkSeenMax     = 512
	larkDefaultPing = 120 * time.Second
	larkMaxBackoff  = 60 * time.Second

	larkFrameControl = 0
	larkFrameData    = 1
)

// Name returns the catalog name.
func (l *Lark) Name() string { return "feishu" }

// appCreds returns the app id (a setting) and app secret (a stored secret).
func (l *Lark) appCreds() (id, secret string) {
	if l == nil || l.Store == nil {
		return "", ""
	}
	if cfg, _ := l.Store.GetChannelConfig(l.Instance); cfg != nil {
		id = strings.TrimSpace(cfg.Settings[SettingAppID])
	}
	if v, _, set := Credential(l.Store, l.Instance, KindAppSecret, nil); set {
		secret = strings.TrimSpace(v)
	}
	return id, secret
}

// larkCredsSet reports a tenant Lark channel that has both its app id and its
// app secret.
func larkCredsSet(st store.StoreIface, name string) bool {
	l := &Lark{Store: st, Instance: name}
	id, secret := l.appCreds()
	return id != "" && secret != ""
}

// hosts lists the API hosts to try: the domain setting's, or Lark then Feishu.
func (l *Lark) hosts() []string {
	if l.APIBase != "" {
		return []string{strings.TrimRight(l.APIBase, "/")}
	}
	domain := ""
	if cfg, _ := l.Store.GetChannelConfig(l.Instance); cfg != nil {
		domain = strings.ToLower(strings.TrimSpace(cfg.Settings[SettingDomain]))
	}
	switch {
	case domain == "lark" || domain == "larksuite":
		return []string{larkHost}
	case domain == "feishu":
		return []string{feishuHost}
	case strings.HasPrefix(domain, "https://"):
		return []string{strings.TrimRight(domain, "/")}
	case l.tryHosts != nil:
		return l.tryHosts
	default:
		return []string{larkHost, feishuHost}
	}
}

// Start checks the app's credentials and bot, then holds the long connection
// in the background until Stop.
func (l *Lark) Start(ctx context.Context, mgr *Manager) {
	if l == nil || store.LiteEnabled() {
		return
	}
	if !larkCredsSet(l.Store, l.Instance) {
		return
	}
	if err := l.ProbeToken(ctx); err != nil {
		mgr.SetFailed(l.Instance, err.Error())
		return
	}
	l.mu.Lock()
	if l.stopped || l.stop != nil {
		l.mu.Unlock()
		return
	}
	cctx, cancel := context.WithCancel(ctx)
	l.stop = cancel
	l.mu.Unlock()
	go l.hold(cctx, mgr)
}

// Stop closes the long connection.
func (l *Lark) Stop() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stopped = true
	if l.stop != nil {
		l.stop()
		l.stop = nil
	}
}

// ProbeToken gets a tenant access token and the bot's identity. With no domain
// set it tries Lark, then Feishu, and keeps the one that answers.
func (l *Lark) ProbeToken(ctx context.Context) error {
	id, secret := l.appCreds()
	if id == "" || secret == "" {
		return fmt.Errorf("missing app id or app secret")
	}
	var last error
	for _, host := range l.hosts() {
		tok, exp, err := l.fetchToken(ctx, host, id, secret)
		if err != nil {
			last = err
			continue
		}
		l.mu.Lock()
		l.base, l.token, l.tokenExp = host, tok, exp
		l.mu.Unlock()
		botID, err := l.fetchBotID(ctx)
		if err != nil {
			return err
		}
		l.mu.Lock()
		l.botID = botID
		l.mu.Unlock()
		return nil
	}
	return last
}

type larkReply struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func (l *Lark) client() *http.Client {
	if l.HTTPClient != nil {
		return l.HTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}

// do sends body and decodes the reply into out. Lark answers errors with
// a JSON code, so a non-zero code is an error too.
func (l *Lark) do(ctx context.Context, method, u, token string, body any, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := l.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var head larkReply
	if err := json.Unmarshal(raw, &head); err != nil {
		return fmt.Errorf("lark: http %d", resp.StatusCode)
	}
	if head.Code != 0 {
		return &larkError{Code: head.Code, Msg: head.Msg}
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

type larkError struct {
	Code int
	Msg  string
}

func (e *larkError) Error() string { return fmt.Sprintf("lark API error %d: %s", e.Code, e.Msg) }

// larkTokenExpired reports Lark's codes for a stale or invalid access token.
func larkTokenExpired(err error) bool {
	var e *larkError
	return errors.As(err, &e) && (e.Code == 99991663 || e.Code == 99991661 || e.Code == 99991668)
}

func (l *Lark) fetchToken(ctx context.Context, host, id, secret string) (string, time.Time, error) {
	var out struct {
		Token  string `json:"tenant_access_token"`
		Expire int    `json:"expire"`
	}
	err := l.do(ctx, http.MethodPost, host+"/open-apis/auth/v3/tenant_access_token/internal", "",
		map[string]string{"app_id": id, "app_secret": secret}, &out)
	if err != nil {
		return "", time.Time{}, err
	}
	if out.Token == "" {
		return "", time.Time{}, fmt.Errorf("lark: empty tenant access token")
	}
	ttl := time.Duration(out.Expire) * time.Second
	if ttl <= 0 {
		ttl = time.Hour
	}
	// Renew five minutes early.
	return out.Token, time.Now().Add(ttl - 5*time.Minute), nil
}

// accessToken returns a live tenant access token, renewing it when stale.
func (l *Lark) accessToken(ctx context.Context, force bool) (base, token string, err error) {
	l.mu.Lock()
	base, token, exp := l.base, l.token, l.tokenExp
	l.mu.Unlock()
	if base != "" && token != "" && !force && time.Now().Before(exp) {
		return base, token, nil
	}
	if base == "" {
		if err := l.ProbeToken(ctx); err != nil {
			return "", "", err
		}
		l.mu.Lock()
		defer l.mu.Unlock()
		return l.base, l.token, nil
	}
	id, secret := l.appCreds()
	tok, newExp, err := l.fetchToken(ctx, base, id, secret)
	if err != nil {
		return "", "", err
	}
	l.mu.Lock()
	l.token, l.tokenExp = tok, newExp
	l.mu.Unlock()
	return base, tok, nil
}

func (l *Lark) fetchBotID(ctx context.Context) (string, error) {
	base, token, err := l.accessToken(ctx, false)
	if err != nil {
		return "", err
	}
	var out struct {
		Bot struct {
			OpenID string `json:"open_id"`
		} `json:"bot"`
	}
	if err := l.do(ctx, http.MethodGet, base+"/open-apis/bot/v3/info", token, nil, &out); err != nil {
		return "", err
	}
	return out.Bot.OpenID, nil
}

// larkEndpoint is the long connection's address and timing from Lark.
type larkEndpoint struct {
	URL       string
	ServiceID int32
	Ping      time.Duration
}

func (l *Lark) endpoint(ctx context.Context) (larkEndpoint, error) {
	l.mu.Lock()
	base := l.base
	l.mu.Unlock()
	if base == "" {
		if err := l.ProbeToken(ctx); err != nil {
			return larkEndpoint{}, err
		}
		l.mu.Lock()
		base = l.base
		l.mu.Unlock()
	}
	id, secret := l.appCreds()
	var out struct {
		Data struct {
			URL          string `json:"URL"`
			ClientConfig struct {
				PingInterval int `json:"PingInterval"`
			} `json:"ClientConfig"`
		} `json:"data"`
	}
	if err := l.do(ctx, http.MethodPost, base+"/callback/ws/endpoint", "", map[string]string{"AppID": id, "AppSecret": secret}, &out); err != nil {
		return larkEndpoint{}, err
	}
	u, err := url.Parse(out.Data.URL)
	if err != nil || out.Data.URL == "" {
		return larkEndpoint{}, fmt.Errorf("lark: bad long connection url")
	}
	svc, _ := strconv.Atoi(u.Query().Get("service_id"))
	ep := larkEndpoint{URL: out.Data.URL, ServiceID: int32(svc), Ping: larkDefaultPing}
	if p := out.Data.ClientConfig.PingInterval; p > 0 {
		ep.Ping = time.Duration(p) * time.Second
	}
	return ep, nil
}

// hold keeps one long connection open, reconnecting with backoff until ctx
// ends. Health shows running while connected and failed while it cannot reach
// Lark.
func (l *Lark) hold(ctx context.Context, mgr *Manager) {
	backoff := 2 * time.Second
	for ctx.Err() == nil {
		connected, err := l.session(ctx, mgr)
		if ctx.Err() != nil {
			return
		}
		if connected {
			backoff = 2 * time.Second
		}
		if err != nil {
			mgr.SetFailed(l.Instance, redactSecretString(err.Error(), l))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < larkMaxBackoff {
			backoff *= 2
		}
	}
}

// session runs one connection until it drops. connected reports whether it got
// as far as an open socket.
func (l *Lark) session(ctx context.Context, mgr *Manager) (connected bool, err error) {
	ep, err := l.endpoint(ctx)
	if err != nil {
		return false, err
	}
	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	conn, _, err := dialer.DialContext(ctx, ep.URL, nil)
	if err != nil {
		return false, fmt.Errorf("lark: long connection: %w", err)
	}
	defer conn.Close()
	mgr.SetRunning(l.Instance, "websocket")

	var wmu sync.Mutex
	write := func(f larkFrame) error {
		wmu.Lock()
		defer wmu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteMessage(websocket.BinaryMessage, f.encode())
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		// Closing the socket ends ReadMessage when ctx ends.
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	ping := func() error {
		return write(larkFrame{Service: ep.ServiceID, Method: larkFrameControl, Headers: [][2]string{{"type", "ping"}}})
	}
	if err := ping(); err != nil {
		return true, err
	}
	go func() {
		t := time.NewTicker(ep.Ping)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				if ping() != nil {
					return
				}
			}
		}
	}()
	parts := map[string][][]byte{}
	for {
		_ = conn.SetReadDeadline(time.Now().Add(3*ep.Ping + 30*time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return true, nil
			}
			return true, fmt.Errorf("lark: long connection closed: %w", err)
		}
		f, err := decodeLarkFrame(raw)
		if err != nil || f.Method != larkFrameData {
			continue
		}
		start := time.Now()
		payload, whole := f.assemble(parts)
		if !whole {
			continue
		}
		if f.header("type") == "event" {
			l.dispatchEvent(payload)
		}
		// Lark resends an event that is not acknowledged in time, so the ack
		// goes back before the answer is ready.
		ack := f
		ack.Headers = append(append([][2]string(nil), f.Headers...), [2]string{"biz_rt", strconv.FormatInt(time.Since(start).Milliseconds(), 10)})
		ack.Payload = []byte(`{"code":200,"headers":null,"data":null}`)
		if err := write(ack); err != nil {
			return true, err
		}
	}
}

// larkEvent is the im.message.receive_v1 event body.
type larkEvent struct {
	Header struct {
		EventID   string `json:"event_id"`
		EventType string `json:"event_type"`
	} `json:"header"`
	Event struct {
		Sender struct {
			SenderID struct {
				OpenID string `json:"open_id"`
			} `json:"sender_id"`
			SenderType string `json:"sender_type"`
		} `json:"sender"`
		Message struct {
			MessageID   string `json:"message_id"`
			ChatID      string `json:"chat_id"`
			ChatType    string `json:"chat_type"`
			ThreadID    string `json:"thread_id"`
			RootID      string `json:"root_id"`
			MessageType string `json:"message_type"`
			Content     string `json:"content"`
			Mentions    []struct {
				Key string `json:"key"`
				ID  struct {
					OpenID string `json:"open_id"`
				} `json:"id"`
			} `json:"mentions"`
		} `json:"message"`
	} `json:"event"`
}

var larkMentionKey = regexp.MustCompile(`@_user_\d+`)

// parseLarkEvent reads a text message event; ok is false for anything else.
func (l *Lark) parseLarkEvent(payload []byte) (LarkMessage, bool) {
	var ev larkEvent
	if json.Unmarshal(payload, &ev) != nil || ev.Header.EventType != larkTextEvent {
		return LarkMessage{}, false
	}
	m := ev.Event.Message
	if m.ChatID == "" || ev.Event.Sender.SenderType != "user" {
		return LarkMessage{}, false
	}
	rawText, atts, ok := larkContent(m.MessageType, m.Content)
	if !ok {
		return LarkMessage{}, false
	}
	l.mu.Lock()
	botID := l.botID
	l.mu.Unlock()
	mentioned := false
	for _, mn := range m.Mentions {
		if botID != "" && mn.ID.OpenID == botID {
			mentioned = true
		}
	}
	text := strings.Join(strings.Fields(larkMentionKey.ReplaceAllString(rawText, "")), " ")
	if text == "" && len(atts) == 0 {
		return LarkMessage{}, false
	}
	return LarkMessage{
		MessageID: m.MessageID, ChatID: m.ChatID, ChatType: m.ChatType, ThreadID: m.ThreadID,
		RootID: m.RootID, SenderID: ev.Event.Sender.SenderID.OpenID, Text: text, Mentioned: mentioned, Attachments: atts,
	}, true
}

// larkContent reads a message's content JSON by its type: text, image,
// file, audio, media (video), sticker or post (rich text with pictures).
// ok is false for a type it does not read.
func larkContent(msgType, content string) (text string, atts []Attachment, ok bool) {
	var c struct {
		Text     string          `json:"text"`
		ImageKey string          `json:"image_key"`
		FileKey  string          `json:"file_key"`
		FileName string          `json:"file_name"`
		Title    string          `json:"title"`
		Content  json.RawMessage `json:"content"`
	}
	if json.Unmarshal([]byte(content), &c) != nil {
		return "", nil, false
	}
	switch msgType {
	case "text":
		return c.Text, nil, true
	case "image":
		if c.ImageKey == "" {
			return "", nil, false
		}
		return "", []Attachment{{Kind: MediaImage, Ref: c.ImageKey}}, true
	case "file", "audio", "media":
		if c.FileKey == "" {
			return "", nil, false
		}
		kind := map[string]string{"file": MediaDocument, "audio": MediaVoice, "media": MediaVideo}[msgType]
		return "", []Attachment{{Kind: kind, Name: c.FileName, Ref: c.FileKey}}, true
	case "sticker":
		return "", []Attachment{{Kind: MediaSticker, Note: "[sticker]"}}, true
	case "post":
		return larkPost(content)
	}
	return "", nil, false
}

// larkPost flattens a rich-text post into its words and pictures. The post
// is either {"title","content"} or that wrapped in a language key.
func larkPost(content string) (string, []Attachment, bool) {
	type node struct {
		Tag      string `json:"tag"`
		Text     string `json:"text"`
		ImageKey string `json:"image_key"`
	}
	type post struct {
		Title   string   `json:"title"`
		Content [][]node `json:"content"`
	}
	var p post
	if json.Unmarshal([]byte(content), &p) != nil || (p.Title == "" && len(p.Content) == 0) {
		var langs map[string]post
		if json.Unmarshal([]byte(content), &langs) != nil {
			return "", nil, false
		}
		for _, v := range langs {
			if v.Title != "" || len(v.Content) > 0 {
				p = v
				break
			}
		}
	}
	var lines []string
	if t := strings.TrimSpace(p.Title); t != "" {
		lines = append(lines, t)
	}
	var atts []Attachment
	for _, para := range p.Content {
		var line []string
		for _, n := range para {
			switch n.Tag {
			case "text", "a", "md":
				if t := strings.TrimSpace(n.Text); t != "" {
					line = append(line, t)
				}
			case "img":
				if n.ImageKey != "" {
					atts = append(atts, Attachment{Kind: MediaImage, Ref: n.ImageKey})
				}
			}
		}
		if len(line) > 0 {
			lines = append(lines, strings.Join(line, " "))
		}
	}
	return strings.Join(lines, "\n"), atts, true
}

// mediaFetcher downloads this message's resources with the tenant token.
// A stale token is renewed once. The token never reaches an error.
func (l *Lark) mediaFetcher(messageID string) MediaFetcher {
	return func(ctx context.Context, a Attachment, limit int64) ([]byte, error) {
		if messageID == "" || strings.ContainsAny(messageID+a.Ref, "/?#") {
			return nil, errMediaNoRef
		}
		typ := "file"
		if a.Kind == MediaImage {
			typ = "image"
		}
		var lastErr error
		for attempt := 0; attempt < 2; attempt++ {
			base, token, err := l.accessToken(ctx, attempt > 0)
			if err != nil {
				return nil, errors.New("download failed: no access token")
			}
			u := base + "/open-apis/im/v1/messages/" + url.PathEscape(messageID) + "/resources/" + url.PathEscape(a.Ref) + "?type=" + typ
			data, err := fetchURL(ctx, l.HTTPClient, u, limit, "Authorization", "Bearer "+token)
			if err == nil || errors.Is(err, errMediaTooLarge) {
				return data, err
			}
			lastErr = err
		}
		return nil, lastErr
	}
}

// dispatchEvent answers a text message once, in the background.
func (l *Lark) dispatchEvent(payload []byte) {
	msg, ok := l.parseLarkEvent(payload)
	if !ok || !l.firstSight(msg.MessageID) {
		return
	}
	run := l.run
	if run == nil {
		run = func(f func()) { go f() }
	}
	run(func() { l.ingest(msg) })
}

func (l *Lark) firstSight(id string) bool {
	if id == "" {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.seen == nil {
		l.seen = map[string]bool{}
	}
	if l.seen[id] {
		return false
	}
	l.seen[id] = true
	l.order = append(l.order, id)
	if len(l.order) > larkSeenMax {
		delete(l.seen, l.order[0])
		l.order = l.order[1:]
	}
	return true
}

func (l *Lark) ingest(m LarkMessage) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	name := l.Instance
	tenant, owned := TenantOf(name)
	if !owned || l.Store == nil {
		return
	}
	cfg, _ := l.Store.GetChannelConfig(name)
	if cfg == nil || !cfg.Enabled {
		return
	}
	peer := "direct"
	if m.ChatType == "group" || m.ChatType == "topic_group" {
		peer = "group"
	}
	send := l.Sender
	if send == nil {
		send = l.reply
	}
	in := Inbound{Channel: "feishu", SenderID: m.SenderID, ChatID: m.ChatID, PeerKind: peer, Text: m.Text, Mention: m.Mentioned}
	paired := m.SenderID != "" && SenderPaired(l.Store, name, m.SenderID, time.Time{})
	switch CheckPolicy("feishu", MergePolicy(name, cfg), in, paired) {
	case PolicyReject, PolicyNeedMention:
		return
	case PolicyNeedPairing:
		if msg := OfferPairingCode(l.Store, name, m.SenderID, time.Time{}); msg != "" {
			_ = send(ctx, m, msg)
		}
		return
	}
	agent := boundAgent(l.Store, cfg, tenant)
	if agent == nil {
		return
	}
	ObserveDefault(Sighting{Channel: "feishu", Dest: m.ChatID, Kind: peer, SenderID: m.SenderID, AgentID: agent.ID, TenantID: agent.TenantID})
	if BufferIfNeeded(nil, agent, "feishu", m.ChatID) {
		return
	}
	id := larkIdentity(name, cfg, m)
	sess := ensureChannelSession(l.Store, agent, id)
	if sess == nil {
		return
	}
	turnCtx := withContextUser(ctx, id)
	turnCtx, text := withInboundMedia(turnCtx, agent, name, m.Text, m.Attachments, l.mediaFetcher(m.MessageID))
	reply, err := answerTurnErr(turnCtx, l.Store, agent, sess, l.LLM, l.Meter, text)
	// A model error is never sent to the customer.
	if err != nil || strings.TrimSpace(reply) == "" {
		return
	}
	for _, part := range splitRunes(reply, larkMaxText) {
		if send(ctx, m, part) != nil {
			return
		}
	}
}

// larkIdentity is GoClaw's identity of a Lark message. Only chat_type
// "group" is a group there. With topic_session_mode "enabled", a message in a
// topic has the chat id "<chat_id>:topic:<root_id>", so the topic gets its
// own session and, in a group, its own user key.
func larkIdentity(name string, cfg *store.ChannelConfig, m LarkMessage) Identity {
	id := Identity{Instance: name, Peer: "direct", SenderID: m.SenderID, ChatID: m.ChatID}
	if m.ChatType == "group" {
		id.Peer = "group"
	}
	if m.RootID != "" && cfg != nil && strings.TrimSpace(cfg.Settings[SettingTopicSessionMode]) == "enabled" {
		id.ChatID = m.ChatID + ":topic:" + m.RootID
	}
	return id
}

// reply answers the message in place, so a group sees which message it
// answers. A stale token is renewed once.
func (l *Lark) reply(ctx context.Context, m LarkMessage, text string) error {
	content, _ := json.Marshal(map[string]string{"text": text})
	body := map[string]any{"msg_type": "text", "content": string(content)}
	// A message inside a topic is answered inside that topic.
	if m.ThreadID != "" {
		body["reply_in_thread"] = true
	}
	for attempt := 0; attempt < 2; attempt++ {
		base, token, err := l.accessToken(ctx, attempt > 0)
		if err != nil {
			return err
		}
		err = l.do(ctx, http.MethodPost, base+"/open-apis/im/v1/messages/"+url.PathEscape(m.MessageID)+"/reply", token, body, nil)
		if err == nil || !larkTokenExpired(err) {
			return err
		}
	}
	return fmt.Errorf("lark: access token refused")
}

// redactSecretString cuts the app secret out of an error shown on the channel.
func redactSecretString(s string, l *Lark) string {
	if _, secret := l.appCreds(); secret != "" {
		s = strings.ReplaceAll(s, secret, "[redacted]")
	}
	return s
}
