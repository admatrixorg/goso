// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/connector"
)

// ConnectorName is the synthetic connector id for built-in tools.
const ConnectorName = "builtin"

const (
	ToolWebSearch = "web_search"
	ToolSandbox   = "sandbox"
	ToolBrowser   = "browser"
	ToolMedia     = "media"
)

// InstantAnswerBase is the DuckDuckGo Instant Answer endpoint.
// Tests replace this with an httptest URL. Never call live 20127 from tests.
var InstantAnswerBase = "https://api.duckduckgo.com/"

// InstantAnswerClient is the HTTP client for web_search. Nil → 10s default.
var InstantAnswerClient *http.Client

// Spec is one advertised builtin tool.
type Spec struct {
	Name             string
	Description      string
	RequiresApproval bool
	InputSchema      json.RawMessage
}

var catalog = []Spec{
	{
		Name:             ToolWebSearch,
		Description:      "DuckDuckGo Instant Answer search. Fail-closed unless GOSO_WEB_SEARCH=ddg|1 and the UI flag is on.",
		RequiresApproval: false,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}},"required":["q"]}`),
	},
	{
		Name:             ToolSandbox,
		Description:      "Code sandbox stub. Never spawns processes.",
		RequiresApproval: true,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{}}`),
	},
	{
		Name:             ToolBrowser,
		Description:      "Browser overlay stub. Never launches Chrome.",
		RequiresApproval: true,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{}}`),
	},
	{
		Name:             ToolMedia,
		Description:      "Media overlay stub. Never runs ffmpeg or downloads.",
		RequiresApproval: true,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{}}`),
	},
}

// Catalog returns the builtin tool list (always advertised).
func Catalog() []Spec {
	out := make([]Spec, len(catalog))
	copy(out, catalog)
	return out
}

// IsName reports whether name is a builtin tool.
func IsName(name string) bool {
	switch strings.TrimSpace(name) {
	case ToolWebSearch, ToolSandbox, ToolBrowser, ToolMedia:
		return true
	}
	return false
}

// Tools converts the catalog to connector.Tool values.
func Tools() []connector.Tool {
	out := make([]connector.Tool, 0, len(catalog))
	for _, s := range catalog {
		out = append(out, connector.Tool{
			Name:             s.Name,
			Description:      s.Description,
			InputSchema:      s.InputSchema,
			RequiresApproval: s.RequiresApproval,
		})
	}
	return out
}

// WebSearchNetworkAllowed is the env gate (independent of the UI flag).
func WebSearchNetworkAllowed() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("GOSO_WEB_SEARCH")))
	return v == "ddg" || v == "1"
}

func notConfigured(name string) *connector.InvokeResult {
	return &connector.InvokeResult{
		Tool:      name,
		Connector: ConnectorName,
		Status:    "not_configured",
		Content:   map[string]any{"error": "not_configured"},
	}
}

// Invoke runs a builtin tool. sandbox/browser/media never spawn.
// web_search networks only when the UI flag is on and GOSO_WEB_SEARCH=ddg|1.
func Invoke(ctx context.Context, name string, args map[string]any, uiEnabled bool) (*connector.InvokeResult, error) {
	name = strings.TrimSpace(name)
	if !IsName(name) {
		return notConfigured(name), nil
	}
	switch name {
	case ToolWebSearch:
		if !uiEnabled || !WebSearchNetworkAllowed() {
			return notConfigured(name), nil
		}
		return webSearch(ctx, args)
	default:
		// sandbox, browser, media: persist UI flags but never exec/docker/chrome/ffmpeg.
		return notConfigured(name), nil
	}
}

func queryArg(args map[string]any) string {
	if args == nil {
		return ""
	}
	for _, k := range []string{"q", "query"} {
		if v, ok := args[k]; ok {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func webSearch(ctx context.Context, args map[string]any) (*connector.InvokeResult, error) {
	q := queryArg(args)
	if q == "" {
		return &connector.InvokeResult{
			Tool:      ToolWebSearch,
			Connector: ConnectorName,
			Status:    "error",
			Content:   map[string]any{"error": "q is required"},
		}, nil
	}
	base := InstantAnswerBase
	if strings.TrimSpace(base) == "" {
		base = "https://api.duckduckgo.com/"
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	qs := u.Query()
	qs.Set("q", q)
	qs.Set("format", "json")
	qs.Set("no_html", "1")
	qs.Set("skip_disambig", "1")
	u.RawQuery = qs.Encode()

	client := InstantAnswerClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("web_search %d", resp.StatusCode)
	}
	var ddg struct {
		AbstractText  string `json:"AbstractText"`
		Abstract      string `json:"Abstract"`
		Answer        string `json:"Answer"`
		Heading       string `json:"Heading"`
		RelatedTopics []struct {
			Text string `json:"Text"`
		} `json:"RelatedTopics"`
	}
	if err := json.Unmarshal(body, &ddg); err != nil {
		return nil, err
	}
	related := make([]string, 0, 3)
	for _, t := range ddg.RelatedTopics {
		if s := strings.TrimSpace(t.Text); s != "" {
			related = append(related, s)
			if len(related) == 3 {
				break
			}
		}
	}
	abstract := strings.TrimSpace(ddg.AbstractText)
	if abstract == "" {
		abstract = strings.TrimSpace(ddg.Abstract)
	}
	return &connector.InvokeResult{
		Tool:      ToolWebSearch,
		Connector: ConnectorName,
		Status:    "ok",
		Content: map[string]any{
			"heading":  ddg.Heading,
			"answer":   ddg.Answer,
			"abstract": abstract,
			"related":  related,
		},
		Raw: body,
	}, nil
}
