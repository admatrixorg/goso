// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import "testing"

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
	}
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
