// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// A bridged Zalo channel is a tenant's zalo-personal instance whose Zalo
// account lives in ZaloCRM. ZaloCRM posts each customer message to
// ZCRMBridgePath, signed with the tenant's bridge key; the agent's answer goes
// back to ZaloCRM's reply endpoint, signed with the same key. ZaloCRM sends
// it to the customer, unless a person took the conversation over.
//
// The instance's settings say which ZaloCRM account it answers for
// (account_id), where ZaloCRM is (crm_base_url) and whether it answers in
// groups (group_enabled).

// ZCRMBridgePath is where ZaloCRM posts customer messages.
const ZCRMBridgePath = "/api/channels/zalo-personal/bridge"

const (
	zcrmSignatureHeader = "X-Goclaw-Signature"
	zcrmTimestampHeader = "X-Goclaw-Timestamp"
	zcrmNonceHeader     = "X-Goclaw-Nonce"
	zcrmVersionHeader   = "X-Goclaw-Version"
	zcrmContentType     = "application/vnd.goclaw-bridge+json"
	// zcrmVersion is the bridge contract version both sides speak.
	zcrmVersion = 2
	// zcrmWindow is how far a call's timestamp may be from now.
	zcrmWindow = 5 * time.Minute
	// zcrmReplyPath is ZaloCRM's endpoint for an agent's answer.
	zcrmReplyPath = "/api/internal/goclaw/reply"
	zcrmMaxBody   = 1 << 20
)

// Settings keys of a bridged Zalo channel.
const (
	SettingAccountID    = "account_id"
	SettingCRMBaseURL   = "crm_base_url"
	SettingGroupEnabled = "group_enabled"
)

// ZCRMSelfAuth reports a ZaloCRM bridge call that carries its signature. The
// gateway lets it past the admin token: ZaloCRM signs instead. The handler
// checks the signature against the channel's own key.
func ZCRMSelfAuth(r *http.Request) bool {
	return r.Method == http.MethodPost && r.URL.Path == ZCRMBridgePath && r.Header.Get(zcrmSignatureHeader) != ""
}

// ZCRMSign is the signature of one bridge call: hex HMAC-SHA256 of
// "timestamp.nonce.body" under the bridge key.
func ZCRMSign(secret, timestamp, nonce string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "." + nonce + "."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// ZCRMEvent is one customer message from ZaloCRM.
type ZCRMEvent struct {
	EventID   string `json:"eventId"`
	Event     string `json:"event"`
	Timestamp string `json:"timestamp"`
	Data      struct {
		AccountID      string `json:"accountId"`
		Channel        string `json:"channel"`
		MessageID      string `json:"messageId"`
		ConversationID string `json:"conversationId"`
		SenderUID      string `json:"senderUid"`
		SenderName     string `json:"senderName"`
		Content        string `json:"content"`
		ContentType    string `json:"contentType"`
		ThreadType     string `json:"threadType"`
		AgentLease     *int64 `json:"agentLease"`
		AgentKey       string `json:"agentKey"`
	} `json:"data"`
}

// ZCRMBridge serves ZCRMBridgePath for every tenant's bridged Zalo channels.
type ZCRMBridge struct {
	Store      store.StoreIface
	LLM        llm.Provider
	Meter      *billing.Store
	HTTPClient *http.Client

	// now and run are replaced in tests. run answers one message; by default
	// in the background, so ZaloCRM is not kept waiting on the model.
	now func() time.Time
	run func(func())

	mu     sync.Mutex
	nonces map[string]time.Time
}

func (b *ZCRMBridge) clock() time.Time {
	if b.now != nil {
		return b.now()
	}
	return time.Now()
}

// bridgeChannel is one bridged Zalo channel that may own an event.
type bridgeChannel struct {
	cfg    *store.ChannelConfig
	tenant string
	secret string
}

// candidates are the bridged channels set up for a ZaloCRM account.
func (b *ZCRMBridge) candidates(accountID string) []bridgeChannel {
	if b.Store == nil || accountID == "" {
		return nil
	}
	var out []bridgeChannel
	for _, cfg := range b.Store.ListChannelConfigs() {
		if cfg == nil || CatalogOf(cfg.Name) != "zalo-personal" {
			continue
		}
		tenant, owned := TenantOf(cfg.Name)
		if !owned || strings.TrimSpace(cfg.Settings[SettingAccountID]) != accountID {
			continue
		}
		sec, _, _ := Credential(b.Store, cfg.Name, KindBridge, nil)
		out = append(out, bridgeChannel{cfg: cfg, tenant: tenant, secret: sec})
	}
	return out
}

// seen records a nonce and reports whether it was already used inside the
// time window. Old nonces are dropped as new ones come.
func (b *ZCRMBridge) seen(nonce string, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.nonces == nil {
		b.nonces = map[string]time.Time{}
	}
	for n, exp := range b.nonces {
		if now.After(exp) {
			delete(b.nonces, n)
		}
	}
	if _, ok := b.nonces[nonce]; ok {
		return true
	}
	b.nonces[nonce] = now.Add(2 * zcrmWindow)
	return false
}

func writeBridgeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": msg})
}

