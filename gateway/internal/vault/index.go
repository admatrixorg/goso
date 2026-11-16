// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import (
	"context"
	"errors"
	"log"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/mqglobal/goso/gateway/internal/embed"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Hybrid match labels on VaultSearchHit.Match.
const (
	MatchKeyword = "keyword"
	MatchVector  = "vector"
	MatchBoth    = "both"
)

// Embedding states reported by the status APIs.
const (
	EmbedNotConfigured    = "not_configured"
	EmbedConfigError      = "config_error"
	EmbedStoreUnsupported = "store_unsupported"
	EmbedReady            = "ready"
)

// Chunking: about 1200 characters with 200 of overlap, at most 64 chunks
// per doc. A longer doc is embedded up to the cap; keyword search covers all.
const (
	chunkRunes   = 1200
	chunkOverlap = 200
	maxChunks    = 64
	rrfK         = 60.0
	queryTimeout = 10 * time.Second
)

// Indexer embeds vault doc chunks into the store's vector index. Writes are
// queued with Notify and embedded by one worker; Backfill walks docs whose
// chunks are missing or stale. Both go through the client's rate limit.
type Indexer struct {
	Store  store.StoreIface
	Vec    store.VaultVectorIndex
	Client *embed.Client

	queue   chan string
	mu      sync.Mutex
	pending map[string]struct{}
	lastErr atomic.Value // string
	started atomic.Bool
}

