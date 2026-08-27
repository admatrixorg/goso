// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package observe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Exporter ships finished spans. No Grafana Cloud keys (DI-18).
type Exporter interface {
	Export(ctx context.Context, spans []Span) error
}

// NoopExporter drops spans. Used when GOSO_OTEL_ENDPOINT is empty.
type NoopExporter struct{}

// Export implements Exporter.
func (NoopExporter) Export(context.Context, []Span) error { return nil }

// FakeExporter records batches for tests.
type FakeExporter struct {
	mu      sync.Mutex
	Batches [][]Span
}

// Export appends a copy of spans.
func (f *FakeExporter) Export(_ context.Context, spans []Span) error {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]Span, len(spans))
	copy(cp, spans)
	f.Batches = append(f.Batches, cp)
	return nil
}

// All returns a flattened copy of recorded spans.
func (f *FakeExporter) All() []Span {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Span
	for _, b := range f.Batches {
		out = append(out, b...)
	}
	return out
}

// HTTPExporter POSTs a thin OTLP JSON payload to Endpoint.
// It does not read Grafana Cloud keys or add vendor Authorization headers.
type HTTPExporter struct {
	Endpoint string
	Client   *http.Client
}

// ExporterFromEnv returns NoopExporter unless GOSO_OTEL_ENDPOINT is set.
func ExporterFromEnv() Exporter {
	ep := strings.TrimSpace(os.Getenv("GOSO_OTEL_ENDPOINT"))
	if ep == "" {
		return NoopExporter{}
	}
	return &HTTPExporter{
		Endpoint: ep,
		Client:   &http.Client{Timeout: 5 * time.Second},
	}
}

// Export POSTs JSON to Endpoint. Empty endpoint or empty spans are a no-op.
func (e *HTTPExporter) Export(ctx context.Context, spans []Span) error {
	if e == nil || strings.TrimSpace(e.Endpoint) == "" || len(spans) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	client := e.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	body, err := json.Marshal(otlpPayload(spans))
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("otlp export %s", resp.Status)
	}
	return nil
}

func otlpPayload(spans []Span) map[string]any {
	out := make([]map[string]any, 0, len(spans))
	for _, s := range spans {
		attrs := []map[string]any{
			otlpKV("goso.span.kind", s.Kind),
			otlpKV("goso.cache_read_tokens", fmt.Sprintf("%d", s.CacheReadTokens)),
		}
		for k, v := range s.Attributes {
			attrs = append(attrs, otlpKV(k, v))
		}
		if s.Error != "" {
			attrs = append(attrs, otlpKV("goso.error", s.Error))
		}
		item := map[string]any{
			"traceId":           s.TraceID,
			"spanId":            s.SpanID,
			"name":              s.Name,
			"kind":              s.Kind,
			"startTimeUnixNano": s.Start.UnixNano(),
			"endTimeUnixNano":   s.End.UnixNano(),
			"attributes":        attrs,
		}
		if s.ParentID != "" {
			item["parentSpanId"] = s.ParentID
		}
		if s.Status != "" {
			msg := s.Status
			if s.Error != "" {
				msg = s.Error
			}
			item["status"] = map[string]any{"message": msg}
		}
		out = append(out, item)
	}
	return map[string]any{
		"resourceSpans": []map[string]any{{
			"resource": map[string]any{
				"attributes": []map[string]any{otlpKV("service.name", "goso-gateway")},
			},
			"scopeSpans": []map[string]any{{"spans": out}},
		}},
	}
}

func otlpKV(k, v string) map[string]any {
	return map[string]any{"key": k, "value": map[string]any{"stringValue": v}}
}
