// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package workstation

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/mqglobal/goso/gateway/internal/store"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrConfirm         = errors.New("confirm does not match")
	ErrConfirmRequired = errors.New("confirm is required")
	ErrDisplayRequired = errors.New("display is required")
	ErrHostRequired    = errors.New("host is required")
	ErrBackend         = errors.New("backend must be ssh or docker")
	ErrHost            = errors.New("host is invalid")
	ErrPort            = errors.New("port is invalid")
	ErrUserRequired    = errors.New("user is required for ssh")
	ErrUser            = errors.New("user is invalid")
	ErrIdentity        = errors.New("identity must be a path or ref, never a private key")
	ErrKeyMaterial     = errors.New("identity is a path/ref, never a private key")
	ErrAgent           = errors.New("agent id is invalid")
	ErrCap             = errors.New("too many workstations")
	ErrNotDisconnected = errors.New("workstation already disconnected")
)

const (
	maxDisplay      = 64
	maxHost         = 253
	maxUser         = 32
	maxIdentity     = 256
	maxAgent        = 64
	maxRows         = 64
	backendSSH      = "ssh"
	backendDocker   = "docker"
	healthUnknown   = "unknown"
	healthOK        = "ok"
	healthFailed    = "failed"
	healthOffline   = "disconnected"
	defaultSSHPort  = 22
	defaultDockPort = 2375
)

// Input is create/update fields. IdentityRef is a path or named ref only.
type Input struct {
	Display     string
	Backend     string
	Host        string
	Port        int
	User        string
	IdentityRef string
	AgentID     string
	TenantID    string
	At          time.Time
}

// Public is the GET row. No private-key fields exist.
type Public struct {
	ID          string     `json:"id"`
	Display     string     `json:"display"`
	Backend     string     `json:"backend"`
	Host        string     `json:"host"`
	Port        int        `json:"port"`
	User        string     `json:"user,omitempty"`
	IdentityRef string     `json:"identity_ref,omitempty"`
	IdentitySet bool       `json:"identity_set"`
	AgentID     string     `json:"agent_id,omitempty"`
	Health      string     `json:"health"`
	LastTested  *time.Time `json:"last_tested,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TestResult is a constrained probe. No identity path, no key material, no SSH transcript.
type TestResult struct {
	OK          bool   `json:"ok"`
	Health      string `json:"health"`
	Summary     string `json:"summary"`
	Backend     string `json:"backend"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	IdentitySet bool   `json:"identity_set"`
}

type row struct {
	ID          string
	TenantID    string
	Display     string
	Backend     string
	Host        string
	Port        int
	User        string
	IdentityRef string
	AgentID     string
	Health      string
	LastTested  time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Workstations is an in-memory SSH/Docker execution-target registry.
type Workstations struct {
	mu   sync.Mutex
	seq  atomic.Int64
	now  func() time.Time
	rows map[string]*row
}

var (
	defaultMu  sync.Mutex
	defaultReg = New()
)

// New returns an empty registry.
func New() *Workstations {
	return &Workstations{rows: map[string]*row{}}
}

// Default is the process-wide registry used by HTTP.
func Default() *Workstations {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	return defaultReg
}

// SetDefault replaces the process-wide registry (tests).
func SetDefault(w *Workstations) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if w == nil {
		w = New()
	}
	defaultReg = w
}

func (w *Workstations) clock() time.Time {
	if w != nil && w.now != nil {
		return w.now().UTC()
	}
	return time.Now().UTC()
}

func (w *Workstations) nextID() string {
	v := w.seq.Add(1)
	return "ws_" + itoa(v)
}

func itoa(v int64) string {
	if v <= 0 {
		return "0"
	}
	b := make([]byte, 0, 20)
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}

func clip(s string, max int) string {
	s = strings.TrimSpace(s)
	if max > 0 && len(s) > max {
		return s[:max]
	}
	return s
}

