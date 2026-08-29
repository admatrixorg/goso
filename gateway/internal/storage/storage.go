// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package storage

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mqglobal/goso/gateway/internal/security"
)

const (
	// MaxFileBytes is the upload/download/preview file cap (1 MiB, same as MaxAPIBody).
	MaxFileBytes = 1 << 20
	// PreviewBytes is the bounded text preview cap.
	PreviewBytes = 64 << 10
	// DefaultQuotaBytes is used when GOSO_STORAGE_MAX_BYTES is unset.
	DefaultQuotaBytes = 64 << 20
	maxListEnts       = 512
	maxWalkFiles      = 10_000
	binarySniff       = 512
)

var (
	ErrNotConfigured   = errors.New("not_configured")
	ErrPathEscape      = errors.New("path escape")
	ErrNotFound        = errors.New("not found")
	ErrNotFile         = errors.New("not a file")
	ErrNotDir          = errors.New("not a directory")
	ErrTooLarge        = errors.New("too large")
	ErrType            = errors.New("type not allowed")
	ErrHidden          = errors.New("hidden path")
	ErrSecret          = errors.New("secret path")
	ErrConfirm         = errors.New("confirm does not match")
	ErrConfirmRequired = errors.New("confirm is required")
	ErrQuota           = errors.New("quota exceeded")
	ErrName            = errors.New("name is invalid")
	ErrNotEmpty        = errors.New("directory not empty")
	ErrRoot            = errors.New("cannot delete workspace root")
)

// Crumb is one breadcrumb segment.
type Crumb struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Entry is listing metadata only — never file contents.
type Entry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Dir   bool   `json:"dir"`
	Size  int64  `json:"size"`
	Type  string `json:"type"`
	Mtime string `json:"mtime"`
}

// Listing is GET /api/storage.
type Listing struct {
	Configured    bool    `json:"configured"`
	Path          string  `json:"path"`
	Parent        string  `json:"parent,omitempty"`
	Breadcrumbs   []Crumb `json:"breadcrumbs"`
	Entries       []Entry `json:"entries"`
	UsedBytes     int64   `json:"used_bytes"`
	MaxBytes      int64   `json:"max_bytes"`
	HiddenSkipped int     `json:"hidden_skipped"`
	Truncated     bool    `json:"truncated"`
}

