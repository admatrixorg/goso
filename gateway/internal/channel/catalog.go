// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"os"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// Info is one row of GET /api/channels. Names are always listed; configured
// is true only when every required env token is non-empty. Env / EnvNames are
// variable NAMES only (never secret values). Missing is true when any required
// env is empty.
type Info struct {
	Name           string   `json:"name"`
	Configured     bool     `json:"configured"`
	Missing        bool     `json:"missing"`
	Env            string   `json:"env"`
	EnvNames       []string `json:"env_names"`
	Health         string   `json:"health,omitempty"`
	Transport      string   `json:"transport,omitempty"`
	SecretSet      bool     `json:"secret_set,omitempty"`
	FromEnv        bool     `json:"from_env,omitempty"`
	Writable       []string `json:"writable,omitempty"`
	BoundAgentID   string   `json:"bound_agent_id,omitempty"`
	DMPolicy       string   `json:"dm_policy,omitempty"`
	GroupPolicy    string   `json:"group_policy,omitempty"`
	RequireMention bool     `json:"require_mention,omitempty"`
	AllowFrom      []string `json:"allow_from,omitempty"`
	AllowFromCount int      `json:"allow_from_count,omitempty"`
	Phase          int      `json:"phase,omitempty"`
	LastError      string   `json:"last_error,omitempty"`
	Enabled        bool     `json:"enabled,omitempty"`
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

// requiredEnv must all be non-empty for configured=true (except zalo-personal box).
var requiredEnv = map[string][]string{
	"telegram":      {"GOSO_TELEGRAM_BOT_TOKEN"},
	"zalo-personal": {"GOSO_ZALO_PERSONAL_TOKEN"},
	"zalo-oa":       {"GOSO_ZALO_OA_ACCESS_TOKEN", "GOSO_ZALO_OA_SECRET"},
	"discord":       {"GOSO_DISCORD_BOT_TOKEN"},
	"slack":         {"GOSO_SLACK_BOT_TOKEN", "GOSO_SLACK_APP_TOKEN"},
	"feishu":        {"GOSO_FEISHU_APP_SECRET"},
	"whatsapp":      {"GOSO_WHATSAPP_ACCESS_TOKEN"},
}

var extraEnv = map[string][]string{
	"zalo-oa": {"GOSO_ZALO_OA_APP_ID"},
}

var phase2 = map[string]bool{
	"discord": true, "slack": true, "feishu": true, "whatsapp": true,
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

func helpEnvNames(n string) []string {
	names := append([]string(nil), requiredEnv[n]...)
	names = append(names, extraEnv[n]...)
	return names
}

func requiredFilled(n string) bool {
	for _, env := range requiredEnv[n] {
		if strings.TrimSpace(os.Getenv(env)) == "" {
			return false
		}
	}
	return len(requiredEnv[n]) > 0
}

func defaultTransport(n string) string {
	switch n {
	case "telegram":
		mode := strings.ToLower(strings.TrimSpace(os.Getenv("GOSO_TELEGRAM_MODE")))
		if mode == "webhook" {
			return "webhook"
		}
		return "poll"
	case "zalo-oa":
		return "webhook"
	case "zalo-personal":
		return "sidecar"
	default:
		return "none"
	}
}

func phaseOf(n string) int {
	if phase2[n] {
		return 2
	}
	return 1
}

// Catalog returns all 7 channel names with configured/missing flags and env var names.
func Catalog() []Info {
	return CatalogWith(nil, nil)
}

// CatalogWith overlays store credentials and manager health (SPEC 084).
func CatalogWith(st store.StoreIface, mgr *Manager) []Info {
	out := make([]Info, 0, len(Names))
	for _, n := range Names {
		names := helpEnvNames(n)
		configured := requiredFilled(n)
		secretSet := configured
		fromEnv := configured
		switch n {
		case "telegram":
			_, fromEnv, secretSet = Credential(st, n, KindBot, requiredEnv[n])
			configured = secretSet
		case "zalo-oa":
			_, aEnv, aSet := Credential(st, n, KindAccess, []string{"GOSO_ZALO_OA_ACCESS_TOKEN"})
			_, sEnv, sSet := Credential(st, n, KindAppSecret, []string{"GOSO_ZALO_OA_SECRET"})
			secretSet = aSet && sSet
			configured = secretSet
			fromEnv = (aEnv && aSet) || (sEnv && sSet)
		case "zalo-personal":
			_, fromEnv, secretSet = Credential(st, n, kindSession, requiredEnv[n])
			configured = secretSet
		}
		env := ""
		if len(names) > 0 {
			env = names[0]
		}
		row := Info{
			Name:       n,
			Configured: configured,
			Missing:    !configured,
			Env:        env,
			EnvNames:   names,
			SecretSet:  secretSet,
			FromEnv:    fromEnv && secretSet,
			Writable:   WritableFields(n),
			Phase:      phaseOf(n),
			Transport:  defaultTransport(n),
		}
		row.Health = deriveHealth(row, mgr)
		if mgr != nil {
			if e := mgr.LastError(n); e != "" {
				row.LastError = redactErr(e)
				if row.Health != "parked" && row.Health != "missing" {
					row.Health = "failed"
				}
			}
			if t := mgr.Transport(n); t != "" {
				row.Transport = t
			}
		}
		out = append(out, row)
	}
	return out
}

func deriveHealth(row Info, mgr *Manager) string {
	if row.Phase == 2 {
		return "parked"
	}
	if store.LiteEnabled() {
		if row.Missing {
			return "missing"
		}
		return "stopped"
	}
	if row.Missing {
		return "missing"
	}
	// Webhook/sidecar inject paths are always mounted for phase-1 OA/Personal.
	if row.Name == "zalo-oa" || row.Name == "zalo-personal" {
		return "running"
	}
	if mgr != nil && mgr.Running(row.Name) {
		return "running"
	}
	if mgr != nil && mgr.Failed(row.Name) {
		return "failed"
	}
	return "stopped"
}

func redactErr(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	for _, n := range []string{
		os.Getenv("GOSO_TELEGRAM_BOT_TOKEN"),
		os.Getenv("GOSO_TELEGRAM_WEBHOOK_SECRET"),
		os.Getenv("GOSO_ZALO_OA_ACCESS_TOKEN"),
		os.Getenv("GOSO_ZALO_OA_SECRET"),
		os.Getenv("GOSO_ZALO_PERSONAL_TOKEN"),
	} {
		if n != "" && strings.Contains(s, n) {
			s = strings.ReplaceAll(s, n, "[redacted]")
		}
	}
	return s
}
