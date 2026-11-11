// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Package vaultimport loads one tenant from an offline GoClaw vault export
// into goso's existing vault_docs and vault_links rows.
//
// The export is JSON with the GoClaw column names vault_documents,
// vault_links, and vault_versions. Embedding and tsv are not fields of the
// reader, so the decoder drops them. Only the highest vault_versions row for
// each document is kept, and that text is written into the existing body
// column. This package does not create a vault_versions table.
//
// Link type and context are stored in the existing vault_links.raw text
// ("[[<goso id>|<type>: <context>]]"). Resolution still uses the goso id.
//
// Each imported body ends with a one-line provenance marker (source id and
// a hash of the title plus the body prefix). A later run overwrites a row
// only when that marker names the same GoClaw document and the hash still
// matches. Every other explicit row at the path is rejected as path-taken.
//
// KeepPrefix stores the path with the root still attached. The default
// strips that prefix. A workspace root (".", "/app/workspace") accepts
// every safe relative path in one run. Any tenants/<other> path is
// foreign-tenant. Two scopes at one logical path keep the narrower
// document (personal before team) and report scope-conflict. goso has no
// scope column, so the winner is an ordinary tenant row.
//
// A run names one source tenant id and one root folder. A document is imported
// only when both match. Paths that leave the root, walk with "..", are
// absolute, look like a symlink, or enter tenants/<other> are rejected and
// logged as a path. Logs and the report contain paths and reason codes only.
package vaultimport

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// errSymlink is a destination that is a symlink or is reached through one.
// The importer does not follow it and does not replace it.
var errSymlink = errors.New("symlink")

// Options selects the single tenant for this run.
type Options struct {
	// SourceTenant is the GoClaw vault_documents.tenant_id. Match is case-insensitive after trim.
	SourceTenant string
	// Root is that tenant's folder. A path must resolve inside it.
	Root string
	// DestTenant is the goso tenant_id written on vault_docs. It is not defaulted.
	DestTenant string
	// VaultRoot, when set, also publishes the body under t/<tenant>/ so the
	// existing vault service can confirm the file. Empty writes the registry only.
	VaultRoot string
	// KeepPrefix stores the cleaned source path without removing Root.
	// The default strips Root. Use KeepPrefix with root "." or /app/workspace
	// to keep original folder prefixes in one run.
	KeepPrefix bool
	// SrcDir, when set, is the source workspace. A document whose file is
	// missing there is path-missing and is not inserted. The version text in
	// the export is still the body when the file exists.
	SrcDir string
	// DryRun reports counts and rejections and does not write.
	DryRun bool
	// Log receives rejection lines. Nil discards them. Arguments are paths and
	// reason codes, never document bodies.
	Log Logger
}

// Logger is the rejection log. Printf must not be given a document body.
type Logger interface {
	Printf(format string, args ...any)
}

// Report is the idempotent run summary.
//
// Inserted and updated count vault_docs rows this run would write.
// Skipped counts source rows whose tenant id does not match, plus rows that
// already match. Rejected counts path-policy failures and dropped links.
type Report struct {
	Inserted   int         `json:"inserted"`
	Updated    int         `json:"updated"`
	Skipped    int         `json:"skipped"`
	Rejected   int         `json:"rejected"`
	DryRun     bool        `json:"dry_run"`
	Rejections []Rejection `json:"rejections,omitempty"`
}

// Rejection is one dropped path. Path and Target are export paths, not bodies.
type Rejection struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Target string `json:"target,omitempty"`
	Reason string `json:"reason"`
}

// Export is an offline dump. Embedding and tsv are not represented.
type Export struct {
	Documents []Document `json:"vault_documents"`
	Links     []Link     `json:"vault_links"`
	Versions  []Version  `json:"vault_versions"`
}

