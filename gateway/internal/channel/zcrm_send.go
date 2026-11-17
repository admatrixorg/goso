// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// zcrmIdentity is GoClaw's identity of a bridged Zalo message. The chat is
// ZaloCRM's conversation id: it is all the reply endpoint takes, so it is
// also what GoClaw's bridge channel keyed sessions and groups by.
func zcrmIdentity(name string, ev ZCRMEvent) Identity {
	return Identity{
		Instance: name,
		Peer:     peerKind(ev.Data.ThreadType),
		SenderID: strings.TrimSpace(ev.Data.SenderUID),
		ChatID:   strings.TrimSpace(ev.Data.ConversationID),
	}
}

func leaseKey(name, conv string) string { return name + "\x00" + conv }

// rememberLease keeps the latest agent lease ZaloCRM gave for a
// conversation. ZaloCRM refuses a reply without the current lease, so a
// message the agent starts later (the message tool) needs it.
func (b *ZCRMBridge) rememberLease(name, conv string, lease *int64) {
	if lease == nil || conv == "" {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.leases == nil {
		b.leases = map[string]int64{}
	}
	b.leases[leaseKey(name, conv)] = *lease
}

func (b *ZCRMBridge) lease(name, conv string) (int64, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	v, ok := b.leases[leaseKey(name, conv)]
	return v, ok
}

// errNoLease means ZaloCRM has not handed this conversation to the agent
// since goso started.
var errNoLease = errors.New("zcrm: no agent lease for this conversation")

// bridgeFor is the bridged channel name as the bridge sees it: a tenant's
// zalo-personal instance with an account, a ZaloCRM address and a key.
func (b *ZCRMBridge) bridgeFor(name string) (bridgeChannel, bool) {
	if b == nil || b.Store == nil || CatalogOf(name) != "zalo-personal" {
		return bridgeChannel{}, false
	}
	tenant, owned := TenantOf(name)
	if !owned {
		return bridgeChannel{}, false
	}
	cfg, err := b.Store.GetChannelConfig(name)
	if err != nil || cfg == nil {
		return bridgeChannel{}, false
	}
	if strings.TrimSpace(cfg.Settings[SettingAccountID]) == "" || strings.TrimSpace(cfg.Settings[SettingCRMBaseURL]) == "" {
		return bridgeChannel{}, false
	}
	sec, _, _ := Credential(b.Store, name, KindBridge, nil)
	if sec == "" {
		return bridgeChannel{}, false
	}
	return bridgeChannel{cfg: cfg, tenant: tenant, secret: sec}, true
}

// Ready reports whether the bridge can send on this channel.
func (b *ZCRMBridge) Ready(name string) bool {
	_, ok := b.bridgeFor(name)
	return ok
}

// SendText sends a message the agent starts on its own (the message tool)
// into a ZaloCRM conversation, under the conversation's current agent lease.
// It is sent once: a refusal (a person took over, the lease moved on) is
// final.
func (b *ZCRMBridge) SendText(ctx context.Context, name, conv, text string) error {
	c, ok := b.bridgeFor(name)
	if !ok {
		return fmt.Errorf("zcrm: %s is not a bridged channel", name)
	}
	lease, ok := b.lease(name, conv)
	if !ok {
		return errNoLease
	}
	body, _ := json.Marshal(map[string]any{
		"conversationId": conv,
		"content":        text,
		"idempotencyKey": "goso:tool:" + randomNonce(),
		"agentLease":     lease,
	})
	client := b.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	base := strings.TrimRight(strings.TrimSpace(c.cfg.Settings[SettingCRMBaseURL]), "/")
	status, err := b.postSigned(ctx, client, base+zcrmReplyPath, c.secret, body)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("zcrm reply %d", status)
	}
	return nil
}
