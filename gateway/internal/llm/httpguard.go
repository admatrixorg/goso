// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"net"
	"net/http"
	"time"

	"github.com/mqglobal/goso/gateway/internal/security"
)

const (
	llmDialTimeout = 10 * time.Second
	llmTLSTimeout  = 10 * time.Second
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

// chatClient is the goso→provider client for a chat or stream call.
// It has no overall Timeout (that cut long 9router streams at the old budget).
// ResponseHeaderTimeout bounds the wait for status/headers. Dial and TLS
// keep their own deadlines. The request context cancels the body.
func chatClient(existing *http.Client) *http.Client {
	header := ResponseHeaderTimeout()
	var c http.Client
	if existing != nil {
		c = *existing
	}
	c.Timeout = 0
	switch tr := c.Transport.(type) {
	case nil:
		c.Transport = defaultChatTransport(header)
	case *http.Transport:
		clone := tr.Clone()
		clone.ResponseHeaderTimeout = header
		if clone.TLSHandshakeTimeout == 0 {
			clone.TLSHandshakeTimeout = llmTLSTimeout
		}
		if clone.DialContext == nil && clone.Dial == nil {
			clone.DialContext = (&net.Dialer{Timeout: llmDialTimeout, KeepAlive: 30 * time.Second}).DialContext
		}
		c.Transport = clone
	}
	security.GuardClient(&c)
	return &c
}

func defaultChatTransport(header time.Duration) *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   llmDialTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   llmTLSTimeout,
		ResponseHeaderTimeout: header,
		ExpectContinueTimeout: time.Second,
	}
}

func checkEndpoint(raw string) error {
	return security.CheckURL(raw)
}
