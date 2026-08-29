// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import "os"

// Info is one row of GET /api/channels. Names are always listed; configured
// is true only when every required env token is non-empty. Env / EnvNames are
// variable NAMES only (never secret values). Missing is true when any required
// env is empty.
type Info struct {
	Name            string   `json:"name"`
	Configured      bool     `json:"configured"`
	Missing         bool     `json:"missing"`
	Env             string   `json:"env"`
	EnvNames        []string `json:"env_names"`
	Health          string   `json:"health,omitempty"`
	Transport       string   `json:"transport,omitempty"`
	SecretSet       bool     `json:"secret_set,omitempty"`
	BoundAgentID    string   `json:"bound_agent_id,omitempty"`
	DMPolicy        string   `json:"dm_policy,omitempty"`
	GroupPolicy     string   `json:"group_policy,omitempty"`
	RequireMention  bool     `json:"require_mention,omitempty"`
	AllowFrom       []string `json:"allow_from,omitempty"`
	AllowFromCount  int      `json:"allow_from_count,omitempty"`
	Phase           int      `json:"phase,omitempty"`
	LastError       string   `json:"last_error,omitempty"`
	Enabled         bool     `json:"enabled,omitempty"`
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

// requiredEnv is the public required-env help list per adapter. Adapters read
// these names; live values stay in the process environment (DI-01..07 parked).
var requiredEnv = map[string][]string{
	"telegram":      {"GOSO_TELEGRAM_BOT_TOKEN"},
	"zalo-personal": {"GOSO_ZALO_PERSONAL_TOKEN"},
	"zalo-oa":       {"GOSO_ZALO_OA_ACCESS_TOKEN"},
	"discord":       {"GOSO_DISCORD_BOT_TOKEN"},
	"slack":         {"GOSO_SLACK_BOT_TOKEN"},
	"feishu":        {"GOSO_FEISHU_APP_SECRET"},
	"whatsapp":      {"GOSO_WHATSAPP_ACCESS_TOKEN"},
}

// Known reports whether name is one of the seven catalog adapters.
func Known(name string) bool {
	for _, n := range Names {
		if n == name {
			return true
		}
	}
	return false
}

// Catalog returns all 7 channel names with configured/missing flags and env var names.
func Catalog() []Info {
	out := make([]Info, 0, len(Names))
	for _, n := range Names {
		names := append([]string(nil), requiredEnv[n]...)
		if names == nil {
			names = []string{}
		}
		configured := len(names) > 0
		for _, env := range names {
			if os.Getenv(env) == "" {
				configured = false
				break
			}
		}
		env := ""
		if len(names) > 0 {
			env = names[0]
		}
		out = append(out, Info{
			Name:       n,
			Configured: configured,
			Missing:    !configured,
			Env:        env,
			EnvNames:   names,
		})
	}
	return out
}
