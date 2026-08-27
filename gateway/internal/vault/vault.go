// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package vault

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// Service is the knowledge vault: registry + markdown/text files on disk.
type Service struct {
	Store store.StoreIface
	Root  string
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
	rel = filepath.FromSlash(rel)
	p := filepath.Join(s.Root, rel)
	if !underRoot(s.Root, p) {
		return "", errors.New("path escape")
	}
	return p, nil
}

func (s *Service) refreshLinks(id, body string) error {
	return s.Store.ReplaceVaultLinks(id, ParseWikilinks(body))
}

// Put writes {slug}.md (or the existing path for that title) and upserts the registry.
func (s *Service) Put(title, body string) (*store.VaultDoc, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errors.New("title is required")
	}
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return nil, err
	}
	var rel string
	existing, err := s.Store.FindVaultDocByTitle(title)
	if err == nil && existing != nil && existing.Path != "" {
		rel = existing.Path
	} else {
		rel = uniqueRel(s.Root, Slug(title)+".md")
	}
	abs, err := s.absPath(rel)
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
		Title:  title,
		Path:   filepath.ToSlash(rel),
		SHA256: sha256Hex([]byte(body)),
		Mtime:  fi.ModTime().UTC(),
		Body:   body,
	}
	if existing != nil {
		doc.ID = existing.ID
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

// Get returns a registry row, preferring on-disk body when the file is still under root.
func (s *Service) Get(id string) (*store.VaultDoc, error) {
	d, err := s.Store.GetVaultDoc(id)
	if err != nil {
		return nil, err
	}
	if d.Path == "" {
		return d, nil
	}
	abs, err := s.absPath(d.Path)
	if err != nil {
		return d, nil
	}
	if !fileInsideRoot(s.Root, abs) {
		return d, nil
	}
	b, err := os.ReadFile(abs)
	if err == nil {
		d.Body = string(b)
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

// Search is lexical FTS5 / substring. Semantic search is DI-09.
func (s *Service) Search(q string) ([]store.VaultSearchHit, error) {
	hits, err := s.Store.SearchVault(q)
	if err != nil {
		return nil, err
	}
	if hits == nil {
		hits = []store.VaultSearchHit{}
	}
	return hits, nil
}

type diskFile struct {
	Rel  string
	Body []byte
	Hash string
	Mod  os.FileInfo
}

func fileInsideRoot(root, path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	target := path
	if info.Mode()&os.ModeSymlink != 0 {
		ev, err := filepath.EvalSymlinks(path)
		if err != nil {
			return false
		}
		target = ev
	}
	return underRoot(root, target)
}

func (s *Service) collectFiles() ([]diskFile, error) {
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return nil, err
	}
	var out []diskFile
	err := filepath.WalkDir(s.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == s.Root {
			return nil
		}
		if d.IsDir() {
			if d.Type()&os.ModeSymlink != 0 && !fileInsideRoot(s.Root, path) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext != ".md" && ext != ".txt" {
			return nil
		}
		if !fileInsideRoot(s.Root, path) {
			return nil
		}
		rel, err := filepath.Rel(s.Root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == ".." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil
		}
		out = append(out, diskFile{Rel: rel, Body: body, Hash: sha256Hex(body), Mod: info})
		return nil
	})
	return out, err
}

// Sync walks GOSO_VAULT_DIR for *.md / *.txt, upserts by relative path, drops vanished rows.
func (s *Service) Sync() (*SyncResult, error) {
	files, err := s.collectFiles()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(files))
	res := &SyncResult{}
	for _, f := range files {
		seen[f.Rel] = struct{}{}
		existing, err := s.Store.FindVaultDocByPath(f.Rel)
		if err == nil && existing != nil && existing.SHA256 == f.Hash {
			res.Skipped++
			continue
		}
		body := string(f.Body)
		title := titleFrom(f.Rel, body)
		if existing != nil && !hasH1(body) {
			title = existing.Title
		}
		doc := store.VaultDoc{
			Title:  title,
			Path:   f.Rel,
			SHA256: f.Hash,
			Mtime:  f.Mod.ModTime().UTC(),
			Body:   body,
		}
		if existing != nil {
			doc.ID = existing.ID
		}
		saved, err := s.Store.PutVaultDoc(doc)
		if err != nil {
			return nil, err
		}
		if err := s.refreshLinks(saved.ID, body); err != nil {
			return nil, err
		}
		res.Upserted++
	}
	for _, d := range s.Store.ListVaultDocs() {
		if d == nil {
			continue
		}
		if _, ok := seen[d.Path]; ok {
			continue
		}
		if err := s.Store.DeleteVaultDoc(d.ID); err != nil {
			return nil, err
		}
		res.Deleted++
	}
	_ = s.Store.ReResolveVaultLinks()
	return res, nil
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
