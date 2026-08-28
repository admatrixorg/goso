// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package security

import (
	"fmt"
	"os"
	"strings"
)

// Production is GOSO_ENV=production (case-insensitive). Demo and unset are not.
func Production() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("GOSO_ENV")), "production")
}

// CheckProduction refuses start when production is missing a WS origin allowlist.
// Demo / unset GOSO_ENV still boot with empty GOSO_WS_ORIGINS (allow-all).
func CheckProduction() error {
	if !Production() {
		return nil
	}
	if strings.TrimSpace(os.Getenv("GOSO_WS_ORIGINS")) == "" {
		return fmt.Errorf("GOSO_WS_ORIGINS is required when GOSO_ENV=production")
	}
	return nil
}
