// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package connector

import (
	"context"
	"errors"
	"net"
	"time"
)

// RetryConfig wraps a connector with timeout and retry. Offline → ErrUnavailable.
type RetryConfig struct {
	Timeout time.Duration
	Retries int
}

type resilient struct {
	inner Connector
	cfg   RetryConfig
}

// WithRetry wraps c so Invoke/Health/ListTools honor timeout and retry.
func WithRetry(c Connector, cfg RetryConfig) Connector {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.Retries < 0 {
		cfg.Retries = 0
	}
	return &resilient{inner: c, cfg: cfg}
}

func (r *resilient) Name() string { return r.inner.Name() }

func (r *resilient) ListTools(ctx context.Context) ([]Tool, error) {
	var last []Tool
	err := r.loop(ctx, func(cctx context.Context) error {
		var e error
		last, e = r.inner.ListTools(cctx)
		return e
	})
	return last, err
}

func (r *resilient) Invoke(ctx context.Context, tool string, args map[string]any) (*InvokeResult, error) {
	var last *InvokeResult
	err := r.loop(ctx, func(cctx context.Context) error {
		var e error
		last, e = r.inner.Invoke(cctx, tool, args)
		return e
	})
	return last, err
}

func (r *resilient) Health(ctx context.Context) error {
	return r.loop(ctx, r.inner.Health)
}

func (r *resilient) loop(ctx context.Context, fn func(context.Context) error) error {
	var last error
	attempts := r.cfg.Retries + 1
	for i := 0; i < attempts; i++ {
		if ctx.Err() != nil {
			return unavailable(ctx.Err())
		}
		cctx, cancel := context.WithTimeout(ctx, r.cfg.Timeout)
		err := fn(cctx)
		cancel()
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return unavailable(ctx.Err())
		}
		last = err
		if !isRetryable(err) {
			if isOffline(err) {
				return unavailable(err)
			}
			return err
		}
	}
	return unavailable(last)
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	return isOffline(err)
}

func isOffline(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrUnavailable) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne)
}
