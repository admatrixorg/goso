// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package skill

import (
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// BM25 parameters (k1=1.2, b=0.75, top 5).
const (
	bm25K1       = 1.2
	bm25B        = 0.75
	SearchLimit  = 5
	snippetRunes = 160
)

// Hit is one ranked skill. Body is never included.
type Hit struct {
	Name    string  `json:"name"`
	Score   float64 `json:"score"`
	Snippet string  `json:"snippet"`
}

type bm25Doc struct {
	Name    string
	TF      map[string]int
	Len     int
	Snippet string
}

type bm25Index struct {
	root  string
	stamp int64
	count int
	n     int
	avgDL float64
	df    map[string]int
	docs  []bm25Doc
}

var (
	idxMu   sync.Mutex
	liveIdx bm25Index
)

func invalidateIndex() {
	idxMu.Lock()
	liveIdx.stamp = -1
	idxMu.Unlock()
}

// Search ranks skills by BM25 over name + description + body. Max SearchLimit.
// Empty env → ErrNotConfigured and no walk. Empty/1-char-only query → no hits.
func Search(query string) ([]Hit, error) {
	if !Configured() {
		return nil, ErrNotConfigured
	}
	terms := tokenize(query)
	idx, err := ensureIndex()
	if err != nil {
		return nil, err
	}
	if len(terms) == 0 || idx.n == 0 {
		return []Hit{}, nil
	}
	seen := map[string]struct{}{}
	qterms := make([]string, 0, len(terms))
	for _, t := range terms {
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		qterms = append(qterms, t)
	}
	type scored struct {
		hit   Hit
		score float64
	}
	ranked := make([]scored, 0, len(idx.docs))
	avgDL := idx.avgDL
	if avgDL <= 0 {
		avgDL = 1
	}
	n := float64(idx.n)
	for _, d := range idx.docs {
		var score float64
		for _, t := range qterms {
			tf := float64(d.TF[t])
			if tf == 0 {
				continue
			}
			df := float64(idx.df[t])
			idf := math.Log((n-df+0.5)/(df+0.5) + 1)
			denom := tf + bm25K1*(1-bm25B+bm25B*float64(d.Len)/avgDL)
			if denom <= 0 {
				continue
			}
			score += idf * tf * (bm25K1 + 1) / denom
		}
		if score <= 0 {
			continue
		}
		ranked = append(ranked, scored{hit: Hit{Name: d.Name, Score: score, Snippet: d.Snippet}, score: score})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].hit.Name < ranked[j].hit.Name
		}
		return ranked[i].score > ranked[j].score
	})
	if len(ranked) > SearchLimit {
		ranked = ranked[:SearchLimit]
	}
	out := make([]Hit, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, r.hit)
	}
	return out, nil
}

func ensureIndex() (bm25Index, error) {
	root, err := resolveRoot()
	if err != nil {
		return bm25Index{}, err
	}
	stamp, count, err := dirStamp(root)
	if err != nil {
		return bm25Index{}, err
	}
	idxMu.Lock()
	if liveIdx.root == root && liveIdx.stamp == stamp && liveIdx.count == count && liveIdx.stamp >= 0 {
		cur := liveIdx
		idxMu.Unlock()
		return cur, nil
	}
	idxMu.Unlock()

	built, err := buildIndex()
	if err != nil {
		return bm25Index{}, err
	}
	stamp2, count2, err := dirStamp(root)
	if err != nil {
		return bm25Index{}, err
	}
	if stamp2 != stamp || count2 != count {
		built, err = buildIndex()
		if err != nil {
			return bm25Index{}, err
		}
		stamp, count = stamp2, count2
	}
	built.root = root
	built.stamp = stamp
	built.count = count

	idxMu.Lock()
	liveIdx = built
	idxMu.Unlock()
	return built, nil
}

func dirStamp(root string) (stamp int64, count int, err error) {
	list, err := List()
	if err != nil {
		return 0, 0, err
	}
	var max int64
	for _, info := range list {
		p := filepath.Join(root, filepath.FromSlash(info.Path))
		st, err := os.Lstat(p)
		if err != nil {
			continue
		}
		if t := st.ModTime().UnixNano(); t > max {
			max = t
		}
	}
	return max, len(list), nil
}

func buildIndex() (bm25Index, error) {
	list, err := List()
	if err != nil {
		return bm25Index{}, err
	}
	docs := make([]bm25Doc, 0, len(list))
	df := map[string]int{}
	var totalLen int
	for _, info := range list {
		doc, err := Load(info.Name)
		if err != nil {
			continue
		}
		desc := parseDescription(doc.Body)
		text := strings.TrimSpace(doc.Name + " " + desc + " " + doc.Body)
		toks := tokenize(text)
		tf := map[string]int{}
		for _, t := range toks {
			tf[t]++
		}
		for t := range tf {
			df[t]++
		}
		totalLen += len(toks)
		docs = append(docs, bm25Doc{
			Name:    doc.Name,
			TF:      tf,
			Len:     len(toks),
			Snippet: makeSnippet(desc, doc.Body),
		})
	}
	n := len(docs)
	avg := 0.0
	if n > 0 {
		avg = float64(totalLen) / float64(n)
	}
	return bm25Index{
		n:     n,
		avgDL: avg,
		df:    df,
		docs:  docs,
	}, nil
}

func tokenize(s string) []string {
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte(' ')
		}
	}
	parts := strings.Fields(b.String())
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if utf8.RuneCountInString(p) <= 1 {
			continue
		}
		out = append(out, p)
	}
	return out
}

func parseDescription(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	if strings.HasPrefix(body, "---\n") {
		rest := body[4:]
		end := strings.Index(rest, "\n---")
		if end >= 0 {
			fm := rest[:end]
			for _, line := range strings.Split(fm, "\n") {
				line = strings.TrimSpace(line)
				key, val, ok := strings.Cut(line, ":")
				if !ok {
					continue
				}
				if strings.EqualFold(strings.TrimSpace(key), "description") {
					return strings.TrimSpace(strings.Trim(val, `"'`))
				}
			}
		}
	}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "---" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.Index(line, ":"); i > 0 && !strings.Contains(line, " ") {
			continue
		}
		return line
	}
	return ""
}

func makeSnippet(desc, body string) string {
	s := strings.TrimSpace(desc)
	if s == "" {
		s = strings.Join(strings.Fields(stripFrontmatter(body)), " ")
	}
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) > snippetRunes {
		return string(runes[:snippetRunes])
	}
	return s
}

func stripFrontmatter(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	if !strings.HasPrefix(body, "---\n") {
		return body
	}
	rest := body[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return body
	}
	after := rest[end+4:]
	return strings.TrimLeft(after, "\n")
}
