// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/vault"
)

const (
	ToolVaultSearch = "vault_search"
	ToolVaultRead   = "vault_read"
	// VaultReadMaxBytes caps the body vault_read returns, so one large document
	// cannot fill the model context. The cut lands on a rune boundary.
	VaultReadMaxBytes = 64 << 10
)

// VaultToolSpecs are always advertised. LLM ToolCalls only (no keyword match).
func VaultToolSpecs() []llm.ToolSpec {
	return []llm.ToolSpec{
		{
			Name:        ToolVaultSearch,
			Description: "Search the knowledge vault of the calling agent's tenant (title and full text). Returns up to 50 hits with id, title, path and snippet. Only documents this agent may see: tenant-wide ones, its own, and its teams' ones; in a customer conversation only documents for no customer or this customer. Read-only. Args: query.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"q":{"type":"string"}},"required":["query"]}`),
		},
		{
			Name:        ToolVaultRead,
			Description: "Read one vault document of the calling agent's tenant by id (from vault_search) or by path. Body capped at 64KiB; truncated is true when cut. Same visibility as vault_search. Read-only. Args: id or path.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"},"path":{"type":"string"}}}`),
		},
	}
}

// IsVaultTool reports vault_search / vault_read (no connector).
func IsVaultTool(name string) bool {
	switch advertisedBase(name) {
	case ToolVaultSearch, ToolVaultRead:
		return true
	default:
		return false
	}
}

type vaultReadResult struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Path       string `json:"path"`
	Body       string `json:"body"`
	BodyStatus string `json:"body_status,omitempty"`
	Truncated  bool   `json:"truncated"`
}

// DispatchVaultTool runs vault_search / vault_read jailed to tenant, as the
// agent of the calling session. See DispatchVaultToolAs.
func DispatchVaultTool(st store.StoreIface, root, tenant, callerSessionID string, call llm.ToolCall) (string, error) {
	return DispatchVaultToolAs(st, root, tenant, "", callerSessionID, call)
}

// DispatchVaultToolAs runs vault_search / vault_read jailed to tenant and to
// what agentID may see: tenant docs, its own agent docs, and docs of its
// teams. An empty agentID is taken from the calling session. In a channel
// thread personal docs are hidden and only docs with no customer_key or the
// thread's customer key are served. root is the vault directory; empty means
// vault.Dir().
func DispatchVaultToolAs(st store.StoreIface, root, tenant, agentID, callerSessionID string, call llm.ToolCall) (string, error) {
	if st == nil {
		return `{"error":"store required"}`, errors.New("store required")
	}
	name := advertisedBase(call.Name)
	tenant = store.NormalizeTenant(tenant)
	viewer := vaultViewerFor(st, tenant, agentID, callerSessionID)
	svc := vault.New(st, root)
	switch name {
	case ToolVaultSearch:
		q := argString(call.Arguments, "query", "q")
		if q == "" {
			return `{"error":"query is required"}`, errors.New("query is required")
		}
		hits, err := svc.SearchAs(tenant, q, viewer)
		if err != nil {
			return `{"error":` + jsonQuote(err.Error()) + `}`, err
		}
		return encodeVault(map[string]any{"hits": hits})
	case ToolVaultRead:
		doc, err := readVaultDoc(st, svc, tenant, viewer, call.Arguments)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return `{"error":"not found"}`, err
			}
			return `{"error":` + jsonQuote(err.Error()) + `}`, err
		}
		body, cut := capBody(doc.Body, VaultReadMaxBytes)
		return encodeVault(vaultReadResult{
			ID:         doc.ID,
			Title:      doc.Title,
			Path:       doc.Path,
			Body:       body,
			BodyStatus: doc.BodyStatus,
			Truncated:  cut,
		})
	default:
		return `{"error":"unknown tool"}`, errors.New("unknown tool")
	}
}

// vaultViewerFor is the agent view for a tool call. A channel session fixes
// the agent and the customer; otherwise agentID, or the session's agent.
func vaultViewerFor(st store.StoreIface, tenant, agentID, callerSessionID string) store.VaultViewer {
	sc := scopeForCaller(st, callerSessionID)
	if sc.channel {
		return vault.ViewerForAgent(st, tenant, sc.agentID, true, sc.customer)
	}
	if strings.TrimSpace(agentID) == "" && strings.TrimSpace(callerSessionID) != "" {
		if sess, err := st.GetSession(strings.TrimSpace(callerSessionID)); err == nil && sess != nil {
			agentID = sess.AgentID
		}
	}
	return vault.ViewerForAgent(st, tenant, agentID, false, "")
}

// readVaultDoc resolves id first, then path. Both go through Service.GetAs, so
// another tenant's row, an unclear-owner row and a doc the viewer may not see
// are all not found.
func readVaultDoc(st store.StoreIface, svc *vault.Service, tenant string, v store.VaultViewer, args map[string]any) (*store.VaultDoc, error) {
	id := argString(args, "id")
	if id == "" {
		path := argString(args, "path")
		if path == "" {
			return nil, errors.New("id or path is required")
		}
		row, err := st.FindVaultDocByPath(tenant, path)
		if err != nil {
			return nil, err
		}
		if row == nil {
			return nil, store.ErrNotFound
		}
		id = row.ID
	}
	doc, err := svc.GetAs(tenant, id, v)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, store.ErrNotFound
	}
	return doc, nil
}

func capBody(s string, max int) (string, bool) {
	if len(s) <= max {
		return s, false
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut], true
}

func encodeVault(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"error":"encode"}`, err
	}
	return string(b), nil
}
