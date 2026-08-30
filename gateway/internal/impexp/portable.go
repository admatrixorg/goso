// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package impexp

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

const (
	Schema        = "goso.portable/v1"
	SchemaVersion = 1
	SecretPolicy  = "excluded"

	ConflictSkip      = "skip"
	ConflictOverwrite = "overwrite"
	ConflictRename    = "rename"

	KindExport   = "export"
	KindImport   = "import"
	KindRollback = "rollback"

	StatusPending    = "pending"
	StatusRunning    = "running"
	StatusDone       = "done"
	StatusFailed     = "failed"
	StatusRolledBack = "rolled_back"
	StatusRolling    = "rolling_back"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrInvalidArchive = errors.New("invalid archive")
	ErrSchema         = errors.New("unsupported schema")
	ErrVersion        = errors.New("unsupported schema_version")
	ErrEmptySelection = errors.New("selection is empty")
	ErrNoRollback     = errors.New("rollback unavailable")
	ErrDryRun         = errors.New("dry run cannot roll back")
	ErrConflict       = errors.New("unknown conflict strategy")
	ErrAlreadyRolled  = errors.New("already rolled back")
	ErrImportNotDone  = errors.New("import is not complete")
	secretKeySet      = map[string]struct{}{
		"token": {}, "secret": {}, "password": {}, "hmac": {}, "hmac_key": {},
		"bot_token": {}, "access_token": {}, "api_key": {}, "authorization": {},
		"private_key": {}, "bearer": {}, "credential": {}, "args": {}, "arguments": {},
	}
	secretVal = regexp.MustCompile(`(?i)\b(sk-[A-Za-z0-9_-]{8,}|gsk_[A-Za-z0-9]+|xai-[A-Za-z0-9]+|AIza[A-Za-z0-9_-]+|gk_[0-9a-f]{16,}|Bearer\s+[A-Za-z0-9._\-+=/]{8,}|ghp_[A-Za-z0-9]+|token=)`)
)

// Selection is the operator-chosen export scope.
type Selection struct {
	TeamIDs    []string `json:"team_ids"`
	AgentIDs   []string `json:"agent_ids"`
	SkillNames []string `json:"skill_names"`
	MCPNames   []string `json:"mcp_names"`
}

// Catalog is the selectable inventory. GET never includes tokens.
type Catalog struct {
	Teams            []CatalogTeam  `json:"teams"`
	Agents           []CatalogAgent `json:"agents"`
	Skills           []CatalogSkill `json:"skills"`
	MCP              []CatalogMCP   `json:"mcp"`
	SkillsConfigured bool           `json:"skills_configured"`
	GeneratedAt      time.Time      `json:"generated_at"`
}

type CatalogTeam struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	LeadAgentID string `json:"lead_agent_id,omitempty"`
	Members     int    `json:"members"`
}

type CatalogAgent struct {
	ID          string `json:"id"`
	AgentKey    string `json:"agent_key"`
	DisplayName string `json:"display_name"`
	Enabled     bool   `json:"enabled"`
	Model       string `json:"model,omitempty"`
}

type CatalogSkill struct {
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
}

type CatalogMCP struct {
	Name      string `json:"name"`
	Transport string `json:"transport"`
	Endpoint  string `json:"endpoint,omitempty"`
	Enabled   bool   `json:"enabled"`
	TokenSet  bool   `json:"token_set"`
	EnvOwned  bool   `json:"env_owned"`
}

// Archive is the portable JSON document. Secrets are never present after Strip.
type Archive struct {
	Schema         string      `json:"schema"`
	SchemaVersion  int         `json:"schema_version"`
	ExportedAt     time.Time   `json:"exported_at"`
	IncludeSecrets bool        `json:"include_secrets"`
	Manifest       Manifest    `json:"manifest"`
	Teams          []TeamItem  `json:"teams"`
	Agents         []AgentItem `json:"agents"`
	Skills         []SkillItem `json:"skills"`
	MCP            []MCPItem   `json:"mcp"`
	Warnings       []string    `json:"warnings,omitempty"`
}

type Manifest struct {
	SchemaVersion int            `json:"schema_version"`
	SecretPolicy  string         `json:"secret_policy"`
	Teams         []ManifestItem `json:"teams"`
	Agents        []ManifestItem `json:"agents"`
	Skills        []ManifestItem `json:"skills"`
	MCP           []ManifestItem `json:"mcp"`
}

type ManifestItem struct {
	Kind string `json:"kind"`
	ID   string `json:"id,omitempty"`
	Key  string `json:"key,omitempty"`
	Name string `json:"name"`
}

type TeamItem struct {
	Name         string       `json:"name"`
	LeadAgentKey string       `json:"lead_agent_key,omitempty"`
	Members      []MemberItem `json:"members,omitempty"`
}

type MemberItem struct {
	AgentKey string `json:"agent_key"`
	Role     string `json:"role"`
}

type AgentItem struct {
	AgentKey          string   `json:"agent_key"`
	DisplayName       string   `json:"display_name"`
	Model             string   `json:"model,omitempty"`
	LLMProvider       string   `json:"llm_provider,omitempty"`
	Instructions      string   `json:"instructions,omitempty"`
	OrchestrationMode string   `json:"orchestration_mode,omitempty"`
	Enabled           bool     `json:"enabled"`
	Links             []string `json:"links,omitempty"`
}

