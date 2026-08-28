// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"net/http"
	"time"

	"github.com/mqglobal/goso/gateway/internal/security"
)

func guardedClient(existing *http.Client, timeout time.Duration) *http.Client {
	c := existing
	if c == nil {
		c = &http.Client{Timeout: timeout}
	}
	security.GuardClient(c)
	return c
}

func checkEndpoint(raw string) error {
	return security.CheckURL(raw)
}