// EmbeddingStatus is what the status APIs report.
type EmbeddingStatus struct {
	State     string `json:"state"`
	Model     string `json:"model,omitempty"`
	Dim       int    `json:"dim,omitempty"`
	Docs      int    `json:"docs"`
	Embedded  int    `json:"embedded"`
	LastError string `json:"last_error,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

var (
	activeIndexer atomic.Pointer[Indexer]
	embedState    atomic.Value // EmbeddingStatus without counts
)

func init() { embedState.Store(EmbeddingStatus{State: EmbedNotConfigured}) }

// SetIndexer installs ix as the process indexer (nil removes it).
func SetIndexer(ix *Indexer) {
	activeIndexer.Store(ix)
	if ix == nil {
		embedState.Store(EmbeddingStatus{State: EmbedNotConfigured})
		return
	}
	embedState.Store(EmbeddingStatus{State: EmbedReady, Model: ix.Client.Model()})
}

// ActiveIndexer is the process indexer, or nil.
func ActiveIndexer() *Indexer { return activeIndexer.Load() }

// NewIndexer returns nil when the client is nil or the store has no ready
// vector index.
func NewIndexer(st store.StoreIface, c *embed.Client) *Indexer {
	if st == nil || c == nil {
		return nil
	}
	vec, ok := st.(store.VaultVectorIndex)
	if !ok || !vec.VaultVectorReady() {
		return nil
	}
	return &Indexer{Store: st, Vec: vec, Client: c, queue: make(chan string, 512), pending: map[string]struct{}{}}
}

// StartEmbeddings reads GOSO_EMBED_* and, when the store supports vectors,
// installs and starts the indexer. It never fails startup: a bad setting or
// a store without pgvector is recorded in the status and search stays
// keyword-only.
func StartEmbeddings(ctx context.Context, st store.StoreIface) *Indexer {
	cfg, err := embed.FromEnv()
	if err != nil {
		embedState.Store(EmbeddingStatus{State: EmbedConfigError, Detail: err.Error()})
		log.Printf("vault embeddings off: %v", err)
		return nil
	}
	if !cfg.Configured() {
		embedState.Store(EmbeddingStatus{State: EmbedNotConfigured})
		return nil
	}
	ix := NewIndexer(st, embed.New(cfg))
	if ix == nil {
		embedState.Store(EmbeddingStatus{State: EmbedStoreUnsupported, Model: cfg.Model,
			Detail: "vector search needs Postgres with the pgvector extension"})
		log.Printf("vault embeddings off: store has no pgvector")
		return nil
	}
	SetIndexer(ix)
	ix.Start(ctx)
	log.Printf("vault embeddings on: model=%s", cfg.Model)
	return ix
}

// Status reports the embedding state with counts for tenant ("" = all).
func Status(tenant string) EmbeddingStatus {
	st, _ := embedState.Load().(EmbeddingStatus)
	ix := ActiveIndexer()
	if ix == nil {
		return st
	}
	st.Dim = ix.Client.Dim()
	if s, _ := ix.lastErr.Load().(string); s != "" {
		st.LastError = s
	}
	if st.Dim > 0 {
		if docs, emb, err := ix.Vec.VaultEmbedCounts(tenant, ix.Client.Model(), st.Dim); err == nil {
			st.Docs, st.Embedded = docs, emb
		}
	}
	return st
}

// Start runs the Notify worker until ctx ends.
func (ix *Indexer) Start(ctx context.Context) {
	if ix == nil || !ix.started.CompareAndSwap(false, true) {
		return
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case id := <-ix.queue:
				ix.mu.Lock()
				delete(ix.pending, id)
				ix.mu.Unlock()
				if err := ix.IndexDoc(ctx, id); err != nil && !errors.Is(err, context.Canceled) {
					ix.noteErr(err)
				}
			}
		}
	}()
}

// Notify queues one doc. A full queue drops it; Backfill picks it up later.
func (ix *Indexer) Notify(id string) {
	if ix == nil || id == "" {
		return
	}
	ix.mu.Lock()
	if _, ok := ix.pending[id]; ok {
		ix.mu.Unlock()
		return
	}
	ix.pending[id] = struct{}{}
	ix.mu.Unlock()
	select {
	case ix.queue <- id:
	default:
		ix.mu.Lock()
		delete(ix.pending, id)
		ix.mu.Unlock()
	}
}

func notifyIndexer(d *store.VaultDoc) {
	if d == nil {
		return
	}
	if ix := ActiveIndexer(); ix != nil {
		ix.Notify(d.ID)
	}
}

func (ix *Indexer) noteErr(err error) {
	msg := err.Error()
	ix.lastErr.Store(msg)
	log.Printf("vault embeddings: %s", msg)
}

// IndexDoc embeds the current body of one served doc and replaces its chunks.
// A doc that is gone or not served is skipped.
func (ix *Indexer) IndexDoc(ctx context.Context, id string) error {
	d, err := ix.Store.GetVaultDoc(id)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !served(d) {
		return nil
	}
	parts := Chunk(d.Body)
	inputs := make([]string, len(parts))
	for i, p := range parts {
		inputs[i] = d.Title + "\n\n" + p
	}
	var vecs [][]float32
	if len(inputs) > 0 {
		vecs, err = ix.Client.Embed(ctx, inputs)
		if err != nil {
			return err
		}
	}
	dim := ix.Client.Dim()
	if dim == 0 {
		// Empty body before any response: nothing to store until dim is known.
		return nil
	}
	chunks := make([]store.VaultChunk, len(parts))
	for i := range parts {
		chunks[i] = store.VaultChunk{Index: i, Body: parts[i], Embedding: vecs[i]}
	}
	return ix.Vec.ReplaceVaultChunks(d.TenantID, d.ID, d.SHA256, ix.Client.Model(), dim, chunks)
}

// BackfillResult counts one Backfill run.
type BackfillResult struct {
	Embedded int    `json:"embedded"`
	Failed   int    `json:"failed"`
	Remain   bool   `json:"remain"`
	Error    string `json:"error,omitempty"`
}

// Backfill embeds up to limit docs of tenant ("" = all) whose chunks are
// missing or stale. Requests follow the client's rate limit. It stops at the
// first endpoint error so a bad key does not burn through the backlog.
func (ix *Indexer) Backfill(ctx context.Context, tenant string, limit int) (BackfillResult, error) {
	var res BackfillResult
	if ix == nil {
		return res, errors.New("embeddings not configured")
	}
	if limit <= 0 {
		limit = 200
	}
	if ix.Client.Dim() == 0 {
		// Learn the vector size with one probe so the backlog query can match it.
		if _, err := ix.Client.Embed(ctx, []string{"dim probe"}); err != nil {
			res.Error = err.Error()
			ix.noteErr(err)
			return res, err
		}
	}
	after := ""
	for res.Embedded+res.Failed < limit {
		batch, err := ix.Vec.VaultEmbedBacklog(tenant, ix.Client.Model(), ix.Client.Dim(), after, 50)
		if err != nil {
			return res, err
		}
		if len(batch) == 0 {
			return res, nil
		}
		for _, d := range batch {
			if res.Embedded+res.Failed >= limit {
				res.Remain = true
				return res, nil
			}
			after = d.ID
			if err := ix.IndexDoc(ctx, d.ID); err != nil {
				res.Failed++
				res.Error = err.Error()
				ix.noteErr(err)
				if errors.Is(err, embed.ErrResponse) || ctx.Err() != nil {
					res.Remain = true
					return res, err
				}
				continue
			}
			res.Embedded++
		}
	}
	more, err := ix.Vec.VaultEmbedBacklog(tenant, ix.Client.Model(), ix.Client.Dim(), after, 1)
	res.Remain = err == nil && len(more) > 0
	return res, nil
}

// SearchVector embeds q and ranks visible chunks.
func (ix *Indexer) SearchVector(ctx context.Context, tenant, q string, v store.VaultViewer, limit int) ([]store.VaultVectorHit, error) {
	vecs, err := ix.Client.Embed(ctx, []string{q})
	if err != nil {
		return nil, err
	}
	return ix.Vec.SearchVaultVector(tenant, v, ix.Client.Model(), vecs[0], limit)
}

// hybrid merges keyword hits with vector hits by reciprocal rank. Without an
// indexer, or on any vector error, the keyword hits are returned unchanged.
func (s *Service) hybrid(tenant, q string, v store.VaultViewer, kw []store.VaultSearchHit) []store.VaultSearchHit {
	ix := ActiveIndexer()
	if ix == nil || strings.TrimSpace(q) == "" {
		return kw
	}
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()
	vh, err := ix.SearchVector(ctx, tenant, q, v, store.VaultSearchLimit)
	if err != nil {
		ix.noteErr(err)
		return kw
	}
	return mergeRRF(kw, vh, store.VaultSearchLimit)
}

func mergeRRF(kw []store.VaultSearchHit, vh []store.VaultVectorHit, limit int) []store.VaultSearchHit {
	type entry struct {
		hit   store.VaultSearchHit
		score float64
		kw    bool
		vec   bool
	}
	byID := map[string]*entry{}
	var order []string
	for i, h := range kw {
		e := byID[h.ID]
		if e == nil {
			e = &entry{hit: h}
			byID[h.ID] = e
			order = append(order, h.ID)
		}
		if !e.kw {
			e.kw = true
			e.score += 1 / (rrfK + float64(i+1))
		}
	}
	rank := 0
	for _, h := range vh {
		e := byID[h.DocID]
		if e != nil && e.vec {
			continue // best chunk per doc only
		}
		rank++
		if e == nil {
			e = &entry{hit: store.VaultSearchHit{ID: h.DocID, Snippet: clip(h.Body, 240)}}
			byID[h.DocID] = e
			order = append(order, h.DocID)
		}
		e.vec = true
		e.score += 1 / (rrfK + float64(rank))
	}
	out := make([]*entry, 0, len(order))
	for _, id := range order {
		e := byID[id]
		switch {
		case e.kw && e.vec:
			e.hit.Match = MatchBoth
		case e.vec:
			e.hit.Match = MatchVector
		default:
			e.hit.Match = MatchKeyword
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].score > out[j].score })
	if len(out) > limit {
		out = out[:limit]
	}
	res := make([]store.VaultSearchHit, len(out))
	for i, e := range out {
		res[i] = e.hit
	}
	return res
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

// Chunk splits body into overlapping pieces of about chunkRunes characters,
// preferring a paragraph or line break near the end of each piece.
func Chunk(body string) []string {
	r := []rune(strings.TrimSpace(body))
	if len(r) == 0 {
		return nil
	}
	var out []string
	for start := 0; start < len(r) && len(out) < maxChunks; {
		end := start + chunkRunes
		if end >= len(r) {
			out = append(out, strings.TrimSpace(string(r[start:])))
			break
		}
		cut := end
		for i := end; i > end-chunkRunes/4; i-- {
			if r[i-1] == '\n' {
				cut = i
				break
			}
		}
		piece := strings.TrimSpace(string(r[start:cut]))
		if piece != "" {
			out = append(out, piece)
		}
		next := cut - chunkOverlap
		if next <= start {
			next = cut
		}
		start = next
	}
	return out
}
