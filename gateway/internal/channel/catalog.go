// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import "os"

// Info is one row of GET /api/channels. Names are always listed; configured
// is true only when the matching env token is non-empty (never the secret).
type Info struct {
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
}

// Names is the fixed 7-channel catalog (C0).
var Names = []string{
	"telegram",
	"zalo-personal",
	"zalo-oa",
	"discord",
	"slack",
	"feishu",
	"whatsapp",
}

var tokenEnv = map[string]string{
	"telegram":      "GOSO_TELEGRAM_BOT_TOKEN",
	"zalo-personal": "GOSO_ZALO_PERSONAL_TOKEN",
	"zalo-oa":       "GOSO_ZALO_OA_ACCESS_TOKEN",
	"discord":       "GOSO_DISCORD_BOT_TOKEN",
	"slack":         "GOSO_SLACK_BOT_TOKEN",
	"feishu":        "GOSO_FEISHU_APP_SECRET",
	"whatsapp":      "GOSO_WHATSAPP_ACCESS_TOKEN",
}

// Catalog returns all 7 channel names with configured flags from env.
func Catalog() []Info {
	out := make([]Info, 0, len(Names))
	for _, n := range Names {
		out = append(out, Info{Name: n, Configured: os.Getenv(tokenEnv[n]) != ""})
	}
	return out
}
