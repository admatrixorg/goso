// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package mcpimport

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode"
)

// maxExportBytes is the file cap. A larger file is refused before decode.
var maxExportBytes = 32 << 20

// ErrExportTooLarge is returned when the file is over maxExportBytes.
var ErrExportTooLarge = errors.New("export is too large")

// Export is a decoded file. Secret columns are reduced to presence flags
// during decode and are not retained.
type Export struct {
	servers            []serverInput
	agentGrants        []grantInput
	userGrants         []tenantInput
	builtinConfigs     []tenantInput
	userCredentials    []tenantInput
	contextCredentials []tenantInput
	builtinTools       int
}

type serverInput struct {
	ID              string
	Name            string
	Transport       string
	Endpoint        string
	TenantID        string
	TimeoutMS       int
	Bad             bool
	Control         bool
	EnabledBad      bool
	TimeoutBad      bool
	EndpointReason  string
	SecretHold      bool
	NeedsCredential bool
	HadDisplayName  bool
	HadArgs         bool
	HadHeaders      bool
	HadEnv          bool
	HadAPIKey       bool
	HadToolPrefix   bool
	HadSettings     bool
	HadCreatedBy    bool
	HadCreatedAt    bool
	HadUpdatedAt    bool
}

type grantInput struct {
	ServerID     string
	AgentID      string
	TenantID     string
	Enabled      bool
	Bad          bool
	Control      bool
	HadAllow     bool
	HadDeny      bool
	HadOverrides bool
	HadGrantedBy bool
	HadCreatedAt bool
}

type tenantInput struct {
	TenantID string
	Bad      bool
}

// ReadExport decodes a JSON object keyed by GoClaw table name, or JSONL
// with one object per line and a table field. Credential values are not
// kept on the result.
func ReadExport(r io.Reader) (*Export, error) {
	if r == nil {
		return nil, errors.New("read export")
	}
	body, err := io.ReadAll(io.LimitReader(r, int64(maxExportBytes)+1))
	if err != nil {
		return nil, errors.New("read export")
	}
	if len(body) > maxExportBytes {
		return nil, ErrExportTooLarge
	}
	if bytes.HasPrefix(body, []byte{0xEF, 0xBB, 0xBF}) {
		body = body[3:]
	}
	trim := bytes.TrimSpace(body)
	if len(trim) == 0 {
		return nil, errors.New("read export")
	}
	if trim[0] == '{' {
		ex, kind, err := readJSON(body)
		if err != nil || kind == jsonDoc {
			return ex, err
		}
		return readJSONL(body)
	}
	if trim[0] == '[' {
		return nil, errors.New("read export")
	}
	return readJSONL(body)
}

const (
	jsonDoc = iota
	jsonLines
)

func readJSON(body []byte) (*Export, int, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	var probe map[string]json.RawMessage
	if err := dec.Decode(&probe); err != nil {
		return nil, jsonDoc, errors.New("read export")
	}
	if dec.More() || (hasTable(probe) && !hasTableArrays(probe)) {
		return nil, jsonLines, nil
	}
	ex := &Export{}
	var err error
	if ex.servers, err = readServers(probe["mcp_servers"]); err != nil {
		return nil, jsonDoc, err
	}
	if ex.agentGrants, err = readGrants(probe["mcp_agent_grants"]); err != nil {
		return nil, jsonDoc, err
	}
	if ex.userGrants, err = readTenants(probe["mcp_user_grants"]); err != nil {
		return nil, jsonDoc, err
	}
	if ex.builtinConfigs, err = readTenants(probe["builtin_tool_tenant_configs"]); err != nil {
		return nil, jsonDoc, err
	}
	if ex.userCredentials, err = readTenants(probe["mcp_user_credentials"]); err != nil {
		return nil, jsonDoc, err
	}
	if ex.contextCredentials, err = readTenants(probe["mcp_context_credentials"]); err != nil {
		return nil, jsonDoc, err
	}
	n, err := countRows(probe["builtin_tools"])
	if err != nil {
		return nil, jsonDoc, err
	}
	ex.builtinTools = n
	return ex, jsonDoc, nil
}

