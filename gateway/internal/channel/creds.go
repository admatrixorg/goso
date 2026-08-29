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
)

// SecretName is the AES-GCM box key channel:<name>:<kind>.
func SecretName(name, kind string) string {
	return "channel:" + strings.TrimSpace(name) + ":" + strings.TrimSpace(kind)
}

// WritableFields are PUT /secrets JSON keys for a catalog name. Empty = no UI form.
func WritableFields(name string) []string {
	switch name {
	case "telegram":
		return []string{"bot_token"}
	case "zalo-oa":
		return []string{"access_token", "app_secret"}
	default:
		return nil
	}
}

// FieldKind maps a PUT JSON key to the box kind for name.
func FieldKind(name, field string) (kind string, ok bool) {
	field = strings.ToLower(strings.TrimSpace(field))
	switch name {
	case "telegram":
		if field == "bot_token" {
			return KindBot, true
		}
	case "zalo-oa":
		if field == "access_token" {
			return KindAccess, true
		}
		if field == "app_secret" {
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
