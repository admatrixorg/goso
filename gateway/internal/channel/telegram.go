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
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Telegram handles one Telegram bot. Instance is the channel name it runs as:
// empty or "telegram" for the older single bot, or "<tenant>/telegram/<name>"
// for a bot owned by one tenant (see instance.go).
type Telegram struct {
	BotToken string
	Instance string
	Store    store.StoreIface
	LLM      llm.Provider
	Meter    *billing.Store
	// Sender can be overridden in tests to capture sendMessage calls.
	Sender       func(ctx context.Context, chatID int64, text string) error
	HTTPClient   *http.Client
	APIBase      string
	probeEvery   time.Duration
	onProbeFail  func(error)
	pollStop     context.CancelFunc
	pollCancelMu sync.Mutex
	stopped      bool
}

// Name returns channel name.
func (t *Telegram) Name() string { return "telegram" }

// instance is the channel name used for config, secrets, pairing and health.
func (t *Telegram) instance() string {
	if t == nil || strings.TrimSpace(t.Instance) == "" {
		return "telegram"
	}
	return strings.TrimSpace(t.Instance)
}

// tenantBot reports whether this bot belongs to one tenant.
func (t *Telegram) tenantBot() (string, bool) {
	return TenantOf(t.instance())
}

// Start begins poll or webhook registration. Implemented in live Start (SPEC 084).
func (t *Telegram) Start(ctx context.Context, mgr *Manager) {
	if t == nil {
		return
	}
	t.startLive(ctx, mgr)
}

// Stop ends the poll loop.
func (t *Telegram) Stop() {
	if t == nil {
		return
	}
	t.stopLive()
}

