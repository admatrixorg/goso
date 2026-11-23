package store

import (
	"errors"
	"strings"
	"time"
)

// ErrInvalidTeamTaskComment reports a comment or event without its key fields.
var ErrInvalidTeamTaskComment = errors.New("task id and body are required")

// TeamTaskComment is one human or agent note on a task.
type TeamTaskComment struct {
	ID        string `json:"id"`
	TenantID  string `json:"-"`
	TaskID    string `json:"task_id"`
	Author    string `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

// TeamTaskEvent is one audit row on a task.
type TeamTaskEvent struct {
	ID        string `json:"id"`
	TenantID  string `json:"-"`
	TaskID    string `json:"task_id"`
	Kind      string `json:"kind"`
	Detail    string `json:"detail,omitempty"`
	Actor     string `json:"actor,omitempty"`
	CreatedAt string `json:"created_at"`
}

// TeamTaskStore is the task comment and event capability.
type TeamTaskStore interface {
	ListTeamTaskComments(tenant, taskID string) []*TeamTaskComment
	PutTeamTaskComment(TeamTaskComment) (*TeamTaskComment, error)
	ListTeamTaskEvents(tenant, taskID string, limit int) []*TeamTaskEvent
	PutTeamTaskEvent(TeamTaskEvent) (*TeamTaskEvent, error)
}

// TeamTasks returns the task extras capability of st, or nil.
func TeamTasks(st StoreIface) TeamTaskStore {
	if x, ok := st.(TeamTaskStore); ok {
		return x
	}
	return nil
}

func (s *SQLiteStore) migrateTeamTaskExtras() error {
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS team_task_comments (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			task_id TEXT NOT NULL,
			author TEXT NOT NULL DEFAULT '',
			body TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS ix_team_task_comments_task ON team_task_comments(task_id)`,
		`CREATE TABLE IF NOT EXISTS team_task_events (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			task_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			detail TEXT NOT NULL DEFAULT '',
			actor TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS ix_team_task_events_task ON team_task_events(task_id)`,
	} {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) ListTeamTaskComments(tenant, taskID string) []*TeamTaskComment {
	rows, err := s.db.Query(`SELECT id, tenant_id, task_id, author, body, created_at
		FROM team_task_comments WHERE tenant_id=? AND task_id=? ORDER BY created_at`,
		NormalizeTenant(tenant), strings.TrimSpace(taskID))
	if err != nil {
		return []*TeamTaskComment{}
	}
	defer rows.Close()
	out := []*TeamTaskComment{}
	for rows.Next() {
		var c TeamTaskComment
		if rows.Scan(&c.ID, &c.TenantID, &c.TaskID, &c.Author, &c.Body, &c.CreatedAt) != nil {
			continue
		}
		out = append(out, &c)
	}
	return out
}

func (s *SQLiteStore) PutTeamTaskComment(c TeamTaskComment) (*TeamTaskComment, error) {
	c.TenantID = NormalizeTenant(c.TenantID)
	c.TaskID = strings.TrimSpace(c.TaskID)
	c.Body = strings.TrimSpace(c.Body)
	if c.TaskID == "" || c.Body == "" {
		return nil, ErrInvalidTeamTaskComment
	}
	if c.ID == "" {
		c.ID = "ttc_" + newID()
		c.CreatedAt = formatTime(time.Now().UTC())
	}
	_, err := s.db.Exec(`INSERT INTO team_task_comments(id, tenant_id, task_id, author, body, created_at)
		VALUES(?,?,?,?,?,?)`, c.ID, c.TenantID, c.TaskID, c.Author, c.Body, c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *SQLiteStore) ListTeamTaskEvents(tenant, taskID string, limit int) []*TeamTaskEvent {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT id, tenant_id, task_id, kind, detail, actor, created_at FROM team_task_events WHERE tenant_id=?`
	args := []any{NormalizeTenant(tenant)}
	if strings.TrimSpace(taskID) != "" {
		q += ` AND task_id=?`
		args = append(args, strings.TrimSpace(taskID))
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return []*TeamTaskEvent{}
	}
	defer rows.Close()
	out := []*TeamTaskEvent{}
	for rows.Next() {
		var e TeamTaskEvent
		if rows.Scan(&e.ID, &e.TenantID, &e.TaskID, &e.Kind, &e.Detail, &e.Actor, &e.CreatedAt) != nil {
			continue
		}
		out = append(out, &e)
	}
	return out
}

func (s *SQLiteStore) PutTeamTaskEvent(e TeamTaskEvent) (*TeamTaskEvent, error) {
	e.TenantID = NormalizeTenant(e.TenantID)
	e.TaskID = strings.TrimSpace(e.TaskID)
	e.Kind = strings.TrimSpace(e.Kind)
	if e.TaskID == "" || e.Kind == "" {
		return nil, ErrInvalidTeamTaskComment
	}
	if e.ID == "" {
		e.ID = "tte_" + newID()
		e.CreatedAt = formatTime(time.Now().UTC())
	}
	_, err := s.db.Exec(`INSERT INTO team_task_events(id, tenant_id, task_id, kind, detail, actor, created_at)
		VALUES(?,?,?,?,?,?,?)`, e.ID, e.TenantID, e.TaskID, e.Kind, e.Detail, e.Actor, e.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &e, nil
}