// Preview is bounded GET /api/storage/preview. Never includes credential values.
type Preview struct {
	Path      string `json:"path"`
	Type      string `json:"type"`
	Size      int64  `json:"size"`
	Kind      string `json:"kind"`
	Text      string `json:"text,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	Bytes     int    `json:"bytes"`
}

var secretBases = map[string]struct{}{
	"secrets":         {},
	"credentials":     {},
	"runtime":         {},
	".ssh":            {},
	".git":            {},
	".env":            {},
	"id_rsa":          {},
	"id_dsa":          {},
	"id_ecdsa":        {},
	"id_ed25519":      {},
	"authorized_keys": {},
}

var secretExt = map[string]struct{}{
	".pem": {}, ".key": {}, ".p12": {}, ".pfx": {}, ".crt": {}, ".cer": {},
	".ppk": {}, ".p8": {}, ".der": {},
}

var allowExt = map[string]struct{}{
	".txt": {}, ".md": {}, ".markdown": {}, ".csv": {}, ".json": {},
	".yaml": {}, ".yml": {}, ".toml": {}, ".log": {}, ".pdf": {},
	".png": {}, ".jpg": {}, ".jpeg": {}, ".gif": {}, ".webp": {},
}

var secretVal = [][]byte{
	[]byte("-----BEGIN "),
	[]byte("sk-"),
	[]byte("gsk_"),
	[]byte("xai-"),
	[]byte("Bearer "),
	[]byte("PRIVATE KEY"),
	[]byte("API_KEY="),
	[]byte("api_key="),
	[]byte("SECRET="),
	[]byte("password="),
}

func configured() bool {
	return strings.TrimSpace(os.Getenv("GOSO_WORKSPACE")) != ""
}

func workspaceAbs() (string, error) {
	raw := strings.TrimSpace(os.Getenv("GOSO_WORKSPACE"))
	if raw == "" {
		return "", ErrNotConfigured
	}
	if security.HasDotDot(raw) {
		return "", ErrPathEscape
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", ErrPathEscape
	}
	if ev, err := filepath.EvalSymlinks(abs); err == nil {
		abs = ev
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return "", ErrNotDir
	}
	return abs, nil
}

func canonPath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if ev, err := filepath.EvalSymlinks(abs); err == nil {
		return ev
	}
	var missing []string
	cur := abs
	for {
		if ev, err := filepath.EvalSymlinks(cur); err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				ev = filepath.Join(ev, missing[i])
			}
			return ev
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs
		}
		missing = append(missing, filepath.Base(cur))
		cur = parent
	}
}

func confineUnder(root, p string) error {
	if security.HasDotDot(p) {
		return ErrPathEscape
	}
	if err := security.Confine(p); err != nil {
		return ErrPathEscape
	}
	absRoot := canonPath(root)
	absP := canonPath(p)
	rel, err := filepath.Rel(absRoot, absP)
	if err != nil {
		return ErrPathEscape
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return ErrPathEscape
	}
	return nil
}

// Jail resolves a relative or absolute path inside GOSO_WORKSPACE.
func Jail(arg string) (abs, rel string, err error) {
	root, err := workspaceAbs()
	if err != nil {
		return "", "", err
	}
	arg = strings.TrimSpace(arg)
	if strings.IndexByte(arg, 0) >= 0 {
		return "", "", ErrPathEscape
	}
	if security.HasDotDot(arg) {
		return "", "", ErrPathEscape
	}
	var cand string
	if arg == "" || arg == "." || arg == "/" {
		cand = root
	} else if filepath.IsAbs(arg) {
		cand = arg
	} else {
		cand = filepath.Join(root, arg)
	}
	cand, err = filepath.Abs(cand)
	if err != nil {
		return "", "", ErrPathEscape
	}
	if err := confineUnder(root, cand); err != nil {
		return "", "", err
	}
	canonCand := canonPath(cand)
	if err := confineUnder(root, canonCand); err != nil {
		return "", "", err
	}
	rel, err = filepath.Rel(canonPath(root), canonCand)
	if err != nil {
		return "", "", ErrPathEscape
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", "", ErrPathEscape
	}
	if rel == "." {
		rel = ""
	}
	return canonCand, filepath.ToSlash(rel), nil
}

// SecretName reports credential/runtime names that must never be listed or served.
func SecretName(name string) bool {
	base := strings.ToLower(strings.TrimSpace(name))
	if base == "" {
		return false
	}
	if _, ok := secretBases[base]; ok {
		return true
	}
	if strings.HasPrefix(base, ".env") {
		return true
	}
	ext := filepath.Ext(base)
	if _, ok := secretExt[ext]; ok {
		return true
	}
	if strings.Contains(base, "credential") || strings.Contains(base, "secret") {
		return true
	}
	return false
}

// HiddenName is skipped from default listings (dotfiles + secret names).
func HiddenName(name string) bool {
	base := strings.TrimSpace(name)
	if base == "" || base == "." || base == ".." {
		return true
	}
	if strings.HasPrefix(base, ".") {
		return true
	}
	return SecretName(base)
}

func secretRel(rel string) bool {
	rel = strings.Trim(filepath.ToSlash(rel), "/")
	if rel == "" {
		return false
	}
	for _, part := range strings.Split(rel, "/") {
		if SecretName(part) {
			return true
		}
	}
	return false
}

func hiddenRel(rel string, showHidden bool) bool {
	rel = strings.Trim(filepath.ToSlash(rel), "/")
	if rel == "" {
		return false
	}
	for _, part := range strings.Split(rel, "/") {
		if SecretName(part) {
			return true
		}
		if !showHidden && HiddenName(part) {
			return true
		}
	}
	return false
}

func looksSecretBytes(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	low := bytes.ToLower(b)
	for _, needle := range secretVal {
		if bytes.Contains(b, needle) || bytes.Contains(low, bytes.ToLower(needle)) {
			return true
		}
	}
	return false
}

func AllowedExt(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	_, ok := allowExt[ext]
	return ok
}

func mimeOf(name string, dir bool) string {
	if dir {
		return "directory"
	}
	ext := strings.ToLower(filepath.Ext(name))
	if mt := mime.TypeByExtension(ext); mt != "" {
		if i := strings.IndexByte(mt, ';'); i > 0 {
			mt = mt[:i]
		}
		return mt
	}
	switch ext {
	case ".md", ".markdown", ".log":
		return "text/plain"
	case ".yml", ".yaml":
		return "text/yaml"
	case ".toml":
		return "text/plain"
	default:
		return "application/octet-stream"
	}
}

func QuotaMax() int64 {
	raw := strings.TrimSpace(os.Getenv("GOSO_STORAGE_MAX_BYTES"))
	if raw == "" {
		return DefaultQuotaBytes
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return DefaultQuotaBytes
	}
	return n
}

func usedBytes(root string) int64 {
	var sum int64
	var n int
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		n++
		if n > maxWalkFiles {
			return filepath.SkipAll
		}
		info, e := d.Info()
		if e != nil {
			return nil
		}
		if info.Mode().IsRegular() {
			sum += info.Size()
		}
		return nil
	})
	return sum
}

func crumbs(rel string) []Crumb {
	out := []Crumb{{Name: "workspace", Path: ""}}
	rel = strings.Trim(filepath.ToSlash(rel), "/")
	if rel == "" {
		return out
	}
	parts := strings.Split(rel, "/")
	acc := ""
	for _, p := range parts {
		if acc == "" {
			acc = p
		} else {
			acc = acc + "/" + p
		}
		out = append(out, Crumb{Name: p, Path: acc})
	}
	return out
}

func parentOf(rel string) string {
	rel = strings.Trim(filepath.ToSlash(rel), "/")
	if rel == "" {
		return ""
	}
	i := strings.LastIndexByte(rel, '/')
	if i < 0 {
		return ""
	}
	return rel[:i]
}

func rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func EmptyListing() Listing {
	return Listing{
		Configured:  false,
		Entries:     []Entry{},
		Breadcrumbs: []Crumb{{Name: "workspace", Path: ""}},
		MaxBytes:    0,
	}
}

// List returns metadata for a directory inside the workspace jail.
func List(path string, showHidden bool) (Listing, error) {
	if !configured() {
		return EmptyListing(), ErrNotConfigured
	}
	abs, rel, err := Jail(path)
	if err != nil {
		return Listing{}, err
	}
	if secretRel(rel) {
		return Listing{}, ErrSecret
	}
	st, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return Listing{}, ErrNotFound
		}
		return Listing{}, err
	}
	if !st.IsDir() {
		return Listing{}, ErrNotDir
	}
	ents, err := os.ReadDir(abs)
	if err != nil {
		return Listing{}, err
	}
	root, err := workspaceAbs()
	if err != nil {
		return Listing{}, err
	}
	out := Listing{
		Configured:  true,
		Path:        rel,
		Parent:      parentOf(rel),
		Breadcrumbs: crumbs(rel),
		Entries:     make([]Entry, 0, len(ents)),
		UsedBytes:   usedBytes(root),
		MaxBytes:    QuotaMax(),
	}
	skipped := 0
	for _, e := range ents {
		name := e.Name()
		childRel := name
		if rel != "" {
			childRel = rel + "/" + name
		}
		if hiddenRel(childRel, showHidden) {
			skipped++
			continue
		}
		info, ie := e.Info()
		if ie != nil {
			continue
		}
		dir := e.IsDir()
		size := int64(0)
		if !dir {
			size = info.Size()
		}
		out.Entries = append(out.Entries, Entry{
			Name:  name,
			Path:  childRel,
			Dir:   dir,
			Size:  size,
			Type:  mimeOf(name, dir),
			Mtime: rfc3339(info.ModTime()),
		})
		if len(out.Entries) >= maxListEnts {
			out.Truncated = true
			break
		}
	}
	out.HiddenSkipped = skipped
	return out, nil
}

func guardRead(rel string) error {
	if secretRel(rel) {
		return ErrSecret
	}
	if hiddenRel(rel, false) {
		return ErrHidden
	}
	return nil
}

// OpenFile returns a regular file handle inside the jail. Caller must Close.
func OpenFile(path string) (*os.File, os.FileInfo, string, string, error) {
	if !configured() {
		return nil, nil, "", "", ErrNotConfigured
	}
	abs, rel, err := Jail(path)
	if err != nil {
		return nil, nil, "", "", err
	}
	if err := guardRead(rel); err != nil {
		return nil, nil, "", "", err
	}
	st, err := os.Lstat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, "", "", ErrNotFound
		}
		return nil, nil, "", "", err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return nil, nil, "", "", ErrPathEscape
	}
	if !st.Mode().IsRegular() {
		return nil, nil, "", "", ErrNotFile
	}
	if st.Size() > MaxFileBytes {
		return nil, nil, "", "", ErrTooLarge
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, nil, "", "", err
	}
	return f, st, rel, mimeOf(filepath.Base(rel), false), nil
}

// GuardContent rejects files whose leading bytes look like credentials. Seeks back to 0 on success.
func GuardContent(f *os.File) error {
	head := make([]byte, binarySniff)
	n, err := f.Read(head)
	if err != nil && err != io.EOF {
		return err
	}
	if looksSecretBytes(head[:n]) {
		return ErrSecret
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return nil
}

// PreviewFile returns bounded metadata + optional text. Never credential values.
func PreviewFile(path string) (Preview, error) {
	f, st, rel, typ, err := OpenFile(path)
	if err != nil {
		return Preview{}, err
	}
	defer f.Close()
	head := make([]byte, PreviewBytes+1)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return Preview{}, err
	}
	head = head[:n]
	if looksSecretBytes(head) {
		return Preview{Path: rel, Type: typ, Size: st.Size(), Kind: "denied", Bytes: 0}, nil
	}
	p := Preview{Path: rel, Type: typ, Size: st.Size(), Bytes: n}
	if isText(head, typ) {
		p.Kind = "text"
		if n > PreviewBytes {
			p.Truncated = true
			head = head[:PreviewBytes]
			p.Bytes = PreviewBytes
		}
		p.Text = string(head)
		return p, nil
	}
	p.Kind = "binary"
	p.Bytes = 0
	return p, nil
}

func isText(b []byte, typ string) bool {
	if strings.HasPrefix(typ, "text/") || typ == "application/json" {
		if bytes.IndexByte(b[:min(len(b), binarySniff)], 0) >= 0 {
			return false
		}
		return utf8.Valid(b)
	}
	if len(b) == 0 {
		return false
	}
	sniff := b
	if len(sniff) > binarySniff {
		sniff = sniff[:binarySniff]
	}
	if bytes.IndexByte(sniff, 0) >= 0 {
		return false
	}
	return utf8.Valid(sniff) && (typ == "application/octet-stream" || strings.HasPrefix(typ, "text/"))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func cleanUploadName(name string) (string, error) {
	name = strings.TrimSpace(filepath.Base(name))
	name = strings.ReplaceAll(name, "\\", "")
	if name == "" || name == "." || name == ".." {
		return "", ErrName
	}
	if strings.IndexByte(name, 0) >= 0 || security.HasDotDot(name) {
		return "", ErrName
	}
	if strings.ContainsAny(name, `/\`) {
		return "", ErrName
	}
	if SecretName(name) || HiddenName(name) {
		return "", ErrSecret
	}
	if !AllowedExt(name) {
		return "", ErrType
	}
	return name, nil
}

// Upload writes a new file under destDir (relative). Overwrites in-jail files of the same name.
func Upload(destDir, filename string, r io.Reader, size int64) (Entry, error) {
	if !configured() {
		return Entry{}, ErrNotConfigured
	}
	filename, err := cleanUploadName(filename)
	if err != nil {
		return Entry{}, err
	}
	if size > MaxFileBytes {
		return Entry{}, ErrTooLarge
	}
	dirAbs, dirRel, err := Jail(destDir)
	if err != nil {
		return Entry{}, err
	}
	if secretRel(dirRel) {
		return Entry{}, ErrSecret
	}
	st, err := os.Stat(dirAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return Entry{}, ErrNotFound
		}
		return Entry{}, err
	}
	if !st.IsDir() {
		return Entry{}, ErrNotDir
	}
	root, err := workspaceAbs()
	if err != nil {
		return Entry{}, err
	}
	used := usedBytes(root)
	max := QuotaMax()
	if used+size > max && size > 0 {
		return Entry{}, ErrQuota
	}
	destRel := filename
	if dirRel != "" {
		destRel = dirRel + "/" + filename
	}
	destAbs := filepath.Join(dirAbs, filename)
	if err := confineUnder(root, destAbs); err != nil {
		return Entry{}, err
	}
	if SecretName(filename) {
		return Entry{}, ErrSecret
	}
	limited := io.LimitReader(r, MaxFileBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return Entry{}, err
	}
	if int64(len(body)) > MaxFileBytes {
		return Entry{}, ErrTooLarge
	}
	if looksSecretBytes(body) {
		return Entry{}, ErrSecret
	}
	tmp, err := os.CreateTemp(dirAbs, ".goso-up-*")
	if err != nil {
		return Entry{}, err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(body); err != nil {
		return Entry{}, err
	}
	if err := tmp.Close(); err != nil {
		return Entry{}, err
	}
	if err := os.Rename(tmpName, destAbs); err != nil {
		return Entry{}, err
	}
	ok = true
	info, err := os.Stat(destAbs)
	if err != nil {
		return Entry{}, err
	}
	return Entry{
		Name:  filename,
		Path:  destRel,
		Dir:   false,
		Size:  info.Size(),
		Type:  mimeOf(filename, false),
		Mtime: rfc3339(info.ModTime()),
	}, nil
}

func confirmMatch(got, rel, name string) bool {
	got = strings.TrimSpace(got)
	if got == "" {
		return false
	}
	if strings.EqualFold(got, name) {
		return true
	}
	if strings.EqualFold(got, rel) {
		return true
	}
	return false
}

// Delete removes a file or empty directory after confirm matches basename or relative path.
func Delete(path, confirm string) (Entry, error) {
	if !configured() {
		return Entry{}, ErrNotConfigured
	}
	if strings.TrimSpace(confirm) == "" {
		return Entry{}, ErrConfirmRequired
	}
	abs, rel, err := Jail(path)
	if err != nil {
		return Entry{}, err
	}
	if rel == "" {
		return Entry{}, ErrRoot
	}
	if secretRel(rel) {
		return Entry{}, ErrSecret
	}
	name := filepath.Base(rel)
	if !confirmMatch(confirm, rel, name) {
		return Entry{}, ErrConfirm
	}
	st, err := os.Lstat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return Entry{}, ErrNotFound
		}
		return Entry{}, err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return Entry{}, ErrPathEscape
	}
	ent := Entry{
		Name:  name,
		Path:  rel,
		Dir:   st.IsDir(),
		Size:  st.Size(),
		Type:  mimeOf(name, st.IsDir()),
		Mtime: rfc3339(st.ModTime()),
	}
	if st.IsDir() {
		ents, re := os.ReadDir(abs)
		if re != nil {
			return Entry{}, re
		}
		if len(ents) > 0 {
			return Entry{}, ErrNotEmpty
		}
	}
	if err := os.Remove(abs); err != nil {
		return Entry{}, err
	}
	return ent, nil
}