// Document is one vault_documents row. Unknown JSON fields, including
// embedding and tsv, are ignored by the decoder.
type Document struct {
	ID          string          `json:"id"`
	TenantID    string          `json:"tenant_id"`
	Path        string          `json:"path"`
	Title       string          `json:"title"`
	DocType     string          `json:"doc_type"`
	Summary     string          `json:"summary"`
	Metadata    json.RawMessage `json:"metadata"`
	Scope       string          `json:"scope"`
	CustomScope string          `json:"custom_scope"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

// Link is one vault_links row. Metadata is not copied; type and context map
// into the existing raw column.
type Link struct {
	ID       string `json:"id"`
	FromDoc  string `json:"from_doc_id"`
	ToDoc    string `json:"to_doc_id"`
	LinkType string `json:"link_type"`
	Context  string `json:"context"`
}

// Version is one vault_versions row. Only the latest version per document is used.
type Version struct {
	ID        string `json:"id"`
	DocID     string `json:"doc_id"`
	Version   int    `json:"version"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

// ConfigError is a bad option. The text names the option, not a document.
type ConfigError struct{ Msg string }

func (e *ConfigError) Error() string { return e.Msg }

func errConfig(msg string) error { return &ConfigError{Msg: msg} }

// ReadExport decodes a GoClaw-shaped JSON dump from r.
func ReadExport(r io.Reader) (*Export, error) {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	var ex Export
	if err := dec.Decode(&ex); err != nil {
		return nil, fmt.Errorf("read export: %w", err)
	}
	return &ex, nil
}

type keptDoc struct {
	src      Document
	logical  string
	title    string
	body     string
	sum      string
	mtime    time.Time
	gosoID   string
	existing *store.VaultDoc
	docSame  bool
	fileSame bool
	drop     bool
}

type wantLink struct {
	to       *keptDoc
	linkType string
	context  string
}

// Import reads ex into st. A second call with the same ex and options does
// not write when the first call finished. st is the existing goso store.
func Import(st store.StoreIface, ex *Export, opt Options) (*Report, error) {
	if st == nil || ex == nil {
		return nil, errConfig("store and export are required")
	}
	root, err := cleanRoot(opt.Root)
	if err != nil {
		return nil, err
	}
	source := strings.TrimSpace(opt.SourceTenant)
	if source == "" {
		return nil, errConfig("source tenant id is required")
	}
	dest := strings.TrimSpace(opt.DestTenant)
	if !store.TenantOK(dest) {
		return nil, errConfig("dest tenant id is empty or not a goso tenant id")
	}
	if strings.TrimSpace(opt.SrcDir) != "" {
		if err := checkSrcDir(opt.SrcDir); err != nil {
			return nil, err
		}
	}
	rep := &Report{DryRun: opt.DryRun}
	logf := opt.Log
	if logf == nil {
		logf = discardLog{}
	}

	latest := latestVersions(ex.Versions)
	pathByID := map[string]string{}
	for _, doc := range ex.Documents {
		if id := strings.TrimSpace(doc.ID); id != "" {
			pathByID[id] = strings.TrimSpace(doc.Path)
		}
	}

	docs := append([]Document(nil), ex.Documents...)
	sort.Slice(docs, func(i, j int) bool {
		if docs[i].Path != docs[j].Path {
			return docs[i].Path < docs[j].Path
		}
		return docs[i].ID < docs[j].ID
	})

	seenID := map[string]struct{}{}
	var candidates []*keptDoc
	for _, doc := range docs {
		path := strings.TrimSpace(doc.Path)
		if !tenantMatch(doc.TenantID, source) {
			d := classifyPath(doc.Path, root, opt.KeepPrefix)
			if d.Reason != "" {
				reject(rep, logf, "doc", path, "", d.Reason)
				continue
			}
			rep.Skipped++
			logf.Printf("vaultimport: skipped doc path=%q reason=%s", path, "tenant-mismatch")
			continue
		}
		d := classifyPath(doc.Path, root, opt.KeepPrefix)
		if d.Reason != "" {
			reject(rep, logf, "doc", path, "", d.Reason)
			continue
		}
		id := strings.TrimSpace(doc.ID)
		if !sourceIDOK(id) {
			reject(rep, logf, "doc", path, "", "source-id")
			continue
		}
		if _, ok := seenID[id]; ok {
			reject(rep, logf, "doc", path, "", "duplicate-id")
			continue
		}
		seenID[id] = struct{}{}
		content := ""
		var versionAt time.Time
		if v, ok := latest[id]; ok {
			content = v.content
			versionAt = v.at
		}
		meta := scrubMeta(doc.Metadata, map[string]string{
			"doc_type":     doc.DocType,
			"scope":        doc.Scope,
			"custom_scope": doc.CustomScope,
		})
		plain := composeBody(content, doc.Summary, meta)
		title := strings.TrimSpace(doc.Title)
		if title == "" {
			title = titleFromLogical(d.Logical)
		}
		if title == "" {
			reject(rep, logf, "doc", path, "", "title-required")
			continue
		}
		body := stampImport(title, plain, id)
		mtime := versionAt
		if t, ok := parseTS(doc.UpdatedAt); ok && t.After(mtime) {
			mtime = t
		}
		if mtime.IsZero() {
			if t, ok := parseTS(doc.CreatedAt); ok {
				mtime = t
			}
		}
		if mtime.IsZero() {
			mtime = time.Unix(0, 0).UTC()
		}
		item := &keptDoc{
			src:     doc,
			logical: d.Logical,
			title:   title,
			body:    body,
			sum:     hashBody(body),
			mtime:   mtime.UTC(),
		}
		candidates = append(candidates, item)
	}
	kept, losers := preferNarrowerScope(candidates)
	for _, loser := range losers {
		reject(rep, logf, "doc", strings.TrimSpace(loser.item.src.Path), "", loser.reason)
	}
	bySource := map[string]*keptDoc{}
	for _, item := range kept {
		bySource[strings.TrimSpace(item.src.ID)] = item
	}
	if strings.TrimSpace(opt.SrcDir) != "" {
		var present []*keptDoc
		for _, item := range kept {
			state, err := sourceFileState(opt.SrcDir, item.src.Path)
			if err != nil {
				return rep, err
			}
			if state == sourceMissing {
				reject(rep, logf, "doc", strings.TrimSpace(item.src.Path), "", "path-missing")
				delete(bySource, strings.TrimSpace(item.src.ID))
				continue
			}
			if state == sourceSymlink {
				reject(rep, logf, "doc", strings.TrimSpace(item.src.Path), "", reasonSymlink)
				delete(bySource, strings.TrimSpace(item.src.ID))
				continue
			}
			present = append(present, item)
		}
		kept = present
	}

	for _, item := range kept {
		existing, err := st.FindVaultDocByPath(dest, item.logical)
		if errors.Is(err, store.ErrNotFound) {
			existing = nil
		} else if err != nil {
			return rep, err
		}
		if existing != nil && !existing.OwnerExplicit {
			reject(rep, logf, "doc", strings.TrimSpace(item.src.Path), "", "owner-unclear")
			item.drop = true
			delete(bySource, strings.TrimSpace(item.src.ID))
			continue
		}
		// Same logical path is not enough. Overwrite only a row this importer
		// wrote from the same GoClaw id and that nobody has edited since.
		if existing != nil && !importUntouched(existing.Title, existing.Body, strings.TrimSpace(item.src.ID)) {
			reject(rep, logf, "doc", strings.TrimSpace(item.src.Path), "", "path-taken")
			item.drop = true
			delete(bySource, strings.TrimSpace(item.src.ID))
			continue
		}
		item.existing = existing
		if existing != nil {
			item.gosoID = existing.ID
			item.docSame = existing.Title == item.title &&
				existing.Path == item.logical &&
				existing.Body == item.body &&
				existing.SHA256 == item.sum &&
				existing.Mtime.Equal(item.mtime) &&
				existing.OwnerExplicit
		}
		item.fileSame = true
		if strings.TrimSpace(opt.VaultRoot) != "" {
			same, err := fileMatches(opt.VaultRoot, dest, item.logical, []byte(item.body))
			if errors.Is(err, errSymlink) {
				reject(rep, logf, "doc", strings.TrimSpace(item.src.Path), "", reasonSymlink)
				item.drop = true
				delete(bySource, strings.TrimSpace(item.src.ID))
				continue
			}
			if err != nil {
				return rep, err
			}
			item.fileSame = same
		}
	}

	wants := map[string][]wantLink{}
	for _, l := range ex.Links {
		fromID := strings.TrimSpace(l.FromDoc)
		toID := strings.TrimSpace(l.ToDoc)
		from := bySource[fromID]
		to := bySource[toID]
		fromPath := pathByID[fromID]
		toPath := pathByID[toID]
		if from == nil {
			reason := "source-rejected"
			if _, known := pathByID[fromID]; !known {
				reason = "source-missing"
			}
			reject(rep, logf, "link", fromPath, toPath, reason)
			continue
		}
		if to == nil {
			reason := "outside-tenant"
			if _, known := pathByID[toID]; !known {
				reason = "target-missing"
				toPath = ""
			}
			reject(rep, logf, "link", fromPath, toPath, reason)
			continue
		}
		wants[fromID] = append(wants[fromID], wantLink{to: to, linkType: l.LinkType, context: l.Context})
	}

	var fresh []*keptDoc
	for _, item := range kept {
		if item.drop || item.existing != nil {
			continue
		}
		fresh = append(fresh, item)
		if opt.DryRun {
			rep.Inserted++
			continue
		}
		if strings.TrimSpace(opt.VaultRoot) != "" {
			if _, err := publishFile(opt.VaultRoot, dest, item.logical, []byte(item.body)); err != nil {
				if errors.Is(err, errSymlink) {
					reject(rep, logf, "doc", strings.TrimSpace(item.src.Path), "", reasonSymlink)
					item.drop = true
					delete(bySource, strings.TrimSpace(item.src.ID))
					continue
				}
				return rep, err
			}
		}
		saved, err := st.PutVaultDoc(store.VaultDoc{
			Title:         item.title,
			Path:          item.logical,
			SHA256:        item.sum,
			Mtime:         item.mtime,
			Body:          item.body,
			TenantID:      dest,
			OwnerExplicit: true,
		})
		if err != nil {
			return rep, err
		}
		item.gosoID = saved.ID
		rep.Inserted++
	}

	replaced := false
	for _, item := range kept {
		if item.drop || item.existing == nil {
			continue
		}
		inners, pending := innersFor(item, wants)
		linksSame := false
		if !pending {
			linksSame, err = sameLinks(st, item.gosoID, inners)
			if err != nil {
				return rep, err
			}
		}
		if item.docSame && item.fileSame && linksSame {
			rep.Skipped++
			continue
		}
		if opt.DryRun {
			rep.Updated++
			continue
		}
		if !item.fileSame {
			if _, err := publishFile(opt.VaultRoot, dest, item.logical, []byte(item.body)); err != nil {
				if errors.Is(err, errSymlink) {
					reject(rep, logf, "doc", strings.TrimSpace(item.src.Path), "", reasonSymlink)
					continue
				}
				return rep, err
			}
		}
		if !item.docSame {
			if _, err := st.PutVaultDoc(store.VaultDoc{
				ID:            item.existing.ID,
				Title:         item.title,
				Path:          item.logical,
				SHA256:        item.sum,
				Mtime:         item.mtime,
				Body:          item.body,
				TenantID:      dest,
				OwnerExplicit: true,
			}); err != nil {
				return rep, err
			}
		}
		if !linksSame {
			if err := st.ReplaceVaultLinks(item.gosoID, inners); err != nil {
				return rep, err
			}
			replaced = true
		}
		rep.Updated++
	}

	if !opt.DryRun {
		for _, item := range fresh {
			if item.drop || item.gosoID == "" {
				continue
			}
			inners, _ := innersFor(item, wants)
			if len(inners) == 0 {
				continue
			}
			if err := st.ReplaceVaultLinks(item.gosoID, inners); err != nil {
				return rep, err
			}
			replaced = true
		}
		if replaced {
			if err := st.ReResolveVaultLinks(); err != nil {
				return rep, err
			}
		}
	}
	return rep, nil
}

type scopeLoser struct {
	item   *keptDoc
	reason string
}

// preferNarrowerScope keeps one document per logical path. personal beats
// team, team beats shared, shared beats custom, and a named scope beats an
// empty one. Same-scope extras are path-collision. vault_docs has no scope
// column, so this is the only distinction the existing row can carry.
func preferNarrowerScope(items []*keptDoc) ([]*keptDoc, []scopeLoser) {
	groups := map[string][]*keptDoc{}
	var order []string
	for _, item := range items {
		if _, ok := groups[item.logical]; !ok {
			order = append(order, item.logical)
		}
		groups[item.logical] = append(groups[item.logical], item)
	}
	var winners []*keptDoc
	var losers []scopeLoser
	for _, logical := range order {
		group := append([]*keptDoc(nil), groups[logical]...)
		sort.Slice(group, func(i, j int) bool {
			return strings.TrimSpace(group[i].src.ID) < strings.TrimSpace(group[j].src.ID)
		})
		best := group[0]
		for _, item := range group[1:] {
			if scopeRank(item.src.Scope) < scopeRank(best.src.Scope) {
				best = item
			}
		}
		winners = append(winners, best)
		bestRank := scopeRank(best.src.Scope)
		for _, item := range group {
			if item == best {
				continue
			}
			reason := "path-collision"
			if scopeRank(item.src.Scope) != bestRank {
				reason = "scope-conflict"
			}
			losers = append(losers, scopeLoser{item: item, reason: reason})
		}
	}
	return winners, losers
}

func scopeRank(scope string) int {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "personal":
		return 0
	case "team":
		return 1
	case "shared":
		return 2
	case "custom":
		return 3
	default:
		return 4
	}
}

