// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package security

import (
	"net/http"
	"os"
	"strings"
)

const (
	// MaxAPIBody is the HTTP /api and /v1 body cap (1 MiB).
	MaxAPIBody = 1 << 20
	// MaxWSRead is the WebSocket frame cap (512 KiB).
	MaxWSRead = 512 << 10
)

// LimitAPI wraps /api and /v1 request bodies with http.MaxBytesReader.
// POST /api/vault/upload is the only exemption, and only while GOSO_VAULT_UPLOAD
// is on. The flag off keeps that route on the 1 MiB cap. No /v1 path is exempt.
func LimitAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && (strings.HasPrefix(r.URL.Path, "/api") || strings.HasPrefix(r.URL.Path, "/v1")) && !vaultUploadExempt(r) {
			r.Body = http.MaxBytesReader(w, r.Body, MaxAPIBody)
		}
		next.ServeHTTP(w, r)
	})
}

func vaultUploadExempt(r *http.Request) bool {
	if r == nil || r.Method != http.MethodPost || r.URL == nil || r.URL.Path != "/api/vault/upload" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GOSO_VAULT_UPLOAD"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
