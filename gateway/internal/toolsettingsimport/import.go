// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Package toolsettingsimport folds GoClaw builtin_tools /
// builtin_tool_tenant_configs media settings (provider chains) into goso's
// per-tenant builtin_tool_settings. Provider names are mapped to goso
// providers of the destination tenant; entries whose provider does not
// resolve are dropped and reported.
package toolsettingsimport

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"

	"github.com/mqglobal/goso/gateway/internal/builtin"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// MaxExportBytes caps the export read.
const MaxExportBytes = 32 << 20

// ToolRow is one builtin_tools row (global default settings).
type ToolRow struct {
	Name     string          `json:"name"`
	Settings json.RawMessage `json:"settings"`
}

// TenantRow is one builtin_tool_tenant_configs row.
type TenantRow struct {
	ToolName string          `json:"tool_name"`
	TenantID string          `json:"tenant_id"`
	Enabled  *bool           `json:"enabled"`
	Settings json.RawMessage `json:"settings"`
}

// ProviderRow is an llm_providers row, used only to turn an entry's
// provider_id into a name. Any other column is ignored.
type ProviderRow struct {
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
}

// Export is the GoClaw dump: {"builtin_tools":[...],
// "builtin_tool_tenant_configs":[...], "llm_providers":[...]}.
type Export struct {
	BuiltinTools  []ToolRow     `json:"builtin_tools"`
	TenantConfigs []TenantRow   `json:"builtin_tool_tenant_configs"`
	Providers     []ProviderRow `json:"llm_providers"`
}

// ReadExport decodes one JSON export.
func ReadExport(r io.Reader) (*Export, error) {
	raw, err := io.ReadAll(io.LimitReader(r, MaxExportBytes+1))
	if err != nil {
		return nil, errors.New("read export failed")
	}
	if len(raw) > MaxExportBytes {
		return nil, errors.New("export too large")
	}
	var ex Export
	if err := json.Unmarshal(raw, &ex); err != nil {
		return nil, errors.New("export is not valid JSON")
	}
	if len(ex.BuiltinTools) == 0 && len(ex.TenantConfigs) == 0 {
		return nil, errors.New("export has no builtin_tools or builtin_tool_tenant_configs")
	}
	return &ex, nil
}

// Options selects tenants and the provider map.
type Options struct {
	SourceTenant string
	DestTenant   string
	Apply        bool
	Overwrite    bool
	Deny         map[string]struct{}
	// ProviderMap maps a GoClaw provider name to a goso provider name.
	// Unlisted names map to themselves.
	ProviderMap map[string]string
	// EnvProviders are goso env-registry provider names that exist on the
	// server (the importer's own env may differ).
	EnvProviders map[string]struct{}
}

// ToolResult is the outcome for one tool.
type ToolResult struct {
	Tool      string   `json:"tool"`
	Source    string   `json:"source,omitempty"` // tenant | global
	Action    string   `json:"action"`           // insert | update | skip
	Reason    string   `json:"reason,omitempty"`
	Providers []string `json:"providers,omitempty"` // goso provider/model kept, in order
}

// Unmapped is a chain entry dropped because its provider has no goso
// provider in the destination tenant.
type Unmapped struct {
	Tool     string `json:"tool"`
	Provider string `json:"provider"`
	Model    string `json:"model,omitempty"`
}

// Report never carries a key or a params value.
type Report struct {
	DryRun       bool         `json:"dry_run"`
	SourceTenant string       `json:"source_tenant"`
	DestTenant   string       `json:"dest_tenant"`
	Tools        []ToolResult `json:"tools"`
	Unmapped     []Unmapped   `json:"unmapped_providers"`
	Warnings     []string     `json:"warnings,omitempty"`
	NotImported  []string     `json:"not_imported"`
}

// ConfigError is a bad option or input; nothing was written.
type ConfigError struct{ Msg string }

func (e *ConfigError) Error() string { return e.Msg }

func errConfig(msg string) error { return &ConfigError{Msg: msg} }

// goclawChainTools are the GoClaw tool names with provider chains that goso
// has. Their goso settings name is the same.
var goclawChainTools = []string{"read_image", "read_document", "read_video", "read_audio", "create_image"}