type versionPick struct {
	n       int
	at      time.Time
	content string
}

func latestVersions(versions []Version) map[string]versionPick {
	out := map[string]versionPick{}
	for _, v := range versions {
		id := strings.TrimSpace(v.DocID)
		if id == "" {
			continue
		}
		at, _ := parseTS(v.CreatedAt)
		cur, ok := out[id]
		replace := !ok || v.Version > cur.n || (v.Version == cur.n && at.After(cur.at))
		if replace {
			out[id] = versionPick{n: v.Version, at: at, content: v.Content}
		}
	}
	return out
}

func tenantMatch(docTenant, source string) bool {
	return strings.EqualFold(strings.TrimSpace(docTenant), strings.TrimSpace(source))
}

func titleFromLogical(logical string) string {
	base := logical
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	base = strings.TrimSuffix(base, ".md")
	base = strings.TrimSuffix(base, ".txt")
	return strings.TrimSpace(base)
}

// innersFor builds link inners. pending is true when a target does not yet
// have a goso id (a new document during dry-run), so the link set cannot match.
func innersFor(from *keptDoc, wants map[string][]wantLink) ([]string, bool) {
	var inners []string
	seen := map[string]struct{}{}
	pending := false
	for _, w := range wants[strings.TrimSpace(from.src.ID)] {
		if w.to == nil || w.to.drop {
			continue
		}
		if w.to.gosoID == "" {
			pending = true
			continue
		}
		inner := linkInner(w.to.gosoID, w.linkType, w.context)
		if _, ok := seen[inner]; ok {
			continue
		}
		seen[inner] = struct{}{}
		inners = append(inners, inner)
	}
	sort.Strings(inners)
	return inners, pending
}

