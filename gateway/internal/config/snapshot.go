// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package config

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"
)

// Field is one operator-visible config cell. Secrets use boolean set flags only.
type Field struct {
	Key      string `json:"key"`
	Value    any    `json:"value,omitempty"`
	Set      bool   `json:"set"`
	EnvOwned bool   `json:"env_owned"`
	Editable bool   `json:"editable"`
}

// Snapshot is GET /api/config. Auth tokens and DSNs never appear as values.
type Snapshot struct {
	UpdatedAt    string          `json:"updated_at"`
	Server       ServerView      `json:"server"`
	Auth         AuthView        `json:"auth"`
	Behavior     BehaviorView    `json:"behavior"`
	Quota        QuotaView       `json:"quota"`
	Tools        ToolsView       `json:"tools"`
	Integrations IntegrationView `json:"integrations"`
}

type ServerView struct {
	Port     Field `json:"port"`
	Host     Field `json:"host"`
	Env      Field `json:"env"`
	LogLevel Field `json:"log_level"`
}

type AuthView struct {
	TokenSet     Field `json:"token_set"`
	ViewTokenSet Field `json:"view_token_set"`
	MasterKeySet Field `json:"master_key_set"`
}

type BehaviorView struct {
	ContextDir           Field `json:"context_dir"`
	Workspace            Field `json:"workspace"`
	KGExtract            Field `json:"kg_extract"`
	CacheMode            Field `json:"cache_mode"`
	Heartbeat            Field `json:"heartbeat"`
	HeartbeatIntervalSec Field `json:"heartbeat_interval_sec"`
}

type QuotaView struct {
	DayLimit Field `json:"day_limit"`
	Enabled  Field `json:"enabled"`
}

type ToolsView struct {
	Injection Field `json:"injection"`
	SSRF      Field `json:"ssrf"`
}

type IntegrationView struct {
	OTELSet        Field `json:"otel_set"`
	DatabaseURLSet Field `json:"database_url_set"`
	MultiTenant    Field `json:"multi_tenant"`
	SkillsDir      Field `json:"skills_dir"`
	VaultDir       Field `json:"vault_dir"`
}

func boolField(key, envKey string, truthy bool) Field {
	owned := EnvOwned(envKey)
	return Field{Key: key, Value: truthy, Set: truthy, EnvOwned: owned, Editable: false}
}

func stringField(key, envKey, value string, editable bool) Field {
	owned := EnvOwned(envKey)
	v := strings.TrimSpace(value)
	return Field{
		Key:      key,
		Value:    omitEmpty(v),
		Set:      v != "",
		EnvOwned: owned,
		Editable: editable && !owned,
	}
}

