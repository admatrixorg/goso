package httpapi

import (
	"context"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/channel"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/zalo/protocol"
)

func wireZaloInbound(st store.StoreIface, provider llm.Provider, meter *billing.Store) {
	zaloManager.OnInbound = func(instance string, msg protocol.InboundMessage) {
		channel.IngestInstance(context.Background(), st, provider, meter, instance, msg.FromID, msg.ThreadID, msg.ThreadType, msg.Text, func(ctx context.Context, threadID, text string) error {
			return zaloManager.Send(instance, threadID, msg.ThreadType, text)
		})
	}
}

// StartConfiguredZaloListeners starts a native session for every enabled
// zalo-personal instance that has a stored cookie set.
func StartConfiguredZaloListeners(st store.StoreIface) {
	if st == nil {
		return
	}
	for _, cfg := range st.ListChannelConfigs() {
		if cfg == nil || !cfg.Enabled {
			continue
		}
		if channel.CatalogOf(cfg.Name) != "zalo-personal" {
			continue
		}
		cred, err := storedZaloCredential(st, cfg.Name)
		if err != nil {
			continue
		}
		name := cfg.Name
		c := cred
		go func() {
			_, _ = zaloManager.Start(context.Background(), name, c)
		}()
	}
}

func startZaloListener(st store.StoreIface, name string) {
	cred, err := storedZaloCredential(st, name)
	if err != nil {
		return
	}
	go func() {
		_, _ = zaloManager.Start(context.Background(), name, cred)
	}()
}

var _ = strings.TrimSpace
