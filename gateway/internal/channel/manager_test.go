// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"context"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestManager_LiteForbidsStart(t *testing.T) {
	t.Setenv("GOSO_LITE", "1")
	t.Setenv("GOSO_TELEGRAM_BOT_TOKEN", "tok")
	m := NewManager()
	m.Telegram = &Telegram{BotToken: "tok"}
	m.StartAll(context.Background())
	if m.Running("telegram") {
		t.Fatal("lite must not start")
	}
	got := CatalogWith(store.New(), m)
	for _, c := range got {
		if c.Name == "telegram" && c.Health != "stopped" {
			t.Fatalf("lite telegram health %s", c.Health)
		}
		if c.Name == "slack" && c.Health != "parked" {
			t.Fatalf("slack %s", c.Health)
		}
	}
}

func TestManager_Phase2NotStarted(t *testing.T) {
	t.Setenv("GOSO_LITE", "")
	t.Setenv("GOSO_SLACK_BOT_TOKEN", "b")
	t.Setenv("GOSO_SLACK_APP_TOKEN", "a")
	m := NewManager()
	m.StartAll(context.Background())
	if m.Running("slack") {
		t.Fatal("slack parked")
	}
}
