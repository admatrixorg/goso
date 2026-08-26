// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package observe

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

const (
	// HeaderRequestID is the HTTP request/correlation id header.
	HeaderRequestID = "X-Request-ID"
	// DefaultTraceCapacity is the in-memory LLM trace ring size (SPEC 008).
	DefaultTraceCapacity = 200
	// DefaultTraceLimit is the default GET /api/traces page size.
	DefaultTraceLimit = 20
)

type ctxKey int

const requestIDKey ctxKey = 1

// Observer is the gateway observability hub: access logs, LLM traces, counters.
type Observer struct {
	started time.Time
	out     io.Writer
	traces  *Buffer
	reqs    atomic.Int64
	llms    atomic.Int64
}

// New returns an Observer that logs JSON to stdout and keeps 200 LLM traces.
func New() *Observer {
	return NewWithWriter(os.Stdout)
}

// NewWithWriter is like New but writes access/LLM logs to w (tests).
func NewWithWriter(w io.Writer) *Observer {
	if w == nil {
		w = os.Stdout
	}
	return &Observer{
		started: time.Now().UTC(),
		out:     w,
		traces:  NewBuffer(DefaultTraceCapacity),
	}
}

// Traces returns the LLM trace ring buffer.
func (o *Observer) Traces() *Buffer { return o.traces }

// Register mounts GET /api/traces, GET /api/stats, and GET /metrics.
func (o *Observer) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/traces", o.HandleTraces)
	mux.HandleFunc("GET /api/stats", o.HandleStats)
	mux.HandleFunc("GET /api/metrics", o.HandleStats) // SPEC 018 alias
	mux.HandleFunc("GET /metrics", o.HandleMetrics)
}

// RequestIDFromContext returns the request id stored by Middleware, or "".
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(requestIDKey).(string)
	return v
}

// WithRequestID stores request id on ctx (tests / nested spans).
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}
