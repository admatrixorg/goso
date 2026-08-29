// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/security"
)

const (
	ToolWebFetch    = "web_fetch"
	maxFetchBody    = 1 << 20
	webFetchTimeout = 10 * time.Second
)

// WebFetchClient is the HTTP client for web_fetch. Nil → 10s default.
// Tests may replace Transport. GuardClient is always applied on a per-call clone.
var WebFetchClient *http.Client

func urlArg(args map[string]any) string {
	if args == nil {
		return ""
	}
	v, ok := args["url"]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func fetchClient() *http.Client {
	c := &http.Client{Timeout: webFetchTimeout}
	if WebFetchClient != nil {
		c = &http.Client{
			Transport: WebFetchClient.Transport,
			Jar:       WebFetchClient.Jar,
			Timeout:   WebFetchClient.Timeout,
		}
		if c.Timeout == 0 {
			c.Timeout = webFetchTimeout
		}
	}
	security.GuardClient(c)
	return c
}

func fetchErr(msg string) *connector.InvokeResult {
	return &connector.InvokeResult{
		Tool:      ToolWebFetch,
		Connector: ConnectorName,
		Status:    "error",
		Content:   map[string]any{"error": msg},
	}
}

func webFetch(ctx context.Context, args map[string]any) (*connector.InvokeResult, error) {
	raw := urlArg(args)
	if raw == "" {
		return fetchErr("url is required"), nil
	}
	if err := security.CheckURL(raw); err != nil {
		return fetchErr(err.Error()), nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return fetchErr(err.Error()), nil
	}
	resp, err := fetchClient().Do(req)
	if err != nil {
		msg := err.Error()
		if !strings.Contains(strings.ToLower(msg), "ssrf") {
			msg = "fetch failed"
		}
		return fetchErr(msg), nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBody))
	if err != nil {
		return fetchErr(err.Error()), nil
	}
	return &connector.InvokeResult{
		Tool:      ToolWebFetch,
		Connector: ConnectorName,
		Status:    "ok",
		Content: map[string]any{
			"status":       resp.StatusCode,
			"content_type": resp.Header.Get("Content-Type"),
			"body":         string(body),
		},
	}, nil
}
