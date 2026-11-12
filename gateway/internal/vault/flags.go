// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultIngestMaxBytes is the per-file cap when GOSO_VAULT_INGEST_MAX_BYTES is unset.
	DefaultIngestMaxBytes int64 = 20 << 20
	// MultipartOverhead is extra room for MIME headers around a file of exactly
	// the per-file cap. The file part itself stays capped at IngestMaxBytes.
	MultipartOverhead int64 = 64 << 10
	// uploadChunk is the only buffer the raw writer keeps. It is far below the file cap.
	uploadChunk = 32 << 10
)

// UploadChunkHook, when set, receives each chunk length copied to the temp file.
// Tests use it to prove the raw path does not write the whole file in one buffer.
var UploadChunkHook func(n int)

// UploadEnabled reports GOSO_VAULT_UPLOAD. Default is off.
func UploadEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GOSO_VAULT_UPLOAD"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// IngestMaxBytes is GOSO_VAULT_INGEST_MAX_BYTES, or 20 MiB when unset or invalid.
func IngestMaxBytes() int64 {
	v := strings.TrimSpace(os.Getenv("GOSO_VAULT_INGEST_MAX_BYTES"))
	if v == "" {
		return DefaultIngestMaxBytes
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return DefaultIngestMaxBytes
	}
	return n
}

// QuotaBytes is GOSO_VAULT_INGEST_QUOTA_BYTES. The boolean is false when the
// value is missing, zero, negative, or not an integer. There is no unlimited default.
func QuotaBytes() (int64, bool) {
	v := strings.TrimSpace(os.Getenv("GOSO_VAULT_INGEST_QUOTA_BYTES"))
	if v == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// QuotaConfigured reports a positive per-tenant disk quota.
func QuotaConfigured() bool {
	_, ok := QuotaBytes()
	return ok
}

// DoclingEnabled reports GOSO_VAULT_DOCLING. Default is off. Off leaves an
// uploaded job where it is and does not call the sidecar.
func DoclingEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GOSO_VAULT_DOCLING"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// DoclingURL is the internal sidecar base URL. Empty means the sidecar is
// not configured. The value is not a secret; callers still should not log it
// next to document text.
func DoclingURL() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv("GOSO_VAULT_DOCLING_URL")), "/")
}

// DefaultPageTimeout is the per-page limit when GOSO_VAULT_DOCLING_PAGE_TIMEOUT
// is unset. It is a dev default. It has not been measured on VPS2.
const DefaultPageTimeout = 60 * time.Second

// DefaultPageAttempts is the per-page retry cap. The spec suggests 2.
const DefaultPageAttempts = 2

// DefaultPageBudget is how many page slots the gateway waits for one sidecar
// call (timeout × attempts × budget). It is a dev ceiling, not a VPS2 measurement.
const DefaultPageBudget = 32

// PageTimeout reads GOSO_VAULT_DOCLING_PAGE_TIMEOUT. A Go duration ("50ms")
// or a positive number of seconds is accepted. Invalid values use the default.
func PageTimeout() time.Duration {
	v := strings.TrimSpace(os.Getenv("GOSO_VAULT_DOCLING_PAGE_TIMEOUT"))
	if v == "" {
		return DefaultPageTimeout
	}
	if d, err := time.ParseDuration(v); err == nil && d > 0 {
		return d
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return DefaultPageTimeout
	}
	return time.Duration(n) * time.Second
}

// PageAttempts reads GOSO_VAULT_DOCLING_PAGE_ATTEMPTS. Invalid or non-positive
// values use 2. Values above 10 are capped so a bad env cannot retry without bound.
func PageAttempts() int {
	v := strings.TrimSpace(os.Getenv("GOSO_VAULT_DOCLING_PAGE_ATTEMPTS"))
	if v == "" {
		return DefaultPageAttempts
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return DefaultPageAttempts
	}
	if n > 10 {
		return 10
	}
	return n
}

// PageBudget reads GOSO_VAULT_DOCLING_PAGE_BUDGET. Invalid values use 32.
// Values above 500 are capped.
func PageBudget() int {
	v := strings.TrimSpace(os.Getenv("GOSO_VAULT_DOCLING_PAGE_BUDGET"))
	if v == "" {
		return DefaultPageBudget
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return DefaultPageBudget
	}
	if n > 500 {
		return 500
	}
	return n
}

// QuotaStatus is "quota_required" when upload is on and the disk quota is not
// a positive byte count. Otherwise it is empty. The byte count itself is not returned.
func QuotaStatus() string {
	if UploadEnabled() && !QuotaConfigured() {
		return "quota_required"
	}
	return ""
}
