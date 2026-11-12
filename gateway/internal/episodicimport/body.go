// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package episodicimport

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

// CustomerPrefix is the customer_key written for a non-empty GoClaw user_id.
// This importer does not guess a channel contact. A later command remaps.
const CustomerPrefix = "goclaw:"

// CustomerKey maps a GoClaw user_id. Empty (after trim) is the only input
// that returns an empty key. Any other input returns goclaw:<user_id>.
func CustomerKey(userID string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ""
	}
	return CustomerPrefix + userID
}

// SessionID is gc-sess:<sha256> of the Goso agent id, a newline, and the
// GoClaw session_key. The hash input is those UTF-8 bytes and nothing else.
func SessionID(gosoAgent, sessionKey string) string {
	sum := sha256.Sum256([]byte(gosoAgent + "\n" + sessionKey))
	return "gc-sess:" + hex.EncodeToString(sum[:])
}

// MemoryID is gc-ep:<lowercase source uuid>.
func MemoryID(sourceID string) string {
	return "gc-ep:" + strings.ToLower(strings.TrimSpace(sourceID))
}

// Body is the summary. When key_topics has a non-empty value, one
// "Topics: a, b" line is added. A whitespace-only summary contributes
// nothing. Dropped columns are not appended.
func Body(summary string, topics []string) string {
	var kept []string
	for _, t := range topics {
		t = strings.TrimSpace(t)
		if t != "" {
			kept = append(kept, t)
		}
	}
	summary = strings.TrimRight(summary, "\n")
	if len(kept) == 0 {
		if strings.TrimSpace(summary) == "" {
			return ""
		}
		return summary
	}
	line := "Topics: " + strings.Join(kept, ", ")
	if strings.TrimSpace(summary) == "" {
		return line
	}
	return summary + "\n" + line
}

// FormatTime stores created_at as UTC RFC3339Nano.
func FormatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
