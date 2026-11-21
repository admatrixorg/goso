package pipeline

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/mqglobal/goso/gateway/internal/brain"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

const (
	ToolBrainSearch = "brain_search"
	ToolBrainRead   = "brain_read"
	ToolBrainList   = "brain_list"
	ToolBrainIndex  = "brain_index"
	ToolWikiSearch  = "search_wiki"
	ToolWikiRead    = "read_wiki_page"
	ToolWikiList    = "list_wiki_pages"
	ToolWikiIndex   = "read_wiki_index"
	ToolWikiSources = "list_sources"
)

func BrainToolSpecs() []llm.ToolSpec {
	schemaQ := json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"q":{"type":"string"}},"required":["query"]}`)
	schemaID := json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"},"slug":{"type":"string"}}}`)
	schemaEmpty := json.RawMessage(`{"type":"object","properties":{}}`)
	return []llm.ToolSpec{
		{Name: ToolBrainSearch, Description: "Search this tenant company wiki compiled from internal docs (Arkon MRP). Use before answering process/policy/product/people questions. Args: query.", InputSchema: schemaQ},
		{Name: ToolWikiSearch, Description: "Alias of brain_search (Arkon search_wiki).", InputSchema: schemaQ},
		{Name: ToolBrainRead, Description: "Read one company wiki page by id or slug from brain_search.", InputSchema: schemaID},
		{Name: ToolWikiRead, Description: "Alias of brain_read (Arkon read_wiki_page).", InputSchema: schemaID},
		{Name: ToolBrainList, Description: "List company wiki page titles and slugs for this tenant.", InputSchema: schemaEmpty},
		{Name: ToolWikiList, Description: "Alias of brain_list (Arkon list_wiki_pages).", InputSchema: schemaEmpty},
		{Name: ToolBrainIndex, Description: "Short index of the tenant company wiki.", InputSchema: schemaEmpty},
		{Name: ToolWikiIndex, Description: "Alias of brain_index (Arkon read_wiki_index).", InputSchema: schemaEmpty},
		{Name: ToolWikiSources, Description: "List source documents compiled into this tenant wiki.", InputSchema: schemaEmpty},
	}
}

func IsBrainTool(name string) bool {
	switch advertisedBase(name) {
	case ToolBrainSearch, ToolBrainRead, ToolBrainList, ToolBrainIndex, ToolWikiSearch, ToolWikiRead, ToolWikiList, ToolWikiIndex, ToolWikiSources:
		return true
	default:
		return false
	}
}

func DispatchBrainTool(st store.StoreIface, tenant string, call llm.ToolCall) (string, error) {
	name := advertisedBase(call.Name)
	tenant = store.NormalizeTenant(tenant)
	sql := brainSQL(st)
	switch name {
	case ToolBrainSearch, ToolWikiSearch:
		q := argString(call.Arguments, "query", "q")
		if q == "" {
			return `{"error":"query is required"}`, errors.New("query is required")
		}
		hits, err := brain.Search(st, tenant, q, 20)
		if err != nil {
			return `{"error":"search failed"}`, err
		}
		b, _ := json.Marshal(map[string]any{"hits": hits})
		return string(b), nil
	case ToolBrainRead, ToolWikiRead:
		id := argString(call.Arguments, "id", "slug")
		if id == "" {
			return `{"error":"id or slug is required"}`, errors.New("id or slug is required")
		}
		page, err := brain.Get(st, tenant, id)
		if err != nil {
			return `{"error":"not found"}`, err
		}
		body := page.Body
		cut := false
		if len(body) > VaultReadMaxBytes {
			body = body[:VaultReadMaxBytes]
			cut = true
		}
		b, _ := json.Marshal(map[string]any{"id": page.ID, "slug": page.Slug, "title": page.Title, "body": body, "rev": page.Rev, "truncated": cut})
		return string(b), nil
	case ToolBrainList, ToolWikiList, ToolBrainIndex, ToolWikiIndex:
		pages, err := brain.List(st, tenant)
		if err != nil {
			return `{"error":"list failed"}`, err
		}
		idx := make([]map[string]any, 0, len(pages))
		capn := len(pages)
		if name == ToolBrainIndex || name == ToolWikiIndex {
			if capn > 80 {
				capn = 80
			}
		}
		for i := 0; i < capn; i++ {
			idx = append(idx, map[string]any{"id": pages[i].ID, "slug": pages[i].Slug, "title": pages[i].Title, "rev": pages[i].Rev})
		}
		b, _ := json.Marshal(map[string]any{"tenant": tenant, "count": len(pages), "pages": idx})
		return string(b), nil
	case ToolWikiSources:
		if sql == nil {
			return `{"sources":[]}`, nil
		}
		srcs, err := sql.ListBrainSources(tenant)
		if err != nil {
			return `{"error":"list failed"}`, err
		}
		b, _ := json.Marshal(map[string]any{"sources": srcs})
		return string(b), nil
	default:
		return `{"error":"unknown tool"}`, errors.New("unknown tool")
	}
}

func brainSQL(st store.StoreIface) *store.SQLiteStore {
	switch s := st.(type) {
	case *store.SQLiteStore:
		return s
	case *store.PostgresStore:
		return s.SQLiteStore
	default:
		return nil
	}
}

func argInt(args map[string]any, keys ...string) int {
	for _, k := range keys {
		switch v := args[k].(type) {
		case float64:
			return int(v)
		case int:
			return v
		case string:
			n, _ := strconv.Atoi(v)
			return n
		}
	}
	return 0
}