// ConfirmOK is true when typed matches id or display.
func ConfirmOK(typed string, p Public) bool {
	v := strings.TrimSpace(typed)
	if v == "" {
		return false
	}
	return v == p.ID || v == p.Display
}

// LooksLikeKey is true when s is PEM / private-key material rather than a path/ref.
func LooksLikeKey(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	u := strings.ToUpper(s)
	if strings.Contains(u, "PRIVATE KEY") || strings.Contains(s, "-----") {
		return true
	}
	if strings.ContainsAny(s, "\n\r") {
		return true
	}
	if strings.Contains(u, "BEGIN ") {
		return true
	}
	if len(s) > 200 && !strings.ContainsAny(s, "/~") {
		return true
	}
	return false
}

func validIdentityRef(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if LooksLikeKey(s) || len(s) > maxIdentity {
		return ErrIdentity
	}
	if strings.Contains(s, "://") {
		return ErrIdentity
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("/._-~:", r) {
			continue
		}
		return ErrIdentity
	}
	return nil
}

func validHost(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return ErrHostRequired
	}
	if len(s) > maxHost || strings.ContainsAny(s, " \t@") || strings.Contains(s, "://") {
		return ErrHost
	}
	ok := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(".-:[]", r) {
			ok = true
			continue
		}
		return ErrHost
	}
	if !ok {
		return ErrHost
	}
	return nil
}

func validUser(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if len(s) > maxUser {
		return ErrUser
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("._-", r) {
			continue
		}
		return ErrUser
	}
	return nil
}

func validAgent(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if len(s) > maxAgent {
		return ErrAgent
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_-", r) {
			continue
		}
		return ErrAgent
	}
	return nil
}

func normalizeBackend(s string) (string, error) {
	b := strings.ToLower(strings.TrimSpace(s))
	if b == backendSSH || b == backendDocker {
		return b, nil
	}
	return "", ErrBackend
}

func defaultPort(backend string, port int) (int, error) {
	if port == 0 {
		if backend == backendDocker {
			return defaultDockPort, nil
		}
		return defaultSSHPort, nil
	}
	if port < 1 || port > 65535 {
		return 0, ErrPort
	}
	return port, nil
}

func validate(in Input) (Input, error) {
	out := Input{
		Display:     clip(in.Display, maxDisplay),
		Host:        strings.TrimSpace(in.Host),
		Port:        in.Port,
		User:        clip(in.User, maxUser),
		IdentityRef: strings.TrimSpace(in.IdentityRef),
		AgentID:     clip(in.AgentID, maxAgent),
		TenantID:    store.NormalizeTenant(in.TenantID),
		At:          in.At,
	}
	if out.Display == "" {
		return Input{}, ErrDisplayRequired
	}
	backend, err := normalizeBackend(in.Backend)
	if err != nil {
		return Input{}, err
	}
	out.Backend = backend
	if err := validHost(out.Host); err != nil {
		return Input{}, err
	}
	port, err := defaultPort(backend, in.Port)
	if err != nil {
		return Input{}, err
	}
	out.Port = port
	if err := validUser(out.User); err != nil {
		return Input{}, err
	}
	if backend == backendSSH && out.User == "" {
		return Input{}, ErrUserRequired
	}
	if err := validIdentityRef(out.IdentityRef); err != nil {
		return Input{}, err
	}
	if err := validAgent(out.AgentID); err != nil {
		return Input{}, err
	}
	return out, nil
}

