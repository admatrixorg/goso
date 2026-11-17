// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Package embed calls an OpenAI-compatible /embeddings endpoint. It is off
// unless GOSO_EMBED_URL and GOSO_EMBED_MODEL are set. The API key is sent as a
// bearer token and never appears in errors or logs.
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mqglobal/goso/gateway/internal/config"
)

// Defaults for the optional env vars.
const (
	DefaultRPS     = 2.0
	DefaultBatch   = 16
	DefaultTimeout = 30 * time.Second
	maxResponse    = 64 << 20
)

// Config is the embeddings endpoint. URL may be a base (…/v1) or the full
// …/embeddings URL. Dim, when set, is sent as "dimensions" and enforced.
type Config struct {
	URL     string
	Model   string
	APIKey  string
	Dim     int
	RPS     float64
	Batch   int
	Timeout time.Duration
}

// Configured reports whether URL and model are both set.
func (c Config) Configured() bool {
	return strings.TrimSpace(c.URL) != "" && strings.TrimSpace(c.Model) != ""
}

// FromEnv reads GOSO_EMBED_URL, GOSO_EMBED_MODEL, GOSO_EMBED_API_KEY,
// GOSO_EMBED_DIM, GOSO_EMBED_RPS and GOSO_EMBED_BATCH. A malformed number is
// an error so a typo does not silently change the vector size.
func FromEnv() (Config, error) {
	cfg := Config{
		URL:    config.Lookup("GOSO_EMBED_URL"),
		Model:  config.Lookup("GOSO_EMBED_MODEL"),
		APIKey: config.Lookup("GOSO_EMBED_API_KEY"),
		RPS:    DefaultRPS,
		Batch:  DefaultBatch,
	}
	if v := config.Lookup("GOSO_EMBED_DIM"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 16000 {
			return cfg, fmt.Errorf("GOSO_EMBED_DIM must be a positive integer")
		}
		cfg.Dim = n
	}
	if v := config.Lookup("GOSO_EMBED_RPS"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f <= 0 {
			return cfg, fmt.Errorf("GOSO_EMBED_RPS must be a positive number")
		}
		cfg.RPS = f
	}
	if v := config.Lookup("GOSO_EMBED_BATCH"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 || n > 2048 {
			return cfg, fmt.Errorf("GOSO_EMBED_BATCH must be a positive integer")
		}
		cfg.Batch = n
	}
	return cfg, nil
}

// Client embeds text. It is safe for concurrent use. Requests are spaced by
// the RPS limit across all callers.
type Client struct {
	cfg      Config
	endpoint string
	http     *http.Client
	dim      atomic.Int64

	mu   sync.Mutex
	next time.Time
}

// New builds a client. It returns nil when cfg is not configured.
func New(cfg Config) *Client {
	if !cfg.Configured() {
		return nil
	}
	if cfg.RPS <= 0 {
		cfg.RPS = DefaultRPS
	}
	if cfg.Batch <= 0 {
		cfg.Batch = DefaultBatch
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	u := strings.TrimRight(strings.TrimSpace(cfg.URL), "/")
	if !strings.HasSuffix(u, "/embeddings") {
		u += "/embeddings"
	}
	c := &Client{cfg: cfg, endpoint: u, http: &http.Client{Timeout: cfg.Timeout}}
	c.dim.Store(int64(cfg.Dim))
	return c
}

// Model is the configured model name.
func (c *Client) Model() string { return c.cfg.Model }

// Dim is the configured dimension, or the one learned from the first
// response when GOSO_EMBED_DIM is unset. Zero until known.
func (c *Client) Dim() int { return int(c.dim.Load()) }

// Batch is the number of inputs sent per request.
func (c *Client) Batch() int { return c.cfg.Batch }

// ErrResponse is a non-2xx or malformed endpoint reply.
var ErrResponse = errors.New("embeddings endpoint")

type request struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions int      `json:"dimensions,omitempty"`
}

type response struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