// Import maps the chains of opt.SourceTenant (tenant override, else the
// global default) into opt.DestTenant.
func Import(st store.StoreIface, ex *Export, opt Options) (*Report, error) {
	if st == nil || ex == nil {
		return nil, errConfig("export is empty")
	}
	ts, ok := st.(store.ToolSettingsStore)
	if !ok {
		return nil, errConfig("store has no tool settings")
	}
	opt.SourceTenant = strings.TrimSpace(opt.SourceTenant)
	opt.DestTenant = strings.TrimSpace(opt.DestTenant)
	if !plainToken(opt.SourceTenant) {
		return nil, errConfig("source tenant is required")
	}
	if !store.TenantOK(opt.DestTenant) {
		return nil, errConfig("dest tenant is invalid")
	}
	if denied(opt.Deny, opt.SourceTenant) || denied(opt.Deny, opt.DestTenant) {
		return nil, errConfig("tenant is denied")
	}
	rep := &Report{DryRun: !opt.Apply, SourceTenant: opt.SourceTenant, DestTenant: opt.DestTenant, Tools: []ToolResult{}, Unmapped: []Unmapped{}, NotImported: notImported()}

	global := map[string]json.RawMessage{}
	other := map[string]bool{}
	for _, r := range ex.BuiltinTools {
		name := strings.TrimSpace(r.Name)
		if isChainTool(name) {
			global[name] = r.Settings
		} else if name != "" {
			other[name] = true
		}
	}
	tenant := map[string]json.RawMessage{}
	for _, r := range ex.TenantConfigs {
		if !strings.EqualFold(strings.TrimSpace(r.TenantID), opt.SourceTenant) {
			continue
		}
		name := strings.TrimSpace(r.ToolName)
		if r.Enabled != nil && !*r.Enabled {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s is disabled for the source tenant; goso tool flags are set per agent (PATCH /api/agents/{id}/tools/%s)", name, name))
		}
		if !isChainTool(name) {
			if name != "" && !isNullJSON(r.Settings) {
				other[name] = true
			}
			continue
		}
		if !isNullJSON(r.Settings) {
			tenant[name] = r.Settings
		}
	}
	providerNames := map[string]string{}
	for _, p := range ex.Providers {
		if id := strings.TrimSpace(p.ID); id != "" && strings.EqualFold(strings.TrimSpace(p.TenantID), opt.SourceTenant) {
			providerNames[id] = strings.TrimSpace(p.Name)
		}
	}

	for _, tool := range goclawChainTools {
		raw, source := tenant[tool], "tenant"
		if raw == nil {
			raw, source = global[tool], "global"
		}
		if isNullJSON(raw) {
			continue
		}
		res := ToolResult{Tool: tool, Source: source}
		entries, err := parseGoClawChain(raw)
		if err != nil {
			res.Action, res.Reason = "skip", "invalid-settings"
			rep.Tools = append(rep.Tools, res)
			continue
		}
		var kept []builtin.MediaProviderEntry
		for _, e := range entries {
			name := e.Provider
			if name == "" {
				name = providerNames[e.ProviderID]
			}
			if name == "" {
				rep.Unmapped = append(rep.Unmapped, Unmapped{Tool: tool, Provider: "(provider_id only)", Model: e.Model})
				continue
			}
			target := name
			if m, ok := opt.ProviderMap[name]; ok {
				target = m
			}
			status := builtin.ChainProviderStatus(st, opt.DestTenant, target)
			if _, ok := opt.EnvProviders[target]; ok && status == "" {
				status = "env"
			}
			switch status {
			case "":
				rep.Unmapped = append(rep.Unmapped, Unmapped{Tool: tool, Provider: name, Model: e.Model})
				continue
			case "disabled":
				rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s: provider %q is disabled in goso; the chain skips it until it is enabled", tool, target))
			}
			for k := range e.Params {
				if builtin.SecretParamKey(k) {
					delete(e.Params, k)
					rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s: dropped params.%s (keys stay in the provider secret)", tool, k))
				}
			}
			e.Provider = target
			e.ProviderID = ""
			kept = append(kept, e)
		}
		if len(kept) > builtin.MaxChainProviders {
			kept = kept[:builtin.MaxChainProviders]
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s: chain cut to %d providers", tool, builtin.MaxChainProviders))
		}
		if len(kept) == 0 {
			res.Action, res.Reason = "skip", "no-mapped-providers"
			rep.Tools = append(rep.Tools, res)
			continue
		}
		settings := builtin.MediaToolSettings{Providers: kept}
		if err := builtin.ValidateMediaSettings(&settings); err != nil {
			res.Action, res.Reason = "skip", "invalid-settings"
			rep.Tools = append(rep.Tools, res)
			continue
		}
		for _, e := range kept {
			n := e.Provider
			if e.Model != "" {
				n += "/" + e.Model
			}
			if !e.On() {
				n += " (disabled)"
			}
			res.Providers = append(res.Providers, n)
		}
		_, gerr := ts.GetToolSettings(opt.DestTenant, tool)
		exists := gerr == nil
		switch {
		case exists && !opt.Overwrite:
			res.Action, res.Reason = "skip", "exists"
			rep.Tools = append(rep.Tools, res)
			continue
		case exists:
			res.Action = "update"
		default:
			res.Action = "insert"
		}
		if opt.Apply {
			canon, _ := json.Marshal(settings)
			if _, err := ts.PutToolSettings(store.ToolSettings{TenantID: opt.DestTenant, Tool: tool, Settings: canon}); err != nil {
				return nil, fmt.Errorf("write %s settings failed", tool)
			}
		}
		rep.Tools = append(rep.Tools, res)
	}
	names := make([]string, 0, len(other))
	for n := range other {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		rep.NotImported = append(rep.NotImported, "builtin tool settings of "+n+" (no goso provider chain)")
	}
	return rep, nil
}

