// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"time"
)

// Agent represents an AI agent.
type Agent struct {
	ID                string    `json:"id"`
	AgentKey          string    `json:"agent_key"`
	DisplayName       string    `json:"display_name"`
	Model             string    `json:"model,omitempty"`
	Instructions      string    `json:"instructions,omitempty"`
	OrchestrationMode string    `json:"orchestration_mode,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

// Lite caps when GOSO_LITE=1 (desktop default may set this later).
const (
	LiteMaxAgents = 5
	LiteMaxTeams  = 1
)

// Team is a lead + members group with a shared board and mailbox.
type Team struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	LeadAgentID string    `json:"lead_agent_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// TeamMember is one agent on a team (role lead|member).
type TeamMember struct {
	TeamID  string `json:"team_id"`
	AgentID string `json:"agent_id"`
	Role    string `json:"role"`
}

// TeamTask is a Kanban card (status todo|doing|done).
type TeamTask struct {
	ID              string `json:"id"`
	TeamID          string `json:"team_id"`
	Title           string `json:"title"`
	Status          string `json:"status"`
	AssigneeAgentID string `json:"assignee_agent_id,omitempty"`
}

// TeamMessage is one mailbox row.
type TeamMessage struct {
	ID          string    `json:"id"`
	TeamID      string    `json:"team_id"`
	FromAgentID string    `json:"from_agent_id"`
	Body        string    `json:"body"`
	CreatedAt   time.Time `json:"created_at"`
}

// AgentLink is a one-way delegation edge. Bidirectional = two rows.
type AgentLink struct {
	FromAgentID string `json:"from_agent_id"`
	ToAgentID   string `json:"to_agent_id"`
}

// AgentMetrics counts chat runs and tool outcomes for self-evolution.
type AgentMetrics struct {
	AgentID    string         `json:"agent_id"`
	ChatRuns   int            `json:"chat_runs"`
	ToolErrors int            `json:"tool_errors"`
	ToolUses   map[string]int `json:"tool_uses,omitempty"`
	Advertised []string       `json:"advertised,omitempty"`
}

// Session represents a conversation session.
type Session struct {
	ID        string    `json:"id"`
	AgentID   string    `json:"agent_id"`
	Label     string    `json:"label,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Cron job persistence (SPEC 054). Cap is store-side; empty list is a no-op for the runner.
const CronJobCap = 20

// CronJob is one scheduled chat into a session.
type CronJob struct {
	ID        string     `json:"id"`
	Spec      string     `json:"spec"`
	SessionID string     `json:"session_id"`
	Message   string     `json:"message"`
	Enabled   bool       `json:"enabled"`
	LastRun   *time.Time `json:"last_run,omitempty"`
}

// Message represents a chat message.
type Message struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"` // user | assistant | tool
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// Memory kinds for L1 episodic notes and FTS-backed message hits.
const (
	KindEpisodic = "episodic"
	KindMessage  = "message"
)

// Memory is an L1 episodic (or caller-supplied) note.
type Memory struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Kind      string    `json:"kind"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// SearchHit is one FTS5 / substring match.
type SearchHit struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Kind      string `json:"kind"`
	Snippet   string `json:"snippet"`
}

// VaultDoc is a knowledge-vault registry row. Disk is source of truth after sync;
// Body is an optional cache.
type VaultDoc struct {
	ID     string    `json:"id"`
	Title  string    `json:"title"`
	Path   string    `json:"path"`
	SHA256 string    `json:"sha256"`
	Mtime  time.Time `json:"mtime"`
	Body   string    `json:"body,omitempty"`
}

// VaultLink is one [[wikilink]] edge. ToID is empty until the target resolves.
type VaultLink struct {
	FromID string `json:"from_id"`
	ToID   string `json:"to_id,omitempty"`
	Raw    string `json:"raw"`
}

// VaultSearchHit is one lexical (FTS5 / substring) vault match. Semantic search is DI-09.
type VaultSearchHit struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Path    string `json:"path"`
	Snippet string `json:"snippet"`
}

// ConnectorRecord is a persisted connector registration (config only, no secrets).
type ConnectorRecord struct {
	Name          string          `json:"name"`
	Transport     string          `json:"transport"`
	Endpoint      string          `json:"endpoint"`
	CredentialRef string          `json:"credential_ref,omitempty"`
	SchemaVersion string          `json:"schema_version,omitempty"`
	Enabled       bool            `json:"enabled"`
	ManifestURL   string          `json:"manifest_url,omitempty"`
	ManifestJSON  json.RawMessage `json:"manifest,omitempty"`
	TimeoutMS     int             `json:"timeout_ms,omitempty"`
	Retries       int             `json:"retries,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

// SecretRow is an AES-256-GCM ciphertext blob (never plaintext).
type SecretRow struct {
	Name  string
	Nonce []byte
	CT    []byte
}

// StoreIface is satisfied by both Store (memory) and SQLiteStore.
type StoreIface interface {
	CreateAgent(Agent) (*Agent, error)
	ListAgents() []*Agent
	GetAgent(string) (*Agent, error)
	CreateSession(Session) (*Session, error)
	ListSessions() []*Session
	GetSession(string) (*Session, error)
	AddMessage(Message) (*Message, error)
	ListMessages(string) ([]*Message, error)
	CreateConnector(ConnectorRecord) (*ConnectorRecord, error)
	ListConnectors() []*ConnectorRecord
	GetConnector(string) (*ConnectorRecord, error)
	SetConnectorEnabled(name string, enabled bool) error
	UpdateConnector(name string, enabled *bool, endpoint *string, credentialRef *string) (*ConnectorRecord, error)
	GetToolFlag(name string) bool
	SetToolFlag(name string, enabled bool) error
	ListToolFlags() map[string]bool
	LinkAgentConnector(agentID, connectorName string) error
	ListAgentConnectors(agentID string) ([]string, error)
	PutMemory(Memory) (*Memory, error)
	ListMemories(string) ([]*Memory, error)
	SaveSummary(sessionID, body string) (*Memory, error)
	LatestSummary(sessionID string) (*Memory, error)
	SearchMemory(q string) ([]SearchHit, error)
	PutVaultDoc(VaultDoc) (*VaultDoc, error)
	GetVaultDoc(string) (*VaultDoc, error)
	ListVaultDocs() []*VaultDoc
	FindVaultDocByPath(string) (*VaultDoc, error)
	FindVaultDocByTitle(string) (*VaultDoc, error)
	DeleteVaultDoc(string) error
	ReplaceVaultLinks(fromID string, raws []string) error
	ListVaultDocLinks(id string) ([]VaultLink, []VaultLink, error)
	SearchVault(q string) ([]VaultSearchHit, error)
	ReResolveVaultLinks() error
	UpdateAgent(Agent) (*Agent, error)
	CreateTeam(Team) (*Team, error)
	ListTeams() []*Team
	GetTeam(string) (*Team, error)
	UpdateTeam(Team) (*Team, error)
	DeleteTeam(string) error
	AddTeamMember(TeamMember) (*TeamMember, error)
	ListTeamMembers(teamID string) ([]*TeamMember, error)
	RemoveTeamMember(teamID, agentID string) error
	TeamOfAgent(agentID string) (*Team, error)
	CreateTeamTask(TeamTask) (*TeamTask, error)
	ListTeamTasks(teamID, status string) ([]*TeamTask, error)
	GetTeamTask(string) (*TeamTask, error)
	UpdateTeamTask(TeamTask) (*TeamTask, error)
	CreateTeamMessage(TeamMessage) (*TeamMessage, error)
	ListTeamMessages(teamID string) ([]*TeamMessage, error)
	AddAgentLink(fromID, toID string) error
	ListAgentLinks(fromID string) ([]AgentLink, error)
	HasAgentLink(fromID, toID string) bool
	RecordChatRun(agentID string)
	RecordToolUse(agentID, tool string, failed bool)
	RecordAdvertisedTools(agentID string, names []string)
	GetAgentMetrics(agentID string) AgentMetrics
	MarkEvolutionApplied(agentID, suggestionID string) error
	EvolutionApplied(agentID, suggestionID string) bool
	PutSecret(SecretRow) error
	GetSecret(name string) (*SecretRow, error)
	CreateCronJob(CronJob) (*CronJob, error)
	ListCronJobs() []*CronJob
	GetCronJob(string) (*CronJob, error)
	DeleteCronJob(string) error
	MarkCronRun(id string, at time.Time) error
}

var (
	ErrNotFound = errors.New("not found")
	ErrExists   = errors.New("already exists")
	ErrLiteCap  = errors.New("lite cap exceeded")
	ErrCronCap  = errors.New("cron job cap exceeded")
)

// LiteEnabled reports whether GOSO_LITE=1 (or true) is set.
func LiteEnabled() bool {
	v := strings.TrimSpace(os.Getenv("GOSO_LITE"))
	return v == "1" || strings.EqualFold(v, "true")
}

// Store is an in-memory store. Safe for concurrent use.
type Store struct {
	mu          sync.RWMutex
	agents      map[string]*Agent
	sessions    map[string]*Session
	messages    map[string][]*Message // session_id -> messages
	memories    map[string][]*Memory  // session_id -> memories
	connectors  map[string]*ConnectorRecord
	agentConns  map[string]map[string]struct{} // agent_id -> connector names
	vaultDocs   map[string]*VaultDoc
	vaultLinks  map[string][]VaultLink // from_id -> outbound
	teams       map[string]*Team
	teamMembers map[string][]*TeamMember  // team_id
	teamTasks   map[string]*TeamTask      // task id
	teamMsgs    map[string][]*TeamMessage // team_id
	agentLinks  map[string][]string       // from -> to ids
	metrics     map[string]*AgentMetrics
	evoApplied  map[string]map[string]bool // agent_id -> suggestion_id
	secrets     map[string]SecretRow
	toolFlags   map[string]bool
	cronJobs    map[string]*CronJob
	seq         int64
}

var _ StoreIface = (*Store)(nil)

func Open(path string) (StoreIface, func() error, error) {
	if path == "" || path == ":memory:" {
		s := New()
		return s, func() error { return nil }, nil
	}
	s, err := OpenSQLite(path)
	if err != nil {
		return nil, nil, err
	}
	return s, s.Close, nil
}

func New() *Store {
	return &Store{
		agents:      make(map[string]*Agent),
		sessions:    make(map[string]*Session),
		messages:    make(map[string][]*Message),
		memories:    make(map[string][]*Memory),
		connectors:  make(map[string]*ConnectorRecord),
		agentConns:  make(map[string]map[string]struct{}),
		vaultDocs:   make(map[string]*VaultDoc),
		vaultLinks:  make(map[string][]VaultLink),
		teams:       make(map[string]*Team),
		teamMembers: make(map[string][]*TeamMember),
		teamTasks:   make(map[string]*TeamTask),
		teamMsgs:    make(map[string][]*TeamMessage),
		agentLinks:  make(map[string][]string),
		metrics:     make(map[string]*AgentMetrics),
		evoApplied:  make(map[string]map[string]bool),
		secrets:     make(map[string]SecretRow),
		toolFlags:   make(map[string]bool),
		cronJobs:    make(map[string]*CronJob),
	}
}

func (s *Store) nextID() string {
	s.seq++
	return time.Now().UTC().Format("20060102") + "-" + itoa(s.seq)
}

// SnippetAround returns a window of runes around the first case-insensitive match of q.
func SnippetAround(body, q string, window int) string {
	if window <= 0 {
		window = 80
	}
	br := []rune(body)
	if len(br) == 0 {
		return ""
	}
	qr := []rune(strings.ToLower(q))
	bl := []rune(strings.ToLower(body))
	idx := -1
	if len(qr) > 0 && len(qr) <= len(bl) {
		for i := 0; i+len(qr) <= len(bl); i++ {
			match := true
			for j := range qr {
				if bl[i+j] != qr[j] {
					match = false
					break
				}
			}
			if match {
				idx = i
				break
			}
		}
	}
	if idx < 0 {
		if len(br) <= window {
			return body
		}
		return string(br[:window])
	}
	start := idx - window/4
	if start < 0 {
		start = 0
	}
	end := start + window
	if end > len(br) {
		end = len(br)
		start = end - window
		if start < 0 {
			start = 0
		}
	}
	return string(br[start:end])
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	b := make([]byte, 0, 20)
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// --- Agent ---

func (s *Store) CreateAgent(a Agent) (*Agent, error) {
	if a.AgentKey == "" {
		return nil, errors.New("agent_key is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if LiteEnabled() && len(s.agents) >= LiteMaxAgents {
		return nil, ErrLiteCap
	}
	for _, v := range s.agents {
		if v.AgentKey == a.AgentKey {
			return nil, ErrExists
		}
	}
	a.ID = s.nextID()
	a.CreatedAt = time.Now().UTC()
	cp := a
	s.agents[cp.ID] = &cp
	return &cp, nil
}

func (s *Store) ListAgents() []*Agent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Agent, 0, len(s.agents))
	for _, v := range s.agents {
		cp := *v
		out = append(out, &cp)
	}
	return out
}

func (s *Store) GetAgent(id string) (*Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.agents[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *v
	return &cp, nil
}

func (s *Store) UpdateAgent(a Agent) (*Agent, error) {
	if strings.TrimSpace(a.ID) == "" {
		return nil, errors.New("id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.agents[a.ID]
	if !ok {
		return nil, ErrNotFound
	}
	// Prompt prefix and mode only. Never rewrite agent_key or display_name.
	cur.Instructions = a.Instructions
	if strings.TrimSpace(a.OrchestrationMode) != "" {
		cur.OrchestrationMode = a.OrchestrationMode
	}
	if strings.TrimSpace(a.Model) != "" {
		cur.Model = a.Model
	}
	cp := *cur
	return &cp, nil
}

// --- Session ---

func (s *Store) CreateSession(sess Session) (*Session, error) {
	if sess.AgentID == "" {
		return nil, errors.New("agent_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.agents[sess.AgentID]; !ok {
		return nil, errors.New("agent not found")
	}
	sess.ID = s.nextID()
	sess.CreatedAt = time.Now().UTC()
	cp := sess
	s.sessions[cp.ID] = &cp
	return &cp, nil
}

func (s *Store) ListSessions() []*Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Session, 0, len(s.sessions))
	for _, v := range s.sessions {
		cp := *v
		out = append(out, &cp)
	}
	return out
}

func (s *Store) GetSession(id string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.sessions[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *v
	return &cp, nil
}

// --- Message ---

func (s *Store) AddMessage(m Message) (*Message, error) {
	if m.SessionID == "" {
		return nil, errors.New("session_id is required")
	}
	if m.Role == "" {
		m.Role = "user"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[m.SessionID]; !ok {
		return nil, errors.New("session not found")
	}
	m.ID = s.nextID()
	m.CreatedAt = time.Now().UTC()
	cp := m
	s.messages[cp.SessionID] = append(s.messages[cp.SessionID], &cp)
	return &cp, nil
}

func (s *Store) ListMessages(sessionID string) ([]*Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.sessions[sessionID]; !ok {
		return nil, ErrNotFound
	}
	msgs := s.messages[sessionID]
	out := make([]*Message, len(msgs))
	for i, m := range msgs {
		cp := *m
		out[i] = &cp
	}
	if out == nil {
		out = []*Message{}
	}
	return out, nil
}

// --- Connector ---

func (s *Store) CreateConnector(c ConnectorRecord) (*ConnectorRecord, error) {
	if c.Name == "" {
		return nil, errors.New("name is required")
	}
	if c.Transport == "" {
		c.Transport = "http"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.connectors[c.Name]; ok {
		return nil, ErrExists
	}
	c.CreatedAt = time.Now().UTC()
	cp := c
	s.connectors[cp.Name] = &cp
	return &cp, nil
}

func (s *Store) ListConnectors() []*ConnectorRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*ConnectorRecord, 0, len(s.connectors))
	for _, v := range s.connectors {
		cp := *v
		out = append(out, &cp)
	}
	return out
}

func (s *Store) GetConnector(name string) (*ConnectorRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.connectors[name]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *v
	return &cp, nil
}

func (s *Store) SetConnectorEnabled(name string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.connectors[name]
	if !ok {
		return ErrNotFound
	}
	v.Enabled = enabled
	return nil
}

func (s *Store) UpdateConnector(name string, enabled *bool, endpoint *string, credentialRef *string) (*ConnectorRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.connectors[name]
	if !ok {
		return nil, ErrNotFound
	}
	if enabled != nil {
		v.Enabled = *enabled
	}
	if endpoint != nil {
		v.Endpoint = *endpoint
	}
	if credentialRef != nil {
		v.CredentialRef = *credentialRef
	}
	cp := *v
	return &cp, nil
}

func (s *Store) GetToolFlag(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.toolFlags[strings.TrimSpace(name)]
}

func (s *Store) SetToolFlag(name string, enabled bool) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("name is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.toolFlags[name] = enabled
	return nil
}

func (s *Store) ListToolFlags() map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]bool, len(s.toolFlags))
	for k, v := range s.toolFlags {
		out[k] = v
	}
	return out
}

func (s *Store) LinkAgentConnector(agentID, connectorName string) error {
	if agentID == "" || connectorName == "" {
		return errors.New("agent_id and connector_name are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.agents[agentID]; !ok {
		return errors.New("agent not found")
	}
	if _, ok := s.connectors[connectorName]; !ok {
		return errors.New("connector not found")
	}
	if s.agentConns[agentID] == nil {
		s.agentConns[agentID] = make(map[string]struct{})
	}
	s.agentConns[agentID][connectorName] = struct{}{}
	return nil
}

func (s *Store) ListAgentConnectors(agentID string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.agents[agentID]; !ok {
		return nil, ErrNotFound
	}
	set := s.agentConns[agentID]
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	return out, nil
}

// --- Memory ---

func (s *Store) PutMemory(m Memory) (*Memory, error) {
	if m.SessionID == "" {
		return nil, errors.New("session_id is required")
	}
	if strings.TrimSpace(m.Body) == "" {
		return nil, errors.New("body is required")
	}
	if m.Kind == "" {
		m.Kind = KindEpisodic
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[m.SessionID]; !ok {
		return nil, errors.New("session not found")
	}
	m.ID = s.nextID()
	m.CreatedAt = time.Now().UTC()
	cp := m
	s.memories[cp.SessionID] = append(s.memories[cp.SessionID], &cp)
	return &cp, nil
}

func (s *Store) ListMemories(sessionID string) ([]*Memory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.sessions[sessionID]; !ok {
		return nil, ErrNotFound
	}
	list := s.memories[sessionID]
	out := make([]*Memory, len(list))
	for i, m := range list {
		cp := *m
		out[i] = &cp
	}
	if out == nil {
		out = []*Memory{}
	}
	return out, nil
}

func (s *Store) SaveSummary(sessionID, body string) (*Memory, error) {
	if sessionID == "" {
		return nil, errors.New("session_id is required")
	}
	if strings.TrimSpace(body) == "" {
		return nil, errors.New("body is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sessions[sessionID]; !ok {
		return nil, errors.New("session not found")
	}
	list := s.memories[sessionID]
	for i := len(list) - 1; i >= 0; i-- {
		if list[i] != nil && list[i].Kind == KindEpisodic {
			list[i].Body = body
			list[i].CreatedAt = time.Now().UTC()
			cp := *list[i]
			return &cp, nil
		}
	}
	m := Memory{
		ID:        s.nextID(),
		SessionID: sessionID,
		Kind:      KindEpisodic,
		Body:      body,
		CreatedAt: time.Now().UTC(),
	}
	s.memories[sessionID] = append(s.memories[sessionID], &m)
	cp := m
	return &cp, nil
}

func (s *Store) LatestSummary(sessionID string) (*Memory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.sessions[sessionID]; !ok {
		return nil, ErrNotFound
	}
	list := s.memories[sessionID]
	for i := len(list) - 1; i >= 0; i-- {
		if list[i] != nil && list[i].Kind == KindEpisodic {
			cp := *list[i]
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) SearchMemory(q string) ([]SearchHit, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return []SearchHit{}, nil
	}
	needle := strings.ToLower(q)
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []SearchHit
	for _, list := range s.memories {
		for _, m := range list {
			if m == nil {
				continue
			}
			if strings.Contains(strings.ToLower(m.Body), needle) {
				out = append(out, SearchHit{
					ID: m.ID, SessionID: m.SessionID, Kind: m.Kind,
					Snippet: SnippetAround(m.Body, q, 80),
				})
			}
		}
	}
	for sid, msgs := range s.messages {
		for _, m := range msgs {
			if m == nil {
				continue
			}
			if strings.Contains(strings.ToLower(m.Content), needle) {
				out = append(out, SearchHit{
					ID: m.ID, SessionID: sid, Kind: KindMessage,
					Snippet: SnippetAround(m.Content, q, 80),
				})
			}
		}
	}
	if out == nil {
		out = []SearchHit{}
	}
	return out, nil
}
