// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package skill

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// MaxBody is the SKILL.md read cap (64KiB). Larger files are truncated on read
// and rejected on create.
const MaxBody = 64 * 1024

const (
	skillFile     = "SKILL.md"
	tenantSegment = "t"
)

// NamePattern is the jailed skill folder name: lowercase, digits, _ and -.
var NamePattern = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)

// Sentinel errors for the loader and HTTP/tool surfaces.
var (
	ErrNotConfigured = errors.New("not_configured")
	ErrNotFound      = errors.New("not_found")
	ErrPathEscape    = errors.New("path escape")
	ErrInvalidName   = errors.New("invalid name")
	ErrTooLarge      = errors.New("too large")
)

// Info is a listed skill (name = folder). Body is never included.
type Info struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Doc is one SKILL.md payload.
type Doc struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Body string `json:"body"`
}

type tenantCtxKey struct{}

// WithTenant attaches the tenant used by use_skill and skill_search.
func WithTenant(ctx context.Context, tenant string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, tenantCtxKey{}, tenant)
}

// HasTenant reports whether WithTenant stored a value.
func HasTenant(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	_, ok := ctx.Value(tenantCtxKey{}).(string)
	return ok
}

// TenantFrom returns the tenant stored by WithTenant, or default when unset.
func TenantFrom(ctx context.Context) string {
	if ctx == nil {
		return store.DefaultTenant
	}
	v, ok := ctx.Value(tenantCtxKey{}).(string)
	if !ok || strings.TrimSpace(v) == "" {
		return store.DefaultTenant
	}
	return v
}

// Dir is GOSO_SKILLS_DIR trimmed. Empty means not configured.
func Dir() string {
	return strings.TrimSpace(os.Getenv("GOSO_SKILLS_DIR"))
}

// Configured reports a non-empty skills dir env (no filesystem walk).
func Configured() bool {
	return Dir() != ""
}

// TenantOwned reports a path that lives under t/<tenant>/ rather than the shared root.
func TenantOwned(tenant, relPath string) bool {
	tenant, err := checkedTenant(tenant)
	if err != nil {
		return false
	}
	prefix := tenantSegment + "/" + tenant + "/"
	return strings.HasPrefix(filepath.ToSlash(relPath), prefix)
}

// List returns the skills visible to tenant.
// The default tenant sees the shared root only.
// Any other tenant sees that root plus <dir>/t/<tenant>, and its own file wins on a name clash.
// Empty/unset env → ErrNotConfigured and no walk.
func List(tenant string) ([]Info, error) {
	root, err := resolveRoot()
	if err != nil {
		return nil, err
	}
	tenant, err = checkedTenant(tenant)
	if err != nil {
		return nil, err
	}
	shared, err := listOne(root, root, "")
	if err != nil {
		return nil, err
	}
	if writesRoot(tenant) {
		return shared, nil
	}
	base, err := tenantDir(root, tenant)
	if err != nil {
		return nil, err
	}
	own, err := listOne(root, base, tenantSegment+"/"+tenant)
	if err != nil {
		return nil, err
	}
	return overlay(shared, own), nil
}

// Load reads one visible SKILL.md. A tenant file overrides the shared file.
// Never executes skill scripts.
func Load(tenant, name string) (*Doc, error) {
	root, err := resolveRoot()
	if err != nil {
		return nil, err
	}
	tenant, err = checkedTenant(tenant)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrNotFound
	}
	if err := validName(name); err != nil {
		return nil, err
	}
	if !writesRoot(tenant) {
		base, err := tenantDir(root, tenant)
		if err != nil {
			return nil, err
		}
		rel := tenantSegment + "/" + tenant + "/" + name + "/" + skillFile
		doc, err := loadFile(root, filepath.Join(base, name, skillFile), rel, name)
		if err == nil || !errors.Is(err, ErrNotFound) {
			return doc, err
		}
	}
	rel := name + "/" + skillFile
	return loadFile(root, filepath.Join(root, name, skillFile), rel, name)
}