func hasTable(probe map[string]json.RawMessage) bool {
	_, ok := probe["table"]
	return ok
}

func hasTableArrays(probe map[string]json.RawMessage) bool {
	for _, key := range []string{
		"mcp_servers",
		"mcp_agent_grants",
		"mcp_user_grants",
		"builtin_tools",
		"builtin_tool_tenant_configs",
		"mcp_user_credentials",
		"mcp_context_credentials",
	} {
		if _, ok := probe[key]; ok {
			return true
		}
	}
	return false
}

func readJSONL(body []byte) (*Export, error) {
	ex := &Export{}
	lines := bytes.Split(body, []byte("\n"))
	n := 0
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		n++
		var head struct {
			Table string `json:"table"`
		}
		if err := json.Unmarshal(line, &head); err != nil || strings.TrimSpace(head.Table) == "" {
			return nil, errors.New("read export")
		}
		switch strings.TrimSpace(head.Table) {
		case "mcp_servers":
			ex.servers = append(ex.servers, parseServer(line))
		case "mcp_agent_grants":
			ex.agentGrants = append(ex.agentGrants, parseGrant(line))
		case "mcp_user_grants":
			ex.userGrants = append(ex.userGrants, parseTenant(line))
		case "builtin_tool_tenant_configs":
			ex.builtinConfigs = append(ex.builtinConfigs, parseTenant(line))
		case "mcp_user_credentials":
			ex.userCredentials = append(ex.userCredentials, parseTenant(line))
		case "mcp_context_credentials":
			ex.contextCredentials = append(ex.contextCredentials, parseTenant(line))
		case "builtin_tools":
			ex.builtinTools++
		default:
			// Unknown tables are ignored here and reported by the caller
			// only when the JSON object names them. A JSONL line with an
			// unknown table is a read error so a secret table cannot hide
			// inside a typo and be skipped silently.
			return nil, errors.New("read export")
		}
	}
	if n == 0 {
		return nil, errors.New("read export")
	}
	return ex, nil
}

func readServers(raw json.RawMessage) ([]serverInput, error) {
	rows, err := rawRows(raw)
	if err != nil {
		return nil, err
	}
	out := make([]serverInput, 0, len(rows))
	for _, row := range rows {
		out = append(out, parseServer(row))
	}
	return out, nil
}

func readGrants(raw json.RawMessage) ([]grantInput, error) {
	rows, err := rawRows(raw)
	if err != nil {
		return nil, err
	}
	out := make([]grantInput, 0, len(rows))
	for _, row := range rows {
		out = append(out, parseGrant(row))
	}
	return out, nil
}

func readTenants(raw json.RawMessage) ([]tenantInput, error) {
	rows, err := rawRows(raw)
	if err != nil {
		return nil, err
	}
	out := make([]tenantInput, 0, len(rows))
	for _, row := range rows {
		out = append(out, parseTenant(row))
	}
	return out, nil
}

func countRows(raw json.RawMessage) (int, error) {
	rows, err := rawRows(raw)
	if err != nil {
		return 0, err
	}
	return len(rows), nil
}

func rawRows(raw json.RawMessage) ([]json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, errors.New("read export")
	}
	return rows, nil
}

