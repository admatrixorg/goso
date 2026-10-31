// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package bridge

import (
	"encoding/json"
	"log"
	"net/http"
	"time"
)

// accessLog writes one JSON line per request. It records method, path, status,
// latency, tenant, and agent. It does not read or log the body, the API key,
// or the HMAC signature.
func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logJSON(map[string]any{
			"event":      "bridge_access",
			"method":     r.Method,
			"path":       r.URL.Path,
			"status":     rec.status,
			"latency_ms": time.Since(start).Milliseconds(),
			"tenant":     r.Header.Get(headerTenant),
			"agent":      r.Header.Get(headerAgent),
		})
	})
}

func logJSON(fields map[string]any) {
	b, err := json.Marshal(fields)
	if err != nil {
		return
	}
	log.Printf("%s", b)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote {
		s.status = code
		s.wrote = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wrote {
		s.wrote = true
	}
	return s.ResponseWriter.Write(b)
}

func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }
