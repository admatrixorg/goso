// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package security

import (
	"os"
	"strings"
)

// Documented injection substrings scanned in user chat text (case-insensitive).
// Six goso-owned phrases: keep the original four; add role-override and delimiter-escape.
var injectionPatterns = []string{
	"ignore previous instructions",
	"exfiltrate system prompt",
	"drop table",
	"dump credentials",
	"you are now",
	"end of system",
}

const (
	// InjectionLog records a match and allows the request (default).
	InjectionLog = "log"
	// InjectionBlock records a match and rejects the request.
	InjectionBlock = "block"
)

// InjectionMode is GOSO_INJECTION: log or block.
// Production default is block when unset. Dev/demo default is log.
func InjectionMode() string {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("GOSO_INJECTION")))
	if v == InjectionBlock {
		return InjectionBlock
	}
	if v == InjectionLog {
		return InjectionLog
	}
	if Production() {
		return InjectionBlock
	}
	return InjectionLog
}

// ScanInjection returns the first documented pattern found in text.
func ScanInjection(text string) string {
	low := strings.ToLower(text)
	for _, p := range injectionPatterns {
		if strings.Contains(low, p) {
			return p
		}
	}
	return ""
}

// InspectChat reports a matched pattern and whether the request must be blocked.
func InspectChat(text string) (matched string, block bool) {
	matched = ScanInjection(text)
	if matched == "" {
		return "", false
	}
	return matched, InjectionMode() == InjectionBlock
}