// HandleUpdate handles POST ZCRMBridgePath.
func (b *ZCRMBridge) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	if v := r.Header.Get(zcrmVersionHeader); v != "" && v != strconv.Itoa(zcrmVersion) {
		writeBridgeErr(w, http.StatusPreconditionFailed, "bridge version incompatible")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, zcrmMaxBody+1))
	if err != nil || len(body) > zcrmMaxBody {
		writeBridgeErr(w, http.StatusRequestEntityTooLarge, "body too large")
		return
	}
	var ev ZCRMEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		writeBridgeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	sig := strings.TrimSpace(r.Header.Get(zcrmSignatureHeader))
	ts := strings.TrimSpace(r.Header.Get(zcrmTimestampHeader))
	nonce := strings.TrimSpace(r.Header.Get(zcrmNonceHeader))
	now := b.clock()
	ms, err := strconv.ParseInt(ts, 10, 64)
	if sig == "" || nonce == "" || err != nil {
		writeBridgeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if d := now.Sub(time.UnixMilli(ms)); d > zcrmWindow || d < -zcrmWindow {
		writeBridgeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	accountID := strings.TrimSpace(ev.Data.AccountID)
	var owner *bridgeChannel
	for _, c := range b.candidates(accountID) {
		if c.secret == "" {
			continue
		}
		want := ZCRMSign(c.secret, ts, nonce, body)
		if subtle.ConstantTimeCompare([]byte(sig), []byte(want)) == 1 {
			c := c
			owner = &c
			break
		}
	}
	// An unknown account and a wrong signature look the same to the caller.
	if owner == nil {
		writeBridgeErr(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if b.seen(nonce, now) {
		writeBridgeErr(w, http.StatusConflict, "duplicate request")
		return
	}
	if !owner.cfg.Enabled {
		writeBridgeErr(w, http.StatusServiceUnavailable, "channel disabled")
		return
	}
	if ev.Event != "" && ev.Event != "message.received" {
		writeOK(w)
		return
	}
	if isGroupThread(ev.Data.ThreadType) && !settingTrue(owner.cfg.Settings[SettingGroupEnabled]) {
		writeOK(w)
		return
	}
	if strings.TrimSpace(ev.Data.ConversationID) == "" || bridgeText(ev) == "" {
		writeOK(w)
		return
	}
	agent := b.agentFor(owner, ev.Data.AgentKey)
	if agent == nil {
		writeBridgeErr(w, http.StatusServiceUnavailable, "no agent bound to "+owner.cfg.Name)
		return
	}
	run := b.run
	if run == nil {
		run = func(f func()) { go f() }
	}
	ch := *owner
	run(func() { b.answer(ch, agent, ev) })
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// agentFor is the agent that answers: the one ZaloCRM names, when it is in
// the channel's tenant, else the one bound to the channel.
func (b *ZCRMBridge) agentFor(c *bridgeChannel, key string) *store.Agent {
	if key = strings.TrimSpace(key); key != "" {
		for _, a := range b.Store.ListAgents() {
			if a != nil && a.AgentKey == key && store.SameTenant(a.TenantID, c.tenant) {
				return a
			}
		}
	}
	return boundAgent(b.Store, c.cfg, c.tenant)
}

func isGroupThread(t string) bool {
	t = strings.ToLower(strings.TrimSpace(t))
	return t == "group" || t == "1"
}

func settingTrue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// bridgeText is what the agent reads for one message. Text is passed as is;
// other kinds become a short note so the agent knows something arrived.
func bridgeText(ev ZCRMEvent) string {
	content := strings.TrimSpace(ev.Data.Content)
	kind := strings.ToLower(strings.TrimSpace(ev.Data.ContentType))
	if kind == "" || kind == "text" {
		return content
	}
	return "[" + kind + "]"
}

func (b *ZCRMBridge) answer(c bridgeChannel, agent *store.Agent, ev ZCRMEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	conv := strings.TrimSpace(ev.Data.ConversationID)
	ObserveDefault(Sighting{
		Channel: "zalo-personal", Dest: conv, Kind: peerKind(ev.Data.ThreadType),
		SenderID: ev.Data.SenderUID, AgentID: agent.ID, TenantID: agent.TenantID,
	})
	sess := ensureLabeledSession(b.Store, agent, c.cfg.Name+":"+conv)
	if sess == nil {
		return
	}
	reply, err := answerTurnErr(ctx, b.Store, agent, sess, b.LLM, b.Meter, bridgeText(ev))
	if err != nil || strings.TrimSpace(reply) == "" {
		return
	}
	_ = b.sendReply(ctx, c, ev, reply)
}

func peerKind(threadType string) string {
	if isGroupThread(threadType) {
		return "group"
	}
	return "direct"
}

// sendReply posts the answer to ZaloCRM. A refusal (4xx), such as a person
// having taken the conversation over, is final; a network error or 5xx is
// tried again a few times with the same idempotency key.
func (b *ZCRMBridge) sendReply(ctx context.Context, c bridgeChannel, ev ZCRMEvent, text string) error {
	base := strings.TrimRight(strings.TrimSpace(c.cfg.Settings[SettingCRMBaseURL]), "/")
	if base == "" {
		return fmt.Errorf("zcrm: crm_base_url not set for %s", c.cfg.Name)
	}
	payload := map[string]any{
		"conversationId": ev.Data.ConversationID,
		"content":        text,
		"idempotencyKey": "goso:" + ev.Data.MessageID,
	}
	if ev.Data.AgentLease != nil {
		payload["agentLease"] = *ev.Data.AgentLease
	}
	body, _ := json.Marshal(payload)
	client := b.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}
		status, err := b.postSigned(ctx, client, base+zcrmReplyPath, c.secret, body)
		if err == nil && status >= 200 && status < 300 {
			return nil
		}
		if err == nil && status < 500 {
			return fmt.Errorf("zcrm reply %d", status)
		}
		last = err
		if last == nil {
			last = fmt.Errorf("zcrm reply %d", status)
		}
	}
	return last
}

func (b *ZCRMBridge) postSigned(ctx context.Context, client *http.Client, url, secret string, body []byte) (int, error) {
	ts := strconv.FormatInt(b.clock().UnixMilli(), 10)
	nonce := randomNonce()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", zcrmContentType)
	req.Header.Set(zcrmVersionHeader, strconv.Itoa(zcrmVersion))
	req.Header.Set(zcrmTimestampHeader, ts)
	req.Header.Set(zcrmNonceHeader, nonce)
	req.Header.Set(zcrmSignatureHeader, ZCRMSign(secret, ts, nonce, body))
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, nil
}

func randomNonce() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