// Create registers a workstation. Key material is rejected.
func (w *Workstations) Create(in Input) (Public, error) {
	if w == nil {
		return Public{}, ErrNotFound
	}
	if LooksLikeKey(in.IdentityRef) {
		return Public{}, ErrKeyMaterial
	}
	norm, err := validate(in)
	if err != nil {
		return Public{}, err
	}
	at := norm.At
	if at.IsZero() {
		at = w.clock()
	} else {
		at = at.UTC()
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, r := range w.rows {
		if r != nil && store.SameTenant(r.TenantID, norm.TenantID) {
			n++
		}
	}
	if n >= maxRows {
		return Public{}, ErrCap
	}
	row := &row{
		ID:          w.nextID(),
		TenantID:    norm.TenantID,
		Display:     norm.Display,
		Backend:     norm.Backend,
		Host:        norm.Host,
		Port:        norm.Port,
		User:        norm.User,
		IdentityRef: norm.IdentityRef,
		AgentID:     norm.AgentID,
		Health:      healthUnknown,
		CreatedAt:   at,
		UpdatedAt:   at,
	}
	w.rows[row.ID] = row
	return publicRow(row), nil
}

// List returns tenant workstations, oldest first.
func (w *Workstations) List(tenant string) []Public {
	if w == nil {
		return []Public{}
	}
	tenant = store.NormalizeTenant(tenant)
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]Public, 0)
	for _, r := range w.rows {
		if r == nil || !store.SameTenant(r.TenantID, tenant) {
			continue
		}
		out = append(out, publicRow(r))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

// Get returns one workstation in the tenant.
func (w *Workstations) Get(id, tenant string) (Public, error) {
	r, err := w.get(id, tenant)
	if err != nil {
		return Public{}, err
	}
	return publicRow(r), nil
}

func (w *Workstations) get(id, tenant string) (*row, error) {
	if w == nil {
		return nil, ErrNotFound
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrNotFound
	}
	tenant = store.NormalizeTenant(tenant)
	w.mu.Lock()
	defer w.mu.Unlock()
	r := w.rows[id]
	if r == nil || !store.SameTenant(r.TenantID, tenant) {
		return nil, ErrNotFound
	}
	cp := *r
	return &cp, nil
}

// Update replaces validated fields. agentSet/identitySet mark optional clears.
func (w *Workstations) Update(id, tenant string, in Input, identitySet, agentSet bool, now time.Time) (Public, error) {
	if w == nil {
		return Public{}, ErrNotFound
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Public{}, ErrNotFound
	}
	if identitySet && LooksLikeKey(in.IdentityRef) {
		return Public{}, ErrKeyMaterial
	}
	tenant = store.NormalizeTenant(tenant)
	if now.IsZero() {
		now = w.clock()
	} else {
		now = now.UTC()
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	r := w.rows[id]
	if r == nil || !store.SameTenant(r.TenantID, tenant) {
		return Public{}, ErrNotFound
	}
	next := Input{
		Display:     r.Display,
		Backend:     r.Backend,
		Host:        r.Host,
		Port:        r.Port,
		User:        r.User,
		IdentityRef: r.IdentityRef,
		AgentID:     r.AgentID,
		TenantID:    r.TenantID,
	}
	if strings.TrimSpace(in.Display) != "" {
		next.Display = in.Display
	}
	if strings.TrimSpace(in.Backend) != "" {
		next.Backend = in.Backend
	}
	if strings.TrimSpace(in.Host) != "" {
		next.Host = in.Host
	}
	if in.Port != 0 {
		next.Port = in.Port
	}
	if strings.TrimSpace(in.User) != "" {
		next.User = in.User
	}
	if identitySet {
		next.IdentityRef = strings.TrimSpace(in.IdentityRef)
	}
	if agentSet {
		next.AgentID = strings.TrimSpace(in.AgentID)
	}
	norm, err := validate(next)
	if err != nil {
		return Public{}, err
	}
	r.Display = norm.Display
	r.Backend = norm.Backend
	r.Host = norm.Host
	r.Port = norm.Port
	r.User = norm.User
	r.IdentityRef = norm.IdentityRef
	r.AgentID = norm.AgentID
	r.UpdatedAt = now
	return publicRow(r), nil
}

// Test validates stored config locally. It never opens SSH or reads identity files.
func (w *Workstations) Test(id, tenant string, now time.Time) (TestResult, Public, error) {
	if w == nil {
		return TestResult{}, Public{}, ErrNotFound
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return TestResult{}, Public{}, ErrNotFound
	}
	tenant = store.NormalizeTenant(tenant)
	if now.IsZero() {
		now = w.clock()
	} else {
		now = now.UTC()
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	r := w.rows[id]
	if r == nil || !store.SameTenant(r.TenantID, tenant) {
		return TestResult{}, Public{}, ErrNotFound
	}
	_, err := validate(Input{
		Display:     r.Display,
		Backend:     r.Backend,
		Host:        r.Host,
		Port:        r.Port,
		User:        r.User,
		IdentityRef: r.IdentityRef,
		AgentID:     r.AgentID,
		TenantID:    r.TenantID,
	})
	summary := r.Backend + " config valid"
	health := healthOK
	ok := true
	if err != nil {
		ok = false
		health = healthFailed
		summary = "invalid config"
	}
	r.Health = health
	r.LastTested = now
	r.UpdatedAt = now
	tr := TestResult{
		OK:          ok,
		Health:      health,
		Summary:     summary,
		Backend:     r.Backend,
		Host:        r.Host,
		Port:        r.Port,
		IdentitySet: r.IdentityRef != "",
	}
	return tr, publicRow(r), nil
}

// Disconnect marks a target disconnected. Confirm must match id or display.
func (w *Workstations) Disconnect(id, tenant, confirm string, now time.Time) (Public, error) {
	return w.mutate(id, tenant, confirm, now, func(r *row) error {
		if r.Health == healthOffline {
			return ErrNotDisconnected
		}
		r.Health = healthOffline
		return nil
	})
}

// Delete removes a workstation. Confirm must match id or display.
func (w *Workstations) Delete(id, tenant, confirm string) (Public, error) {
	if strings.TrimSpace(confirm) == "" {
		return Public{}, ErrConfirmRequired
	}
	if w == nil {
		return Public{}, ErrNotFound
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Public{}, ErrNotFound
	}
	tenant = store.NormalizeTenant(tenant)

	w.mu.Lock()
	defer w.mu.Unlock()
	r := w.rows[id]
	if r == nil || !store.SameTenant(r.TenantID, tenant) {
		return Public{}, ErrNotFound
	}
	pub := publicRow(r)
	if !ConfirmOK(confirm, pub) {
		return Public{}, ErrConfirm
	}
	delete(w.rows, id)
	return pub, nil
}

func (w *Workstations) mutate(id, tenant, confirm string, now time.Time, fn func(*row) error) (Public, error) {
	if strings.TrimSpace(confirm) == "" {
		return Public{}, ErrConfirmRequired
	}
	if w == nil {
		return Public{}, ErrNotFound
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Public{}, ErrNotFound
	}
	if now.IsZero() {
		now = w.clock()
	} else {
		now = now.UTC()
	}
	tenant = store.NormalizeTenant(tenant)

	w.mu.Lock()
	defer w.mu.Unlock()
	r := w.rows[id]
	if r == nil || !store.SameTenant(r.TenantID, tenant) {
		return Public{}, ErrNotFound
	}
	if !ConfirmOK(confirm, publicRow(r)) {
		return Public{}, ErrConfirm
	}
	if err := fn(r); err != nil {
		return Public{}, err
	}
	r.UpdatedAt = now
	return publicRow(r), nil
}

func publicRow(r *row) Public {
	if r == nil {
		return Public{}
	}
	out := Public{
		ID:          r.ID,
		Display:     r.Display,
		Backend:     r.Backend,
		Host:        r.Host,
		Port:        r.Port,
		User:        r.User,
		IdentityRef: r.IdentityRef,
		IdentitySet: r.IdentityRef != "",
		AgentID:     r.AgentID,
		Health:      r.Health,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
	if !r.LastTested.IsZero() {
		at := r.LastTested
		out.LastTested = &at
	}
	return out
}
