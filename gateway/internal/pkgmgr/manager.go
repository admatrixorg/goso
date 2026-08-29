// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pkgmgr

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrConfirm         = errors.New("confirm does not match")
	ErrConfirmRequired = errors.New("confirm is required")
	ErrEcosystem       = errors.New("ecosystem must be system, python, node, or github")
	ErrName            = errors.New("name is required")
	ErrNameInvalid     = errors.New("name is invalid")
	ErrPin             = errors.New("pin is required")
	ErrPinInvalid      = errors.New("pin must be an exact version, not a range or latest")
	ErrAllow           = errors.New("package is not on the allowlist")
	ErrPinMismatch     = errors.New("version does not match the allowlist pin")
	ErrExists          = errors.New("package already installed")
	ErrNotPartial      = errors.New("package is not in a recoverable state")
	ErrUseRecover      = errors.New("use recover for a partial or failed install")
	ErrBusy            = errors.New("package job already running")
	ErrRuntime         = errors.New("runtime is incompatible")
	ErrSecret          = errors.New("secret-shaped value is not allowed")
	ErrKind            = errors.New("credential kind must be github, npm, or pypi")
	ErrToken           = errors.New("token is required")
	ErrCap             = errors.New("too many packages")
	ErrNotSet          = errors.New("credential is not set")
)

const (
	EcoSystem = "system"
	EcoPython = "python"
	EcoNode   = "node"
	EcoGitHub = "github"

	StatusMissing      = "missing"
	StatusInstalling   = "installing"
	StatusInstalled    = "installed"
	StatusPartial      = "partial"
	StatusFailed       = "failed"
	StatusUninstalling = "uninstalling"

	JobQueued    = "queued"
	JobRunning   = "running"
	JobSucceeded = "succeeded"
	JobFailed    = "failed"
	JobPartial   = "partial"

	ActionInstall   = "install"
	ActionUninstall = "uninstall"
	ActionRecover   = "recover"

	KindGitHub = "github"
	KindNPM    = "npm"
	KindPyPI   = "pypi"

	maxName    = 80
	maxPin     = 64
	maxRows    = 128
	maxJobs    = 64
	maxLogLine = 240
	maxLogs    = 24
)

var (
	tokenShape   = regexp.MustCompile(`(?i)(sk-[A-Za-z0-9_-]{8,}|gsk_[A-Za-z0-9]+|xai-[A-Za-z0-9]+|AIza[A-Za-z0-9_-]+|Bearer\s+[A-Za-z0-9._\-+=/]{8,}|token=|ghp_[A-Za-z0-9]+|npm_[A-Za-z0-9]+)`)
	pinLoose     = regexp.MustCompile(`(?i)^(latest|\*|x|any)$`)
	pinRange     = regexp.MustCompile(`[~^<>*]|>=|<=`)
	sysName      = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	pyName       = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)
	nodeName     = regexp.MustCompile(`^(?:@[a-z0-9._-]+/)?[a-z0-9._-]{1,64}$`)
	ghName       = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)
	pinExact     = regexp.MustCompile(`^[A-Za-z0-9._+-]{1,64}$`)
	secretFields = []string{"token", "secret", "password", "api_key", "access_token", "authorization", "hmac", "private_key"}
)

// Runtime is a probed host tool. No credentials.
type Runtime struct {
	Name       string `json:"name"`
	Ecosystem  string `json:"ecosystem,omitempty"`
	Present    bool   `json:"present"`
	Version    string `json:"version,omitempty"`
	Compatible bool   `json:"compatible"`
	Warning    string `json:"warning,omitempty"`
}

// AllowEntry is a pinned allowlist row.
type AllowEntry struct {
	ID        string    `json:"id"`
	Ecosystem string    `json:"ecosystem"`
	Name      string    `json:"name"`
	Pin       string    `json:"pin"`
	CreatedAt time.Time `json:"created_at"`
}

