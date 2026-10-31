// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package bridge

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/mqglobal/goso/gateway/internal/secrets"
)

// ErrNotFound is returned when a tenant config row does not exist.
var ErrNotFound = errors.New("not found")

// Provider is one Omni AI provider. APIKey is plaintext only in memory.
type Provider struct {
	TenantID  string
	ID        string
	Name      string
	Type      string
	BaseURL   string
	Models    []string
	Active    bool
	APIKey    string
	KeySet    bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Agent is one Omni AI agent synced into goso.
type Agent struct {
	TenantID          string
	GosoTenant        string
	ID                string
	Name              string
	Description       string
	HarnessType       string
	ProviderID        string
	ModelName         string
	SystemPrompt      string
	Temperature       *float64
	DailyReplyCap     int
	Status            string
	HarnessConfigJSON string
	ParamsJSON        string
	GosoAgentID       string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Tenant is the Omni id → goso id mapping. Created is true on first insert.
type Tenant struct {
	OmniID  string
	GosoID  string
	Created bool
}

// KnowledgeDoc is one file from core's PUT .../agents/{id}/knowledge body.
type KnowledgeDoc struct {
	ID       string
	FileName string
	MimeType string
	FileSize int64
	Content  string
}

// API keys are sealed with GOSO_MASTER_KEY before they are stored.
type Catalog interface {
	EnsureTenant(omniID string) (Tenant, error)
	PutProvider(p Provider) error
	DeleteProvider(tenantID, providerID string) error
	GetProvider(tenantID, providerID string) (Provider, error)
	ListProviders(tenantID string) ([]Provider, error)
	PutAgent(a Agent) error
	DeleteAgent(tenantID, agentID string) error
	GetAgent(tenantID, agentID string) (Agent, error)
	BindGosoAgent(tenantID, agentID, gosoAgentID string) error
	ReplaceKnowledge(tenantID, agentID string, docs []KnowledgeDoc) error
	ListKnowledge(tenantID, agentID string) ([]KnowledgeDoc, error)
}

type providerRow struct {
	Provider
	nonce []byte
	ct    []byte
}

type memCatalog struct {
	mu        sync.Mutex
	tenants   map[string]Tenant
	providers map[string]providerRow
	agents    map[string]Agent
	knowledge map[string][]KnowledgeDoc
}

func newMem() *memCatalog {
	return &memCatalog{
		tenants:   map[string]Tenant{},
		providers: map[string]providerRow{},
		agents:    map[string]Agent{},
		knowledge: map[string][]KnowledgeDoc{},
	}
}

// NewMemoryCatalog is an in-process catalog. Process restart drops it.
func NewMemoryCatalog() Catalog { return newMem() }

func key(tenant, id string) string { return tenant + "\x00" + id }

func (m *memCatalog) EnsureTenant(omniID string) (Tenant, error) {
	omniID = strings.TrimSpace(omniID)
	if omniID == "" {
		return Tenant{}, badRequest("tenant id is required")
	}
	gosoID := MapTenantID(omniID)
	m.mu.Lock()
	defer m.mu.Unlock()
	if cur, ok := m.tenants[omniID]; ok {
		cur.Created = false
		return cur, nil
	}
	t := Tenant{OmniID: omniID, GosoID: gosoID, Created: true}
	m.tenants[omniID] = t
	return t, nil
}

func (m *memCatalog) PutProvider(p Provider) error {
	if err := p.normalize(); err != nil {
		return err
	}
	nonce, ct, err := seal(p.APIKey)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key(p.TenantID, p.ID)
	now := time.Now().UTC()
	if cur, ok := m.providers[k]; ok {
		p.CreatedAt = cur.CreatedAt
	} else if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	stored := p
	stored.APIKey = ""
	m.providers[k] = providerRow{Provider: stored, nonce: nonce, ct: ct}
	return nil
}

func (m *memCatalog) DeleteProvider(tenantID, providerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.providers, key(tenantID, providerID))
	return nil
}

func (m *memCatalog) GetProvider(tenantID, providerID string) (Provider, error) {
	m.mu.Lock()
	row, ok := m.providers[key(tenantID, providerID)]
	m.mu.Unlock()
	if !ok {
		return Provider{}, ErrNotFound
	}
	return unsealProvider(row)
}

func (m *memCatalog) ListProviders(tenantID string) ([]Provider, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Provider
	prefix := tenantID + "\x00"
	for k, row := range m.providers {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		p, err := unsealProvider(row)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (m *memCatalog) PutAgent(a Agent) error {
	if err := a.normalize(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key(a.TenantID, a.ID)
	now := time.Now().UTC()
	if cur, ok := m.agents[k]; ok {
		a.CreatedAt = cur.CreatedAt
		if a.GosoAgentID == "" {
			a.GosoAgentID = cur.GosoAgentID
		}
		if a.GosoTenant == "" {
			a.GosoTenant = cur.GosoTenant
		}
	} else if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	m.agents[k] = a
	return nil
}

func (m *memCatalog) DeleteAgent(tenantID, agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.agents, key(tenantID, agentID))
	return nil
}

func (m *memCatalog) GetAgent(tenantID, agentID string) (Agent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.agents[key(tenantID, agentID)]
	if !ok {
		return Agent{}, ErrNotFound
	}
	return a, nil
}

func (m *memCatalog) BindGosoAgent(tenantID, agentID, gosoAgentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key(tenantID, agentID)
	a, ok := m.agents[k]
	if !ok {
		return ErrNotFound
	}
	a.GosoAgentID = gosoAgentID
	m.agents[k] = a
	return nil
}

func (m *memCatalog) ReplaceKnowledge(tenantID, agentID string, docs []KnowledgeDoc) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]KnowledgeDoc, len(docs))
	copy(cp, docs)
	m.knowledge[key(tenantID, agentID)] = cp
	return nil
}

