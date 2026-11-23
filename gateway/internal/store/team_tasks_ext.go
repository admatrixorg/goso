package store

import (
	"errors"
	"strings"
	"time"
)

// TeamTaskAttachment is one file attached to a Kanban task.
type TeamTaskAttachment struct {
	ID               string `json:"id"`
	TenantID         string `json:"-"`
	TaskID           string `json:"task_id"`
	TeamID           string `json:"team_id"`
	ChatID           string `json:"chat_id,omitempty"`
	Path             string `json:"path"`
	FileSize         int64 `json:"file_size"`
	MimeType         string `json:"mime_type,omitempty"`
	CreatedByAgentID string `json:"created_by_agent_id,omitempty"`
	CreatedBySender  string `json:"created_by_sender_id,omitempty"`
	Metadata         string `json:"metadata,omitempty"`
	CreatedAt        string `json:"created_at"`
}

// TeamTaskAttachments is the attachment capability.
type TeamTaskAttachments interface {
	ListTeamTaskAttachments(tenant, taskID string) []*TeamTaskAttachment
	PutTeamTaskAttachment(TeamTaskAttachment) (*TeamTaskAttachment, error)
	DeleteTeamTaskAttachment(tenant, id string) error
	CountTeamTaskAttachments(taskID string) int
}

// Attachments returns the attachment capability of st, or nil.
func Attachments(st StoreIface) TeamTaskAttachments {
	if x, ok := st.(TeamTaskAttachments); ok {
		return x
	}
	return nil
}

var teamTaskStatusAliases = map[string]string{
	"todo": "todo",
	"doing": "doing",
	"done": "done",
	"pending": "pending",
	"in_progress": "in_progress",
	"completed": "completed",
	"blocked": "blocked",
	"failed": "failed",
	"in_review": "in_review",
	"cancelled": "cancelled",
}

// NormalizeTeamTaskStatus keeps every status either vocabulary already uses.
func NormalizeTeamTaskStatus(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	v, ok := teamTaskStatusAliases[s]
	return v, ok
}

