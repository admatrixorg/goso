// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Package hooks runs tenant-configured lifecycle hooks (table hooks). A hook
// fires on one pipeline event, for one agent or every agent of its tenant,
// and either POSTs the event to a URL (handler "http", signed with HMAC) or,
// on PreToolUse only, denies the tool call (handler "block"). Hooks never
// break a run: delivery is asynchronous and every failure is only logged.
package hooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mqglobal/goso/gateway/internal/pipeline"
	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Handler types.
const (
	HandlerHTTP  = "http"
	HandlerBlock = "block"
)

// Delivery headers of an http hook.
const (
	HeaderEvent     = "X-Goso-Event"
	HeaderHookID    = "X-Goso-Hook-Id"
	HeaderTimestamp = "X-Goso-Timestamp"
	HeaderSignature = "X-Goso-Signature"
)

const (
	defaultTimeout = 3 * time.Second
	maxTimeout     = 10 * time.Second
	maxResult      = 4000
	cacheTTL       = 30 * time.Second
)

// Events lists the hookable pipeline events.
var Events = []string{pipeline.SessionStart, pipeline.UserPromptSubmit, pipeline.PreToolUse, pipeline.PostToolUse, pipeline.Stop}

// Config is the handler JSON of a hook.
type Config struct {
	URL       string `json:"url,omitempty"`
	TimeoutMS int    `json:"timeout_ms,omitempty"`
	Message   string `json:"message,omitempty"`
}

// ParseConfig decodes hook config JSON. Empty is an empty config.
func ParseConfig(raw string) (Config, error) {
	var c Config
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return c, nil
	}
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return c, errors.New("config must be a JSON object")
	}
	c.URL = strings.TrimSpace(c.URL)
	c.Message = strings.TrimSpace(c.Message)
	return c, nil
}

func (c Config) timeout() time.Duration {
	if c.TimeoutMS <= 0 {
		return defaultTimeout
	}
	d := time.Duration(c.TimeoutMS) * time.Millisecond
	if d > maxTimeout {
		return maxTimeout
	}
	return d
}

func isToolEvent(ev string) bool {
	return ev == pipeline.PreToolUse || ev == pipeline.PostToolUse
}

// Validate checks a hook before it is stored: a known event, a known
// handler, block only on PreToolUse, a valid tool glob only on tool events,
// and for http a public http(s) URL (SSRF guard).
func Validate(h *store.Hook) error {
	h.Event = strings.TrimSpace(h.Event)
	h.HandlerType = strings.ToLower(strings.TrimSpace(h.HandlerType))
	h.Matcher = strings.TrimSpace(h.Matcher)
	known := false
	for _, e := range Events {
		if e == h.Event {
			known = true
		}
	}
	if !known {
		return fmt.Errorf("event must be one of %s", strings.Join(Events, ", "))
	}
	if h.Matcher != "" {
		if !isToolEvent(h.Event) {
			return errors.New("matcher applies only to PreToolUse and PostToolUse")
		}
		if _, err := path.Match(h.Matcher, ""); err != nil {
			return errors.New("matcher is not a valid glob")
		}
	}
	cfg, err := ParseConfig(h.Config)
	if err != nil {
		return err
	}
	switch h.HandlerType {
	case HandlerBlock:
		if h.Event != pipeline.PreToolUse {
			return errors.New("handler block is only allowed on PreToolUse")
		}
		cfg.URL = ""
	case HandlerHTTP:
		u, err := url.Parse(cfg.URL)
		if cfg.URL == "" || err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("http hook needs config.url with http or https")
		}
		if err := security.CheckURL(cfg.URL); err != nil {
			return errors.New("config.url is not allowed: " + err.Error())
		}
	default:
		return errors.New("handler_type must be http or block")
	}
	if cfg.TimeoutMS < 0 {
		cfg.TimeoutMS = 0
	}
	b, _ := json.Marshal(cfg)
	h.Config = string(b)
	return nil
}

// SealSecret encrypts an HMAC signing secret with the gateway master key.
// Without GOSO_MASTER_KEY it returns secrets.ErrNoMasterKey.
func SealSecret(secret string) (string, error) {
	nonce, ct, err := secrets.Encrypt([]byte(secret))
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(nonce) + ":" + hex.EncodeToString(ct), nil
}