type SkillItem struct {
	Name string `json:"name"`
	Body string `json:"body"`
}

type MCPItem struct {
	Name           string `json:"name"`
	Transport      string `json:"transport"`
	Endpoint       string `json:"endpoint,omitempty"`
	SchemaVersion  string `json:"schema_version,omitempty"`
	Enabled        bool   `json:"enabled"`
	ManifestURL    string `json:"manifest_url,omitempty"`
	TimeoutMS      int    `json:"timeout_ms,omitempty"`
	Retries        int    `json:"retries,omitempty"`
	TokenSet       bool   `json:"token_set"`
	EnvOwned       bool   `json:"env_owned"`
	CredentialKind string `json:"credential_kind,omitempty"`
	EnvName        string `json:"env_name,omitempty"`
}

type Job struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	Progress  int       `json:"progress"`
	DryRun    bool      `json:"dry_run"`
	Conflict  string    `json:"conflict,omitempty"`
	Steps     []Step    `json:"steps"`
	Report    Report    `json:"report"`
	Archive   *Archive  `json:"archive,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	rollback  *rollbackPlan
}

type Step struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type Report struct {
	Created           []ReportItem     `json:"created"`
	Skipped           []ReportItem     `json:"skipped"`
	Overwritten       []ReportItem     `json:"overwritten"`
	Renamed           []ReportItem     `json:"renamed"`
	Failed            []ReportItem     `json:"failed"`
	CredentialsNeeded []CredentialNeed `json:"credentials_needed"`
}

type ReportItem struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	ID     string `json:"id,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type CredentialNeed struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Reason  string `json:"reason"`
	EnvName string `json:"env_name,omitempty"`
}

type Preview struct {
	Valid     bool           `json:"valid"`
	Errors    []string       `json:"errors"`
	Warnings  []string       `json:"warnings"`
	Manifest  Manifest       `json:"manifest"`
	Conflicts []ConflictItem `json:"conflicts"`
	Archive   *Archive       `json:"archive,omitempty"`
}

type ConflictItem struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Existing string `json:"existing,omitempty"`
}

type rollbackPlan struct {
	CreatedAgents []string
	CreatedTeams  []string
	CreatedSkills []string
	CreatedMCP    []string
	PrevAgents    []agentSnap
	PrevTeams     []teamSnap
	PrevSkills    []SkillItem
	PrevMCP       []mcpSnap
}

type agentSnap struct {
	ID                string
	Instructions      string
	Model             string
	LLMProvider       string
	OrchestrationMode string
	Enabled           bool
}

type teamSnap struct {
	ID          string
	Name        string
	LeadAgentID string
}

type mcpSnap struct {
	Name     string
	Endpoint string
	Enabled  bool
}

// NormalizeConflict maps operator input onto skip|overwrite|rename.
func NormalizeConflict(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", ConflictSkip:
		return ConflictSkip, nil
	case ConflictOverwrite:
		return ConflictOverwrite, nil
	case ConflictRename:
		return ConflictRename, nil
	default:
		return "", ErrConflict
	}
}

// DecodeArchive unmarshals without filling schema identity or stripping.
func DecodeArchive(raw []byte) (*Archive, error) {
	raw = bytesTrim(raw)
	if len(raw) == 0 {
		return nil, ErrInvalidArchive
	}
	var a Archive
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, ErrInvalidArchive
	}
	return &a, nil
}

