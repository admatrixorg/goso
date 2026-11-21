package brain

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/store"
)

const maxSourceChars = 12000
const llmDocCap = 12
const llmCallTimeout = 12 * time.Second

type compiledPage struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

func sqlStore(st store.StoreIface) *store.SQLiteStore {
	switch s := st.(type) {
	case *store.SQLiteStore:
		return s
	case *store.PostgresStore:
		return s.SQLiteStore
	default:
		return nil
	}
}

func Migrate(st store.StoreIface) error {
	s := sqlStore(st)
	if s == nil {
		return nil
	}
	return s.MigrateBrain()
}

func CompileTenant(ctx context.Context, st store.StoreIface, provider llm.Provider, tenant string) (int, error) {
	s := sqlStore(st)
	if s == nil {
		return 0, errors.New("brain requires sqlite/postgres store")
	}
	if err := s.MigrateBrain(); err != nil {
		return 0, err
	}
	tenant = store.NormalizeTenant(tenant)
	docs := s.ListVaultDocsByTenant(tenant)
	useLLM := provider != nil && provider.Name() != "echo" && os.Getenv("GOSO_BRAIN_LLM") != "0" && len(docs) <= llmDocCap
	n := 0
	for i, doc := range docs {
		if ctx.Err() != nil {
			return n, ctx.Err()
		}
		var p llm.Provider
		if useLLM && i < llmDocCap {
			p = provider
		}
		c, err := compileOne(ctx, s, p, doc)
		if err != nil {
			return n, err
		}
		n += c
	}
	return n, nil
}

func compileOne(ctx context.Context, s *store.SQLiteStore, provider llm.Provider, doc *store.VaultDoc) (int, error) {
	body := strings.ToValidUTF8(strings.TrimSpace(doc.Body), "")
	if body == "" {
		return 0, nil
	}
	body = cutUTF8(body, maxSourceChars)
	var pages []compiledPage
	if provider != nil {
		cctx, cancel := context.WithTimeout(ctx, llmCallTimeout)
		raw, err := provider.Chat(cctx, []llm.Message{
			{Role: "system", Content: compilePrompt()},
			{Role: "user", Content: "Title: " + doc.Title + "\nPath: " + doc.Path + "\n\n" + body},
		})
		cancel()
		if err == nil {
			pages = parsePages(raw)
		}
	}
	if len(pages) == 0 {
		pages = append(pages, compiledPage{
			Slug:  slugify(doc.Title, doc.Path, doc.ID),
			Title: firstNonEmpty(doc.Title, doc.Path, "Untitled"),
			Body:  body,
		})
	}
	for i := range pages {
		saved, err := s.UpsertBrainPage(store.BrainPage{
			TenantID:  doc.TenantID,
			Slug:      pages[i].Slug,
			Title:     strings.ToValidUTF8(pages[i].Title, ""),
			Body:      strings.ToValidUTF8(pages[i].Body, ""),
			SourceIDs: doc.ID,
		})
		if err != nil {
			return i, err
		}
		strategy := "extractive"
		if provider != nil {
			strategy = "mrp"
		}
		if saved != nil {
			recordSourcePlanLinks(s, doc, *saved, strategy)
		}
	}
	return len(pages), nil
}

func compilePrompt() string {
	return "You compile company knowledge into wiki pages. Return JSON only: {\"pages\":[{\"slug\":\"kebab-case\",\"title\":\"...\",\"body\":\"markdown\"}]}. Keep claims faithful to the source. Use Vietnamese if the source is Vietnamese."
}

func parsePages(raw string) []compiledPage {
	raw = strings.TrimSpace(raw)
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return nil
	}
	var out struct {
		Pages []compiledPage `json:"pages"`
	}
	if json.Unmarshal([]byte(raw[start:end+1]), &out) != nil {
		return nil
	}
	clean := make([]compiledPage, 0, len(out.Pages))
	for _, p := range out.Pages {
		p.Title = strings.TrimSpace(p.Title)
		p.Body = strings.TrimSpace(p.Body)
		p.Slug = slugify(p.Slug, p.Title)
		if p.Title == "" || p.Body == "" || p.Slug == "" {
			continue
		}
		clean = append(clean, p)
	}
	return clean
}

func slugify(parts ...string) string {
	var b strings.Builder
	for _, p := range parts {
		for _, r := range strings.ToLower(strings.TrimSpace(p)) {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				b.WriteRune(r)
			} else if r == ' ' || r == '-' || r == '_' || r == '/' || r == '.' {
				if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
					b.WriteByte('-')
				}
			}
		}
		if b.Len() > 0 {
			break
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		s = "page"
	}
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func Search(st store.StoreIface, tenant, q string, limit int) ([]store.BrainHit, error) {
	s := sqlStore(st)
	if s == nil {
		return nil, errors.New("store required")
	}
	return s.SearchBrainPages(tenant, q, limit)
}

func Get(st store.StoreIface, tenant, idOrSlug string) (*store.BrainPage, error) {
	s := sqlStore(st)
	if s == nil {
		return nil, errors.New("store required")
	}
	return s.GetBrainPage(tenant, idOrSlug)
}

func List(st store.StoreIface, tenant string) ([]store.BrainPage, error) {
	s := sqlStore(st)
	if s == nil {
		return nil, errors.New("store required")
	}
	return s.ListBrainPages(tenant)
}

func CompileAllTenants(ctx context.Context, st store.StoreIface, provider llm.Provider) (map[string]int, error) {
	s := sqlStore(st)
	if s == nil {
		return nil, errors.New("store required")
	}
	tenants := s.ListVaultTenants()
	out := map[string]int{}
	for _, tenant := range tenants {
		n, err := CompileTenant(ctx, st, provider, tenant)
		if err != nil {
			out[tenant] = -1
			continue
		}
		out[tenant] = n
	}
	return out, nil
}

func cutUTF8(s string, n int) string {
	if len(s) <= n {
		return strings.ToValidUTF8(s, "")
	}
	s = s[:n]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}
