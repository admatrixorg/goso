// Copyright (c) 2026 MQ Global — GOSO Desktop. Clean-room implementation.

package host

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const tokenFileName = "admin.token"

// secret is an admin token that redacts itself in fmt/log output (SPEC 024: never log it).
type secret string

func (s secret) String() string   { return "[redacted]" }
func (s secret) GoString() string { return "secret([redacted])" }

func envTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// DefaultTokenPath is the file that holds the generated local admin token.
// GOSO_ADMIN_TOKEN_PATH overrides. Otherwise it sits next to the SQLite file
// (Application Support/GOSO/admin.token on macOS).
func DefaultTokenPath() string {
	if p := strings.TrimSpace(os.Getenv("GOSO_ADMIN_TOKEN_PATH")); p != "" {
		return p
	}
	if p := os.Getenv("GOSO_DB_PATH"); p != "" && p != ":memory:" {
		return filepath.Join(filepath.Dir(p), tokenFileName)
	}
	return filepath.Join(appSupportDir(), tokenFileName)
}

// ResolveAdminToken returns the token the embedded gateway should require.
// Priority: GOSO_ADMIN_TOKEN env → GOSO_DEV_MODE passthrough (empty) → load/create file.
// The token value is never written to logs.
func ResolveAdminToken() (string, error) {
	if t := strings.TrimSpace(os.Getenv("GOSO_ADMIN_TOKEN")); t != "" {
		return t, nil
	}
	if envTruthy(os.Getenv("GOSO_DEV_MODE")) {
		return "", nil
	}
	return loadOrCreateToken(DefaultTokenPath())
}

func loadOrCreateToken(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("admin token path is empty")
	}
	if b, err := os.ReadFile(path); err == nil {
		if t := strings.TrimSpace(string(b)); t != "" {
			return t, nil
		}
	}
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("generate admin token: %w", err)
	}
	t := hex.EncodeToString(buf[:])
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", fmt.Errorf("admin token dir: %w", err)
		}
	}
	if err := os.WriteFile(path, []byte(t+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write admin token file: %w", err)
	}
	return t, nil
}
