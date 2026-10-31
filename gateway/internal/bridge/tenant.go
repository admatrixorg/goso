// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package bridge

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// MapTenantID maps an Omni tenant id onto a goso tenant id.
// Ids that already satisfy the goso tenant grammar (letters, digits, . _ -)
// are kept, so a UUID is stable and equal on both sides. Anything else is a
// stable sha256 prefix that does not collapse onto the default tenant.
func MapTenantID(omni string) string {
	omni = strings.TrimSpace(omni)
	if store.TenantOK(omni) {
		return omni
	}
	sum := sha256.Sum256([]byte("omni-tenant:" + omni))
	return "t_" + hex.EncodeToString(sum[:16])
}
