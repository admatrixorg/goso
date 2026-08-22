// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type httpConnector struct {
	name     string
	endpoint string
	token    string
	client   *http.Client
	manURL   string
	rawMan   json.RawMessage
	mu       sync.Mutex
	cached   *Manifest
}

func newHTTPConnector(cfg Config) (Connector, error) {
	ep := strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	if ep == "" {
		return nil, fmt.Errorf("http endpoint is required")
	}
	c := &httpConnector{
		name:     cfg.Name,
		endpoint: ep,
		token:    cfg.BearerToken,
		client:   cfg.Client,
		manURL:   cfg.ManifestURL,
		rawMan:   cfg.ManifestJSON,
	}
	if c.client == nil {
		c.client = &http.Client{Timeout: cfg.Timeout}
	}
	if len(cfg.ManifestJSON) > 0 {
		m, err := ParseManifest(cfg.ManifestJSON)
		if err != nil {
			return nil, fmt.Errorf("inline manifest: %w", err)
		}
		c.cached = m
	}
	return c, nil
}

func (c *httpConnector) Name() string { return c.name }

func (c *httpConnector) ListTools(ctx context.Context) ([]Tool, error) {
	m, err := c.loadManifest(ctx)
	if err != nil {
		return nil, err
	}
	return append([]Tool(nil), m.Tools...), nil
}

func (c *httpConnector) Invoke(ctx context.Context, tool string, args map[string]any) (*InvokeResult, error) {
	if args == nil {
		args = map[string]any{}
	}
	m, err := c.loadManifest(ctx)
	if err != nil {
		return nil, err
	}
	t, ok := m.Tool(tool)
	if !ok {
		return nil, fmt.Errorf("unknown tool %q", tool)
	}
	if err := ValidateArgs(t.InputSchema, args); err != nil {
		return nil, err
	}
	body, _ := json.Marshal(args)
	url := c.endpoint + "/tools/" + tool
	start := time.Now()
	resp, err := c.do(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, unavailable(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 500 {
		return nil, unavailable(fmt.Errorf("http %d", resp.StatusCode))
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, bytesTrimSpace(raw))
	}
	var content any
	if len(bytesTrimSpace(raw)) > 0 {
		_ = json.Unmarshal(raw, &content)
	}
	lat := time.Since(start)
	return &InvokeResult{
		Tool:      tool,
		Connector: c.name,
		Content:   content,
		Raw:       json.RawMessage(raw),
		Latency:   lat,
		LatencyMS: lat.Milliseconds(),
		Status:    "ok",
	}, nil
}

func (c *httpConnector) Health(ctx context.Context) error {
	if err := c.ping(ctx); err != nil {
		return unavailable(err)
	}
	m, err := c.loadManifest(ctx)
	if err != nil {
		return err
	}
	return m.Validate()
}

func (c *httpConnector) ping(ctx context.Context) error {
	for _, path := range []string{"/healthz", "/readyz"} {
		resp, err := c.do(ctx, http.MethodGet, c.endpoint+path, nil)
		if err != nil {
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
	}
	return fmt.Errorf("healthz/readyz failed")
}

func (c *httpConnector) loadManifest(ctx context.Context) (*Manifest, error) {
	c.mu.Lock()
	if c.cached != nil {
		m := c.cached
		c.mu.Unlock()
		return m, nil
	}
	c.mu.Unlock()

	url := c.manURL
	if url == "" {
		url = c.endpoint + "/manifest"
	}
	resp, err := c.do(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, unavailable(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, unavailable(err)
	}
	if resp.StatusCode >= 400 {
		return nil, unavailable(fmt.Errorf("manifest http %d", resp.StatusCode))
	}
	m, err := ParseManifest(raw)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.cached = m
	c.mu.Unlock()
	return m, nil
}

func (c *httpConnector) do(ctx context.Context, method, url string, body []byte) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rdr)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return c.client.Do(req)
}

// RelayDecision POSTs an approval decision to the remote owner. Best-effort.
// goso never executes the mutation itself.
func RelayDecision(ctx context.Context, client *http.Client, endpoint, token, approvalID, decision string, extra map[string]any) error {
	if client == nil {
		client = http.DefaultClient
	}
	endpoint = strings.TrimRight(endpoint, "/")
	if endpoint == "" || approvalID == "" {
		return nil
	}
	payload := map[string]any{"decision": decision, "approval_id": approvalID}
	for k, v := range extra {
		payload[k] = v
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/approvals/"+approvalID+"/decision", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 500 {
		return fmt.Errorf("relay http %d", resp.StatusCode)
	}
	return nil
}