func omitEmpty(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func envTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func ssrfEffective() string {
	v := strings.ToLower(Lookup("GOSO_SSRF"))
	if v == "0" || v == "false" || v == "off" || v == "no" {
		return "off"
	}
	if v == "1" || v == "true" || v == "yes" || v == "on" {
		return "on"
	}
	if strings.EqualFold(Lookup("GOSO_ENV"), "production") {
		return "on"
	}
	return "off"
}

func injectionEffective() string {
	v := strings.ToLower(Lookup("GOSO_INJECTION"))
	if v == "block" || v == "log" {
		return v
	}
	if strings.EqualFold(Lookup("GOSO_ENV"), "production") {
		return "block"
	}
	return "log"
}

func cacheEffective() string {
	v := strings.TrimSpace(Lookup("GOSO_ANTHROPIC_CACHE_MODE"))
	if v == "" {
		v = strings.TrimSpace(os.Getenv("GOSO_PROMPT_CACHE"))
	}
	return v
}

func quotaDay() int {
	v := strings.TrimSpace(Lookup("GOSO_QUOTA_DAY"))
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func heartbeatIntervalSec() int {
	raw := strings.TrimSpace(os.Getenv("GOSO_HEARTBEAT_INTERVAL_SEC"))
	if raw == "" {
		return 60
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 60
	}
	if n < 30 {
		return 30
	}
	return n
}

func formatStamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// BuildSnapshot is the public GET payload. updatedAt is the overlay stamp.
func BuildSnapshot(updatedAt time.Time) Snapshot {
	cfg := Load()
	host := strings.TrimSpace(os.Getenv("GOSO_HOST"))
	quota := quotaDay()
	cache := cacheEffective()
	return Snapshot{
		UpdatedAt: formatStamp(updatedAt),
		Server: ServerView{
			Port:     Field{Key: "port", Value: cfg.Port, Set: true, EnvOwned: EnvOwned("GOSO_PORT"), Editable: false},
			Host:     stringField("host", "GOSO_HOST", host, false),
			Env:      stringField("env", "GOSO_ENV", cfg.Env, false),
			LogLevel: stringField("log_level", "GOSO_LOG_LEVEL", cfg.LogLevel, true),
		},
		Auth: AuthView{
			TokenSet:     boolField("token_set", "GOSO_ADMIN_TOKEN", strings.TrimSpace(os.Getenv("GOSO_ADMIN_TOKEN")) != ""),
			ViewTokenSet: boolField("view_token_set", "GOSO_VIEW_TOKEN", strings.TrimSpace(os.Getenv("GOSO_VIEW_TOKEN")) != ""),
			MasterKeySet: boolField("master_key_set", "GOSO_MASTER_KEY", strings.TrimSpace(os.Getenv("GOSO_MASTER_KEY")) != ""),
		},
		Behavior: BehaviorView{
			ContextDir:           stringField("context_dir", "GOSO_CONTEXT_DIR", os.Getenv("GOSO_CONTEXT_DIR"), false),
			Workspace:            stringField("workspace", "GOSO_WORKSPACE", os.Getenv("GOSO_WORKSPACE"), false),
			KGExtract:            stringField("kg_extract", "GOSO_KG_EXTRACT", onOff(envTruthy(Lookup("GOSO_KG_EXTRACT"))), true),
			CacheMode:            stringField("cache_mode", "GOSO_ANTHROPIC_CACHE_MODE", cache, true),
			Heartbeat:            stringField("heartbeat", "GOSO_HEARTBEAT", onOff(envTruthy(Lookup("GOSO_HEARTBEAT"))), true),
			HeartbeatIntervalSec: Field{Key: "heartbeat_interval_sec", Value: heartbeatIntervalSec(), Set: EnvOwned("GOSO_HEARTBEAT_INTERVAL_SEC"), EnvOwned: EnvOwned("GOSO_HEARTBEAT_INTERVAL_SEC"), Editable: false},
		},
		Quota: QuotaView{
			DayLimit: stringField("day_limit", "GOSO_QUOTA_DAY", strconv.Itoa(quota), true),
			Enabled:  Field{Key: "enabled", Value: quota > 0, Set: quota > 0, EnvOwned: EnvOwned("GOSO_QUOTA_DAY"), Editable: false},
		},
		Tools: ToolsView{
			Injection: stringField("injection", "GOSO_INJECTION", injectionEffective(), true),
			SSRF:      stringField("ssrf", "GOSO_SSRF", ssrfEffective(), true),
		},
		Integrations: IntegrationView{
			OTELSet:        boolField("otel_set", "GOSO_OTEL_ENDPOINT", strings.TrimSpace(os.Getenv("GOSO_OTEL_ENDPOINT")) != ""),
			DatabaseURLSet: boolField("database_url_set", "GOSO_DATABASE_URL", strings.TrimSpace(os.Getenv("GOSO_DATABASE_URL")) != ""),
			MultiTenant:    boolField("multi_tenant", "GOSO_MULTI_TENANT", envTruthy(os.Getenv("GOSO_MULTI_TENANT"))),
			SkillsDir:      stringField("skills_dir", "GOSO_SKILLS_DIR", os.Getenv("GOSO_SKILLS_DIR"), false),
			VaultDir:       stringField("vault_dir", "GOSO_VAULT_DIR", os.Getenv("GOSO_VAULT_DIR"), false),
		},
	}
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

var secretEnvKeys = []string{
	"GOSO_ADMIN_TOKEN",
	"GOSO_VIEW_TOKEN",
	"GOSO_MASTER_KEY",
	"GOSO_DATABASE_URL",
}

// ContainsSecret reports whether encoded JSON leaked a live env secret.
func ContainsSecret(raw []byte) bool {
	s := string(raw)
	for _, key := range secretEnvKeys {
		v := strings.TrimSpace(os.Getenv(key))
		if len(v) >= 4 && strings.Contains(s, v) {
			return true
		}
	}
	low := strings.ToLower(s)
	if strings.Contains(low, `"token"`) || strings.Contains(low, `"admin_token"`) || strings.Contains(low, `"api_key"`) {
		return true
	}
	return false
}

// MarshalPublic encodes a snapshot and refuses to emit secret values.
func MarshalPublic(snap Snapshot) ([]byte, error) {
	b, err := json.Marshal(snap)
	if err != nil {
		return nil, err
	}
	if ContainsSecret(b) {
		return nil, errSecret
	}
	return b, nil
}

var errSecret = errString("config snapshot contained a secret")

type errString string

func (e errString) Error() string { return string(e) }