// TelegramUpdate is a minimal Telegram update payload.
type TelegramUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		MessageID int64 `json:"message_id"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		From *struct {
			ID int64 `json:"id"`
		} `json:"from"`
		Text string `json:"text"`
	} `json:"message"`
}

// TelegramSecretHeader is the header Telegram sets on webhook calls when
// setWebhook was given a secret_token.
const TelegramSecretHeader = "X-Telegram-Bot-Api-Secret-Token"

const telegramWebhookPrefix = "/api/channels/telegram/webhook"

// TelegramSelfAuth reports a Telegram webhook call that carries Telegram's
// secret header. The gateway lets it past the admin token: Telegram cannot
// send one. HandleUpdate then checks the secret against the bot's own.
func TelegramSelfAuth(r *http.Request) bool {
	if r.Method != http.MethodPost || r.Header.Get(TelegramSecretHeader) == "" {
		return false
	}
	p := r.URL.Path
	return p == telegramWebhookPrefix || strings.HasPrefix(p, telegramWebhookPrefix+"/")
}

// webhookSecret is the secret_token this bot gives Telegram. It is derived
// from the bot token, so it changes when the token changes and needs no
// storage. Empty when the bot has no token.
func (t *Telegram) webhookSecret(token string) string {
	if token == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(token))
	mac.Write([]byte("goso telegram webhook\x00" + t.instance()))
	return hex.EncodeToString(mac.Sum(nil))
}

// telegramWebhookAuthorized checks a webhook call. A call with Telegram's
// secret header must match this bot's secret; it reached here without the
// admin token. Any other call already passed the admin token and may also
// need GOSO_TELEGRAM_WEBHOOK_SECRET.
func (t *Telegram) telegramWebhookAuthorized(r *http.Request) bool {
	if got := r.Header.Get(TelegramSecretHeader); got != "" {
		want := t.webhookSecret(t.resolveToken())
		return want != "" && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
	}
	sec := strings.TrimSpace(os.Getenv("GOSO_TELEGRAM_WEBHOOK_SECRET"))
	if sec == "" {
		return true
	}
	got := r.Header.Get("X-Goso-Telegram-Secret")
	if got == "" {
		got = r.URL.Query().Get("secret")
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(sec)) == 1
}

// HandleUpdate handles POST /api/channels/telegram/webhook[/{tenant}/{inst}].
func (t *Telegram) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	if !t.telegramWebhookAuthorized(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var upd TelegramUpdate
	if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	warn := t.ingest(r.Context(), upd)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if warn != "" {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "warning": warn})
		return
	}
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (t *Telegram) ingest(ctx context.Context, upd TelegramUpdate) string {
	if upd.Message == nil || upd.Message.Text == "" {
		return ""
	}
	text := upd.Message.Text
	chatID := upd.Message.Chat.ID
	fromID := ""
	if upd.Message.From != nil {
		fromID = fmt.Sprintf("%d", upd.Message.From.ID)
	}
	peer := "direct"
	if chatID < 0 {
		peer = "group"
	}
	name := t.instance()
	var cfg *store.ChannelConfig
	if t.Store != nil {
		cfg, _ = t.Store.GetChannelConfig(name)
	}
	tenant, owned := t.tenantBot()
	if owned && (cfg == nil || !cfg.Enabled) {
		return ""
	}
	pol := MergePolicy(name, cfg)
	in := Inbound{Channel: "telegram", SenderID: fromID, ChatID: fmt.Sprintf("%d", chatID), PeerKind: peer, Text: text, Mention: strings.Contains(text, "@")}
	paired := false
	if t.Store != nil && fromID != "" {
		paired = SenderPaired(t.Store, name, fromID, time.Time{})
	}
	sendFn := t.Sender
	if sendFn == nil {
		sendFn = t.sendMessage
	}
	switch CheckPolicy("telegram", pol, in, paired) {
	case PolicyReject, PolicyNeedMention:
		return ""
	case PolicyNeedPairing:
		if msg := OfferPairingCode(t.Store, name, fromID, time.Time{}); msg != "" {
			_ = sendFn(ctx, chatID, msg)
		}
		return ""
	}

	var agent *store.Agent
	if owned {
		// A tenant's bot answers only as the agent bound to it, and only when
		// that agent is in the same tenant. It never creates an agent.
		agent = boundAgent(t.Store, cfg, tenant)
		if agent == nil {
			return "no agent bound to " + name
		}
	} else {
		agent = t.ensureAgent()
		if cfg != nil && cfg.AgentID != "" && t.Store != nil {
			if a, err := t.Store.GetAgent(cfg.AgentID); err == nil {
				agent = a
			}
		}
	}
	if agent == nil {
		return "no agent"
	}
	sight := Sighting{Channel: "telegram", Dest: fmt.Sprintf("%d", chatID), Kind: peer, SenderID: fromID}
	if agent != nil {
		sight.AgentID = agent.ID
		sight.TenantID = agent.TenantID
	}
	ObserveDefault(sight)
	if BufferIfNeeded(nil, agent, "telegram", fmt.Sprintf("%d", chatID)) {
		return ""
	}
	sess := t.ensureSession(agent, chatID)
	if sess == nil {
		return "session unavailable"
	}
	ctx = withContextUser(ctx, name, peer, fromID, fmt.Sprintf("%d", chatID))
	reply := answerTurn(ctx, t.Store, agent, sess, t.LLM, t.Meter, text)
	if err := sendFn(ctx, chatID, reply); err != nil {
		return err.Error()
	}
	return ""
}

func (t *Telegram) ensureAgent() *store.Agent {
	// Find existing telegram agent or create one.
	for _, a := range t.Store.ListAgents() {
		if a.AgentKey == "telegram" {
			return a
		}
	}
	a, _ := t.Store.CreateAgent(store.Agent{AgentKey: "telegram", DisplayName: "Telegram Bot"})
	return a
}

func (t *Telegram) ensureSession(agent *store.Agent, chatID int64) *store.Session {
	return ensureLabeledSession(t.Store, agent, fmt.Sprintf("%s:%d", t.instance(), chatID))
}

// boundAgent returns the agent bound to a tenant's channel, or nil when none
// is bound, it is gone, or it belongs to another tenant.
func boundAgent(st store.StoreIface, cfg *store.ChannelConfig, tenant string) *store.Agent {
	if st == nil || cfg == nil || strings.TrimSpace(cfg.AgentID) == "" {
		return nil
	}
	a, err := st.GetAgent(cfg.AgentID)
	if err != nil || a == nil || !store.SameTenant(a.TenantID, tenant) {
		return nil
	}
	return a
}

func (t *Telegram) resolveToken() string {
	if t == nil {
		return ""
	}
	if v := strings.TrimSpace(t.BotToken); v != "" {
		return v
	}
	var env []string
	if _, owned := t.tenantBot(); !owned {
		env = []string{"GOSO_TELEGRAM_BOT_TOKEN"}
	}
	v, _, set := Credential(t.Store, t.instance(), KindBot, env)
	if set {
		return v
	}
	return ""
}

func redactSecret(err error, secret string) error {
	if err == nil || secret == "" {
		return err
	}
	s := strings.ReplaceAll(err.Error(), secret, "[redacted]")
	if s == err.Error() {
		return err
	}
	return fmt.Errorf("%s", s)
}

// ProbeToken runs getMe with the resolved bot token (SPEC 088 test).
func (t *Telegram) ProbeToken(ctx context.Context) error {
	token := t.resolveToken()
	if token == "" {
		return fmt.Errorf("missing bot token")
	}
	return redactSecret(t.getMe(ctx, token), token)
}

func (t *Telegram) sendMessage(ctx context.Context, chatID int64, text string) error {
	token := t.resolveToken()
	if token == "" {
		return fmt.Errorf("telegram: missing bot token")
	}
	body, _ := json.Marshal(map[string]any{"chat_id": chatID, "text": text})
	base := t.APIBase
	if base == "" {
		base = "https://api.telegram.org"
	}
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(base, "/")+"/bot"+token+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := t.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return fmt.Errorf("telegram sendMessage %d: %s", resp.StatusCode, string(b))
	}
	return nil
}