// Package is declared inventory. Status may be partial after a failed job.
type Package struct {
	ID        string    `json:"id"`
	Ecosystem string    `json:"ecosystem"`
	Name      string    `json:"name"`
	Version   string    `json:"version"`
	Status    string    `json:"status"`
	Warning   string    `json:"warning,omitempty"`
	JobID     string    `json:"job_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Job is install/uninstall/recover progress. Logs are redacted.
type Job struct {
	ID         string     `json:"id"`
	Action     string     `json:"action"`
	PackageID  string     `json:"package_id"`
	Ecosystem  string     `json:"ecosystem"`
	Name       string     `json:"name"`
	Version    string     `json:"version"`
	Status     string     `json:"status"`
	Progress   int        `json:"progress"`
	Log        []string   `json:"log"`
	Error      string     `json:"error,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// CLICred is write-only credential metadata. Token/hash are never included.
type CLICred struct {
	Kind      string     `json:"kind"`
	Set       bool       `json:"set"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

// Snapshot is the GET payload. No credential values.
type Snapshot struct {
	Runtimes    []Runtime    `json:"runtimes"`
	Allowlist   []AllowEntry `json:"allowlist"`
	Packages    []Package    `json:"packages"`
	Jobs        []Job        `json:"jobs"`
	Credentials []CLICred    `json:"credentials"`
}

type allowRow struct {
	ID        string
	Ecosystem string
	Name      string
	Pin       string
	CreatedAt time.Time
}

type pkgRow struct {
	ID         string
	Ecosystem  string
	Name       string
	Version    string
	Status     string
	Warning    string
	JobID      string
	LastOK     int
	LastAction string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type jobRow struct {
	Job
	LastOK int
}

type credRow struct {
	Kind      string
	Hash      string
	UpdatedAt time.Time
}

// ProbeFunc returns live runtime inventory.
type ProbeFunc func() []Runtime

// Manager holds declared packages, allowlist, jobs, and hashed CLI credentials.
type Manager struct {
	mu     sync.Mutex
	seq    atomic.Int64
	now    func() time.Time
	probe  ProbeFunc
	FailAt int // tests: fail the next job at this 1-based step (0 = success)
	allow  map[string]*allowRow
	pkgs   map[string]*pkgRow
	jobs   []*jobRow
	creds  map[string]*credRow
}

var (
	defaultMu sync.Mutex
	defaultM  = New(nil)
)

// New returns an empty manager. nil probe uses host detection.
func New(probe ProbeFunc) *Manager {
	if probe == nil {
		probe = ProbeHost
	}
	return &Manager{
		probe: probe,
		allow: map[string]*allowRow{},
		pkgs:  map[string]*pkgRow{},
		creds: map[string]*credRow{},
	}
}

// Default is the process-wide manager used by HTTP.
func Default() *Manager {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	return defaultM
}

// SetDefault replaces the process-wide manager (tests).
func SetDefault(m *Manager) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if m == nil {
		m = New(nil)
	}
	defaultM = m
}

func (m *Manager) clock() time.Time {
	if m != nil && m.now != nil {
		return m.now().UTC()
	}
	return time.Now().UTC()
}

func (m *Manager) nextID(prefix string) string {
	v := m.seq.Add(1)
	return prefix + strconv.FormatInt(v, 10)
}

// Snapshot returns inventory, jobs, allowlist, and credential metadata.
func (m *Manager) Snapshot() Snapshot {
	runtimes := m.probe()
	if runtimes == nil {
		runtimes = []Runtime{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	snap := m.snapshotLocked()
	snap.Runtimes = runtimes
	return snap
}

func (m *Manager) snapshotLocked() Snapshot {
	allow := make([]AllowEntry, 0, len(m.allow))
	for _, r := range m.allow {
		allow = append(allow, AllowEntry{ID: r.ID, Ecosystem: r.Ecosystem, Name: r.Name, Pin: r.Pin, CreatedAt: r.CreatedAt})
	}
	sort.Slice(allow, func(i, j int) bool {
		if allow[i].Ecosystem != allow[j].Ecosystem {
			return allow[i].Ecosystem < allow[j].Ecosystem
		}
		return allow[i].Name < allow[j].Name
	})
	pkgs := make([]Package, 0, len(m.pkgs))
	for _, r := range m.pkgs {
		pkgs = append(pkgs, publicPkg(r))
	}
	sort.Slice(pkgs, func(i, j int) bool {
		if pkgs[i].Ecosystem != pkgs[j].Ecosystem {
			return pkgs[i].Ecosystem < pkgs[j].Ecosystem
		}
		return pkgs[i].Name < pkgs[j].Name
	})
	jobs := make([]Job, 0, len(m.jobs))
	start := 0
	if len(m.jobs) > maxJobs {
		start = len(m.jobs) - maxJobs
	}
	for _, j := range m.jobs[start:] {
		jobs = append(jobs, publicJob(j))
	}
	return Snapshot{
		Runtimes:    []Runtime{},
		Allowlist:   allow,
		Packages:    pkgs,
		Jobs:        jobs,
		Credentials: m.credMetaLocked(),
	}
}

func publicPkg(r *pkgRow) Package {
	return Package{
		ID: r.ID, Ecosystem: r.Ecosystem, Name: r.Name, Version: r.Version,
		Status: r.Status, Warning: r.Warning, JobID: r.JobID,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func publicJob(j *jobRow) Job {
	out := j.Job
	out.Log = append([]string{}, j.Log...)
	return out
}

func (m *Manager) credMetaLocked() []CLICred {
	kinds := []string{KindGitHub, KindNPM, KindPyPI}
	out := make([]CLICred, 0, len(kinds))
	for _, k := range kinds {
		row := m.creds[k]
		c := CLICred{Kind: k, Set: row != nil && row.Hash != ""}
		if c.Set {
			t := row.UpdatedAt
			c.UpdatedAt = &t
		}
		out = append(out, c)
	}
	return out
}

// Get returns one package.
func (m *Manager) Get(id string) (Package, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.pkgs[strings.TrimSpace(id)]
	if !ok {
		return Package{}, ErrNotFound
	}
	return publicPkg(r), nil
}

// Allow adds a pinned allowlist entry.
func (m *Manager) Allow(ecosystem, name, pin string) (AllowEntry, error) {
	eco, name, pin, err := normalizeSpec(ecosystem, name, pin)
	if err != nil {
		return AllowEntry{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.allow) >= maxRows {
		return AllowEntry{}, ErrCap
	}
	key := eco + "|" + name
	if old, ok := m.allow[key]; ok {
		old.Pin = pin
		return AllowEntry{ID: old.ID, Ecosystem: old.Ecosystem, Name: old.Name, Pin: old.Pin, CreatedAt: old.CreatedAt}, nil
	}
	row := &allowRow{ID: m.nextID("al_"), Ecosystem: eco, Name: name, Pin: pin, CreatedAt: m.clock()}
	m.allow[key] = row
	return AllowEntry{ID: row.ID, Ecosystem: row.Ecosystem, Name: row.Name, Pin: row.Pin, CreatedAt: row.CreatedAt}, nil
}

// Unpin removes an allowlist entry. Installed packages stay until uninstall.
func (m *Manager) Unpin(id, confirm string) (AllowEntry, error) {
	id = strings.TrimSpace(id)
	if strings.TrimSpace(confirm) == "" {
		return AllowEntry{}, ErrConfirmRequired
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var found *allowRow
	var key string
	for k, r := range m.allow {
		if r.ID == id {
			found, key = r, k
			break
		}
	}
	if found == nil {
		return AllowEntry{}, ErrNotFound
	}
	if !confirmMatch(confirm, found.ID, found.Name, found.Ecosystem+"/"+found.Name) {
		return AllowEntry{}, ErrConfirm
	}
	delete(m.allow, key)
	return AllowEntry{ID: found.ID, Ecosystem: found.Ecosystem, Name: found.Name, Pin: found.Pin, CreatedAt: found.CreatedAt}, nil
}

// Install starts an allowlisted, pinned install. Confirm must match name or eco/name@version.
func (m *Manager) Install(ecosystem, name, version, confirm string) (Package, Job, error) {
	eco, name, version, err := normalizeSpec(ecosystem, name, version)
	if err != nil {
		return Package{}, Job{}, err
	}
	if strings.TrimSpace(confirm) == "" {
		return Package{}, Job{}, ErrConfirmRequired
	}
	if !confirmMatch(confirm, name, eco+"/"+name, eco+"/"+name+"@"+version) {
		return Package{}, Job{}, ErrConfirm
	}
	runtimes := m.probe()
	m.mu.Lock()
	defer m.mu.Unlock()
	al, ok := m.allow[eco+"|"+name]
	if !ok {
		return Package{}, Job{}, ErrAllow
	}
	if al.Pin != version {
		return Package{}, Job{}, ErrPinMismatch
	}
	if existing := m.findPkg(eco, name); existing != nil {
		switch existing.Status {
		case StatusInstalled, StatusInstalling, StatusUninstalling:
			return Package{}, Job{}, ErrExists
		case StatusPartial, StatusFailed:
			return Package{}, Job{}, ErrUseRecover
		}
	}
	if len(m.pkgs) >= maxRows {
		return Package{}, Job{}, ErrCap
	}
	now := m.clock()
	row := m.findPkg(eco, name)
	if row == nil {
		row = &pkgRow{
			ID: m.nextID("pk_"), Ecosystem: eco, Name: name, Version: version,
			Status: StatusInstalling, CreatedAt: now, UpdatedAt: now,
		}
		m.pkgs[row.ID] = row
	} else {
		row.Version = version
		row.Status = StatusInstalling
		row.Warning = ""
		row.UpdatedAt = now
	}
	job, err := m.runJobLocked(row, ActionInstall, 0, runtimes)
	return publicPkg(row), job, err
}

// Uninstall removes a declared package after confirm.
func (m *Manager) Uninstall(id, confirm string) (Package, Job, error) {
	id = strings.TrimSpace(id)
	if strings.TrimSpace(confirm) == "" {
		return Package{}, Job{}, ErrConfirmRequired
	}
	runtimes := m.probe()
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.pkgs[id]
	if !ok {
		return Package{}, Job{}, ErrNotFound
	}
	if !confirmMatch(confirm, row.ID, row.Name, row.Ecosystem+"/"+row.Name) {
		return Package{}, Job{}, ErrConfirm
	}
	if row.Status == StatusInstalling || row.Status == StatusUninstalling {
		return Package{}, Job{}, ErrBusy
	}
	row.Status = StatusUninstalling
	row.UpdatedAt = m.clock()
	job, err := m.runJobLocked(row, ActionUninstall, 0, runtimes)
	return publicPkg(row), job, err
}

// Recover continues a partial or failed install after confirm.
func (m *Manager) Recover(id, confirm string) (Package, Job, error) {
	id = strings.TrimSpace(id)
	if strings.TrimSpace(confirm) == "" {
		return Package{}, Job{}, ErrConfirmRequired
	}
	runtimes := m.probe()
	m.mu.Lock()
	defer m.mu.Unlock()
	row, ok := m.pkgs[id]
	if !ok {
		return Package{}, Job{}, ErrNotFound
	}
	if !confirmMatch(confirm, row.ID, row.Name, row.Ecosystem+"/"+row.Name) {
		return Package{}, Job{}, ErrConfirm
	}
	if row.Status != StatusPartial && row.Status != StatusFailed {
		return Package{}, Job{}, ErrNotPartial
	}
	if row.LastAction != ActionUninstall {
		al, ok := m.allow[row.Ecosystem+"|"+row.Name]
		if !ok || al.Pin != row.Version {
			return Package{}, Job{}, ErrAllow
		}
		row.Status = StatusInstalling
	} else {
		row.Status = StatusUninstalling
	}
	start := row.LastOK
	row.UpdatedAt = m.clock()
	job, err := m.runJobLocked(row, ActionRecover, start, runtimes)
	return publicPkg(row), job, err
}

// SetCLI stores a hashed CLI credential. The token is never retained.
func (m *Manager) SetCLI(kind, token string) (CLICred, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind != KindGitHub && kind != KindNPM && kind != KindPyPI {
		return CLICred{}, ErrKind
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return CLICred{}, ErrToken
	}
	sum := sha256.Sum256([]byte(token))
	now := m.clock()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.creds[kind] = &credRow{Kind: kind, Hash: hex.EncodeToString(sum[:]), UpdatedAt: now}
	t := now
	return CLICred{Kind: kind, Set: true, UpdatedAt: &t}, nil
}

// ClearCLI drops a stored credential after confirm (kind).
func (m *Manager) ClearCLI(kind, confirm string) (CLICred, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if strings.TrimSpace(confirm) == "" {
		return CLICred{}, ErrConfirmRequired
	}
	if kind != KindGitHub && kind != KindNPM && kind != KindPyPI {
		return CLICred{}, ErrKind
	}
	if !confirmMatch(confirm, kind) {
		return CLICred{}, ErrConfirm
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.creds[kind] == nil || m.creds[kind].Hash == "" {
		return CLICred{}, ErrNotSet
	}
	delete(m.creds, kind)
	return CLICred{Kind: kind, Set: false}, nil
}

func (m *Manager) findPkg(eco, name string) *pkgRow {
	for _, r := range m.pkgs {
		if r.Ecosystem == eco && r.Name == name {
			return r
		}
	}
	return nil
}

func (m *Manager) runJobLocked(row *pkgRow, action string, startFrom int, runtimes []Runtime) (Job, error) {
	now := m.clock()
	j := &jobRow{Job: Job{
		ID: m.nextID("pj_"), Action: action, PackageID: row.ID, Ecosystem: row.Ecosystem,
		Name: row.Name, Version: row.Version, Status: JobRunning, Progress: 0,
		Log: []string{}, StartedAt: now,
	}, LastOK: startFrom}
	row.JobID = j.ID
	failAt := m.FailAt
	m.FailAt = 0

	effective := action
	if action == ActionRecover {
		if row.LastAction == ActionUninstall {
			effective = ActionUninstall
		} else {
			effective = ActionInstall
		}
	} else {
		row.LastAction = action
	}

	steps := installSteps
	if effective == ActionUninstall {
		steps = uninstallSteps
	}
	var lastErr error
	for i, step := range steps {
		n := i + 1
		if n <= startFrom {
			continue
		}
		j.Progress = step.progress
		j.Log = appendLog(j.Log, step.label+" "+row.Ecosystem+"/"+row.Name+"@"+row.Version)
		if failAt == n {
			lastErr = errors.New(step.fail)
			break
		}
		if effective != ActionUninstall && n == 1 {
			if warn, ok := runtimeGate(runtimes, row.Ecosystem); !ok {
				row.Warning = warn
				lastErr = ErrRuntime
				j.Log = appendLog(j.Log, "compatibility: "+warn)
				break
			}
			if row.Ecosystem == EcoGitHub {
				if c := m.creds[KindGitHub]; c == nil || c.Hash == "" {
					row.Warning = "github CLI credential is not configured; private fetches are unavailable"
					j.Log = appendLog(j.Log, "warning: "+row.Warning)
				}
			}
		}
		j.LastOK = n
		row.LastOK = n
	}

	fin := m.clock()
	j.FinishedAt = &fin
	row.UpdatedAt = fin
	if lastErr != nil {
		if j.LastOK > 0 {
			j.Status = JobPartial
			row.Status = StatusPartial
		} else {
			j.Status = JobFailed
			row.Status = StatusFailed
		}
		j.Error = lastErr.Error()
		j.Log = appendLog(j.Log, "failed: "+j.Error)
		m.appendJob(j)
		return publicJob(j), lastErr
	}
	j.Status = JobSucceeded
	j.Progress = 100
	j.Log = appendLog(j.Log, "done")
	if effective == ActionUninstall {
		delete(m.pkgs, row.ID)
	} else {
		row.Status = StatusInstalled
		if row.Warning == "" {
			row.Warning = runtimeWarning(runtimes, row.Ecosystem)
		}
	}
	m.appendJob(j)
	return publicJob(j), nil
}

func (m *Manager) appendJob(j *jobRow) {
	m.jobs = append(m.jobs, j)
	if len(m.jobs) > maxJobs {
		m.jobs = append([]*jobRow(nil), m.jobs[len(m.jobs)-maxJobs:]...)
	}
}

type stepSpec struct {
	label    string
	progress int
	fail     string
}

var installSteps = []stepSpec{
	{label: "check runtime", progress: 25, fail: "runtime check failed"},
	{label: "apply pin", progress: 70, fail: "apply failed"},
	{label: "verify", progress: 100, fail: "verify failed"},
}

var uninstallSteps = []stepSpec{
	{label: "prepare uninstall", progress: 40, fail: "uninstall prepare failed"},
	{label: "remove", progress: 100, fail: "remove failed"},
}

func runtimeGate(runtimes []Runtime, eco string) (string, bool) {
	need := map[string]string{EcoPython: "python", EcoNode: "node", EcoGitHub: "git"}
	want, ok := need[eco]
	if !ok {
		return "", true
	}
	for _, r := range runtimes {
		if r.Name == want || r.Ecosystem == eco {
			if r.Present && r.Compatible {
				return r.Warning, true
			}
			if r.Warning != "" {
				return r.Warning, false
			}
			if !r.Present {
				return want + " is not installed", false
			}
			return want + " is incompatible", false
		}
	}
	return want + " is not installed", false
}

func runtimeWarning(runtimes []Runtime, eco string) string {
	for _, r := range runtimes {
		if r.Ecosystem == eco && r.Warning != "" {
			return r.Warning
		}
	}
	return ""
}

func normalizeSpec(eco, name, pin string) (string, string, string, error) {
	eco = strings.ToLower(strings.TrimSpace(eco))
	name = strings.TrimSpace(name)
	pin = strings.TrimSpace(pin)
	switch eco {
	case EcoSystem, EcoPython, EcoNode, EcoGitHub:
	default:
		return "", "", "", ErrEcosystem
	}
	if name == "" {
		return "", "", "", ErrName
	}
	if eco == EcoPython || eco == EcoNode {
		name = strings.ToLower(name)
	}
	if pin == "" {
		return "", "", "", ErrPin
	}
	if tokenShape.MatchString(name) || tokenShape.MatchString(pin) {
		return "", "", "", ErrSecret
	}
	if !validName(eco, name) {
		return "", "", "", ErrNameInvalid
	}
	if pinLoose.MatchString(pin) || pinRange.MatchString(pin) || !pinExact.MatchString(pin) {
		return "", "", "", ErrPinInvalid
	}
	if len(name) > maxName || len(pin) > maxPin {
		return "", "", "", ErrNameInvalid
	}
	return eco, name, pin, nil
}

func validName(eco, name string) bool {
	switch eco {
	case EcoSystem:
		return sysName.MatchString(name)
	case EcoPython:
		return pyName.MatchString(strings.ToLower(name))
	case EcoNode:
		return nodeName.MatchString(strings.ToLower(name))
	case EcoGitHub:
		return ghName.MatchString(name)
	default:
		return false
	}
}

func confirmMatch(typed string, vals ...string) bool {
	v := strings.TrimSpace(typed)
	if v == "" {
		return false
	}
	for _, x := range vals {
		if strings.EqualFold(v, strings.TrimSpace(x)) {
			return true
		}
	}
	return false
}

func appendLog(log []string, line string) []string {
	line = redactLine(line)
	if len(line) > maxLogLine {
		line = line[:maxLogLine] + "…"
	}
	out := append(log, line)
	if len(out) > maxLogs {
		out = out[len(out)-maxLogs:]
	}
	return out
}

func redactLine(s string) string {
	s = tokenShape.ReplaceAllString(s, "[redacted]")
	lower := strings.ToLower(s)
	for _, k := range secretFields {
		if strings.Contains(lower, k+"=") || strings.Contains(lower, `"`+k+`"`) {
			return strings.Split(s, " ")[0] + " [redacted]"
		}
	}
	return s
}

// SecretField reports whether k is a credential JSON key.
func SecretField(k string) bool {
	lk := strings.ToLower(strings.TrimSpace(k))
	for _, s := range secretFields {
		if lk == s {
			return true
		}
	}
	return false
}
