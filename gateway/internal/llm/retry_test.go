// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryableStatusAndError(t *testing.T) {
	for _, code := range []int{http.StatusTooManyRequests, 500, 502, 503, 504} {
		if !retryableStatus(code) {
			t.Fatalf("status %d should retry", code)
		}
		if !Retryable(&UpstreamError{Status: code}) {
			t.Fatalf("upstream %d should retry", code)
		}
	}
	for _, code := range []int{400, 401, 402, 403, 404, 409} {
		if retryableStatus(code) {
			t.Fatalf("status %d must not retry", code)
		}
		if Retryable(&UpstreamError{Status: code}) {
			t.Fatalf("upstream %d must not retry", code)
		}
	}
	if Retryable(context.Canceled) || Retryable(context.DeadlineExceeded) {
		t.Fatal("context cancellation is not retryable")
	}
	if !Retryable(timeoutNetErr{}) {
		t.Fatal("network timeout should retry")
	}
	if !Retryable(errors.New("dial tcp 127.0.0.1:1: connect: connection refused")) {
		t.Fatal("dial error should retry")
	}
}

func TestBackoffDelayBounds(t *testing.T) {
	for attempt := 0; attempt < 8; attempt++ {
		d := backoffDelay(attempt)
		max := backoffBase << attempt
		if max > backoffCap || max < backoffBase {
			max = backoffCap
		}
		if d < max/2 || d > max {
			t.Fatalf("attempt %d delay %s outside [%s, %s]", attempt, d, max/2, max)
		}
	}
	d := backoffDelay(30)
	if d < backoffCap/2 || d > backoffCap {
		t.Fatalf("cap %s", d)
	}
}

func TestRetryAfterDelay(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	h := http.Header{}
	if _, ok := retryAfterDelay(h, now); ok {
		t.Fatal("missing header")
	}
	h.Set("Retry-After", "0")
	d, ok := retryAfterDelay(h, now)
	if !ok || d != 0 {
		t.Fatalf("zero %s %v", d, ok)
	}
	h.Set("Retry-After", "2")
	d, ok = retryAfterDelay(h, now)
	if !ok || d != 2*time.Second {
		t.Fatalf("2s %s", d)
	}
	h.Set("Retry-After", "120")
	d, ok = retryAfterDelay(h, now)
	if !ok || d != backoffCap {
		t.Fatalf("cap %s", d)
	}
	h.Set("Retry-After", now.Add(10*time.Second).Format(http.TimeFormat))
	d, ok = retryAfterDelay(h, now)
	if !ok || d != 10*time.Second {
		t.Fatalf("date %s", d)
	}
	h.Set("Retry-After", now.Add(-time.Minute).Format(http.TimeFormat))
	d, ok = retryAfterDelay(h, now)
	if !ok || d != 0 {
		t.Fatalf("past %s", d)
	}
	h.Set("Retry-After", "soon")
	if _, ok := retryAfterDelay(h, now); ok {
		t.Fatal("invalid header")
	}
}

func TestOpenAI_Retries429And5xxNot4xx(t *testing.T) {
	t.Setenv("GOSO_SSRF", "0")
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := n.Add(1)
		if i < 3 {
			if i == 1 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	t.Cleanup(srv.Close)
	o := &OpenAI{APIKey: "k", Model: "gcli/grok-4.5-high", BaseURL: srv.URL + "/v1", Client: srv.Client()}
	got, err := o.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err != nil || got != "ok" || n.Load() != 3 {
		t.Fatalf("got %q err %v n %d", got, err, n.Load())
	}

	n.Store(0)
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"no"}`)
	})
	_, err = o.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	var up *UpstreamError
	if !errors.As(err, &up) || up.Status != http.StatusBadRequest || n.Load() != 1 {
		t.Fatalf("4xx err %v n %d", err, n.Load())
	}
}

func TestOpenAI_ContextCancelDoesNotRetry(t *testing.T) {
	t.Setenv("GOSO_SSRF", "0")
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		select {
		case <-r.Context().Done():
		case <-timer.C:
		}
	}))
	t.Cleanup(srv.Close)
	o := &OpenAI{APIKey: "k", Model: "m", BaseURL: srv.URL + "/v1", Client: srv.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := o.Chat(ctx, []Message{{Role: "user", Content: "hi"}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err %v", err)
	}
	if n.Load() != 1 {
		t.Fatalf("attempts %d", n.Load())
	}
	if time.Since(start) > time.Second {
		t.Fatalf("cancel took %s", time.Since(start))
	}
}

func TestChatClientDropsOverallTimeout(t *testing.T) {
	t.Setenv("GOSO_LLM_RESPONSE_HEADER_TIMEOUT", "")
	c := chatClient(&http.Client{Timeout: 20 * time.Second})
	if c.Timeout != 0 {
		t.Fatalf("timeout %s", c.Timeout)
	}
	tr, ok := c.Transport.(*http.Transport)
	if !ok || tr.ResponseHeaderTimeout != 180*time.Second {
		t.Fatalf("header timeout %#v", c.Transport)
	}
	if tr.TLSHandshakeTimeout != llmTLSTimeout {
		t.Fatalf("tls %s", tr.TLSHandshakeTimeout)
	}
	t.Setenv("GOSO_LLM_RESPONSE_HEADER_TIMEOUT", "45")
	c = chatClient(nil)
	tr = c.Transport.(*http.Transport)
	if tr.ResponseHeaderTimeout != 45*time.Second || c.Timeout != 0 {
		t.Fatalf("configured %s timeout %s", tr.ResponseHeaderTimeout, c.Timeout)
	}
}

type timeoutNetErr struct{}

func (timeoutNetErr) Error() string   { return "timeout" }
func (timeoutNetErr) Timeout() bool   { return true }
func (timeoutNetErr) Temporary() bool { return true }

func TestChatClientKeepsCustomTransport(t *testing.T) {
	base := &http.Transport{TLSHandshakeTimeout: 3 * time.Second}
	c := chatClient(&http.Client{Transport: base, Timeout: 20 * time.Second})
	tr := c.Transport.(*http.Transport)
	if tr == base {
		t.Fatal("mutated the shared transport")
	}
	if tr.TLSHandshakeTimeout != 3*time.Second {
		t.Fatalf("tls %s", tr.TLSHandshakeTimeout)
	}
	if !strings.Contains(timeoutNetErr{}.Error(), "timeout") {
		t.Fatal("net error")
	}
	var _ net.Error = timeoutNetErr{}
}