func (s *SQLiteStore) migrateTeamTasksExt() error {
	cols := [][2]string{
		{"description", "TEXT NOT NULL DEFAULT ''"},
		{"subject", "TEXT NOT NULL DEFAULT ''"},
		{"priority", "INTEGER NOT NULL DEFAULT 0"},
		{"result", "TEXT NOT NULL DEFAULT ''"},
		{"user_id", "TEXT NOT NULL DEFAULT ''"},
		{"channel", "TEXT NOT NULL DEFAULT ''"},
		{"chat_id", "TEXT NOT NULL DEFAULT ''"},
		{"task_type", "TEXT NOT NULL DEFAULT 'general'"},
		{"task_number", "INTEGER NOT NULL DEFAULT 0"},
		{"identifier", "TEXT NOT NULL DEFAULT ''"},
		{"created_by_agent_id", "TEXT NOT NULL DEFAULT ''"},
		{"owner_agent_id", "TEXT NOT NULL DEFAULT ''"},
		{"assignee_user_id", "TEXT NOT NULL DEFAULT ''"},
		{"parent_id", "TEXT NOT NULL DEFAULT ''"},
		{"progress_percent", "INTEGER NOT NULL DEFAULT 0"},
		{"progress_step", "TEXT NOT NULL DEFAULT ''"},
		{"blocked_by", "TEXT NOT NULL DEFAULT ''"},
		{"metadata", "TEXT NOT NULL DEFAULT ''"},
		{"followup_at", "TEXT NOT NULL DEFAULT ''"},
		{"followup_count", "INTEGER NOT NULL DEFAULT 0"},
		{"followup_max", "INTEGER NOT NULL DEFAULT 0"},
		{"followup_message", "TEXT NOT NULL DEFAULT ''"},
		{"followup_channel", "TEXT NOT NULL DEFAULT ''"},
		{"followup_chat_id", "TEXT NOT NULL DEFAULT ''"},
		{"tenant_id", "TEXT NOT NULL DEFAULT 'default'"},
		{"created_at", "TEXT NOT NULL DEFAULT ''"},
		{"updated_at", "TEXT NOT NULL DEFAULT ''"},
	}
	for _, c := range cols {
		if err := s.addColumnIfMissing("team_tasks", c[0], c[1]); err != nil {
			return err
		}
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS team_task_attachments (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			task_id TEXT NOT NULL,
			team_id TEXT NOT NULL DEFAULT '',
			chat_id TEXT NOT NULL DEFAULT '',
			path TEXT NOT NULL,
			file_size INTEGER NOT NULL DEFAULT 0,
			mime_type TEXT NOT NULL DEFAULT '',
			created_by_agent_id TEXT NOT NULL DEFAULT '',
			created_by_sender_id TEXT NOT NULL DEFAULT '',
			metadata TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_team_task_attachments_task ON team_task_attachments(task_id)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// backfillTaskSubject keeps title and subject in step for older rows.
func (s *SQLiteStore) backfillTaskSubject() error {
	if _, err := s.db.Exec(`UPDATE team_tasks SET subject = title WHERE (subject = '' OR subject IS NULL) AND title <> ''`); err != nil {
		return err
	}
	_, err := s.db.Exec(`UPDATE team_tasks SET title = subject WHERE (title = '' OR title IS NULL) AND subject <> ''`)
	return err
}

func (s *SQLiteStore) ListTeamTaskAttachments(tenant, taskID string) []*TeamTaskAttachment {
	rows, err := s.db.Query(`SELECT id, tenant_id, task_id, team_id, chat_id, path, file_size, mime_type,
		created_by_agent_id, created_by_sender_id, metadata, created_at
		FROM team_task_attachments WHERE task_id=? ORDER BY created_at`, strings.TrimSpace(taskID))
	if err != nil {
		return []*TeamTaskAttachment{}
	}
	defer rows.Close()
	out := []*TeamTaskAttachment{}
	for rows.Next() {
		var a TeamTaskAttachment
		if rows.Scan(&a.ID, &a.TenantID, &a.TaskID, &a.TeamID, &a.ChatID, &a.Path, &a.FileSize,
			&a.MimeType, &a.CreatedByAgentID, &a.CreatedBySender, &a.Metadata, &a.CreatedAt) != nil {
			continue
		}
		out = append(out, &a)
	}
	return out
}

func (s *SQLiteStore) PutTeamTaskAttachment(a TeamTaskAttachment) (*TeamTaskAttachment, error) {
	a.TaskID = strings.TrimSpace(a.TaskID)
	a.Path = strings.TrimSpace(a.Path)
	if a.TaskID == "" || a.Path == "" {
		return nil, errors.New("task_id and path are required")
	}
	if a.TenantID == "" {
		a.TenantID = NormalizeTenant("")
	}
	if a.ID == "" {
		a.ID = "tta_" + newID()
	}
	if a.CreatedAt == "" {
		a.CreatedAt = formatTime(time.Now().UTC())
	}
	_, err := s.db.Exec(`INSERT INTO team_task_attachments(id, tenant_id, task_id, team_id, chat_id, path,
		file_size, mime_type, created_by_agent_id, created_by_sender_id, metadata, created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.TenantID, a.TaskID, a.TeamID, a.ChatID, a.Path, a.FileSize, a.MimeType,
		a.CreatedByAgentID, a.CreatedBySender, a.Metadata, a.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *SQLiteStore) DeleteTeamTaskAttachment(tenant, id string) error {
	res, err := s.db.Exec(`DELETE FROM team_task_attachments WHERE id=?`, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) CountTeamTaskAttachments(taskID string) int {
	var n int
	_ = s.db.QueryRow(`SELECT count(*) FROM team_task_attachments WHERE task_id=?`, strings.TrimSpace(taskID)).Scan(&n)
	return n
}