func openSecret(enc string) (string, error) {
	enc = strings.TrimSpace(enc)
	if enc == "" {
		return "", nil
	}
	n, c, ok := strings.Cut(enc, ":")
	if !ok {
		return "", errors.New("bad secret")
	}
	nonce, err := hex.DecodeString(n)
	if err != nil {
		return "", err
	}
	ct, err := hex.DecodeString(c)
	if err != nil {
		return "", err
	}
	pt, err := secrets.Decrypt(nonce, ct)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// Sign returns the X-Goso-Signature value for a delivery:
// "sha256=" + hex(HMAC-SHA256(secret, timestamp + "." + body)).
func Sign(secret, timestamp string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(timestamp))
	m.Write([]byte("."))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

// generation is bumped by every hook write; engines drop their cache when
// it moves, so a change applies on the next event.
var generation atomic.Uint64

// Invalidate drops cached hooks after a create, update or delete.
func Invalidate() { generation.Add(1) }

type cached struct {
	at    time.Time
	hooks []*store.Hook
}

// Engine is the pipeline.Source for configured hooks.
type Engine struct {
	st     store.HookStore
	client *http.Client

	mu    sync.Mutex
	gen   uint64
	cache map[string]cached

	wg sync.WaitGroup
}

// NewEngine returns an engine over st, or nil when st has no hook store.
func NewEngine(st store.StoreIface) *Engine {
	hs := store.Hooks(st)
	if hs == nil {
		return nil
	}
	c := &http.Client{}
	security.GuardClient(c)
	return &Engine{st: hs, client: c, cache: map[string]cached{}}
}

// Wait blocks until in-flight http deliveries finish (tests, shutdown).
func (e *Engine) Wait() {
	if e != nil {
		e.wg.Wait()
	}
}

func (e *Engine) load(tenant string) []*store.Hook {
	tenant = store.NormalizeTenant(tenant)
	g := generation.Load()
	e.mu.Lock()
	if g != e.gen {
		e.cache = map[string]cached{}
		e.gen = g
	}
	if c, ok := e.cache[tenant]; ok && time.Since(c.at) < cacheTTL {
		e.mu.Unlock()
		return c.hooks
	}
	e.mu.Unlock()
	list, err := e.st.ListHooks(tenant)
	if err != nil {
		log.Printf("hooks: load tenant hooks: %v", err)
		return nil
	}
	e.mu.Lock()
	if generation.Load() == e.gen {
		e.cache[tenant] = cached{at: time.Now(), hooks: list}
	}
	e.mu.Unlock()
	return list
}

func matches(h *store.Hook, ev pipeline.Event) bool {
	if h == nil || !h.Enabled || h.Event != ev.Name {
		return false
	}
	if h.AgentID != "" && h.AgentID != ev.AgentID {
		return false
	}
	if !isToolEvent(ev.Name) || h.Matcher == "" || h.Matcher == "*" {
		return true
	}
	if ok, _ := path.Match(h.Matcher, ev.Tool); ok {
		return true
	}
	if ev.Connector != "" {
		if ok, _ := path.Match(h.Matcher, ev.Connector+"__"+ev.Tool); ok {
			return true
		}
	}
	return false
}

// Run applies the tenant's hooks to ev. A matching block hook on
// PreToolUse denies the call; http hooks are delivered in the background.
func (e *Engine) Run(ctx context.Context, ev pipeline.Event) pipeline.Decision {
	if e == nil {
		return pipeline.Decision{}
	}
	var dec pipeline.Decision
	for _, h := range e.load(ev.TenantID) {
		if !matches(h, ev) {
			continue
		}
		cfg, _ := ParseConfig(h.Config)
		switch h.HandlerType {
		case HandlerBlock:
			if ev.Name == pipeline.PreToolUse && !dec.Block {
				dec = pipeline.Decision{Block: true, Message: cfg.Message}
			}
		case HandlerHTTP:
			e.deliver(h, cfg, ev)
		}
	}
	return dec
}

type payload struct {
	Event     string         `json:"event"`
	HookID    string         `json:"hook_id"`
	TenantID  string         `json:"tenant_id"`
	AgentID   string         `json:"agent_id,omitempty"`
	SessionID string         `json:"session_id,omitempty"`
	Tool      string         `json:"tool,omitempty"`
	Connector string         `json:"connector,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
	Result    string         `json:"result,omitempty"`
	Error     string         `json:"error,omitempty"`
	Timestamp string         `json:"ts"`
}

func (e *Engine) deliver(h *store.Hook, cfg Config, ev pipeline.Event) {
	now := time.Now().UTC()
	res := ev.Result
	if len(res) > maxResult {
		res = res[:maxResult]
	}
	body, err := json.Marshal(payload{
		Event: ev.Name, HookID: h.ID, TenantID: store.NormalizeTenant(ev.TenantID),
		AgentID: ev.AgentID, SessionID: ev.SessionID, Tool: ev.Tool, Connector: ev.Connector,
		Arguments: ev.Arguments, Result: res, Error: ev.Error, Timestamp: now.Format(time.RFC3339Nano),
	})
	if err != nil {
		log.Printf("hooks: hook %s: encode event: %v", h.ID, err)
		return
	}
	secretEnc := h.SecretEnc
	hookID := h.ID
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		if err := e.post(hookID, cfg, secretEnc, body, ev.Name, now); err != nil {
			log.Printf("hooks: hook %s %s delivery failed: %v", hookID, ev.Name, err)
		}
	}()
}

func (e *Engine) post(hookID string, cfg Config, secretEnc string, body []byte, event string, now time.Time) error {
	if err := security.CheckURL(cfg.URL); err != nil {
		return errors.New("url blocked by ssrf guard")
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return errors.New("bad url")
	}
	ts := strconv.FormatInt(now.Unix(), 10)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderEvent, event)
	req.Header.Set(HeaderHookID, hookID)
	req.Header.Set(HeaderTimestamp, ts)
	if secretEnc != "" {
		secret, err := openSecret(secretEnc)
		if err != nil {
			return errors.New("signing secret unavailable")
		}
		req.Header.Set(HeaderSignature, Sign(secret, ts, body))
	}
	resp, err := e.client.Do(req)
	if err != nil {
		// The error text can carry the URL; keep it out of the log.
		var ue *url.Error
		if errors.As(err, &ue) {
			return ue.Err
		}
		return errors.New("request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}
