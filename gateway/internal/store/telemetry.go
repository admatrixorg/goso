package store

import (
	"errors"
	"strings"
	"time"
)

// ActivityRow is one persisted audit row in the GoClaw shape.
type ActivityRow struct {
	ID         string `json:"id"`
	TenantID   string `json:"-"`
	ActorType  string `json:"actor_type"`
	ActorID    string `json:"actor_id"`
	Action     string `json:"action"`
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id,omitempty"`
	Details    string `json:"details,omitempty"`
	IPAddress  string `json:"ip_address,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// TraceRow is one persisted run trace.
type TraceRow struct {
	ID                string  `json:"id"`
	TenantID          string  `json:"-"`
	AgentID           string  `json:"agent_id,omitempty"`
	UserID            string  `json:"user_id,omitempty"`
	SessionKey        string  `json:"session_key,omitempty"`
	RunID             string  `json:"run_id,omitempty"`
	StartTime         string  `json:"start_time"`
	EndTime           string  `json:"end_time,omitempty"`
	DurationMS        int     `json:"duration_ms"`
	Name              string  `json:"name,omitempty"`
	Channel           string  `json:"channel,omitempty"`
	InputPreview      string  `json:"input_preview,omitempty"`
	OutputPreview     string  `json:"output_preview,omitempty"`
	TotalInputTokens  int     `json:"total_input_tokens"`
	TotalOutputTokens int     `json:"total_output_tokens"`
	TotalCost         float64 `json:"total_cost"`
	SpanCount         int     `json:"span_count"`
	LLMCallCount      int     `json:"llm_call_count"`
	ToolCallCount     int     `json:"tool_call_count"`
	Status            string  `json:"status"`
	Error             string  `json:"error,omitempty"`
	TeamID            string  `json:"team_id,omitempty"`
	ParentTraceID     string  `json:"parent_trace_id,omitempty"`
	CreatedAt         string  `json:"created_at"`
}

// SpanRow is one persisted span inside a trace.
type SpanRow struct {
	ID            string  `json:"id"`
	TenantID      string  `json:"-"`
	TraceID       string  `json:"trace_id"`
	ParentSpanID  string  `json:"parent_span_id,omitempty"`
	AgentID       string  `json:"agent_id,omitempty"`
	SpanType      string  `json:"span_type"`
	Name          string  `json:"name,omitempty"`
	StartTime     string  `json:"start_time"`
	EndTime       string  `json:"end_time,omitempty"`
	DurationMS    int     `json:"duration_ms"`
	Status        string  `json:"status"`
	Error         string  `json:"error,omitempty"`
	Model         string  `json:"model,omitempty"`
	Provider      string  `json:"provider,omitempty"`
	InputTokens   int     `json:"input_tokens"`
	OutputTokens  int     `json:"output_tokens"`
	TotalCost     float64 `json:"total_cost"`
	FinishReason  string  `json:"finish_reason,omitempty"`
	ToolName      string  `json:"tool_name,omitempty"`
	ToolCallID    string  `json:"tool_call_id,omitempty"`
	InputPreview  string  `json:"input_preview,omitempty"`
	OutputPreview string  `json:"output_preview,omitempty"`
	Level         string  `json:"level,omitempty"`
}

// TimelineRow is one run timeline entry.
type TimelineRow struct {
	ID        string `json:"id"`
	TenantID  string `json:"-"`
	RunID     string `json:"run_id"`
	Seq       int    `json:"seq"`
	Kind      string `json:"kind"`
	Detail    string `json:"detail,omitempty"`
	CreatedAt string `json:"created_at"`
}

// TelemetryStore is the trace, span, activity and timeline capability.
type TelemetryStore interface {
	InsertActivity(ActivityRow) error
	ListActivity(tenant, action, entityType, actorID string, limit, offset int) ([]*ActivityRow, int)
	InsertTrace(TraceRow) error
	ListTraces(tenant, agentID, status string, limit, offset int) ([]*TraceRow, int)
	GetTrace(tenant, id string) (*TraceRow, error)
	ListSpans(tenant, traceID string) []*SpanRow
	InsertRunTimeline(TimelineRow) error
	ListRunTimeline(tenant, runID string, limit int) []*TimelineRow
}

// Telemetry returns the telemetry capability of st, or nil.
func Telemetry(st StoreIface) TelemetryStore {
	if x, ok := st.(TelemetryStore); ok {
		return x
	}
	return nil
}

func (s *SQLiteStore) migrateTelemetry() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS activity_logs (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			actor_type TEXT NOT NULL DEFAULT '',
			actor_id TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL DEFAULT '',
			entity_type TEXT NOT NULL DEFAULT '',
			entity_id TEXT NOT NULL DEFAULT '',
			details TEXT NOT NULL DEFAULT '',
			ip_address TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_activity_logs_tenant ON activity_logs(tenant_id, created_at)`,
		`CREATE TABLE IF NOT EXISTS traces (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			agent_id TEXT NOT NULL DEFAULT '',
			user_id TEXT NOT NULL DEFAULT '',
			session_key TEXT NOT NULL DEFAULT '',
			run_id TEXT NOT NULL DEFAULT '',
			start_time TEXT NOT NULL DEFAULT '',
			end_time TEXT NOT NULL DEFAULT '',
			duration_ms INTEGER NOT NULL DEFAULT 0,
			name TEXT NOT NULL DEFAULT '',
			channel TEXT NOT NULL DEFAULT '',
			input_preview TEXT NOT NULL DEFAULT '',
			output_preview TEXT NOT NULL DEFAULT '',
			total_input_tokens INTEGER NOT NULL DEFAULT 0,
			total_output_tokens INTEGER NOT NULL DEFAULT 0,
			total_cost REAL NOT NULL DEFAULT 0,
			span_count INTEGER NOT NULL DEFAULT 0,
			llm_call_count INTEGER NOT NULL DEFAULT 0,
			tool_call_count INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'running',
			error TEXT NOT NULL DEFAULT '',
			team_id TEXT NOT NULL DEFAULT '',
			parent_trace_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_traces_tenant ON traces(tenant_id, start_time)`,
		`CREATE TABLE IF NOT EXISTS spans (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			trace_id TEXT NOT NULL,
			parent_span_id TEXT NOT NULL DEFAULT '',
			agent_id TEXT NOT NULL DEFAULT '',
			span_type TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL DEFAULT '',
			start_time TEXT NOT NULL DEFAULT '',
			end_time TEXT NOT NULL DEFAULT '',
			duration_ms INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'running',
			error TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			provider TEXT NOT NULL DEFAULT '',
			input_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			total_cost REAL NOT NULL DEFAULT 0,
			finish_reason TEXT NOT NULL DEFAULT '',
			tool_name TEXT NOT NULL DEFAULT '',
			tool_call_id TEXT NOT NULL DEFAULT '',
			input_preview TEXT NOT NULL DEFAULT '',
			output_preview TEXT NOT NULL DEFAULT '',
			level TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_spans_trace ON spans(trace_id)`,
		`CREATE TABLE IF NOT EXISTS run_timeline_items (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			run_id TEXT NOT NULL,
			seq INTEGER NOT NULL DEFAULT 0,
			kind TEXT NOT NULL DEFAULT '',
			detail TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_run_timeline_run ON run_timeline_items(run_id, seq)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) InsertActivity(a ActivityRow) error {
	a.TenantID = NormalizeTenant(a.TenantID)
	if a.ID == "" {
		a.ID = "act_" + newID()
	}
	if a.CreatedAt == "" {
		a.CreatedAt = formatTime(time.Now().UTC())
	}
	_, err := s.db.Exec(`INSERT INTO activity_logs(id, tenant_id, actor_type, actor_id, action, entity_type, entity_id, details, ip_address, created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.TenantID, a.ActorType, a.ActorID, a.Action, a.EntityType, a.EntityID, a.Details, a.IPAddress, a.CreatedAt)
	return err
}