func bytesTrim(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

// ValidateSchema checks schema id and version. Unknown future versions fail closed.
func ValidateSchema(a *Archive) []string {
	var errs []string
	if a == nil {
		return []string{ErrInvalidArchive.Error()}
	}
	if strings.TrimSpace(a.Schema) == "" {
		errs = append(errs, ErrSchema.Error())
	} else if a.Schema != Schema {
		errs = append(errs, ErrSchema.Error())
	}
	if a.SchemaVersion != SchemaVersion {
		errs = append(errs, ErrVersion.Error())
	}
	return errs
}

// StripArchive drops secret-bearing fields and token-shaped strings. Always sets include_secrets false.
func StripArchive(a *Archive) {
	if a == nil {
		return
	}
	a.IncludeSecrets = false
	a.Schema = strings.TrimSpace(a.Schema)
	a.Manifest.SchemaVersion = a.SchemaVersion
	a.Manifest.SecretPolicy = SecretPolicy
	for i := range a.Agents {
		a.Agents[i].Instructions = redactText(a.Agents[i].Instructions)
		a.Agents[i].AgentKey = strings.TrimSpace(a.Agents[i].AgentKey)
		a.Agents[i].DisplayName = strings.TrimSpace(a.Agents[i].DisplayName)
		a.Agents[i].LLMProvider = strings.TrimSpace(a.Agents[i].LLMProvider)
		a.Agents[i].Model = strings.TrimSpace(a.Agents[i].Model)
	}
	for i := range a.Skills {
		a.Skills[i].Name = strings.TrimSpace(a.Skills[i].Name)
		a.Skills[i].Body = redactText(a.Skills[i].Body)
	}
	for i := range a.Teams {
		a.Teams[i].Name = strings.TrimSpace(a.Teams[i].Name)
		a.Teams[i].LeadAgentKey = strings.TrimSpace(a.Teams[i].LeadAgentKey)
	}
	clean := make([]MCPItem, 0, len(a.MCP))
	for _, m := range a.MCP {
		m.Name = strings.TrimSpace(m.Name)
		if m.Name == "" {
			continue
		}
		m.Endpoint = redactURL(strings.TrimSpace(m.Endpoint))
		m.ManifestURL = redactURL(strings.TrimSpace(m.ManifestURL))
		m.EnvName = strings.TrimSpace(m.EnvName)
		if !isEnvName(m.EnvName) {
			m.EnvName = ""
		}
		if m.EnvOwned && m.EnvName == "" {
			m.EnvOwned = false
		}
		if m.CredentialKind != "env" && m.CredentialKind != "secret" {
			m.CredentialKind = ""
		}
		if m.CredentialKind == "env" && m.EnvName == "" {
			m.CredentialKind = ""
		}
		clean = append(clean, m)
	}
	a.MCP = clean
	rebuildManifest(a)
}

func rebuildManifest(a *Archive) {
	if a == nil {
		return
	}
	a.Manifest.SchemaVersion = a.SchemaVersion
	a.Manifest.SecretPolicy = SecretPolicy
	a.Manifest.Teams = a.Manifest.Teams[:0]
	a.Manifest.Agents = a.Manifest.Agents[:0]
	a.Manifest.Skills = a.Manifest.Skills[:0]
	a.Manifest.MCP = a.Manifest.MCP[:0]
	for _, t := range a.Teams {
		a.Manifest.Teams = append(a.Manifest.Teams, ManifestItem{Kind: "team", Name: t.Name})
	}
	for _, ag := range a.Agents {
		a.Manifest.Agents = append(a.Manifest.Agents, ManifestItem{Kind: "agent", Key: ag.AgentKey, Name: displayOrKey(ag.DisplayName, ag.AgentKey)})
	}
	for _, sk := range a.Skills {
		a.Manifest.Skills = append(a.Manifest.Skills, ManifestItem{Kind: "skill", Name: sk.Name})
	}
	for _, m := range a.MCP {
		a.Manifest.MCP = append(a.Manifest.MCP, ManifestItem{Kind: "mcp", Name: m.Name})
	}
}

func displayOrKey(name, key string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	return key
}

func redactText(s string) string {
	if s == "" {
		return s
	}
	return secretVal.ReplaceAllString(s, "[redacted]")
}

func redactURL(s string) string {
	if s == "" {
		return s
	}
	if secretVal.MatchString(s) {
		if i := strings.IndexByte(s, '?'); i >= 0 {
			return s[:i]
		}
		return redactText(s)
	}
	return s
}

func isEnvName(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range s {
		if i == 0 && (c < 'A' || c > 'Z') {
			return false
		}
		if !((c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

// ContainsSecrets reports token-shaped keys or values in a JSON-able value.
func ContainsSecrets(v any) bool {
	if v == nil {
		return false
	}
	if s, ok := v.(string); ok {
		return secretVal.MatchString(s)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return false
	}
	var generic any
	if err := json.Unmarshal(b, &generic); err != nil {
		return secretVal.Match(b)
	}
	return walkSecrets(generic)
}

func walkSecrets(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case string:
		return secretVal.MatchString(t)
	case map[string]any:
		for k, val := range t {
			lk := strings.ToLower(strings.TrimSpace(k))
			if _, ok := secretKeySet[lk]; ok {
				if s, is := val.(string); is && s != "" {
					return true
				}
				if lk == "args" || lk == "arguments" {
					return true
				}
			}
			if walkSecrets(val) {
				return true
			}
		}
	case []any:
		for _, item := range t {
			if walkSecrets(item) {
				return true
			}
		}
	}
	return false
}

// PublicJob returns a GET-safe copy (rollback internals omitted; archive already stripped).
func PublicJob(j *Job) *Job {
	if j == nil {
		return nil
	}
	cp := *j
	cp.rollback = nil
	if cp.Archive != nil {
		a := *cp.Archive
		StripArchive(&a)
		cp.Archive = &a
	}
	if cp.Steps == nil {
		cp.Steps = []Step{}
	}
	cp.Report = publicReport(cp.Report)
	return &cp
}

func publicReport(r Report) Report {
	if r.Created == nil {
		r.Created = []ReportItem{}
	}
	if r.Skipped == nil {
		r.Skipped = []ReportItem{}
	}
	if r.Overwritten == nil {
		r.Overwritten = []ReportItem{}
	}
	if r.Renamed == nil {
		r.Renamed = []ReportItem{}
	}
	if r.Failed == nil {
		r.Failed = []ReportItem{}
	}
	if r.CredentialsNeeded == nil {
		r.CredentialsNeeded = []CredentialNeed{}
	}
	return r
}

func emptyReport() Report {
	return publicReport(Report{})
}
