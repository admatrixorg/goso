// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package security

import "strings"

const (
	// UntrustedBegin marks the start of tool output sent back to the LLM.
	UntrustedBegin = "GOSO_UNTRUSTED_BEGIN"
	// UntrustedEnd marks the end of tool output sent back to the LLM.
	UntrustedEnd = "GOSO_UNTRUSTED_END"
)

// WrapUntrusted wraps connector/tool output so the model treats it as data.
func WrapUntrusted(s string) string {
	if strings.Contains(s, UntrustedBegin) && strings.Contains(s, UntrustedEnd) {
		return s
	}
	return UntrustedBegin + "\n" + s + "\n" + UntrustedEnd
}
