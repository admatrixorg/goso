// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"encoding/json"
	"errors"
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

// errVaultChannel is returned to channel callers. The vault is tenant-wide
// knowledge with no customer stamp, so a customer thread never reads it.
var errVaultChannel = errors.New("vault is not available in channel conversations")

// VaultToolSpecs are always advertised. LLM ToolCalls only (no keyword match).
func VaultToolSpecs() []llm.ToolSpec {
	return []llm.ToolSpec{
		{
			Name:        ToolVaultSearch,
			Description: "Search the knowledge vault of the calling agent's tenant (title and full text). Returns up to 50 hits with id, title, path and snippet. Read-only. Not available in channel conversations. Args: query.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"q":{"type":"string"}},"required":["query"]}`),
		},
		{
			Name:        ToolVaultRead,
			Description: "Read one vault document of the calling agent's tenant by id (from vault_search) or by path. Body capped at 64KiB; truncated is true when cut. Read-only. Not available in channel conversations. Args: id or path.",
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

// DispatchVaultTool runs vault_search / vault_read jailed to tenant. Channel
// callers are refused (see errVaultChannel). root is the vault directory;
// empty means vault.Dir().
func DispatchVaultTool(st store.StoreIface, root, tenant, callerSessionID string, call llm.ToolCall) (string, error) {
	if st == nil {
		return `{"error":"store required"}`, errors.New("store required")
	}
	name := advertisedBase(call.Name)
	tenant = store.NormalizeTenant(tenant)
	if scopeForCaller(st, callerSessionID).channel {
		return `{"error":` + jsonQuote(errVaultChannel.Error()) + `}`, errVaultChannel
	}
	svc := vault.New(st, root)
	switch name {
	case ToolVaultSearch:
		q := argString(call.Arguments, "query", "q")
		if q == "" {
			return `{"error":"query is required"}`, errors.New("query is required")
		}
		hits, err := svc.Search(tenant, q)
		if err != nil {
			return `{"error":` + jsonQuote(err.Error()) + `}`, err
		}
		return encodeVault(map[string]any{"hits": hits})
	case ToolVaultRead:
		doc, err := readVaultDoc(st, svc, tenant, call.Arguments)
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

// readVaultDoc resolves id first, then path. Both go through Service.Get, so
// another tenant's row and an unclear-owner row are both not found.
func readVaultDoc(st store.StoreIface, svc *vault.Service, tenant string, args map[string]any) (*store.VaultDoc, error) {
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
	doc, err := svc.Get(tenant, id)
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