// Create writes one SKILL.md. The default tenant writes the shared root.
// Every other tenant writes <dir>/t/<tenant>/<name>/SKILL.md and does not touch the root.
// The write is a temp file in that folder, an fsync, and a rename. Never executes scripts.
func Create(tenant, name, body string) (*Doc, error) {
	root, err := resolveRoot()
	if err != nil {
		return nil, err
	}
	tenant, err = checkedTenant(tenant)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if err := validName(name); err != nil {
		return nil, err
	}
	if len(body) > MaxBody {
		return nil, ErrTooLarge
	}
	dir, rel, err := skillPaths(root, tenant, name)
	if err != nil {
		return nil, err
	}
	file := filepath.Join(dir, skillFile)
	if err := confineFile(root, dir); err != nil {
		return nil, err
	}
	if err := confineFile(root, file); err != nil {
		return nil, err
	}
	if err := mkdirChecked(root, dir); err != nil {
		return nil, err
	}
	if err := writeAtomic(dir, skillFile, []byte(body)); err != nil {
		return nil, err
	}
	invalidateIndex(tenant)
	return &Doc{Name: name, Path: rel, Body: body}, nil
}

// Delete removes the skill folder this tenant is allowed to write.
// A non-default tenant never removes a shared root skill.
func Delete(tenant, name string) error {
	root, err := resolveRoot()
	if err != nil {
		return err
	}
	tenant, err = checkedTenant(tenant)
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if err := validName(name); err != nil {
		return err
	}
	dir, _, err := skillPaths(root, tenant, name)
	if err != nil {
		return err
	}
	file := filepath.Join(dir, skillFile)
	if err := confineFile(root, dir); err != nil {
		return err
	}
	if err := confineFile(root, file); err != nil {
		return err
	}
	if err := rejectSymlinks(root, dir); err != nil {
		return err
	}
	st, err := os.Lstat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return ErrPathEscape
	}
	if !st.IsDir() {
		return ErrNotFound
	}
	fst, err := os.Lstat(file)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrNotFound
		}
		return err
	}
	if fst.Mode()&os.ModeSymlink != 0 {
		return ErrPathEscape
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	invalidateIndex(tenant)
	return nil
}

func skillPaths(root, tenant, name string) (dir, rel string, err error) {
	if writesRoot(tenant) {
		dir = filepath.Join(root, name)
		rel = name + "/" + skillFile
		return dir, rel, nil
	}
	base, err := tenantDir(root, tenant)
	if err != nil {
		return "", "", err
	}
	dir = filepath.Join(base, name)
	rel = tenantSegment + "/" + tenant + "/" + name + "/" + skillFile
	return dir, rel, nil
}

func writesRoot(tenant string) bool {
	return tenant == store.DefaultTenant
}

