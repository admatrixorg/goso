// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package connector

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Tool is one entry in a connector Tool Manifest.
type Tool struct {
	Name             string          `json:"name"`
	Description      string          `json:"description,omitempty"`
	InputSchema      json.RawMessage `json:"input_schema"`
	RequiresApproval bool            `json:"requires_approval"`
}

// Manifest is a versioned list of tools (JSON Schema per tool).
type Manifest struct {
	SchemaVersion string `json:"schema_version,omitempty"`
	Name          string `json:"name,omitempty"`
	Tools         []Tool `json:"tools"`
}

type manifestDTO struct {
	SchemaVersion string            `json:"schema_version"`
	SchemaCamel   string            `json:"schemaVersion"`
	Name          string            `json:"name"`
	Tools         []json.RawMessage `json:"tools"`
}

type toolDTO struct {
	Name             string          `json:"name"`
	Description      string          `json:"description"`
	InputSchema      json.RawMessage `json:"input_schema"`
	InputSchemaCamel json.RawMessage `json:"inputSchema"`
	RequiresApproval bool            `json:"requires_approval"`
	RequiresCamel    *bool           `json:"requiresApproval"`
}

// ParseManifest decodes and validates a Tool Manifest.
func ParseManifest(raw []byte) (*Manifest, error) {
	raw = bytesTrimSpace(raw)
	if len(raw) == 0 {
		return nil, fmt.Errorf("manifest is empty")
	}
	var dto manifestDTO
	if err := json.Unmarshal(raw, &dto); err != nil {
		return nil, fmt.Errorf("manifest json: %w", err)
	}
	m := &Manifest{
		SchemaVersion: firstNonEmpty(dto.SchemaVersion, dto.SchemaCamel),
		Name:          dto.Name,
	}
	if len(dto.Tools) == 0 {
		return nil, fmt.Errorf("manifest has no tools")
	}
	seen := map[string]struct{}{}
	for i, traw := range dto.Tools {
		t, err := parseTool(traw)
		if err != nil {
			return nil, fmt.Errorf("tools[%d]: %w", i, err)
		}
		if _, ok := seen[t.Name]; ok {
			return nil, fmt.Errorf("duplicate tool %q", t.Name)
		}
		seen[t.Name] = struct{}{}
		m.Tools = append(m.Tools, t)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return m, nil
}

func parseTool(raw json.RawMessage) (Tool, error) {
	var dto toolDTO
	if err := json.Unmarshal(raw, &dto); err != nil {
		return Tool{}, err
	}
	name := strings.TrimSpace(dto.Name)
	if name == "" {
		return Tool{}, fmt.Errorf("tool name is required")
	}
	schema := dto.InputSchema
	if len(bytesTrimSpace(schema)) == 0 || string(bytesTrimSpace(schema)) == "null" {
		schema = dto.InputSchemaCamel
	}
	req := dto.RequiresApproval
	if dto.RequiresCamel != nil {
		req = *dto.RequiresCamel
	}
	t := Tool{
		Name:             name,
		Description:      dto.Description,
		InputSchema:      schema,
		RequiresApproval: req,
	}
	if err := validateInputSchema(t.InputSchema); err != nil {
		return Tool{}, fmt.Errorf("tool %q schema: %w", name, err)
	}
	return t, nil
}

// Validate checks every tool schema is parseable JSON Schema (object).
func (m *Manifest) Validate() error {
	if m == nil || len(m.Tools) == 0 {
		return fmt.Errorf("manifest has no tools")
	}
	for _, t := range m.Tools {
		if strings.TrimSpace(t.Name) == "" {
			return fmt.Errorf("tool name is required")
		}
		if err := validateInputSchema(t.InputSchema); err != nil {
			return fmt.Errorf("tool %q schema: %w", t.Name, err)
		}
	}
	return nil
}

// Tool returns the named tool or false.
func (m *Manifest) Tool(name string) (Tool, bool) {
	for _, t := range m.Tools {
		if t.Name == name {
			return t, true
		}
	}
	return Tool{}, false
}

func validateInputSchema(raw json.RawMessage) error {
	if len(bytesTrimSpace(raw)) == 0 {
		return fmt.Errorf("input_schema is required")
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return fmt.Errorf("not valid json: %w", err)
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("input_schema must be a json object")
	}
	if typ, ok := obj["type"]; ok {
		s, _ := typ.(string)
		if s != "" && s != "object" {
			return fmt.Errorf("input_schema.type must be \"object\" (got %q)", s)
		}
	}
	if props, ok := obj["properties"]; ok && props != nil {
		if _, ok := props.(map[string]any); !ok {
			return fmt.Errorf("input_schema.properties must be an object")
		}
	}
	if req, ok := obj["required"]; ok && req != nil {
		arr, ok := req.([]any)
		if !ok {
			return fmt.Errorf("input_schema.required must be an array")
		}
		for _, x := range arr {
			if _, ok := x.(string); !ok {
				return fmt.Errorf("input_schema.required entries must be strings")
			}
		}
	}
	return nil
}

// ValidateArgs checks args against a tool's JSON Schema (required + primitive types).
func ValidateArgs(schema json.RawMessage, args map[string]any) error {
	if args == nil {
		args = map[string]any{}
	}
	var obj map[string]any
	if err := json.Unmarshal(schema, &obj); err != nil {
		return err
	}
	if req, ok := obj["required"].([]any); ok {
		for _, x := range req {
			name, _ := x.(string)
			if name == "" {
				continue
			}
			if _, exists := args[name]; !exists {
				return fmt.Errorf("missing required argument %q", name)
			}
		}
	}
	props, _ := obj["properties"].(map[string]any)
	for k, v := range args {
		if props == nil {
			break
		}
		ps, ok := props[k].(map[string]any)
		if !ok {
			continue
		}
		if err := checkType(ps["type"], v); err != nil {
			return fmt.Errorf("argument %q: %w", k, err)
		}
	}
	return nil
}

func checkType(typ any, v any) error {
	s, _ := typ.(string)
	if s == "" || v == nil {
		return nil
	}
	switch s {
	case "string":
		if _, ok := v.(string); !ok {
			return fmt.Errorf("want string")
		}
	case "number":
		switch v.(type) {
		case float64, json.Number, int, int64, float32:
		default:
			return fmt.Errorf("want number")
		}
	case "integer":
		switch n := v.(type) {
		case float64:
			if n != float64(int64(n)) {
				return fmt.Errorf("want integer")
			}
		case json.Number, int, int64:
		default:
			return fmt.Errorf("want integer")
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return fmt.Errorf("want boolean")
		}
	case "object":
		if _, ok := v.(map[string]any); !ok {
			return fmt.Errorf("want object")
		}
	case "array":
		if _, ok := v.([]any); !ok {
			return fmt.Errorf("want array")
		}
	}
	return nil
}

func bytesTrimSpace(b []byte) []byte {
	i, j := 0, len(b)
	for i < j && (b[i] == ' ' || b[i] == '\n' || b[i] == '\t' || b[i] == '\r') {
		i++
	}
	for j > i && (b[j-1] == ' ' || b[j-1] == '\n' || b[j-1] == '\t' || b[j-1] == '\r') {
		j--
	}
	return b[i:j]
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
