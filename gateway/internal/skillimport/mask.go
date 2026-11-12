// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package skillimport

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
)

// phoneLike matches a phone number or a long digit id. It does not match a
// UUID, because a UUID is split by hyphens and contains letters.
var phoneLike = regexp.MustCompile(`(?:\+|00)?(?:84|0)\d{8,12}|\b\d{8,15}\b`)

// MaskIdentifiers replaces phone numbers and long digit ids with a short hash.
// The same input always yields the same token. The raw value is not kept.
func MaskIdentifiers(s string) string {
	return phoneLike.ReplaceAllStringFunc(s, func(m string) string {
		sum := sha256.Sum256([]byte(m))
		return "id-" + hex.EncodeToString(sum[:4])
	})
}
