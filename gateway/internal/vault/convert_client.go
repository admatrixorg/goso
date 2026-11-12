// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// errSidecarUnreachable is a missing or refusing sidecar. The job becomes
// not_configured. The HTTP server keeps serving.
var errSidecarUnreachable = errors.New("docling sidecar unreachable")

// maxConvertResponse is the largest JSON body accepted from the sidecar.
// A larger body is too_large and is not stored. 64 MiB is a dev ceiling,
// not a measurement from VPS2.
var maxConvertResponse int64 = 64 << 20

// HTTPConverter calls the Docling sidecar. It does not send file bytes.
// The sidecar reads raw/ from its mount and returns markdown.
type HTTPConverter struct {
	URL        string
	Client     *http.Client
	PageBudget int
}

// EnforcesPageTimeout reports that the sidecar applies the per-page timeout
// and the attempt cap. The gateway still bounds the whole call.
func (h HTTPConverter) EnforcesPageTimeout() bool { return true }

// Convert posts one job. An empty URL or a refused connection is not_configured
// and does not retry inside this method.
func (h HTTPConverter) Convert(ctx context.Context, req ConvertRequest) (ConvertResult, error) {
	if h.URL == "" {
		return ConvertResult{ErrorClass: ClassNotConfigured}, nil
	}
	timeout := req.PageTimeout
	if timeout <= 0 {
		timeout = DefaultPageTimeout
	}
	attempts := req.MaxAttempts
	if attempts < 1 {
		attempts = DefaultPageAttempts
	}
	pages := h.PageBudget
	if pages < 1 {
		pages = DefaultPageBudget
	}
	payload, err := json.Marshal(map[string]any{
		"tenant":           req.Tenant,
		"raw_path":         req.RawPath,
		"mime":             req.MIME,
		"page_timeout_sec": timeout.Seconds(),
		"max_attempts":     attempts,
	})
	if err != nil {
		return ConvertResult{ErrorClass: ClassInternal}, nil
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout*time.Duration(attempts)*time.Duration(pages))
	defer cancel()
	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, h.URL+"/v1/convert", bytes.NewReader(payload))
	if err != nil {
		return ConvertResult{}, fmt.Errorf("%w", errSidecarUnreachable)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	client := h.Client
	if client == nil {
		client = &http.Client{}
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return ConvertResult{}, err
		}
		return ConvertResult{}, fmt.Errorf("%w", errSidecarUnreachable)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxConvertResponse+1))
	if err != nil {
		return ConvertResult{ErrorClass: ClassInternal}, nil
	}
	if int64(len(body)) > maxConvertResponse {
		return ConvertResult{ErrorClass: ClassTooLarge}, nil
	}
	var parsed struct {
		Markdown   string `json:"markdown"`
		PageCount  int    `json:"page_count"`
		OCREngine  string `json:"ocr_engine"`
		OCRLang    string `json:"ocr_lang"`
		ErrorClass string `json:"error_class"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ConvertResult{ErrorClass: ClassInternal}, nil
	}
	if resp.StatusCode == http.StatusServiceUnavailable && parsed.ErrorClass == "" {
		parsed.ErrorClass = ClassNotConfigured
	}
	if resp.StatusCode >= 500 && parsed.ErrorClass == "" {
		parsed.ErrorClass = ClassInternal
	}
	return ConvertResult{
		Markdown:   parsed.Markdown,
		PageCount:  parsed.PageCount,
		OCREngine:  parsed.OCREngine,
		OCRLang:    parsed.OCRLang,
		ErrorClass: parsed.ErrorClass,
	}, nil
}

func unreachableConvert(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errSidecarUnreachable) {
		return true
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return true
	}
	var op *net.OpError
	if errors.As(err, &op) {
		return true
	}
	return false
}
