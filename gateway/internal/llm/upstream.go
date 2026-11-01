// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"fmt"
	"strings"
)

// UpstreamError is a non-2xx response from an LLM provider.
// Detail is a short provider message; it must not include API keys.
type UpstreamError struct {
	Provider string
	Status   int
	Detail   string
}

func (e *UpstreamError) Error() string {
	if e == nil {
		return "upstream error"
	}
	name := e.Provider
	if name == "" {
		name = "llm"
	}
	if e.Detail == "" {
		return fmt.Sprintf("%s upstream %d", name, e.Status)
	}
	return fmt.Sprintf("%s upstream %d: %s", name, e.Status, e.Detail)
}

func clipDetail(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	if len(body) > 180 {
		body = body[:180]
	}
	return body
}