func activityWhere(tenant, action, entityType, actorID string) (string, []any) {
	w := " WHERE tenant_id=?"
	args := []any{NormalizeTenant(tenant)}
	if strings.TrimSpace(action) != "" {
		w += " AND action=?"
		args = append(args, strings.TrimSpace(action))
	}
	if strings.TrimSpace(entityType) != "" {
		w += " AND entity_type=?"
		args = append(args, strings.TrimSpace(entityType))
	}
	if strings.TrimSpace(actorID) != "" {
		w += " AND actor_id=?"
		args = append(args, strings.TrimSpace(actorID))
	}
	return w, args
}

func (s *SQLiteStore) ListActivity(tenant, action, entityType, actorID string, limit, offset int) ([]*ActivityRow, int) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	where, args := activityWhere(tenant, action, entityType, actorID)
	var total int
	_ = s.db.QueryRow(`SELECT count(*) FROM activity_logs`+where, args...).Scan(&total)
	rows, err := s.db.Query(`SELECT id, tenant_id, actor_type, actor_id, action, entity_type, entity_id, details, ip_address, created_at FROM activity_logs`+where+` ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		append(args, limit, offset)...)
	if err != nil {
		return []*ActivityRow{}, total
	}
	defer rows.Close()
	out := []*ActivityRow{}
	for rows.Next() {
		var a ActivityRow
		if rows.Scan(&a.ID, &a.TenantID, &a.ActorType, &a.ActorID, &a.Action, &a.EntityType,
			&a.EntityID, &a.Details, &a.IPAddress, &a.CreatedAt) != nil {
			continue
		}
		out = append(out, &a)
	}
	return out, total
}
func (s *SQLiteStore) InsertTrace(t TraceRow) error {
	t.TenantID = NormalizeTenant(t.TenantID)
	if t.ID == "" {
		t.ID = "tr_" + newID()
	}
	if t.StartTime == "" {
		t.StartTime = formatTime(time.Now().UTC())
	}
	if t.CreatedAt == "" {
		t.CreatedAt = t.StartTime
	}
	if t.Status == "" {
		t.Status = "completed"
	}
	_, err := s.db.Exec(`INSERT INTO traces(id, tenant_id, agent_id, user_id, session_key, run_id, start_time, end_time, duration_ms, name, channel, input_preview, output_preview, total_input_tokens, total_output_tokens, total_cost, span_count, llm_call_count, tool_call_count, status, error, team_id, parent_trace_id, created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.TenantID, t.AgentID, t.UserID, t.SessionKey, t.RunID, t.StartTime, t.EndTime,
		t.DurationMS, t.Name, t.Channel, t.InputPreview, t.OutputPreview, t.TotalInputTokens,
		t.TotalOutputTokens, t.TotalCost, t.SpanCount, t.LLMCallCount, t.ToolCallCount,
		t.Status, t.Error, t.TeamID, t.ParentTraceID, t.CreatedAt)
	return err
}

