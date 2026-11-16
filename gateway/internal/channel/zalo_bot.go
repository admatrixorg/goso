// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// ZaloBot runs one tenant's bot on the Zalo Bot platform
// (bot-api.zaloplatforms.com), the "<tenant>/zalo-oa/<name>" channels. Its API
// is shaped like Telegram's: a bot token, getUpdates or a webhook with a
// secret header, sendMessage to a chat id.
type ZaloBot struct {
	Instance   string
	Store      store.StoreIface
	LLM        llm.Provider
	Meter      *billing.Store
	HTTPClient *http.Client
	APIBase    string
	// Sender replaces sendMessage in tests.
	Sender func(ctx context.Context, chatID, text string) error

	// run answers a message; tests make it synchronous.
	run func(func())

	mu      sync.Mutex
	stop    context.CancelFunc
	stopped bool
	seen    map[string]bool
	order   []string
}

// ZaloBotSecretHeader carries the secret_token given to setWebhook.
const ZaloBotSecretHeader = "X-Bot-Api-Secret-Token"

const (
	zaloBotAPIBase       = "https://bot-api.zaloplatforms.com"
	zaloBotWebhookPrefix = "/api/channels/zalo-oa/bot/"
	zaloBotTextEvent     = "message.text.received"
	// zaloBotMaxText is the longest text one sendMessage takes.
	zaloBotMaxText  = 2000
	zaloBotPollWait = 30
	zaloBotSeenMax  = 512
)

// ZaloBotSelfAuth reports a Zalo Bot webhook call carrying the platform's
// secret header. The gateway lets it past the admin token: Zalo cannot send
// one. HandleUpdate then checks the secret against the bot's own.
func ZaloBotSelfAuth(r *http.Request) bool {
	if r.Method != http.MethodPost || r.Header.Get(ZaloBotSecretHeader) == "" {
		return false
	}
	return strings.HasPrefix(r.URL.Path, zaloBotWebhookPrefix)
}

// Name returns the catalog name.
func (z *ZaloBot) Name() string { return "zalo-oa" }

func (z *ZaloBot) resolveToken() string {
	if z == nil {
		return ""
	}
	v, _, set := Credential(z.Store, z.Instance, KindBot, nil)
	if !set {
		return ""
	}
	return strings.TrimSpace(v)
}

// webhookSecret is derived from the bot token, like Telegram's, so it needs no
// storage and changes with the token.
func (z *ZaloBot) webhookSecret(token string) string {
	if token == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write([]byte("goso zalo bot webhook\x00" + z.Instance))
	return hex.EncodeToString(mac.Sum(nil))
}

func (z *ZaloBot) webhookPath() string {
	tenant, _, inst, _ := ParseInstance(z.Instance)
	return zaloBotWebhookPrefix + url.PathEscape(tenant) + "/" + url.PathEscape(inst)
}

// zaloBotMode is GOSO_ZALO_BOT_MODE: "poll" (default, as GoClaw ran it) or
// "webhook" (needs GOSO_PUBLIC_URL).
func zaloBotMode() string {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("GOSO_ZALO_BOT_MODE")), "webhook") {
		return "webhook"
	}
	return "poll"
}

// Start checks the token, then either points the platform's webhook at goso
// or clears it and polls.
func (z *ZaloBot) Start(ctx context.Context, mgr *Manager) {
	if z == nil || store.LiteEnabled() {
		return
	}
	token := z.resolveToken()
	if token == "" {
		return
	}
	mode := zaloBotMode()
	pub := strings.TrimRight(strings.TrimSpace(os.Getenv("GOSO_PUBLIC_URL")), "/")
	if mode == "webhook" && pub == "" {
		mgr.SetFailed(z.Instance, "public url required for webhook mode")
		return
	}
	if err := z.ProbeToken(ctx); err != nil {
		mgr.SetFailed(z.Instance, err.Error())
		return
	}
	if mode == "webhook" {
		_, err := z.call(ctx, token, "setWebhook", map[string]any{"url": pub + z.webhookPath(), "secret_token": z.webhookSecret(token)})
		if err != nil {
			mgr.SetFailed(z.Instance, err.Error())
			return
		}
		mgr.SetRunning(z.Instance, mode)
		return
	}
	// getUpdates does not work while a webhook is set.
	if _, err := z.call(ctx, token, "deleteWebhook", map[string]any{}); err != nil {
		mgr.SetFailed(z.Instance, err.Error())
		return
	}
	z.mu.Lock()
	if z.stopped || z.stop != nil {
		z.mu.Unlock()
		return
	}
	cctx, cancel := context.WithCancel(ctx)
	z.stop = cancel
	z.mu.Unlock()
	mgr.SetRunning(z.Instance, mode)
	go z.poll(cctx, token)
}

