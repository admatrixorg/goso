// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Service is the knowledge vault: registry + markdown/text files on disk.
// Files for a tenant live under Root/t/<tenant>/, a server-assigned namespace.
// The logical path is not an agent, project, or customer grant.
type Service struct {
	Store store.StoreIface
	Root  string
	mu    sync.Mutex
	// collectHook, when set, replaces the tenant directory walk. Tests use it
	// for empty, failed, and partial scans. Production leaves it nil.
	collectHook func(tenant string) ([]diskFile, error)
}

// SyncResult counts filesystem reconciliation work.
type SyncResult struct {
	Upserted int `json:"upserted"`
	Skipped  int `json:"skipped"`
	Deleted  int `json:"deleted"`
}

// Dir returns GOSO_VAULT_DIR or data/vault under the process cwd.
func Dir() string {
	if v := strings.TrimSpace(os.Getenv("GOSO_VAULT_DIR")); v != "" {
		return v
	}
	return filepath.Join("data", "vault")
}

// New wraps a store with a vault root.
func New(st store.StoreIface, root string) *Service {
	if strings.TrimSpace(root) == "" {
		root = Dir()
	}
	return &Service{Store: st, Root: root}
}

// ParseWikilinks returns unique inner targets from [[Title]] / [[id]] (optional |alias).
func ParseWikilinks(body string) []string {
	var out []string
	seen := make(map[string]struct{})
	s := body
	for {
		i := strings.Index(s, "[[")
		if i < 0 {
			break
		}
		s = s[i+2:]
		j := strings.Index(s, "]]")
		if j < 0 {
			break
		}
		inner := strings.TrimSpace(s[:j])
		s = s[j+2:]
		if k := strings.Index(inner, "|"); k >= 0 {
			inner = strings.TrimSpace(inner[:k])
		}
		if inner == "" {
			continue
		}
		if _, ok := seen[inner]; ok {
			continue
		}
		seen[inner] = struct{}{}
		out = append(out, inner)
	}
	return out
}

// Slug turns a title into a filename stem.
func Slug(title string) string {
	title = strings.TrimSpace(strings.ToLower(title))
	var b strings.Builder
	prevDash := false
	for _, r := range title {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "note"
	}
	return out
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func canon(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if ev, err := filepath.EvalSymlinks(abs); err == nil {
		return ev
	}
	dir, base := filepath.Split(abs)
	if ev, err := filepath.EvalSymlinks(dir); err == nil {
		return filepath.Join(ev, base)
	}
	return abs
}

func underRoot(root, p string) bool {
	absRoot := canon(root)
	absP := canon(p)
	rel, err := filepath.Rel(absRoot, absP)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return false
	}
	return true
}

func titleFrom(rel, body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "#\t") {
			t := strings.TrimSpace(strings.TrimPrefix(line, "#"))
			if t != "" {
				return t
			}
		}
	}
	base := filepath.Base(rel)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if stem == "" {
		return base
	}
	return stem
}

func uniqueRel(root, rel string) string {
	ext := filepath.Ext(rel)
	stem := strings.TrimSuffix(rel, ext)
	candidate := rel
	for n := 2; ; n++ {
		p := filepath.Join(root, filepath.FromSlash(candidate))
		_, err := os.Lstat(p)
		if err != nil {
			return candidate
		}
		candidate = stem + "-" + strconv.Itoa(n) + ext
	}
}

func (s *Service) absPath(rel string) (string, error) {
	if security.HasDotDot(rel) {
		return "", errors.New("path escape")
	}
	rel = filepath.FromSlash(rel)
	p := filepath.Join(s.Root, rel)
	if !underRoot(s.Root, p) {
		return "", errors.New("path escape")
	}
	if err := security.Confine(p); err != nil {
		return "", err
	}
	return p, nil
}

func (s *Service) refreshLinks(id, body string) error {
	return s.Store.ReplaceVaultLinks(id, ParseWikilinks(body))
}

// Put writes {slug}.md (or the existing path for that title) and upserts the registry.
func (s *Service) Put(title, body string) (*store.VaultDoc, error) {
	return s.PutTenant(title, body, store.DefaultTenant)
}

// PutTenant writes a vault doc stamped with tenant_id.
func (s *Service) PutTenant(title, body, tenantID string) (*store.VaultDoc, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errors.New("title is required")
	}
	tenantID = store.NormalizeTenant(tenantID)
	dir, err := s.tenantDir(tenantID)
	if err != nil {
		return nil, err
	}
	if err := security.Confine(dir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var rel string
	existing, err := s.Store.FindVaultDocByTitle(tenantID, title)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if existing != nil && !store.SameTenant(existing.TenantID, tenantID) {
		return nil, ErrTargetTenant
	}
	if existing != nil && existing.Path != "" {
		rel = existing.Path
	} else {
		rel = uniqueRel(dir, Slug(title)+".md")
	}
	abs, err := s.namespaced(tenantID, rel)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
		return nil, err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	doc := store.VaultDoc{
		Title:         title,
		Path:          filepath.ToSlash(rel),
		SHA256:        sha256Hex([]byte(body)),
		Mtime:         fi.ModTime().UTC(),
		Body:          body,
		TenantID:      tenantID,
		OwnerExplicit: true,
	}
	if existing != nil {
		doc.ID = existing.ID
		doc.TenantID = tenantID
	}
	saved, err := s.Store.PutVaultDoc(doc)
	if err != nil {
		return nil, err
	}
	if err := s.refreshLinks(saved.ID, body); err != nil {
		return nil, err
	}
	_ = s.Store.ReResolveVaultLinks()
	return saved, nil
}

// Get returns one tenant's registry row. A different tenant is not found and
// its file is not read. On-disk body is preferred when the file is inside
// that tenant's namespace, or a legacy path owned by this row alone.
func (s *Service) Get(tenant, id string) (*store.VaultDoc, error) {
	d, err := s.Store.GetVaultDoc(id)
	if err != nil {
		return nil, err
	}
	if !store.SameTenant(d.TenantID, tenant) {
		return nil, store.ErrNotFound
	}
	if abs, ok := s.locate(d); ok {
		b, err := os.ReadFile(abs)
		if err == nil {
			d.Body = string(b)
		}
	}
	return d, nil
}

// List returns registry rows.
func (s *Service) List() []*store.VaultDoc {
	list := s.Store.ListVaultDocs()
	if list == nil {
		return []*store.VaultDoc{}
	}
	return list
}

// Links returns outbound and inbound [[wikilink]] edges.
func (s *Service) Links(id string) (outbound, inbound []store.VaultLink, err error) {
	return s.Store.ListVaultDocLinks(id)
}

// Search is lexical FTS5 / substring inside one tenant. The store applies the
// tenant predicate before rank and LIMIT. Semantic search is DI-09.
func (s *Service) Search(tenant, q string) ([]store.VaultSearchHit, error) {
	hits, err := s.Store.SearchVault(tenant, q)
	if err != nil {
		return nil, err
	}
	if hits == nil {
		hits = []store.VaultSearchHit{}
	}
	return hits, nil
}

// Sync is the old global reconcile. It no longer scans or deletes. Scoped
// sync is SyncTenant, and only when vault_scoped_sync is on.
func (s *Service) Sync() (*SyncResult, error) {
	return nil, ErrSyncDisabled
}

func hasH1(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "#\t") {
			return true
		}
	}
	return false
}
