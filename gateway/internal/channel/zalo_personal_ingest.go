package channel

import (
	"context"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// IngestInstance answers one native personal-account message for a named
// instance. The webhook path still uses HandleUpdate and the catalog name.
func IngestInstance(ctx context.Context, st store.StoreIface, provider llm.Provider, meter *billing.Store, instance, fromID, threadID, threadType, text string, send func(context.Context, string, string) error) {
	instance = strings.TrimSpace(instance)
	text = strings.TrimSpace(text)
	threadID = strings.TrimSpace(threadID)
	if instance == "" || threadID == "" || text == "" || send == nil {
		return
	}
	var cfg *store.ChannelConfig
	if st != nil {
		cfg, _ = st.GetChannelConfig(instance)
	}
	if cfg == nil || !cfg.Enabled {
		return
	}
	peer := "direct"
	if strings.EqualFold(strings.TrimSpace(threadType), "group") {
		peer = "group"
	}
	pol := MergePolicy(instance, cfg)
	in := Inbound{Channel: instance, SenderID: fromID, ChatID: threadID, PeerKind: peer, Text: text, Mention: strings.Contains(text, "@")}
	paired := st != nil && fromID != "" && SenderPaired(st, instance, fromID, time.Time{})
	switch CheckPolicy("zalo-personal", pol, in, paired) {
	case PolicyReject, PolicyNeedMention:
		return
	case PolicyNeedPairing:
		if msg := OfferPairingCode(st, instance, fromID, time.Time{}); msg != "" {
			_ = send(ctx, threadID, msg)
		}
		return
	}
	tenant, _ := TenantOf(instance)
	agent := boundAgent(st, cfg, tenant)
	if agent == nil {
		return
	}
	id := Identity{Instance: instance, Peer: peer, SenderID: fromID, ChatID: threadID}
	sess := ensureChannelSession(st, agent, id)
	if sess == nil {
		return
	}
	ctx = withContextUser(ctx, id)
	reply := answerTurn(ctx, st, agent, sess, provider, meter, text)
	if strings.TrimSpace(reply) == "" {
		return
	}
	_ = send(ctx, threadID, reply)
}
