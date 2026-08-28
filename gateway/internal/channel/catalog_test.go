// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCatalog_SevenNamesUnconfigured(t *testing.T) {
	t.Setenv("GOSO_TELEGRAM_BOT_TOKEN", "")
	t.Setenv("GOSO_ZALO_PERSONAL_TOKEN", "")
	t.Setenv("GOSO_ZALO_OA_ACCESS_TOKEN", "")
	t.Setenv("GOSO_DISCORD_BOT_TOKEN", "")
	t.Setenv("GOSO_SLACK_BOT_TOKEN", "")
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
		if len(got[i].EnvNames) != 1 || got[i].EnvNames[0] != wantEnv[n] {
			t.Fatalf("%s env_names %v want [%s]", n, got[i].EnvNames, wantEnv[n])
		}
	}
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
