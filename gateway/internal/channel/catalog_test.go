// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
)

func TestCatalog_SevenNamesUnconfigured(t *testing.T) {
	t.Setenv("GOSO_TELEGRAM_BOT_TOKEN", "")
	t.Setenv("GOSO_ZALO_PERSONAL_TOKEN", "")
	t.Setenv("GOSO_ZALO_OA_ACCESS_TOKEN", "")
	t.Setenv("GOSO_ZALO_OA_SECRET", "")
	t.Setenv("GOSO_ZALO_OA_APP_ID", "")
	t.Setenv("GOSO_DISCORD_BOT_TOKEN", "")
	t.Setenv("GOSO_SLACK_BOT_TOKEN", "")
	t.Setenv("GOSO_SLACK_APP_TOKEN", "")
	t.Setenv("GOSO_FEISHU_APP_SECRET", "")
	t.Setenv("GOSO_WHATSAPP_ACCESS_TOKEN", "")
	got := Catalog()
	if len(got) != 7 {
		t.Fatalf("want 7, got %d %+v", len(got), got)
	}
	want := []string{"telegram", "zalo-personal", "zalo-oa", "discord", "slack", "feishu", "whatsapp"}
	for i, n := range want {
		if got[i].Name != n {
			t.Fatalf("idx %d name %q want %q", i, got[i].Name, n)
		}
		if got[i].Configured {
			t.Fatalf("%s configured with empty env", n)
		}
		if !got[i].Missing {
			t.Fatalf("%s missing=false with empty env", n)
		}
		if got[i].Env != wantEnv[n] {
			t.Fatalf("%s env %q want %q", n, got[i].Env, wantEnv[n])
		}
		if len(got[i].EnvNames) < 1 || got[i].EnvNames[0] != wantEnv[n] {
			t.Fatalf("%s env_names %v want first %s", n, got[i].EnvNames, wantEnv[n])
		}
		if n == "websocket" {
			t.Fatal("websocket must not be a catalog row")
		}
	}
	var slack, oa, disc Info
	for _, c := range got {
		switch c.Name {
		case "slack":
			slack = c
		case "zalo-oa":
			oa = c
		case "discord":
			disc = c
		}
	}
	if len(slack.EnvNames) != 2 || slack.EnvNames[1] != "GOSO_SLACK_APP_TOKEN" || slack.Health != "parked" || slack.Phase != 2 {
		t.Fatalf("slack %+v", slack)
	}
	if len(oa.EnvNames) != 3 || oa.EnvNames[1] != "GOSO_ZALO_OA_SECRET" {
		t.Fatalf("oa env %v", oa.EnvNames)
	}
	if disc.Health != "parked" {
		t.Fatalf("discord health %s", disc.Health)
	}
}

func TestCatalog_OAConfiguredIsRunning(t *testing.T) {
	t.Setenv("GOSO_LITE", "")
	t.Setenv("GOSO_ZALO_OA_ACCESS_TOKEN", "tok")
	t.Setenv("GOSO_ZALO_OA_SECRET", "sec")
	got := Catalog()
	for _, c := range got {
		if c.Name == "zalo-oa" {
			if !c.Configured || c.Health != "running" || c.Transport != "webhook" {
				t.Fatalf("oa %+v", c)
			}
			return
		}
	}
	t.Fatal("zalo-oa missing")
}

var wantEnv = map[string]string{
	"telegram":      "GOSO_TELEGRAM_BOT_TOKEN",
	"zalo-personal": "GOSO_ZALO_PERSONAL_TOKEN",
	"zalo-oa":       "GOSO_ZALO_OA_ACCESS_TOKEN",
	"discord":       "GOSO_DISCORD_BOT_TOKEN",
	"slack":         "GOSO_SLACK_BOT_TOKEN",
	"feishu":        "GOSO_FEISHU_APP_SECRET",
	"whatsapp":      "GOSO_WHATSAPP_ACCESS_TOKEN",
}

func TestCatalog_ConfiguredWhenEnvSet(t *testing.T) {
	t.Setenv("GOSO_DISCORD_BOT_TOKEN", "test-placeholder")
	t.Setenv("GOSO_TELEGRAM_BOT_TOKEN", "")
	t.Setenv("GOSO_ZALO_PERSONAL_TOKEN", "")
	t.Setenv("GOSO_ZALO_OA_ACCESS_TOKEN", "")
	t.Setenv("GOSO_SLACK_BOT_TOKEN", "")
	t.Setenv("GOSO_FEISHU_APP_SECRET", "")
	t.Setenv("GOSO_WHATSAPP_ACCESS_TOKEN", "")
	got := Catalog()
	var discord Info
	for _, c := range got {
		if c.Name == "discord" {
			discord = c
		}
		if c.Name == "telegram" && c.Configured {
			t.Fatal("telegram should be unconfigured")
		}
	}
	if !discord.Configured {
		t.Fatal("discord should be configured when env set")
	}
	if discord.Missing {
		t.Fatal("discord missing=true when env set")
	}
	if discord.Env != "GOSO_DISCORD_BOT_TOKEN" {
		t.Fatalf("discord env %q", discord.Env)
	}
	if len(discord.EnvNames) != 1 || discord.EnvNames[0] != "GOSO_DISCORD_BOT_TOKEN" {
		t.Fatalf("discord env_names %v", discord.EnvNames)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "test-placeholder") {
		t.Fatalf("catalog JSON leaked token value: %s", raw)
	}
}

func TestCatalog_TelegramBoxSetsConfigured(t *testing.T) {
	t.Setenv("GOSO_TELEGRAM_BOT_TOKEN", "")
	t.Setenv("GOSO_ZALO_OA_ACCESS_TOKEN", "")
	t.Setenv("GOSO_ZALO_OA_SECRET", "")
	st := store.New()
	key, err := secrets.RandomKeyHex()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOSO_MASTER_KEY", key)
	if err := secrets.Put(st, SecretName("telegram", KindBot), []byte("boxed-bot-token")); err != nil {
		t.Fatal(err)
	}
	got := CatalogWith(st, nil)
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "boxed-bot-token") {
		t.Fatalf("leaked box token: %s", raw)
	}
	for _, c := range got {
		if c.Name != "telegram" {
			continue
		}
		if !c.Configured || !c.SecretSet || c.FromEnv || c.Missing {
			t.Fatalf("telegram box %+v", c)
		}
		if len(c.Writable) != 1 || c.Writable[0] != "bot_token" {
			t.Fatalf("writable %v", c.Writable)
		}
		return
	}
	t.Fatal("telegram missing")
}

func TestKnown(t *testing.T) {
	if !Known("telegram") || !Known("whatsapp") {
		t.Fatal("catalog names should be known")
	}
	if Known("") || Known("sms") {
		t.Fatal("unknown names")
	}
}

func TestAdapterNames(t *testing.T) {
	if (&Discord{}).Name() != "discord" {
		t.Fatal("discord")
	}
	if (&Slack{}).Name() != "slack" {
		t.Fatal("slack")
	}
	if (&Feishu{}).Name() != "feishu" {
		t.Fatal("feishu")
	}
	if (&WhatsApp{}).Name() != "whatsapp" {
		t.Fatal("whatsapp")
	}
}