// Stop ends polling. A Start still checking the token will not poll after it.
func (z *ZaloBot) Stop() {
	if z == nil {
		return
	}
	z.mu.Lock()
	defer z.mu.Unlock()
	z.stopped = true
	if z.stop != nil {
		z.stop()
		z.stop = nil
	}
}

// ProbeToken runs getMe with the bot's token.
func (z *ZaloBot) ProbeToken(ctx context.Context) error {
	token := z.resolveToken()
	if token == "" {
		return fmt.Errorf("missing bot token")
	}
	_, err := z.call(ctx, token, "getMe", map[string]any{})
	return err
}

func (z *ZaloBot) poll(ctx context.Context, token string) {
	for ctx.Err() == nil {
		events, err := z.getUpdates(ctx, token)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
			continue
		}
		for _, ev := range events {
			z.dispatch(ev)
		}
	}
}

func (z *ZaloBot) getUpdates(ctx context.Context, token string) ([]zaloBotEvent, error) {
	raw, err := z.callWith(ctx, token, "getUpdates", map[string]any{"timeout": zaloBotPollWait}, (zaloBotPollWait+15)*time.Second)
	if err != nil {
		if errors408(err) {
			return nil, nil
		}
		return nil, err
	}
	return decodeZaloBotEvents(raw), nil
}

// HandleUpdate takes POST /api/channels/zalo-oa/bot/{tenant}/{inst}.
func (z *ZaloBot) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	want := z.webhookSecret(z.resolveToken())
	got := r.Header.Get(ZaloBotSecretHeader)
	if want == "" || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, `{"error":"bad body"}`, http.StatusBadRequest)
		return
	}
	for _, ev := range decodeZaloBotEvents(body) {
		z.dispatch(ev)
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

type zaloBotEvent struct {
	EventName string `json:"event_name"`
	Message   *struct {
		From struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			IsBot       bool   `json:"is_bot"`
		} `json:"from"`
		Chat struct {
			ID       string `json:"id"`
			ChatType string `json:"chat_type"`
		} `json:"chat"`
		Text      string `json:"text"`
		MessageID string `json:"message_id"`
	} `json:"message"`
}

// decodeZaloBotEvents reads one event, a list, or either wrapped in
// {"ok":true,"result":…} as getUpdates and the webhook send them.
func decodeZaloBotEvents(b []byte) []zaloBotEvent {
	var env struct {
		Result json.RawMessage `json:"result"`
	}
	if json.Unmarshal(b, &env) == nil && len(env.Result) > 0 && string(env.Result) != "null" {
		b = env.Result
	}
	b = bytes.TrimSpace(b)
	var list []zaloBotEvent
	if len(b) > 0 && b[0] == '[' {
		_ = json.Unmarshal(b, &list)
		return list
	}
	var one zaloBotEvent
	if json.Unmarshal(b, &one) == nil && one.Message != nil {
		return []zaloBotEvent{one}
	}
	return nil
}

// dispatch answers a text message once, in the background.
func (z *ZaloBot) dispatch(ev zaloBotEvent) {
	if ev.EventName != zaloBotTextEvent || ev.Message == nil || ev.Message.From.IsBot {
		return
	}
	m := ev.Message
	if strings.TrimSpace(m.Text) == "" || m.Chat.ID == "" || !z.firstSight(m.MessageID) {
		return
	}
	run := z.run
	if run == nil {
		run = func(f func()) { go f() }
	}
	run(func() { z.ingest(ev) })
}

// firstSight drops a message the platform sends again.
func (z *ZaloBot) firstSight(id string) bool {
	if id == "" {
		return true
	}
	z.mu.Lock()
	defer z.mu.Unlock()
	if z.seen == nil {
		z.seen = map[string]bool{}
	}
	if z.seen[id] {
		return false
	}
	z.seen[id] = true
	z.order = append(z.order, id)
	if len(z.order) > zaloBotSeenMax {
		delete(z.seen, z.order[0])
		z.order = z.order[1:]
	}
	return true
}

