package channel

import (
	"context"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestIngestInstanceSendsOnOpenDM(t *testing.T) {
	st := store.New()
	agent, err := st.CreateAgent(store.Agent{TenantID: "tenant-a", AgentKey: "mod-content", DisplayName: "Content"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutChannelConfig(store.ChannelConfig{
		Name: "tenant-a/zalo-personal/shop", Enabled: true, AgentID: agent.ID, DMPolicy: "open",
	}); err != nil {
		t.Fatal(err)
	}
	var sent []string
	IngestInstance(context.Background(), st, nil, nil, "tenant-a/zalo-personal/shop", "u1", "u1", "user", "xin chao", func(_ context.Context, thread, text string) error {
		sent = append(sent, thread+"|"+text)
		return nil
	})
	if len(sent) != 1 || !strings.HasPrefix(sent[0], "u1|") || strings.TrimSpace(sent[0]) == "u1|" {
		t.Fatalf("sent %v", sent)
	}
}

func TestIngestInstanceSilentWhenDisabled(t *testing.T) {
	st := store.New()
	if err := st.PutChannelConfig(store.ChannelConfig{Name: "tenant-a/zalo-personal/shop", Enabled: false, DMPolicy: "open"}); err != nil {
		t.Fatal(err)
	}
	called := false
	IngestInstance(context.Background(), st, nil, nil, "tenant-a/zalo-personal/shop", "u1", "u1", "user", "hi", func(context.Context, string, string) error {
		called = true
		return nil
	})
	if called {
		t.Fatal("disabled channel sent a reply")
	}
}

func TestIngestInstanceOffersPairingOnDM(t *testing.T) {
	st := store.New()
	agent, _ := st.CreateAgent(store.Agent{TenantID: "tenant-a", AgentKey: "a", DisplayName: "A"})
	if err := st.PutChannelConfig(store.ChannelConfig{
		Name: "tenant-a/zalo-personal/shop", Enabled: true, AgentID: agent.ID, DMPolicy: "pairing",
	}); err != nil {
		t.Fatal(err)
	}
	var sent []string
	IngestInstance(context.Background(), st, nil, nil, "tenant-a/zalo-personal/shop", "u1", "u1", "user", "hello", func(_ context.Context, _, text string) error {
		sent = append(sent, text)
		return nil
	})
	if len(sent) != 1 || sent[0] == "" {
		t.Fatalf("pairing offer %v", sent)
	}
}