// parseGoClawChain reads {"providers":[...]} leniently: settings may be a
// JSON string, an entry without "enabled" is off (GoClaw semantics), and
// unknown fields are ignored.
func parseGoClawChain(raw json.RawMessage) ([]builtin.MediaProviderEntry, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) > 0 && raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
		raw = json.RawMessage(s)
	}
	var doc struct {
		Providers []struct {
			ProviderID string         `json:"provider_id"`
			Provider   string         `json:"provider"`
			Model      string         `json:"model"`
			Enabled    *bool          `json:"enabled"`
			Timeout    int            `json:"timeout"`
			MaxRetries int            `json:"max_retries"`
			Params     map[string]any `json:"params"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	out := make([]builtin.MediaProviderEntry, 0, len(doc.Providers))
	for _, p := range doc.Providers {
		on := p.Enabled != nil && *p.Enabled
		e := builtin.MediaProviderEntry{
			ProviderID: strings.TrimSpace(p.ProviderID),
			Provider:   strings.TrimSpace(p.Provider),
			Model:      strings.TrimSpace(p.Model),
			Enabled:    &on,
			Timeout:    clamp(p.Timeout, 0, builtin.MaxChainTimeout),
			MaxRetries: clamp(p.MaxRetries, 0, builtin.MaxChainRetries),
			Params:     p.Params,
		}
		out = append(out, e)
	}
	return out, nil
}

func notImported() []string {
	return []string{
		"builtin_tools.enabled and builtin_tool_tenant_configs.enabled (goso tool flags are per agent)",
		"per-agent media provider overrides",
		"provider_id (goso resolves providers by name)",
	}
}

func isChainTool(name string) bool {
	for _, t := range goclawChainTools {
		if t == name {
			return true
		}
	}
	return false
}

func isNullJSON(raw json.RawMessage) bool {
	t := strings.TrimSpace(string(raw))
	return t == "" || t == "null" || t == `""` || t == "{}"
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func denied(set map[string]struct{}, id string) bool {
	if len(set) == 0 {
		return false
	}
	_, ok := set[strings.ToLower(strings.TrimSpace(id))]
	return ok
}

func plainToken(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || r == '/' || r == '\\' || r == ' ' {
			return false
		}
	}
	return true
}
