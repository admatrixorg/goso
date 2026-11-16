// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"os"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/store"
)

const (
	kindSession   = "session"
	KindBot       = "bot"
	KindAccess    = "access"
	KindAppSecret = "secret"
	// KindBridge is the key ZaloCRM signs a bridged Zalo channel's calls with.
	KindBridge = "bridge"
)

// SecretName is the AES-GCM box key channel:<name>:<kind>.
func SecretName(name, kind string) string {
	return "channel:" + strings.TrimSpace(name) + ":" + strings.TrimSpace(kind)
}

// WritableFields are PUT /secrets JSON keys for a channel: a catalog name or
// a tenant's instance. Empty = no UI form. A tenant's Zalo personal channel
// takes the key ZaloCRM bridges its messages with, and a tenant's Lark channel
// its app secret (the app id is a setting).
func WritableFields(name string) []string {
	_, owned := TenantOf(name)
	switch CatalogOf(name) {
	case "telegram":
		return []string{"bot_token"}
	case "zalo-oa":
		if owned {
			// A tenant's zalo-oa channel is a Zalo Bot platform bot.
			return []string{"bot_token"}
		}
		return []string{"access_token", "app_secret"}
	case "zalo-personal":
		if owned {
			return []string{"bridge_secret"}
		}
		return nil
	case "feishu":
		if owned {
			// The app id is a setting; only the app secret is secret.
			return []string{"app_secret"}
		}
		return nil
	default:
		return nil
	}
}

// FieldKind maps a PUT JSON key to the box kind for a channel.
func FieldKind(name, field string) (kind string, ok bool) {
	field = strings.ToLower(strings.TrimSpace(field))
	_, owned := TenantOf(name)
	switch CatalogOf(name) {
	case "telegram":
		if field == "bot_token" {
			return KindBot, true
		}
	case "zalo-oa":
		if owned {
			if field == "bot_token" {
				return KindBot, true
			}
			return "", false
		}
		if field == "access_token" {
			return KindAccess, true
		}
		if field == "app_secret" {
			return KindAppSecret, true
		}
	case "zalo-personal":
		if owned && field == "bridge_secret" {
			return KindBridge, true
		}
	case "feishu":
		if owned && field == "app_secret" {
			return KindAppSecret, true
		}
	}
	return "", false
}

// EnvFirst returns the first non-empty process env among names.
func EnvFirst(names ...string) string {
	for _, n := range names {
		if v := strings.TrimSpace(os.Getenv(n)); v != "" {
			return v
		}
	}
	return ""
}

// Credential reports env-or-box secret presence. Env wins if set.
func Credential(st store.StoreIface, name, kind string, envNames []string) (value string, fromEnv, set bool) {
	if v := EnvFirst(envNames...); v != "" {
		return v, true, true
	}
	if st == nil {
		return "", false, false
	}
	raw, err := secrets.Get(st, SecretName(name, kind))
	if err != nil || len(raw) == 0 {
		return "", false, false
	}
	return string(raw), false, true
}

// SecretSet is true when env or encrypted box has a credential.
func SecretSet(st store.StoreIface, name, kind string, envNames []string) bool {
	_, _, set := Credential(st, name, kind, envNames)
	return set
}
