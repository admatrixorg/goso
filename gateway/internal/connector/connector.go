// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// ErrUnavailable is the sentinel returned when a connector is disabled or offline.
// JSON APIs should surface the string "connector_unavailable".
var ErrUnavailable = errors.New("connector_unavailable")

const (
	TransportMCPHTTP  = "mcp-http"
	TransportMCPStdio = "mcp-stdio"
	TransportHTTP     = "http"

	DefaultCRMURL = "http://127.0.0.1:8089"
)

// DefaultCRMEndpoint returns GOSOCRM_API_URL or the local CRM default.
func DefaultCRMEndpoint() string {
	if v := strings.TrimSpace(os.Getenv("GOSOCRM_API_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return DefaultCRMURL
}

// Connector is the contract every remote system adapter implements.
type Connector interface {
	Name() string
	ListTools(ctx context.Context) ([]Tool, error)
	Invoke(ctx context.Context, tool string, args map[string]any) (*InvokeResult, error)
	Health(ctx context.Context) error
}

// InvokeResult is a successful (or pending) tool call payload.
type InvokeResult struct {
	Tool             string          `json:"tool"`
	Connector        string          `json:"connector"`
	Content          any             `json:"content,omitempty"`
	Raw              json.RawMessage `json:"raw,omitempty"`
	Latency          time.Duration   `json:"-"`
	LatencyMS        int64           `json:"latency_ms,omitempty"`
	Status           string          `json:"status,omitempty"` // ok | pending_approval | error
	ApprovalID       string          `json:"approval_id,omitempty"`
	RequiresApproval bool            `json:"requires_approval,omitempty"`
}

// Config describes how to construct a runtime connector.
type Config struct {
	Name          string          `json:"name"`
	Transport     string          `json:"transport"`
	Endpoint      string          `json:"endpoint"`
	BearerToken   string          `json:"-"`
	CredentialRef string          `json:"credential_ref,omitempty"`
	SchemaVersion string          `json:"schema_version,omitempty"`
	ManifestURL   string          `json:"manifest_url,omitempty"`
	ManifestJSON  json.RawMessage `json:"manifest,omitempty"`
	Timeout       time.Duration   `json:"-"`
	TimeoutMS     int             `json:"timeout_ms,omitempty"`
	Retries       int             `json:"retries,omitempty"`
	// Client and Stdio are test hooks (in-process; no live remote).
	Client *http.Client
	Stdio  *StdioPipes
}

// Build constructs a Connector from config and wraps it with timeout/retry.
func Build(cfg Config) (Connector, error) {
	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		return nil, errors.New("connector name is required")
	}
	transport := strings.TrimSpace(cfg.Transport)
	if transport == "" {
		transport = TransportHTTP
	}
	if cfg.Timeout == 0 && cfg.TimeoutMS > 0 {
		cfg.Timeout = time.Duration(cfg.TimeoutMS) * time.Millisecond
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	cfg.Name = name
	cfg.Transport = transport
	if cfg.Endpoint == "" && (transport == TransportHTTP || transport == TransportMCPHTTP) {
		if name == "zalocrm" {
			cfg.Endpoint = DefaultCRMEndpoint()
		}
	}
	if cfg.BearerToken == "" && cfg.CredentialRef != "" {
		cfg.BearerToken = os.Getenv(cfg.CredentialRef)
	}

	var inner Connector
	var err error
	switch transport {
	case TransportHTTP:
		inner, err = newHTTPConnector(cfg)
	case TransportMCPHTTP:
		inner, err = newMCPHTTP(cfg)
	case TransportMCPStdio:
		inner, err = newMCPStdio(cfg)
	default:
		return nil, fmt.Errorf("unknown transport %q", transport)
	}
	if err != nil {
		return nil, err
	}
	return WithRetry(inner, RetryConfig{Timeout: cfg.Timeout, Retries: cfg.Retries}), nil
}

func unavailable(err error) error {
	if err == nil {
		return ErrUnavailable
	}
	if errors.Is(err, ErrUnavailable) {
		return err
	}
	return fmt.Errorf("%w: %v", ErrUnavailable, err)
}