func (z *ZaloBot) ingest(ev zaloBotEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	m := ev.Message
	name := z.Instance
	tenant, owned := TenantOf(name)
	if !owned || z.Store == nil {
		return
	}
	cfg, _ := z.Store.GetChannelConfig(name)
	if cfg == nil || !cfg.Enabled {
		return
	}
	peer := "direct"
	if strings.EqualFold(m.Chat.ChatType, "GROUP") {
		peer = "group"
	}
	send := z.Sender
	if send == nil {
		send = z.sendMessage
	}
	in := Inbound{Channel: "zalo-oa", SenderID: m.From.ID, ChatID: m.Chat.ID, PeerKind: peer, Text: m.Text, Mention: strings.Contains(m.Text, "@")}
	paired := m.From.ID != "" && SenderPaired(z.Store, name, m.From.ID, time.Time{})
	switch CheckPolicy("zalo-oa", MergePolicy(name, cfg), in, paired) {
	case PolicyReject, PolicyNeedMention:
		return
	case PolicyNeedPairing:
		if msg := OfferPairingCode(z.Store, name, m.From.ID, time.Time{}); msg != "" {
			_ = send(ctx, m.Chat.ID, msg)
		}
		return
	}
	agent := boundAgent(z.Store, cfg, tenant)
	if agent == nil {
		return
	}
	ObserveDefault(Sighting{Channel: "zalo-oa", Dest: m.Chat.ID, Kind: peer, SenderID: m.From.ID, AgentID: agent.ID, TenantID: agent.TenantID})
	if BufferIfNeeded(nil, agent, "zalo-oa", m.Chat.ID) {
		return
	}
	sess := ensureLabeledSession(z.Store, agent, name+":"+m.Chat.ID)
	if sess == nil {
		return
	}
	reply, err := answerTurnErr(ctx, z.Store, agent, sess, z.LLM, z.Meter, m.Text)
	// A model error is never sent to the customer.
	if err != nil || strings.TrimSpace(reply) == "" {
		return
	}
	for _, part := range splitRunes(reply, zaloBotMaxText) {
		if send(ctx, m.Chat.ID, part) != nil {
			return
		}
	}
}

// splitRunes cuts s into pieces of at most n characters, at a line break or
// space when one is near the end of a piece.
func splitRunes(s string, n int) []string {
	r := []rune(s)
	var out []string
	for len(r) > n {
		cut := n
		for i := n; i > n*3/4; i-- {
			if r[i-1] == '\n' || r[i-1] == ' ' {
				cut = i
				break
			}
		}
		out = append(out, strings.TrimSpace(string(r[:cut])))
		r = r[cut:]
	}
	if t := strings.TrimSpace(string(r)); t != "" {
		out = append(out, t)
	}
	return out
}

func (z *ZaloBot) sendMessage(ctx context.Context, chatID, text string) error {
	token := z.resolveToken()
	if token == "" {
		return fmt.Errorf("zalo bot: missing bot token")
	}
	_, err := z.call(ctx, token, "sendMessage", map[string]any{"chat_id": chatID, "text": text})
	return err
}

// zaloBotError is a refusal from the platform.
type zaloBotError struct {
	Code int
	Desc string
}

func (e *zaloBotError) Error() string {
	return fmt.Sprintf("zalo bot API error %d: %s", e.Code, e.Desc)
}

func errors408(err error) bool {
	e, ok := err.(*zaloBotError)
	return ok && e.Code == http.StatusRequestTimeout
}

func (z *ZaloBot) call(ctx context.Context, token, method string, body any) (json.RawMessage, error) {
	return z.callWith(ctx, token, method, body, 15*time.Second)
}

// callWith posts one API method and returns its result. The token is in the
// URL, so it is cut out of any error.
func (z *ZaloBot) callWith(ctx context.Context, token, method string, body any, timeout time.Duration) (json.RawMessage, error) {
	base := z.APIBase
	if base == "" {
		base = zaloBotAPIBase
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/bot"+token+"/"+method, bytes.NewReader(b))
	if err != nil {
		return nil, redactSecret(err, token)
	}
	req.Header.Set("Content-Type", "application/json")
	client := z.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	resp, err := client.Do(req.WithContext(cctx))
	if err != nil {
		return nil, redactSecret(err, token)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var out struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		ErrorCode   int             `json:"error_code"`
		Description string          `json:"description"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("zalo bot %s: HTTP %d", method, resp.StatusCode)
	}
	if !out.OK {
		code := out.ErrorCode
		if code == 0 {
			code = resp.StatusCode
		}
		return nil, &zaloBotError{Code: code, Desc: strings.ReplaceAll(out.Description, token, "[redacted]")}
	}
	return out.Result, nil
}
