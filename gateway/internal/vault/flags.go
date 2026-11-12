// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import (
	"os"
	"strconv"
	"strings"
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

// QuotaStatus is "quota_required" when upload is on and the disk quota is not
// a positive byte count. Otherwise it is empty. The byte count itself is not returned.
func QuotaStatus() string {
	if UploadEnabled() && !QuotaConfigured() {
		return "quota_required"
	}
	return ""
}
