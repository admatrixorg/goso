// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package security

import (
	"net/http"
	"strings"
)

const (
	// MaxAPIBody is the HTTP /api body cap (1 MiB).
	MaxAPIBody = 1 << 20
	// MaxWSRead is the WebSocket frame cap (512 KiB).
	MaxWSRead = 512 << 10
)

// LimitAPI wraps /api request bodies with http.MaxBytesReader.
func LimitAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api") && r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, MaxAPIBody)
		}
		next.ServeHTTP(w, r)
	})
}