func (m *memCatalog) ListKnowledge(tenantID, agentID string) ([]KnowledgeDoc, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	src := m.knowledge[key(tenantID, agentID)]
	out := make([]KnowledgeDoc, len(src))
	copy(out, src)
	return out, nil
}

func (p *Provider) normalize() error {
	p.TenantID = strings.TrimSpace(p.TenantID)
	p.ID = strings.TrimSpace(p.ID)
	p.Name = strings.TrimSpace(p.Name)
	p.Type = strings.ToLower(strings.TrimSpace(p.Type))
	p.BaseURL = strings.TrimSpace(p.BaseURL)
	if p.TenantID == "" || p.ID == "" {
		return badRequest("tenant id and provider id are required")
	}
	if p.Name == "" {
		return badRequest("provider name is required")
	}
	if p.Type == "" {
		return badRequest("provider type is required")
	}
	if p.Models == nil {
		p.Models = []string{}
	}
	return nil
}

func (a *Agent) normalize() error {
	a.TenantID = strings.TrimSpace(a.TenantID)
	a.ID = strings.TrimSpace(a.ID)
	a.Name = strings.TrimSpace(a.Name)
	a.ProviderID = strings.TrimSpace(a.ProviderID)
	a.ModelName = strings.TrimSpace(a.ModelName)
	a.HarnessType = strings.TrimSpace(a.HarnessType)
	a.Status = strings.TrimSpace(a.Status)
	if a.TenantID == "" || a.ID == "" {
		return badRequest("tenant id and agent id are required")
	}
	if a.Name == "" {
		return badRequest("agent name is required")
	}
	if a.HarnessType == "" {
		a.HarnessType = "GOSO"
	}
	if a.Status == "" {
		a.Status = "ACTIVE"
	}
	if strings.TrimSpace(a.ParamsJSON) == "" {
		a.ParamsJSON = "{}"
	}
	return nil
}

func seal(plain string) (nonce, ct []byte, err error) {
	if plain == "" {
		return nil, nil, nil
	}
	nonce, ct, err = secrets.Encrypt([]byte(plain))
	if err != nil {
		if errors.Is(err, secrets.ErrNoMasterKey) {
			return nil, nil, unavailable("master key required")
		}
		return nil, nil, unavailable("cannot seal provider key")
	}
	return nonce, ct, nil
}

func openSeal(nonce, ct []byte) (string, error) {
	if len(ct) == 0 {
		return "", nil
	}
	b, err := secrets.Decrypt(nonce, ct)
	if err != nil {
		if errors.Is(err, secrets.ErrNoMasterKey) {
			return "", unavailable("master key required")
		}
		return "", unavailable("cannot read provider key")
	}
	return string(b), nil
}

func unsealProvider(row providerRow) (Provider, error) {
	key, err := openSeal(row.nonce, row.ct)
	if err != nil {
		return Provider{}, err
	}
	p := row.Provider
	p.APIKey = key
	p.KeySet = key != ""
	if p.Models == nil {
		p.Models = []string{}
	}
	return p, nil
}

func cloneModels(in []string) []string {
	if in == nil {
		return []string{}
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func modelsJSON(models []string) string {
	if models == nil {
		models = []string{}
	}
	b, err := json.Marshal(models)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func parseModels(raw string) []string {
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil || out == nil {
		return []string{}
	}
	return out
}
