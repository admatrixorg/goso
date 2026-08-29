// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/config"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// MemoryToolSpecs are always advertised. LLM ToolCalls only (no keyword match).
func MemoryToolSpecs() []llm.ToolSpec {
	return []llm.ToolSpec{
		{
			Name:        ToolMemorySearch,
			Description: "Search L1 episodic memory and L2 knowledge-graph entities. Fail-closed if query is empty. Args: query.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"q":{"type":"string"}},"required":["query"]}`),
		},
		{
			Name:        ToolMemoryExpand,
			Description: "Deep retrieve an L2 entity: body, relations, and linked names. Args: id.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`),
		},
	}
}

// DispatchMemoryTool runs memory_search / memory_expand for the calling tenant.
func DispatchMemoryTool(st store.StoreIface, tenant string, call llm.ToolCall) (string, error) {
	if st == nil {
		return `{"error":"store required"}`, errors.New("store required")
	}
	name := strings.TrimSpace(call.Name)
	if c, t, ok := SplitAdvertised(name); ok && c != "" {
		name = t
	}
	tenant = store.NormalizeTenant(tenant)
	switch name {
	case ToolMemorySearch:
		q := argString(call.Arguments, "query", "q")
		if q == "" {
			return `{"error":"q is required"}`, errors.New("q is required")
		}
		hits, err := st.SearchProgressive(q, tenant)
		if err != nil {
			return `{"error":` + jsonQuote(err.Error()) + `}`, err
		}
		if hits == nil {
			hits = []store.KGSearchHit{}
		}
		b, err := json.Marshal(hits)
		if err != nil {
			return `{"error":"encode"}`, err
		}
		return string(b), nil
	case ToolMemoryExpand:
		id := argString(call.Arguments, "id")
		if id == "" {
			return `{"error":"id is required"}`, errors.New("id is required")
		}
		exp, err := st.ExpandKG(id)
		if err != nil {
			return `{"error":` + jsonQuote(err.Error()) + `}`, err
		}
		if exp == nil || exp.Entity == nil || !store.SameTenant(exp.Entity.TenantID, tenant) {
			return `{"error":"not found"}`, store.ErrNotFound
		}
		b, err := json.Marshal(exp)
		if err != nil {
			return `{"error":"encode"}`, err
		}
		return string(b), nil
	default:
		return `{"error":"unknown tool"}`, errors.New("unknown tool")
	}
}

func argString(args map[string]any, keys ...string) string {
	if args == nil {
		return ""
	}
	for _, k := range keys {
		v, ok := args[k]
		if !ok {
			continue
		}
		s, ok := v.(string)
		if !ok {
			continue
		}
		if t := strings.TrimSpace(s); t != "" {
			return t
		}
	}
	return ""
}

func jsonQuote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `"error"`
	}
	return string(b)
}

// KGExtractEnabled reports GOSO_KG_EXTRACT=1/true/yes/on. Default off.
func KGExtractEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(config.Lookup("GOSO_KG_EXTRACT"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// ExtractEntityNames parses assistant text for Name: / Entity: lines.
func ExtractEntityNames(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		var rest string
		switch {
		case strings.HasPrefix(lower, "name:"):
			rest = strings.TrimSpace(line[len("name:"):])
		case strings.HasPrefix(lower, "entity:"):
			rest = strings.TrimSpace(line[len("entity:"):])
		default:
			continue
		}
		if rest == "" {
			continue
		}
		key := strings.ToLower(rest)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, rest)
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func extractKG(st store.StoreIface, sessionID, reply string) {
	if st == nil || !KGExtractEnabled() {
		return
	}
	names := ExtractEntityNames(reply)
	if len(names) == 0 {
		return
	}
	tid := store.DefaultTenant
	if sess, err := st.GetSession(sessionID); err == nil && sess != nil {
		tid = store.NormalizeTenant(sess.TenantID)
	}
	for _, name := range names {
		_, _ = st.PutKGEntity(store.KGEntity{
			TenantID: tid,
			Name:     name,
			Kind:     "extracted",
			Body:     name,
		})
	}
}
