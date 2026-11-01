// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	maxLLMAttempts = 3
	backoffBase    = 300 * time.Millisecond
	backoffCap     = 30 * time.Second
)

// jitterSpan returns a value in [0, n). Tests may compare bounds without stubbing it.
var jitterSpan = func(n int64) int64 {
	if n <= 0 {
		return 0
	}
	return rand.Int63n(n)
}

// backoffDelay is the sleep after a failed attempt (0-based).
// It grows from 300ms and is capped at 30s, with jitter in the upper half.
func backoffDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 16 {
		attempt = 16
	}
	d := backoffBase << attempt
	if d < backoffBase || d > backoffCap {
		d = backoffCap
	}
	half := d / 2
	if half <= 0 {
		return d
	}
	return half + time.Duration(jitterSpan(int64(half)))
}

func retryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= 500
}

// Retryable reports whether err is a network error, 429, or 5xx.
// Context cancellation and other 4xx statuses are not retryable.
func Retryable(err error) bool {
	if err == nil {
		return false
	}
	var up *UpstreamError
	if errors.As(err, &up) && up != nil {
		return retryableStatus(up.Status)
	}
	return retryableNetErr(err)
}

func retryableNetErr(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if transportTimeout(err) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	low := strings.ToLower(err.Error())
	switch {
	case strings.Contains(low, "connection refused"),
		strings.Contains(low, "connection reset"),
		strings.Contains(low, "broken pipe"),
		strings.Contains(low, "timeout awaiting response headers"),
		strings.Contains(low, "tls handshake"),
		strings.Contains(low, "i/o timeout"),
		strings.Contains(low, "eof"):
		return true
	default:
		return strings.Contains(low, "dial ")
	}
}

// transportTimeout is a dial or header timeout from net/http.
// Go's header-timeout error matches context.DeadlineExceeded, and the
// context sentinel itself implements net.Error, so peel url.Error and
// reject the sentinel before treating Timeout() as retryable.
func transportTimeout(err error) bool {
	var ue *url.Error
	for errors.As(err, &ue) && ue != nil {
		err = ue.Err
		ue = nil
	}
	if err == nil || err == context.Canceled || err == context.DeadlineExceeded {
		return false
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// retryAfterDelay reads Retry-After (delta-seconds or HTTP-date).
// The delay is capped at 30s. ok is false when the header is missing or invalid.
func retryAfterDelay(h http.Header, now time.Time) (time.Duration, bool) {
	if h == nil {
		return 0, false
	}
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return 0, false
	}
	if sec, err := strconv.Atoi(v); err == nil {
		if sec < 0 {
			return 0, false
		}
		d := time.Duration(sec) * time.Second
		if d > backoffCap {
			d = backoffCap
		}
		return d, true
	}
	when, err := http.ParseTime(v)
	if err != nil {
		return 0, false
	}
	d := when.Sub(now)
	if d < 0 {
		d = 0
	}
	if d > backoffCap {
		d = backoffCap
	}
	return d, true
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// withAttempts runs fn up to 3 times. fn returns retry=true only for network
// errors, 429, and 5xx. waitSet uses wait (already capped) instead of backoff.
// A cancelled context stops the loop and is never retried.
func withAttempts(ctx context.Context, fn func(attempt int) (retry bool, wait time.Duration, waitSet bool, err error)) error {
	var last error
	for attempt := 0; attempt < maxLLMAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		retry, wait, waitSet, err := fn(attempt)
		if err == nil {
			return nil
		}
		last = err
		if !retry || attempt+1 >= maxLLMAttempts {
			return err
		}
		if !waitSet {
			wait = backoffDelay(attempt)
		}
		if err := sleepCtx(ctx, wait); err != nil {
			return err
		}
	}
	if last == nil {
		last = errors.New("llm retries exhausted")
	}
	return last
}