// Embed returns one vector per input, in order. Inputs are split into
// batches; each request waits for the rate limit.
func (c *Client) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	if c == nil {
		return nil, errors.New("embeddings not configured")
	}
	out := make([][]float32, 0, len(inputs))
	for start := 0; start < len(inputs); start += c.cfg.Batch {
		end := start + c.cfg.Batch
		if end > len(inputs) {
			end = len(inputs)
		}
		vecs, err := c.embedBatch(ctx, inputs[start:end])
		if err != nil {
			return nil, err
		}
		out = append(out, vecs...)
	}
	return out, nil
}

func (c *Client) wait(ctx context.Context) error {
	c.mu.Lock()
	now := time.Now()
	at := c.next
	if at.Before(now) {
		at = now
	}
	c.next = at.Add(time.Duration(float64(time.Second) / c.cfg.RPS))
	c.mu.Unlock()
	d := time.Until(at)
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// maxAttempts bounds retries of a rate-limited (429) or unavailable (503)
// batch; free-tier providers throttle long backfills.
const maxAttempts = 6

// retryBase is the first backoff; tests shorten it.
var retryBase = 2 * time.Second

// retryable is a 429 or 503 answer and how long the provider asked to wait.
type retryable struct{ after time.Duration }

func (r *retryable) Error() string { return "retryable" }

func (c *Client) embedBatch(ctx context.Context, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	backoff := retryBase
	for attempt := 1; ; attempt++ {
		vecs, err := c.embedOnce(ctx, inputs)
		var rt *retryable
		if !errors.As(err, &rt) {
			return vecs, err
		}
		if attempt >= maxAttempts {
			return nil, fmt.Errorf("%w: rate limited after %d attempts", ErrResponse, attempt)
		}
		d := backoff
		if rt.after > d {
			d = rt.after
		}
		if d > time.Minute {
			d = time.Minute
		}
		backoff *= 2
		c.pause(d)
		if err := c.wait(ctx); err != nil {
			return nil, err
		}
	}
}

// pause pushes the shared limiter so concurrent batches also back off.
func (c *Client) pause(d time.Duration) {
	c.mu.Lock()
	if at := time.Now().Add(d); at.After(c.next) {
		c.next = at
	}
	c.mu.Unlock()
}

func (c *Client) embedOnce(ctx context.Context, inputs []string) ([][]float32, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	body, err := json.Marshal(request{Model: c.cfg.Model, Input: inputs, Dimensions: c.cfg.Dim})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: bad URL", ErrResponse)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// The transport error names the URL, never the header.
		return nil, fmt.Errorf("%w: request failed", ErrResponse)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
	if err != nil {
		return nil, fmt.Errorf("%w: read failed", ErrResponse)
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
		after := time.Duration(0)
		if secs, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil && secs > 0 {
			after = time.Duration(secs) * time.Second
		}
		return nil, &retryable{after: after}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// The body is not echoed: some providers repeat the key in it.
		return nil, fmt.Errorf("%w: HTTP %d", ErrResponse, resp.StatusCode)
	}
	var r response
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("%w: invalid JSON", ErrResponse)
	}
	if len(r.Data) != len(inputs) {
		return nil, fmt.Errorf("%w: %d vectors for %d inputs", ErrResponse, len(r.Data), len(inputs))
	}
	out := make([][]float32, len(inputs))
	for i, d := range r.Data {
		idx := d.Index
		if idx < 0 || idx >= len(inputs) || out[idx] != nil {
			idx = i
		}
		if len(d.Embedding) == 0 {
			return nil, fmt.Errorf("%w: empty vector", ErrResponse)
		}
		out[idx] = d.Embedding
	}
	for _, v := range out {
		if v == nil {
			return nil, fmt.Errorf("%w: missing vector", ErrResponse)
		}
		if err := c.checkDim(len(v)); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (c *Client) checkDim(n int) error {
	want := c.dim.Load()
	if want == 0 {
		if c.dim.CompareAndSwap(0, int64(n)) {
			return nil
		}
		want = c.dim.Load()
	}
	if int64(n) != want {
		return fmt.Errorf("%w: vector size %d, want %d", ErrResponse, n, want)
	}
	return nil
}