const traceColumns = `id, tenant_id, agent_id, user_id, session_key, run_id, start_time, end_time, duration_ms, name, channel, input_preview, output_preview, total_input_tokens, total_output_tokens, total_cost, span_count, llm_call_count, tool_call_count, status, error, team_id, parent_trace_id, created_at`

func scanTraceRow(sc scanner) (*TraceRow, error) {
	var t TraceRow
	err := sc.Scan(&t.ID, &t.TenantID, &t.AgentID, &t.UserID, &t.SessionKey, &t.RunID,
		&t.StartTime, &t.EndTime, &t.DurationMS, &t.Name, &t.Channel, &t.InputPreview,
		&t.OutputPreview, &t.TotalInputTokens, &t.TotalOutputTokens, &t.TotalCost,
		&t.SpanCount, &t.LLMCallCount, &t.ToolCallCount, &t.Status, &t.Error, &t.TeamID,
		&t.ParentTraceID, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *SQLiteStore) ListTraces(tenant, agentID, status string, limit, offset int) ([]*TraceRow, int) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	w := " WHERE tenant_id=?"
	args := []any{NormalizeTenant(tenant)}
	if strings.TrimSpace(agentID) != "" {
		w += " AND agent_id=?"
		args = append(args, strings.TrimSpace(agentID))
	}
	if strings.TrimSpace(status) != "" {
		w += " AND status=?"
		args = append(args, strings.TrimSpace(status))
	}
	var total int
	_ = s.db.QueryRow(`SELECT count(*) FROM traces`+w, args...).Scan(&total)
	rows, err := s.db.Query(`SELECT `+traceColumns+` FROM traces`+w+` ORDER BY start_time DESC LIMIT ? OFFSET ?`,
		append(args, limit, offset)...)
	if err != nil {
		return []*TraceRow{}, total
	}
	defer rows.Close()
	out := []*TraceRow{}
	for rows.Next() {
		if t, err := scanTraceRow(rows); err == nil {
			out = append(out, t)
		}
	}
	return out, total
}

