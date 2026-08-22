// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package observe

import (
	"bufio"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"
)

// accessLog is one JSON line per HTTP request. Path only — never query/headers/body.
type accessLog struct {
	Level     string `json:"level"`
	RequestID string `json:"request_id"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Status    int    `json:"status"`
	LatencyMS int64  `json:"latency_ms"`
}

// Middleware assigns X-Request-ID (incoming or generated), logs one JSON line
// per request, and increments the request counter. Query strings and headers
// are never logged (tokens/keys must not leak).
func (o *Observer) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rid := strings.TrimSpace(r.Header.Get(HeaderRequestID))
		if rid == "" {
			rid = newRequestID()
		}
		rec := &statusRecorder{ResponseWriter: w}
		rec.Header().Set(HeaderRequestID, rid)
		r = r.WithContext(WithRequestID(r.Context(), rid))

		defer func() {
			status := rec.status
			if status == 0 {
				status = http.StatusOK
			}
			o.reqs.Add(1)
			o.writeJSON(accessLog{
				Level:     "info",
				RequestID: rid,
				Method:    r.Method,
				Path:      r.URL.Path, // path only — never RawQuery (may contain token)
				Status:    status,
				LatencyMS: time.Since(start).Milliseconds(),
			})
		}()
		next.ServeHTTP(rec, r)
	})
}

func (o *Observer) writeJSON(v any) {
	if o == nil || o.out == nil {
		return
	}
	enc := json.NewEncoder(o.out)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (w *statusRecorder) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusRecorder) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusRecorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *statusRecorder) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errNoHijack
	}
	if w.status == 0 {
		w.status = http.StatusSwitchingProtocols
	}
	return h.Hijack()
}

type hijackError string

func (e hijackError) Error() string { return string(e) }

const errNoHijack hijackError = "observe: ResponseWriter does not implement http.Hijacker"