func sameLinks(st store.StoreIface, fromID string, inners []string) (bool, error) {
	if fromID == "" {
		return len(inners) == 0, nil
	}
	ob, _, err := st.ListVaultDocLinks(fromID)
	if err != nil {
		return false, err
	}
	got := map[string]struct{}{}
	for _, l := range ob {
		if strings.TrimSpace(l.Raw) == "" {
			continue
		}
		got[l.Raw] = struct{}{}
	}
	want := map[string]struct{}{}
	for _, inner := range inners {
		want[storedRaw(inner)] = struct{}{}
	}
	if len(got) != len(want) {
		return false, nil
	}
	for raw := range want {
		if _, ok := got[raw]; !ok {
			return false, nil
		}
	}
	return true, nil
}

func reject(rep *Report, logf Logger, kind, path, target, reason string) {
	rep.Rejected++
	rep.Rejections = append(rep.Rejections, Rejection{Kind: kind, Path: path, Target: target, Reason: reason})
	if target != "" {
		logf.Printf("vaultimport: rejected %s path=%q target=%q reason=%s", kind, path, target, reason)
		return
	}
	logf.Printf("vaultimport: rejected %s path=%q reason=%s", kind, path, reason)
}

type discardLog struct{}

func (discardLog) Printf(string, ...any) {}
