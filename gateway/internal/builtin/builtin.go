// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

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
		Description:      "Web search. Returns {provider, results:[{title,url,snippet}]}. Tries Brave, Exa, Tavily (API keys from env) in the GOSO_WEB_SEARCH order and falls back to DuckDuckGo Instant Answer. Fail-closed unless GOSO_WEB_SEARCH is set and the UI flag is on. Optional count 1-8.",
		RequiresApproval: false,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"q":{"type":"string","description":"search query"},"count":{"type":"integer","minimum":1,"maximum":8}},"required":["q"]}`),
	},
	{
		Name:             ToolWebFetch,
		Description:      "HTTP GET a URL. Body capped at 1MiB as a string (no HTML-to-Markdown). SSRF via GOSO_SSRF is the only network policy.",
		RequiresApproval: false,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"url":{"type":"string"}},"required":["url"]}`),
	},
	{
		Name:             ToolSandbox,
		Description:      "Docker sandbox. Runs only when GOSO_SANDBOX_IMAGE is set and docker is on PATH. docker run --rm --network=none --memory=256m, 15s timeout, cmd as argv (never sh -c). Else not_configured.",
		RequiresApproval: true,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"cmd":{},"args":{"type":"array","items":{"type":"string"}}},"required":["cmd"]}`),
	},
	{
		Name:             ToolBrowser,
		Description:      "Headless browser. Runs only when GOSO_BROWSER_BIN or CHROME_PATH is an existing file. CheckURL then --headless --disable-gpu --no-sandbox, 20s timeout, stdout cap 64KiB. Else not_configured.",
		RequiresApproval: true,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"url":{"type":"string"}},"required":["url"]}`),
	},
	{
		Name:             ToolMedia,
		Description:      "Local ffmpeg health or workspace-jailed transcode {in,out}. Runs when GOSO_FFMPEG is an existing file, or ffmpeg is on PATH and GOSO_MEDIA=1. Missing ffmpeg → not_configured. Never a paid API.",
		RequiresApproval: true,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"in":{"type":"string"},"out":{"type":"string"}}}`),
	},
	// image_gen and tts live in media_tools.go (mediaCatalog).
	{
		Name:             ToolUseSkill,
		Description:      "Read a local SKILL.md by folder name for this agent's tenant. A tenant file overrides a shared root skill. Fail-closed unless GOSO_SKILLS_DIR is set. Never executes skill scripts.",
		RequiresApproval: false,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`),
	},
	{
		Name:             ToolSkillSearch,
		Description:      "BM25 search over this tenant's skills (shared root plus that tenant's own files). Returns at most 5 ranked hits. Empty query fail-closed. Never executes skill scripts.",
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
	{
		Name:             ToolSearch,
		Description:      "Case-insensitive substring search in file contents under GOSO_WORKSPACE. Optional relative path. Skips dirs, files over 1MiB, and NUL in the first 512 bytes. Cap 50 hits. Fail-closed unless GOSO_WORKSPACE is set. Read-only, no exec.",
		RequiresApproval: false,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"},"path":{"type":"string"}},"required":["q"]}`),
	},
	{
		Name:             ToolGlob,
		Description:      "Match slash-normalized relative paths (and basenames) under GOSO_WORKSPACE with filepath.Match. Recursive. Cap 256. Fail-closed unless GOSO_WORKSPACE is set. Read-only, no exec.",
		RequiresApproval: false,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"}},"required":["pattern"]}`),
	},
	{
		Name:             ToolDatetime,
		Description:      "Current date and time. Optional timezone (IANA name, default Asia/Ho_Chi_Minh). Returns iso, date, time, weekday, utc_offset, unix. Read-only, no network.",
		RequiresApproval: false,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"timezone":{"type":"string","description":"IANA timezone, e.g. Asia/Ho_Chi_Minh"}}}`),
	},
	{
		Name:             ToolMessage,
		Description:      "Send a text message to a chat on one of this agent's channels (Telegram, Zalo Bot, Lark). Omit channel and chat_id to write to the current conversation. Only channels of this tenant that are bound to this agent, enabled and running, and only chats that already talked to this agent. Requires approval.",
		RequiresApproval: true,
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"},"channel":{"type":"string","description":"channel name <tenant>/<type>/<name>; default: current conversation"},"chat_id":{"type":"string","description":"chat id on that channel; default: current conversation"}},"required":["text"]}`),
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