func parseServer(raw json.RawMessage) serverInput {
	var row serverRow
	if err := json.Unmarshal(raw, &row); err != nil || row.bad() {
		return serverInput{Bad: true}
	}
	in := serverInput{
		ID:              strings.TrimSpace(row.ID.Value),
		Name:            strings.TrimSpace(row.Name.Value),
		Transport:       strings.TrimSpace(row.Transport.Value),
		TenantID:        strings.TrimSpace(row.TenantID.Value),
		HadDisplayName:  row.DisplayName.Set,
		HadArgs:         false,
		HadHeaders:      row.Headers.Set,
		HadEnv:          row.Env.Set,
		HadAPIKey:       row.APIKey.Set,
		HadToolPrefix:   row.ToolPrefix.Set,
		HadSettings:     row.Settings.Set,
		HadCreatedBy:    row.CreatedBy.Set,
		HadCreatedAt:    row.CreatedAt.Set,
		HadUpdatedAt:    row.UpdatedAt.Set,
		EnabledBad:      row.Enabled.Bad,
		NeedsCredential: row.APIKey.Set || row.Headers.Set || row.Env.Set,
	}
	if row.Timeout.bad {
		in.TimeoutBad = true
	} else {
		in.TimeoutMS = row.Timeout.ms
	}
	if hasControl(in.ID) || hasControl(in.Name) || hasControl(in.Transport) || hasControl(in.TenantID) || hasControl(row.Command.Value) || hasControl(row.URL.Value) {
		in.Control = true
		return in
	}
	switch mapTransport(in.Transport) {
	case transportStdio:
		joined, hold, badArgs := classifyArgs(row.Args)
		if badArgs {
			in.Bad = true
			return in
		}
		if hold {
			in.SecretHold = true
			in.NeedsCredential = true
			return in
		}
		ep, reason := cleanCommand(row.Command.Value)
		if reason == "secret" {
			in.SecretHold = true
			in.NeedsCredential = true
			return in
		}
		in.EndpointReason = reason
		if reason != "" {
			return in
		}
		if joined != "" {
			ep = ep + " " + joined
		}
		in.Endpoint = ep
	case transportHTTP:
		ep, needs, hold, reason := cleanHTTP(row.URL.Value)
		if hold {
			in.SecretHold = true
			in.NeedsCredential = true
			return in
		}
		in.Endpoint = ep
		in.EndpointReason = reason
		if needs {
			in.NeedsCredential = true
		}
	}
	return in
}

func parseGrant(raw json.RawMessage) grantInput {
	var row grantRow
	if err := json.Unmarshal(raw, &row); err != nil || row.bad() {
		return grantInput{Bad: true}
	}
	in := grantInput{
		ServerID:     strings.TrimSpace(row.ServerID.Value),
		AgentID:      strings.TrimSpace(row.AgentID.Value),
		TenantID:     strings.TrimSpace(row.TenantID.Value),
		Enabled:      row.Enabled.on(),
		HadAllow:     row.ToolAllow.Set,
		HadDeny:      row.ToolDeny.Set,
		HadOverrides: row.ConfigOverrides.Set,
		HadGrantedBy: row.GrantedBy.Set,
		HadCreatedAt: row.CreatedAt.Set,
	}
	if row.Enabled.Bad {
		in.Bad = true
	}
	if hasControl(in.ServerID) || hasControl(in.AgentID) || hasControl(in.TenantID) {
		in.Control = true
	}
	return in
}

func parseTenant(raw json.RawMessage) tenantInput {
	var row struct {
		TenantID flexString `json:"tenant_id"`
	}
	if err := json.Unmarshal(raw, &row); err != nil || row.TenantID.Bad {
		return tenantInput{Bad: true}
	}
	return tenantInput{TenantID: strings.TrimSpace(row.TenantID.Value)}
}

const (
	transportStdio = "mcp-stdio"
	transportHTTP  = "mcp-http"
)

func mapTransport(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "stdio", "mcp-stdio":
		return transportStdio
	case "sse", "mcp-sse", "streamable-http", "http", "mcp-http", "mcp":
		return transportHTTP
	default:
		return ""
	}
}

func cleanCommand(raw string) (string, string) {
	s := strings.TrimSpace(raw)
	if s == "" || hasControl(s) || len(s) > 1024 {
		return "", "invalid-endpoint"
	}
	if argSecret(s) {
		// The command itself holds a credential. Do not return it.
		return "", "secret"
	}
	return s, ""
}