func (s *SQLiteStore) GetTrace(tenant, id string) (*TraceRow, error) {
	row := s.db.QueryRow(`SELECT `+traceColumns+` FROM traces WHERE id=? AND tenant_id=?`,
		strings.TrimSpace(id), NormalizeTenant(tenant))
	t, err := scanTraceRow(row)
	if err != nil {
		return nil, ErrNotFound
	}
	return t, nil
}

const spanColumns = `id, tenant_id, trace_id, parent_span_id, agent_id, span_type, name, start_time, end_time, duration_ms, status, error, model, provider, input_tokens, output_tokens, total_cost, finish_reason, tool_name, tool_call_id, input_preview, output_preview, level`

func (s *SQLiteStore) InsertSpan(sp SpanRow) error {
	sp.TenantID = NormalizeTenant(sp.TenantID)
	if sp.ID == "" {
		sp.ID = "sp_" + newID()
	}
	_, err := s.db.Exec(`INSERT INTO spans(`+spanColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		sp.ID, sp.TenantID, sp.TraceID, sp.ParentSpanID, sp.AgentID, sp.SpanType, sp.Name,
		sp.StartTime, sp.EndTime, sp.DurationMS, sp.Status, sp.Error, sp.Model, sp.Provider,
		sp.InputTokens, sp.OutputTokens, sp.TotalCost, sp.FinishReason, sp.ToolName,
		sp.ToolCallID, sp.InputPreview, sp.OutputPreview, sp.Level)
	return err
}

func (s *SQLiteStore) ListSpans(tenant, traceID string) []*SpanRow {
	rows, err := s.db.Query(`SELECT `+spanColumns+` FROM spans WHERE trace_id=? ORDER BY start_time`, strings.TrimSpace(traceID))
	if err != nil {
		return []*SpanRow{}
	}
	defer rows.Close()
	out := []*SpanRow{}
	for rows.Next() {
		var sp SpanRow
		if rows.Scan(&sp.ID, &sp.TenantID, &sp.TraceID, &sp.ParentSpanID, &sp.AgentID, &sp.SpanType,
			&sp.Name, &sp.StartTime, &sp.EndTime, &sp.DurationMS, &sp.Status, &sp.Error, &sp.Model,
			&sp.Provider, &sp.InputTokens, &sp.OutputTokens, &sp.TotalCost, &sp.FinishReason,
			&sp.ToolName, &sp.ToolCallID, &sp.InputPreview, &sp.OutputPreview, &sp.Level) != nil {
			continue
		}
		out = append(out, &sp)
	}
	return out
}

func (s *SQLiteStore) InsertRunTimeline(t TimelineRow) error {
	t.TenantID = NormalizeTenant(t.TenantID)
	if t.ID == "" {
		t.ID = "tl_" + newID()
	}
	if t.CreatedAt == "" {
		t.CreatedAt = formatTime(time.Now().UTC())
	}
	_, err := s.db.Exec(`INSERT INTO run_timeline_items(id, tenant_id, run_id, seq, kind, detail, created_at) VALUES(?,?,?,?,?,?,?)`,
		t.ID, t.TenantID, t.RunID, t.Seq, t.Kind, t.Detail, t.CreatedAt)
	return err
}

func (s *SQLiteStore) ListRunTimeline(tenant, runID string, limit int) []*TimelineRow {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.db.Query(`SELECT id, tenant_id, run_id, seq, kind, detail, created_at FROM run_timeline_items WHERE tenant_id=? AND run_id=? ORDER BY seq LIMIT ?`,
		NormalizeTenant(tenant), strings.TrimSpace(runID), limit)
	if err != nil {
		return []*TimelineRow{}
	}
	defer rows.Close()
	out := []*TimelineRow{}
	for rows.Next() {
		var t TimelineRow
		if rows.Scan(&t.ID, &t.TenantID, &t.RunID, &t.Seq, &t.Kind, &t.Detail, &t.CreatedAt) != nil {
			continue
		}
		out = append(out, &t)
	}
	return out
}

var _ = errors.New