// checkedTenant accepts empty as default. A value store.TenantOK would allow
// is still rejected when it contains "..", a slash, or a backslash.
func checkedTenant(tenant string) (string, error) {
	raw := strings.TrimSpace(tenant)
	if raw == "" {
		return store.DefaultTenant, nil
	}
	if strings.Contains(raw, "..") || strings.ContainsAny(raw, `/\`) || raw == "." {
		return "", ErrPathEscape
	}
	if !store.TenantOK(raw) || raw != filepath.Base(raw) || filepath.Clean(raw) != raw {
		return "", ErrPathEscape
	}
	return raw, nil
}

func tenantDir(root, tenant string) (string, error) {
	tenant, err := checkedTenant(tenant)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, tenantSegment, tenant)
	if err := rejectSymlinks(root, dir); err != nil {
		return "", err
	}
	if info, err := os.Lstat(dir); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", ErrPathEscape
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if !underRoot(root, dir) {
		return "", ErrPathEscape
	}
	if err := security.Confine(dir); err != nil {
		return "", ErrPathEscape
	}
	return dir, nil
}

func rejectSymlinks(root, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return ErrPathEscape
	}
	rel = filepath.Clean(rel)
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return ErrPathEscape
	}
	cur := root
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return ErrPathEscape
		}
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrPathEscape
		}
	}
	return nil
}

func listOne(skillsRoot, scanRoot, relPrefix string) ([]Info, error) {
	if err := rejectSymlinks(skillsRoot, scanRoot); err != nil && scanRoot != skillsRoot {
		return nil, err
	}
	ents, err := os.ReadDir(scanRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return []Info{}, nil
		}
		return nil, err
	}
	out := []Info{}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if err := validName(name); err != nil {
			continue
		}
		rel := name + "/" + skillFile
		if relPrefix != "" {
			rel = relPrefix + "/" + rel
		}
		p := filepath.Join(scanRoot, name, skillFile)
		if err := confineFile(skillsRoot, p); err != nil {
			continue
		}
		st, err := os.Lstat(p)
		if err != nil {
			continue
		}
		if st.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(p)
			if err != nil || confineFile(skillsRoot, resolved) != nil {
				continue
			}
			st, err = os.Stat(resolved)
			if err != nil || st.IsDir() {
				continue
			}
		} else if st.IsDir() {
			continue
		}
		out = append(out, Info{Name: name, Path: filepath.ToSlash(rel)})
	}
	return out, nil
}

func overlay(shared, own []Info) []Info {
	if len(own) == 0 {
		if shared == nil {
			return []Info{}
		}
		return shared
	}
	out := append([]Info(nil), shared...)
	idx := make(map[string]int, len(out))
	for i, info := range out {
		idx[info.Name] = i
	}
	for _, info := range own {
		if i, ok := idx[info.Name]; ok {
			out[i] = info
			continue
		}
		idx[info.Name] = len(out)
		out = append(out, info)
	}
	return out
}

func loadFile(skillsRoot, filePath, rel, name string) (*Doc, error) {
	if err := confineFile(skillsRoot, filePath); err != nil {
		return nil, err
	}
	st, err := os.Lstat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	p := filePath
	if st.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(p)
		if err != nil {
			return nil, ErrPathEscape
		}
		if err := confineFile(skillsRoot, resolved); err != nil {
			return nil, err
		}
		p = resolved
		st, err = os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, ErrNotFound
			}
			return nil, err
		}
	}
	if st.IsDir() {
		return nil, ErrNotFound
	}
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	defer f.Close()
	body, err := io.ReadAll(io.LimitReader(f, MaxBody))
	if err != nil {
		return nil, err
	}
	return &Doc{Name: name, Path: filepath.ToSlash(rel), Body: string(body)}, nil
}

func writeAtomic(dir, name string, body []byte) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) || name != filepath.Base(name) {
		return ErrPathEscape
	}
	final := filepath.Join(dir, name)
	if st, err := os.Lstat(final); err == nil {
		if st.Mode()&os.ModeSymlink != 0 || st.IsDir() {
			return ErrPathEscape
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return err
	}
	tmp := filepath.Join(dir, ".skill-"+hex.EncodeToString(buf[:])+".tmp")
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0o644)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, final); err != nil {
		return err
	}
	committed = true
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

func mkdirChecked(root, dir string) error {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return ErrPathEscape
	}
	rel = filepath.Clean(rel)
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return ErrPathEscape
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	rootInfo, err := os.Stat(root)
	if err != nil || !rootInfo.IsDir() {
		return ErrPathEscape
	}
	if rel == "." {
		return nil
	}
	cur := root
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return ErrPathEscape
		}
		cur = filepath.Join(cur, part)
		st, err := os.Lstat(cur)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(cur, 0o755); err != nil {
				return err
			}
			st, err = os.Lstat(cur)
		}
		if err != nil {
			return err
		}
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return ErrPathEscape
		}
	}
	return nil
}

func resolveRoot() (string, error) {
	raw := Dir()
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
	if err := security.Confine(abs); err != nil {
		return "", ErrPathEscape
	}
	return abs, nil
}

func validName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return ErrPathEscape
	}
	if filepath.IsAbs(name) {
		return ErrPathEscape
	}
	if security.HasDotDot(name) {
		return ErrPathEscape
	}
	if name != filepath.Base(name) {
		return ErrPathEscape
	}
	if strings.ContainsAny(name, `/\`) {
		return ErrPathEscape
	}
	if !NamePattern.MatchString(name) {
		return ErrInvalidName
	}
	if name == tenantSegment {
		return ErrInvalidName
	}
	return nil
}

func confineFile(root, p string) error {
	if security.HasDotDot(p) {
		return ErrPathEscape
	}
	if !underRoot(root, p) {
		return ErrPathEscape
	}
	if err := security.Confine(p); err != nil {
		return ErrPathEscape
	}
	return nil
}

func canon(p string) string {
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
