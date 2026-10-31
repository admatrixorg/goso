// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"net/http"
	"time"

	"github.com/mqglobal/goso/gateway/internal/security"
)

func guardedClient(existing *http.Client, timeout time.Duration) *http.Client {
	return clientWithTimeout(existing, timeout, false)
}

// clientWithTimeout returns a client with Timeout set to timeout.
// raise=false leaves a non-nil client unchanged (probe). raise=true clones a
// preset client so chat uses GOSO_LLM_TIMEOUT instead of the 20s probe client.
func clientWithTimeout(existing *http.Client, timeout time.Duration, raise bool) *http.Client {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	if existing == nil {
		c := &http.Client{Timeout: timeout}
		security.GuardClient(c)
		return c
	}
	if !raise || existing.Timeout == timeout {
		security.GuardClient(existing)
		return existing
	}
	clone := *existing
	clone.Timeout = timeout
	security.GuardClient(&clone)
	return &clone
}

func checkEndpoint(raw string) error {
	return security.CheckURL(raw)
}
