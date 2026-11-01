// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"fmt"
	"strings"
)

// Customer scope for L1 memory.
//
// One channel agent (zalo-personal, zalo-oa, telegram, …) serves every
// customer, so agent-scoped recall would carry customer A's notes into
// customer B's replies. Each channel thread gets its own session labelled
// "<channel>:<thread or user id>" (channel/*.go ensureSession), so that label
// is the customer key. It is stamped on the note when the note is written
// (memories.customer_key) and not read back from the session at recall time,
// because operators can rename a session label.

// ChannelLabelPrefixes mark sessions created by external channel threads.
var ChannelLabelPrefixes = []string{
	"zalo-personal:", "zalo-oa:", "telegram:", "discord:", "slack:", "feishu:", "whatsapp:",
}

// IsChannelLabel reports a session label owned by an external channel thread.
func IsChannelLabel(label string) bool {
	for _, p := range ChannelLabelPrefixes {
		if strings.HasPrefix(label, p) {
			return true
		}
	}
	return false
}

// CustomerKeyForLabel returns the customer key of a channel session label
// (the label itself, e.g. "zalo-personal:123"), or "" for web/API sessions
// and for a channel label with no thread id. Exact, case-sensitive match so it
// agrees with the SQL backfill.
func CustomerKeyForLabel(label string) string {
	for _, p := range ChannelLabelPrefixes {
		if strings.HasPrefix(label, p) && len(label) > len(p) {
			return label
		}
	}
	return ""
}

// customerKeyBackfillSQL stamps customer_key on legacy notes whose session is
// a channel thread. Deterministic (key = session label, same rule as
// CustomerKeyForLabel) and idempotent (only rows with an empty key). Notes of
// sessions whose label no longer has a channel prefix stay unscoped, and
// unscoped notes are never recalled into channel sessions.
func customerKeyBackfillSQL() string {
	conds := make([]string, 0, len(ChannelLabelPrefixes))
	for _, p := range ChannelLabelPrefixes {
		if strings.ContainsAny(p, `'\`) {
			panic("channel prefix must not contain quotes")
		}
		conds = append(conds, fmt.Sprintf("(substr(s.label, 1, %d) = '%s' AND length(s.label) > %d)", len(p), p, len(p)))
	}
	return `UPDATE memories SET customer_key = (SELECT s.label FROM sessions s WHERE s.id = memories.session_id)
		WHERE customer_key = '' AND EXISTS (SELECT 1 FROM sessions s WHERE s.id = memories.session_id AND (` +
		strings.Join(conds, " OR ") + `))`
}