func cleanHTTP(raw string) (string, bool, bool, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" || hasControl(raw) || len(raw) > 2048 {
		return "", false, false, "invalid-endpoint"
	}
	u, err := urlParse(raw)
	if err != nil {
		return "", false, false, "invalid-endpoint"
	}
	if u.hold {
		return "", true, true, ""
	}
	return u.endpoint, u.needs, false, ""
}

func hasControl(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

type serverRow struct {
	ID          flexString      `json:"id"`
	Name        flexString      `json:"name"`
	DisplayName present         `json:"display_name"`
	Transport   flexString      `json:"transport"`
	Command     flexString      `json:"command"`
	Args        json.RawMessage `json:"args"`
	URL         flexString      `json:"url"`
	Headers     present         `json:"headers"`
	Env         present         `json:"env"`
	APIKey      present         `json:"api_key"`
	ToolPrefix  present         `json:"tool_prefix"`
	Timeout     flexTimeout     `json:"timeout_sec"`
	Settings    present         `json:"settings"`
	Enabled     flexBool        `json:"enabled"`
	CreatedBy   present         `json:"created_by"`
	CreatedAt   present         `json:"created_at"`
	UpdatedAt   present         `json:"updated_at"`
	TenantID    flexString      `json:"tenant_id"`
}

func (r serverRow) bad() bool {
	return r.ID.Bad || r.Name.Bad || r.Transport.Bad || r.Command.Bad || r.URL.Bad || r.TenantID.Bad
}

type grantRow struct {
	ServerID        flexString `json:"server_id"`
	AgentID         flexString `json:"agent_id"`
	Enabled         flexBool   `json:"enabled"`
	ToolAllow       present    `json:"tool_allow"`
	ToolDeny        present    `json:"tool_deny"`
	ConfigOverrides present    `json:"config_overrides"`
	GrantedBy       present    `json:"granted_by"`
	CreatedAt       present    `json:"created_at"`
	TenantID        flexString `json:"tenant_id"`
}

func (r grantRow) bad() bool {
	return r.ServerID.Bad || r.AgentID.Bad || r.TenantID.Bad
}

// flexString accepts a JSON string or null. Any other type marks the field
// bad without failing the rest of the file.
type flexString struct {
	Value string
	Bad   bool
}

func (f *flexString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("null")) {
		return nil
	}
	if len(b) > 0 && b[0] == '"' {
		if err := json.Unmarshal(b, &f.Value); err != nil {
			f.Bad = true
		}
		return nil
	}
	f.Bad = true
	return nil
}

// present records that a JSON value was non-empty. The value itself is not stored.
type present struct {
	Set bool
}

func (p *present) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	switch string(b) {
	case "null", `""`, "{}", "[]":
		return nil
	}
	if len(b) >= 2 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			p.Set = true
			return nil
		}
		if strings.TrimSpace(s) != "" {
			p.Set = true
		}
		return nil
	}
	p.Set = true
	return nil
}

type flexBool struct {
	Set   bool
	Value bool
	Bad   bool
}

func (f *flexBool) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	switch string(b) {
	case "null":
		return nil
	case "true":
		f.Set = true
		f.Value = true
		return nil
	case "false":
		f.Set = true
		return nil
	default:
		f.Bad = true
		return nil
	}
}

// on is the column default: a missing enabled is true.
func (f flexBool) on() bool {
	if !f.Set {
		return true
	}
	return f.Value
}

type flexTimeout struct {
	ms  int
	bad bool
}

func (f *flexTimeout) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || bytes.Equal(b, []byte("null")) {
		return nil
	}
	var n int
	if err := json.Unmarshal(b, &n); err != nil || n < 0 || n > 86400 {
		f.bad = true
		return nil
	}
	f.ms = n * 1000
	return nil
}
