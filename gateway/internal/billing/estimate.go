// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package billing

// EstimateTokens approximates token count as ceil(len(text)/4).
// Used when a provider does not return usage (SPEC 010 stub metering).
func EstimateTokens(text string) int {
	n := len(text)
	if n == 0 {
		return 0
	}
	return (n + 3) / 4
}
