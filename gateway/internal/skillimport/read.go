// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package skillimport

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// MaxExportBytes is the export file cap. The tool never opens a database.
const MaxExportBytes = 64 << 20

// Export is one offline GoClaw skill dump. Field names are the column names.
type Export struct {
	Skills        []SkillRow        `json:"skills"`
	Versions      []VersionRow      `json:"skill_versions"`
	AgentGrants   []AgentGrantRow   `json:"skill_agent_grants"`
	UserGrants    []UserGrantRow    `json:"skill_user_grants"`
	TenantConfigs []TenantConfigRow `json:"skill_tenant_configs"`
}

// SkillRow mirrors public.skills. Embedding is accepted and dropped.
type SkillRow struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Slug        string          `json:"slug"`
	Description string          `json:"description"`
	OwnerID     string          `json:"owner_id"`
	Visibility  string          `json:"visibility"`
	Version     int             `json:"version"`
	Status      string          `json:"status"`
	Frontmatter json.RawMessage `json:"frontmatter"`
	FilePath    string          `json:"file_path"`
	FileSize    int64           `json:"file_size"`
	FileHash    string          `json:"file_hash"`
	Embedding   json.RawMessage `json:"embedding"`
	Tags        json.RawMessage `json:"tags"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
	IsSystem    bool            `json:"is_system"`
	Deps        json.RawMessage `json:"deps"`
	Enabled     *bool           `json:"enabled"`
	TenantID    string          `json:"tenant_id"`
}

// VersionRow mirrors public.skill_versions. There is no content column.
type VersionRow struct {
	ID                      string          `json:"id"`
	TenantID                string          `json:"tenant_id"`
	SkillID                 string          `json:"skill_id"`
	Version                 int             `json:"version"`
	ContentHash             string          `json:"content_hash"`
	ChangedFiles            json.RawMessage `json:"changed_files"`
	CreatedByActorType      string          `json:"created_by_actor_type"`
	CreatedByActorID        string          `json:"created_by_actor_id"`
	CreatedFromSuggestionID string          `json:"created_from_suggestion_id"`
	CreatedAt               string          `json:"created_at"`
}

// AgentGrantRow mirrors public.skill_agent_grants.
type AgentGrantRow struct {
	ID            string `json:"id"`
	SkillID       string `json:"skill_id"`
	AgentID       string `json:"agent_id"`
	PinnedVersion int    `json:"pinned_version"`
	GrantedBy     string `json:"granted_by"`
	CreatedAt     string `json:"created_at"`
	TenantID      string `json:"tenant_id"`
	CanManage     bool   `json:"can_manage"`
}

// UserGrantRow mirrors public.skill_user_grants.
type UserGrantRow struct {
	ID        string `json:"id"`
	SkillID   string `json:"skill_id"`
	UserID    string `json:"user_id"`
	GrantedBy string `json:"granted_by"`
	CreatedAt string `json:"created_at"`
	TenantID  string `json:"tenant_id"`
}

// TenantConfigRow mirrors public.skill_tenant_configs.
type TenantConfigRow struct {
	ID        string `json:"id"`
	SkillID   string `json:"skill_id"`
	TenantID  string `json:"tenant_id"`
	Enabled   *bool  `json:"enabled"`
	UpdatedAt string `json:"updated_at"`
}

type jsonlRow struct {
	Table string `json:"table"`
}

// ReadExport reads one JSON object or a JSONL stream.
// JSONL lines use a table discriminator plus the column names.
// The discriminator values are skills, skill_versions, skill_agent_grants,
// skill_user_grants, and skill_tenant_configs.
func ReadExport(r io.Reader) (*Export, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxExportBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxExportBytes {
		return nil, fmt.Errorf("export too large")
	}
	trim := bytes.TrimSpace(data)
	if len(trim) == 0 {
		return nil, fmt.Errorf("empty export")
	}
	if isJSONL(trim) {
		return parseJSONL(trim)
	}
	var ex Export
	if err := json.Unmarshal(trim, &ex); err != nil {
		return nil, err
	}
	return &ex, nil
}

func isJSONL(data []byte) bool {
	lines := 0
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if line[0] != '{' {
			return false
		}
		lines++
	}
	if lines == 0 {
		return false
	}
	if lines == 1 {
		return bytes.Contains(data, []byte(`"table"`))
	}
	return true
}

func parseJSONL(data []byte) (*Export, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), MaxExportBytes)
	var ex Export
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var probe jsonlRow
		if err := json.Unmarshal(line, &probe); err != nil {
			return nil, err
		}
		switch probe.Table {
		case "skills":
			var row SkillRow
			if err := json.Unmarshal(line, &row); err != nil {
				return nil, err
			}
			ex.Skills = append(ex.Skills, row)
		case "skill_versions":
			var row VersionRow
			if err := json.Unmarshal(line, &row); err != nil {
				return nil, err
			}
			ex.Versions = append(ex.Versions, row)
		case "skill_agent_grants":
			var row AgentGrantRow
			if err := json.Unmarshal(line, &row); err != nil {
				return nil, err
			}
			ex.AgentGrants = append(ex.AgentGrants, row)
		case "skill_user_grants":
			var row UserGrantRow
			if err := json.Unmarshal(line, &row); err != nil {
				return nil, err
			}
			ex.UserGrants = append(ex.UserGrants, row)
		case "skill_tenant_configs":
			var row TenantConfigRow
			if err := json.Unmarshal(line, &row); err != nil {
				return nil, err
			}
			ex.TenantConfigs = append(ex.TenantConfigs, row)
		default:
			return nil, fmt.Errorf("unknown table")
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return &ex, nil
}
