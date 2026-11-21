package brain

import (
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

func extractWikiSlugs(body string) []string {
	seen := map[string]struct{}{}
	var out []string
	for {
		i := strings.Index(body, "[[")
		if i < 0 {
			break
		}
		body = body[i+2:]
		j := strings.Index(body, "]]")
		if j < 0 {
			break
		}
		inner := strings.TrimSpace(strings.Split(body[:j], "|")[0])
		body = body[j+2:]
		slug := slugify(inner)
		if slug == "" {
			continue
		}
		if _, ok := seen[slug]; ok {
			continue
		}
		seen[slug] = struct{}{}
		out = append(out, slug)
	}
	return out
}

func recordSourcePlanLinks(s *store.SQLiteStore, doc *store.VaultDoc, page store.BrainPage, strategy string) {
	if s == nil || doc == nil {
		return
	}
	src, err := s.UpsertBrainSource(store.BrainSource{TenantID: doc.TenantID, VaultDocID: doc.ID, Title: doc.Title, Path: doc.Path, Status: "ready", Strategy: strategy})
	if err == nil && src != nil {
		_, _ = s.PutBrainPlan(store.BrainPlan{SourceID: src.ID, TenantID: doc.TenantID, Status: "approved", JSON: "{\"slug\":\"" + page.Slug + "\"}"})
	}
	s.SnapshotBrainVersion(page)
	_ = s.ReplaceBrainLinks(page.TenantID, page.ID, extractWikiSlugs(page.Body))
}
