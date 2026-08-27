// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package security

import "crypto/subtle"

// Equal is a constant-time string compare for Bearer tokens.
func Equal(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
