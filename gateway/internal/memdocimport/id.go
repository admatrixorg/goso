// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package memdocimport

import (
	"crypto/sha256"
	"encoding/hex"
)

// Identity hashes are sha256, hex encoded. Parts are joined with a NUL byte
// so a boundary inside one part cannot collide with the next part.
// The agent argument is the mapped Goso agent id.

// SessionID is the archived session for one Goso agent and one GoClaw user_id.
// Format: gc-docsess:<sha(agent, user)>.
func SessionID(agent, user string) string {
	return "gc-docsess:" + chainHash(agent, user)
}

// MemoryID is the durable note id.
// Format: gc-doc:<agent>:<sha(user, path)>. path is the cleaned source path.
func MemoryID(agent, user, path string) string {
	return "gc-doc:" + agent + ":" + chainHash(user, path)
}

// VaultID is the vault_docs id for one tenant, agent, and logical path.
// Format: gc-vdoc:<sha(tenant, agent, logical)>.
func VaultID(tenant, agent, logical string) string {
	return "gc-vdoc:" + chainHash(tenant, agent, logical)
}

func chainHash(parts ...string) string {
	h := sha256.New()
	for i, p := range parts {
		if i > 0 {
			_, _ = h.Write([]byte{0})
		}
		_, _ = h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
