// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/skill"
)

// ConnectorName is the synthetic connector id for built-in tools.
const ConnectorName = "builtin"

const (
	ToolWebSearch   = "web_search"
	ToolSandbox     = "sandbox"
	ToolBrowser     = "browser"
	ToolMedia       = "media"
	ToolImageGen    = "image_gen"
	ToolTTS         = "tts"
	ToolUseSkill    = "use_skill"
	ToolSkillSearch = "skill_search"
	maxSearchHits   = 8
)

// InstantAnswerBase is the DuckDuckGo Instant Answer endpoint.
// Tests replace this with an httptest URL. Never call live DDG from tests.
// Empty base is fail-closed (not_configured), never a live fallback.
var InstantAnswerBase = "https://api.duckduckgo.com/"

// InstantAnswerClient is the HTTP client for web_search. Nil → 10s default.
var InstantAnswerClient *http.Client

// MediaInvoke is an optional test double. Production stays nil so media
// tools never call a paid API. Both this and GOSO_MEDIA*=1 are required.
var MediaInvoke func(ctx context.Context, name string, args map[string]any) (*connector.InvokeResult, error)

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
		Description:      "Media overlay stub. Fail-closed not_configured unless GOSO_MEDIA*=1 and a test double is injected. Never calls a paid API.",
		RequiresApproval: true,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{}}`),
	},
	{
		Name:             ToolImageGen,
		Description:      "Image generation stub. Fail-closed not_configured unless GOSO_MEDIA*=1 and a test double is injected. Never calls a paid API.",
		RequiresApproval: true,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"prompt":{"type":"string"}}}`),
	},
	{
		Name:             ToolTTS,
		Description:      "Text-to-speech stub. Fail-closed not_configured unless GOSO_MEDIA*=1 and a test double is injected. Never calls a paid API.",
		RequiresApproval: true,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`),
	},
	{
		Name:             ToolUseSkill,
		Description:      "Read a local SKILL.md by folder name. Fail-closed unless GOSO_SKILLS_DIR is set. Never executes skill scripts.",
		RequiresApproval: false,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`),
	},
	{
		Name:             ToolSkillSearch,
		Description:      "BM25 search over local SKILL.md name, description, and body. Returns at most 5 ranked hits. Empty query fail-closed. Never executes skill scripts.",
		RequiresApproval: false,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"q":{"type":"string"}},"required":["query"]}`),
	},
	{
		Name:             ToolReadFile,
		Description:      "Read a file inside GOSO_WORKSPACE. Fail-closed unless GOSO_WORKSPACE is set. Cap 1MiB. No exec.",
		RequiresApproval: false,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
	},
	{
		Name:             ToolWriteFile,
		Description:      "Write a file inside GOSO_WORKSPACE. Fail-closed unless GOSO_WORKSPACE is set. Creates parent dirs in-jail only. Requires approval. No exec or delete.",
		RequiresApproval: true,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`),
	},
	{
		Name:             ToolListFiles,
		Description:      "List a directory inside GOSO_WORKSPACE. Fail-closed unless GOSO_WORKSPACE is set. Path traversal is denied. No exec.",
		RequiresApproval: false,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
	},
	{
		Name:             ToolEdit,
		Description:      "Replace one occurrence of old with new inside a GOSO_WORKSPACE file. Fail-closed unless GOSO_WORKSPACE is set. Requires approval. No exec or delete.",
		RequiresApproval: true,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"old":{"type":"string"},"new":{"type":"string"}},"required":["path","old","new"]}`),
	},
	{
		Name:             ToolSendFile,
		Description:      "Return {path, bytes, mime} for a file inside GOSO_WORKSPACE. Metadata only — never uploads off-box. Fail-closed unless GOSO_WORKSPACE is set.",
		RequiresApproval: false,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
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
	name = strings.TrimSpace(name)
	for _, s := range catalog {
		if s.Name == name {
			return true
		}
	}
	return false
}

// Configured reports whether a builtin is actually runnable (env + hooks),
// independent of the UI enabled flag.
func Configured(name string) bool {
	switch strings.TrimSpace(name) {
	case ToolReadFile, ToolWriteFile, ToolListFiles, ToolEdit, ToolSendFile:
		return workspaceConfigured()
	case ToolWebSearch:
		return WebSearchNetworkAllowed() && strings.TrimSpace(InstantAnswerBase) != ""
	case ToolUseSkill, ToolSkillSearch:
		return skill.Configured()
	case ToolMedia, ToolImageGen, ToolTTS:
		return MediaEnvAllowed() && MediaInvoke != nil
	default:
		return false
	}
}

// MediaEnvAllowed is GOSO_MEDIA=1 or any GOSO_MEDIA_*=1. Env alone never
// calls a vendor; a process-injected MediaInvoke double is also required.
func MediaEnvAllowed() bool {
	if envFlagOn(os.Getenv("GOSO_MEDIA")) {
		return true
	}
	for _, e := range os.Environ() {
		k, v, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		if strings.HasPrefix(k, "GOSO_MEDIA_") && envFlagOn(v) {
			return true
		}
	}
	return false
}

func envFlagOn(v string) bool {
	v = strings.TrimSpace(strings.ToLower(v))
	return v == "1" || v == "on" || v == "true"
}

func isMediaName(name string) bool {
	switch name {
	case ToolMedia, ToolImageGen, ToolTTS:
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

// Invoke runs a builtin tool. sandbox/browser never spawn (DI-12/13).
// web_search networks only when the UI flag is on and GOSO_WEB_SEARCH=ddg|1.
// Filesystem tools fail-closed unless GOSO_WORKSPACE is set; they never exec.
// Media stays not_configured unless GOSO_MEDIA*=1 and MediaInvoke is set.
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
	case ToolUseSkill:
		return useSkill(args)
	case ToolSkillSearch:
		return skillSearch(args)
	case ToolReadFile:
		return readFile(args)
	case ToolWriteFile:
		return writeFile(args)
	case ToolListFiles:
		return listFiles(args)
	case ToolEdit:
		return editFile(args)
	case ToolSendFile:
		return sendFile(args)
	default:
		if isMediaName(name) {
			return invokeMedia(ctx, name, args)
		}
		// sandbox/browser (DI-12/13): persist UI flags but never exec/docker/chrome.
		return notConfigured(name), nil
	}
}

func invokeMedia(ctx context.Context, name string, args map[string]any) (*connector.InvokeResult, error) {
	if MediaInvoke == nil || !MediaEnvAllowed() {
		return notConfigured(name), nil
	}
	res, err := MediaInvoke(ctx, name, args)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return notConfigured(name), nil
	}
	return res, nil
}

func skillNameArg(args map[string]any) string {
	if args == nil {
		return ""
	}
	for _, k := range []string{"name", "skill"} {
		if v, ok := args[k]; ok {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func useSkill(args map[string]any) (*connector.InvokeResult, error) {
	if !skill.Configured() {
		return notConfigured(ToolUseSkill), nil
	}
	n := skillNameArg(args)
	doc, err := skill.Load(n)
	if err != nil {
		status := "error"
		msg := "read failed"
		if errors.Is(err, skill.ErrNotConfigured) {
			return notConfigured(ToolUseSkill), nil
		}
		if errors.Is(err, skill.ErrNotFound) {
			status = "not_found"
			msg = "not_found"
		}
		if errors.Is(err, skill.ErrPathEscape) {
			msg = "path escape"
		}
		return &connector.InvokeResult{
			Tool:      ToolUseSkill,
			Connector: ConnectorName,
			Status:    status,
			Content:   map[string]any{"error": msg},
		}, nil
	}
	return &connector.InvokeResult{
		Tool:      ToolUseSkill,
		Connector: ConnectorName,
		Status:    "ok",
		Content: map[string]any{
			"name": doc.Name,
			"path": doc.Path,
			"body": doc.Body,
		},
	}, nil
}

func skillSearch(args map[string]any) (*connector.InvokeResult, error) {
	if !skill.Configured() {
		return notConfigured(ToolSkillSearch), nil
	}
	q := skillSearchQuery(args)
	if q == "" {
		return &connector.InvokeResult{
			Tool:      ToolSkillSearch,
			Connector: ConnectorName,
			Status:    "error",
			Content:   map[string]any{"error": "query is required"},
		}, nil
	}
	hits, err := skill.Search(q)
	if err != nil {
		if errors.Is(err, skill.ErrNotConfigured) {
			return notConfigured(ToolSkillSearch), nil
		}
		if errors.Is(err, skill.ErrPathEscape) {
			return &connector.InvokeResult{
				Tool:      ToolSkillSearch,
				Connector: ConnectorName,
				Status:    "error",
				Content:   map[string]any{"error": "path escape"},
			}, nil
		}
		return &connector.InvokeResult{
			Tool:      ToolSkillSearch,
			Connector: ConnectorName,
			Status:    "error",
			Content:   map[string]any{"error": "read failed"},
		}, nil
	}
	if hits == nil {
		hits = []skill.Hit{}
	}
	return &connector.InvokeResult{
		Tool:      ToolSkillSearch,
		Connector: ConnectorName,
		Status:    "ok",
		Content:   map[string]any{"skills": hits},
	}, nil
}

func skillSearchQuery(args map[string]any) string {
	if args == nil {
		return ""
	}
	for _, k := range []string{"query", "q"} {
		if v, ok := args[k]; ok {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
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
		// Empty query is fail-closed (not_configured), no network.
		return notConfigured(ToolWebSearch), nil
	}
	base := strings.TrimSpace(InstantAnswerBase)
	if base == "" {
		return notConfigured(ToolWebSearch), nil
	}
	u, err := url.Parse(base)
	if err != nil {
		return notConfigured(ToolWebSearch), nil
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
	results, err := mapSearchHits(body)
	if err != nil {
		return nil, err
	}
	return &connector.InvokeResult{
		Tool:      ToolWebSearch,
		Connector: ConnectorName,
		Status:    "ok",
		Content: map[string]any{
			"results": results,
		},
		Raw: body,
	}, nil
}

func mapSearchHits(body []byte) ([]map[string]any, error) {
	var ddg struct {
		AbstractText  string            `json:"AbstractText"`
		Abstract      string            `json:"Abstract"`
		AbstractURL   string            `json:"AbstractURL"`
		Answer        string            `json:"Answer"`
		Heading       string            `json:"Heading"`
		RelatedTopics []json.RawMessage `json:"RelatedTopics"`
		Results       []json.RawMessage `json:"Results"`
	}
	if err := json.Unmarshal(body, &ddg); err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, maxSearchHits)
	snippet := strings.TrimSpace(ddg.AbstractText)
	if snippet == "" {
		snippet = strings.TrimSpace(ddg.Abstract)
	}
	if snippet == "" {
		snippet = strings.TrimSpace(ddg.Answer)
	}
	title := strings.TrimSpace(ddg.Heading)
	url := strings.TrimSpace(ddg.AbstractURL)
	if title != "" || url != "" || snippet != "" {
		if title == "" {
			title = snippet
		}
		out = append(out, searchHit(title, url, snippet))
	}
	appendTopicHits(&out, ddg.Results)
	appendTopicHits(&out, ddg.RelatedTopics)
	return out, nil
}

func appendTopicHits(out *[]map[string]any, raw []json.RawMessage) {
	for _, r := range raw {
		if len(*out) >= maxSearchHits {
			return
		}
		var item struct {
			Text     string            `json:"Text"`
			FirstURL string            `json:"FirstURL"`
			Topics   []json.RawMessage `json:"Topics"`
		}
		if json.Unmarshal(r, &item) != nil {
			continue
		}
		text := strings.TrimSpace(item.Text)
		u := strings.TrimSpace(item.FirstURL)
		if text != "" || u != "" {
			title, snippet := splitTopicText(text)
			*out = append(*out, searchHit(title, u, snippet))
		}
		if len(item.Topics) > 0 {
			appendTopicHits(out, item.Topics)
		}
	}
}

func splitTopicText(text string) (title, snippet string) {
	text = strings.TrimSpace(text)
	if i := strings.Index(text, " - "); i > 0 {
		return strings.TrimSpace(text[:i]), strings.TrimSpace(text[i+3:])
	}
	return text, text
}

func searchHit(title, u, snippet string) map[string]any {
	return map[string]any{
		"title":   title,
		"url":     u,
		"snippet": snippet,
	}
}