// RequiresApproval reports whether the builtin catalog gates this tool.
func RequiresApproval(name string) bool {
	name = strings.TrimSpace(name)
	for _, s := range catalog {
		if s.Name == name {
			return s.RequiresApproval
		}
	}
	return false
}

// Configured reports whether a builtin is actually runnable (env + hooks),
// independent of the UI enabled flag.
func Configured(name string) bool {
	switch strings.TrimSpace(name) {
	case ToolReadFile, ToolWriteFile, ToolListFiles, ToolEdit, ToolSendFile, ToolSearch, ToolGlob:
		return workspaceConfigured()
	case ToolWebSearch:
		return webSearchConfigured()
	case ToolDatetime:
		return true
	case ToolMessage:
		return MessageSender != nil
	case ToolWebFetch:
		return true
	case ToolUseSkill, ToolSkillSearch:
		return skill.Configured()
	case ToolSandbox:
		return sandboxConfigured()
	case ToolBrowser:
		return browserBin() != ""
	case ToolMedia:
		return ffmpegBin() != ""
	case ToolImageGen, ToolTTS, ToolCreateImage, ToolReadImage, ToolReadDocument, ToolReadAudio, ToolSTT:
		return mediaToolConfigured(name)
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

func notConfigured(name string) *connector.InvokeResult {
	return &connector.InvokeResult{
		Tool:      name,
		Connector: ConnectorName,
		Status:    "not_configured",
		Content:   map[string]any{"error": "not_configured"},
	}
}

// Invoke runs a builtin tool.
// web_search networks only when the UI flag is on and GOSO_WEB_SEARCH is set.
// message needs a MessageSender (wired by serve) and always goes through approval.
// web_fetch always runs; SSRF (security.CheckURL + GuardClient) is the only network policy.
// Filesystem tools fail-closed unless GOSO_WORKSPACE is set; they never exec.
// sandbox runs docker only when GOSO_SANDBOX_IMAGE is set and docker exists; else not_configured.
// browser launches only when GOSO_BROWSER_BIN or CHROME_PATH is an existing file; else not_configured.
// media runs ffmpeg when GOSO_FFMPEG (or PATH ffmpeg + GOSO_MEDIA=1) exists. read_image,
// read_document, read_audio/stt, create_image/image_gen and tts: see media_tools.go.
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
	case ToolWebFetch:
		return webFetch(ctx, args)
	case ToolDatetime:
		return invokeDatetime(args)
	case ToolMessage:
		return invokeMessage(ctx, args)
	case ToolUseSkill:
		return useSkill(ctx, args)
	case ToolSkillSearch:
		return skillSearch(ctx, args)
	case ToolReadFile:
		return readFile(ctx, args)
	case ToolWriteFile:
		return writeFile(ctx, args)
	case ToolListFiles:
		return listFiles(ctx, args)
	case ToolEdit:
		return editFile(ctx, args)
	case ToolSendFile:
		return sendFile(ctx, args)
	case ToolSearch:
		return searchFile(ctx, args)
	case ToolGlob:
		return globFiles(ctx, args)
	case ToolSandbox:
		return invokeSandbox(ctx, args)
	case ToolBrowser:
		return invokeBrowser(ctx, args)
	case ToolImageGen, ToolTTS, ToolCreateImage, ToolReadImage, ToolReadDocument, ToolReadAudio, ToolSTT:
		return invokeMediaTool(ctx, name, args, uiEnabled)
	default:
		if isMediaName(name) {
			return invokeMedia(ctx, name, args)
		}
		return notConfigured(name), nil
	}
}

func invokeMedia(ctx context.Context, name string, args map[string]any) (*connector.InvokeResult, error) {
	if name == ToolMedia {
		if bin := ffmpegBin(); bin != "" {
			return runFFmpeg(ctx, bin, args)
		}
	}
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

func useSkill(ctx context.Context, args map[string]any) (*connector.InvokeResult, error) {
	if !skill.Configured() {
		return notConfigured(ToolUseSkill), nil
	}
	n := skillNameArg(args)
	doc, err := skill.Load(skill.TenantFrom(ctx), n)
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

func skillSearch(ctx context.Context, args map[string]any) (*connector.InvokeResult, error) {
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
	hits, err := skill.Search(skill.TenantFrom(ctx), q)
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
