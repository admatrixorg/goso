package store

import (
	"strings"
	"time"
)

// UsageEvent is one billable metering row.
type UsageEvent struct {
	ID             string `json:"id"`
	TenantID       string `json:"-"`
	EventTime      string `json:"event_time"`
	BucketHour     string `json:"bucket_hour"`
	EventType      string `json:"event_type"`
	ResourceType   string `json:"resource_type"`
	ResourceName   string `json:"resource_name"`
	ResourceID     string `json:"resource_id,omitempty"`
	Source         string `json:"source,omitempty"`
	AgentID        string `json:"agent_id,omitempty"`
	TeamID         string `json:"team_id,omitempty"`
	TraceID        string `json:"trace_id,omitempty"`
	SpanID         string `json:"span_id,omitempty"`
	RunID          string `json:"run_id,omitempty"`
	SessionKey     string `json:"session_key,omitempty"`
	Channel        string `json:"channel,omitempty"`
	Provider       string `json:"provider,omitempty"`
	Model          string `json:"model,omitempty"`
	Status         string `json:"status,omitempty"`
	InputTokens    int `json:"input_tokens"`
	OutputTokens   int `json:"output_tokens"`
	TotalTokens    int `json:"total_tokens"`
	CostUSD        float64 `json:"cost_usd"`
	DurationMS     int `json:"duration_ms"`
	CallCount      int `json:"call_count"`
	ErrorCount     int `json:"error_count"`
	Metadata       string `json:"metadata,omitempty"`
	CreatedAt      string `json:"created_at"`
}

// UsageCapPolicy is one spend or token ceiling.
type UsageCapPolicy struct {
	ID             string `json:"id"`
	TenantID       string `json:"-"`
	AgentID        string `json:"agent_id,omitempty"`
	ProviderID     string `json:"provider_id,omitempty"`
	ProviderType   string `json:"provider_type,omitempty"`
	ModelID        string `json:"model_id,omitempty"`
	Window         string `json:"window"`
	MaxTokens      int64 `json:"max_tokens"`
	MaxCostMicros  int64 `json:"max_cost_micros"`
	Enabled        bool `json:"enabled"`
	Priority       int `json:"priority"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// PricingCatalogEntry is one model price row.
type PricingCatalogEntry struct {
	ID              string `json:"id"`
	ModelID         string `json:"model_id"`
	CanonicalModelID string `json:"canonical_model_id,omitempty"`
	Pricing         string `json:"pricing"`
	SyncedAt        string `json:"synced_at"`
}

// UsageStore is the metering, cap and pricing capability.
type UsageStore interface {
	InsertUsageEvent(UsageEvent) error
	ListUsageEvents(tenant, agentID, provider, model, channel, from, to string, limit, offset int) ([]*UsageEvent, int)
	UsageTotals(tenant, from, to string) map[string]float64
	UsageBreakdown(tenant, groupBy, from, to string) []map[string]any
	UsageSeries(tenant, groupBy, from, to string) []map[string]any
	ListUsageCapPolicies(tenant string) []*UsageCapPolicy
	PutUsageCapPolicy(UsageCapPolicy) (*UsageCapPolicy, error)
	DeleteUsageCapPolicy(tenant, id string) error
	ListPricingCatalog() []*PricingCatalogEntry
}

// Usage returns the usage capability of st, or nil.
func Usage(st StoreIface) UsageStore {
	if x, ok := st.(UsageStore); ok {
		return x
	}
	return nil
}

func (s *SQLiteStore) migrateUsage() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS usage_events (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			event_time TEXT NOT NULL DEFAULT '',
			bucket_hour TEXT NOT NULL DEFAULT '',
			event_type TEXT NOT NULL DEFAULT '',
			resource_type TEXT NOT NULL DEFAULT '',
			resource_name TEXT NOT NULL DEFAULT '',
			resource_id TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT '',
			agent_id TEXT NOT NULL DEFAULT '',
			team_id TEXT NOT NULL DEFAULT '',
			trace_id TEXT NOT NULL DEFAULT '',
			span_id TEXT NOT NULL DEFAULT '',
			run_id TEXT NOT NULL DEFAULT '',
			session_key TEXT NOT NULL DEFAULT '',
			channel TEXT NOT NULL DEFAULT '',
			provider TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT '',
			input_tokens INTEGER NOT NULL DEFAULT 0,
			output_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			cost_usd REAL NOT NULL DEFAULT 0,
			duration_ms INTEGER NOT NULL DEFAULT 0,
			call_count INTEGER NOT NULL DEFAULT 1,
			error_count INTEGER NOT NULL DEFAULT 0,
			metadata TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_events_tenant ON usage_events(tenant_id, event_time)`,
		`CREATE TABLE IF NOT EXISTS usage_cap_policies (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			agent_id TEXT NOT NULL DEFAULT '',
			provider_id TEXT NOT NULL DEFAULT '',
			provider_type TEXT NOT NULL DEFAULT '',
			model_id TEXT NOT NULL DEFAULT '',
			window_key TEXT NOT NULL DEFAULT 'day',
			max_tokens INTEGER NOT NULL DEFAULT 0,
			max_cost_micros INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			priority INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS usage_cap_events (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			policy_id TEXT NOT NULL DEFAULT '',
			decision TEXT NOT NULL DEFAULT '',
			reason TEXT NOT NULL DEFAULT '',
			estimated_tokens INTEGER NOT NULL DEFAULT 0,
			estimated_cost_micros INTEGER NOT NULL DEFAULT 0,
			actual_tokens INTEGER NOT NULL DEFAULT 0,
			actual_cost_micros INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS usage_pricing_catalog (
			id TEXT PRIMARY KEY,
			model_id TEXT NOT NULL UNIQUE,
			canonical_model_id TEXT NOT NULL DEFAULT '',
			pricing TEXT NOT NULL DEFAULT '{}',
			synced_at TEXT NOT NULL DEFAULT ''
		)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

const usageEventColumns = `id, tenant_id, event_time, bucket_hour, event_type, resource_type, resource_name, resource_id, source, agent_id, team_id, trace_id, span_id, run_id, session_key, channel, provider, model, status, input_tokens, output_tokens, total_tokens, cost_usd, duration_ms, call_count, error_count, metadata, created_at`

func (s *SQLiteStore) InsertUsageEvent(e UsageEvent) error {
	e.TenantID = NormalizeTenant(e.TenantID)
	if e.ID == "" {
		e.ID = "ue_" + newID()
	}
	if e.EventTime == "" {
		e.EventTime = formatTime(time.Now().UTC())
	}
	if e.BucketHour == "" {
		e.BucketHour = e.EventTime
	}
	if e.CreatedAt == "" {
		e.CreatedAt = e.EventTime
	}
	if e.CallCount == 0 {
		e.CallCount = 1
	}
	_, err := s.db.Exec(`INSERT INTO usage_events(` + usageEventColumns + `) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.ID, e.TenantID, e.EventTime, e.BucketHour, e.EventType, e.ResourceType, e.ResourceName,
		e.ResourceID, e.Source, e.AgentID, e.TeamID, e.TraceID, e.SpanID, e.RunID, e.SessionKey,
		e.Channel, e.Provider, e.Model, e.Status, e.InputTokens, e.OutputTokens, e.TotalTokens,
		e.CostUSD, e.DurationMS, e.CallCount, e.ErrorCount, e.Metadata, e.CreatedAt)
	return err
}

func scanUsageEvent(sc scanner) (*UsageEvent, error) {
	var e UsageEvent
	err := sc.Scan(&e.ID, &e.TenantID, &e.EventTime, &e.BucketHour, &e.EventType, &e.ResourceType,
		&e.ResourceName, &e.ResourceID, &e.Source, &e.AgentID, &e.TeamID, &e.TraceID, &e.SpanID,
		&e.RunID, &e.SessionKey, &e.Channel, &e.Provider, &e.Model, &e.Status, &e.InputTokens,
		&e.OutputTokens, &e.TotalTokens, &e.CostUSD, &e.DurationMS, &e.CallCount, &e.ErrorCount,
		&e.Metadata, &e.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// usageWhere builds the shared tenant and window predicate.
func usageWhere(tenant, from, to string) (string, []any) {
	w := " WHERE tenant_id=?"
	args := []any{NormalizeTenant(tenant)}
	if strings.TrimSpace(from) != "" {
		w += " AND event_time >= ?"
		args = append(args, strings.TrimSpace(from))
	}
	if strings.TrimSpace(to) != "" {
		w += " AND event_time <= ?"
		args = append(args, strings.TrimSpace(to))
	}
	return w, args
}

func (s *SQLiteStore) ListUsageEvents(tenant, agentID, provider, model, channel, from, to string, limit, offset int) ([]*UsageEvent, int) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	w, args := usageWhere(tenant, from, to)
	for _, pair := range [][2]string{{"agent_id", agentID}, {"provider", provider}, {"model", model}, {"channel", channel}} {
		if strings.TrimSpace(pair[1]) != "" {
			w += " AND " + pair[0] + "=?"
			args = append(args, strings.TrimSpace(pair[1]))
		}
	}
	var total int
	_ = s.db.QueryRow(`SELECT count(*) FROM usage_events` + w, args...).Scan(&total)
	rows, err := s.db.Query(`SELECT ` + usageEventColumns + ` FROM usage_events` + w + ` ORDER BY event_time DESC LIMIT ? OFFSET ?`,
		append(args, limit, offset)...)
	if err != nil {
		return []*UsageEvent{}, total
	}
	defer rows.Close()
	out := []*UsageEvent{}
	for rows.Next() {
		if e, err := scanUsageEvent(rows); err == nil {
			out = append(out, e)
		}
	}
	return out, total
}

// UsageTotals returns the aggregate counters the summary card renders.
func (s *SQLiteStore) UsageTotals(tenant, from, to string) map[string]float64 {
	w, args := usageWhere(tenant, from, to)
	var calls, inTok, outTok, totTok, cost, errors, dur int64
	_ = s.db.QueryRow(`SELECT coalesce(sum(call_count),0), coalesce(sum(input_tokens),0), coalesce(sum(output_tokens),0), coalesce(sum(total_tokens),0), coalesce(sum(cost_usd),0), coalesce(sum(error_count),0), coalesce(sum(duration_ms),0) FROM usage_events` + w, args...).
		Scan(&calls, &inTok, &outTok, &totTok, &cost, &errors, &dur)
	out := map[string]float64{
		"requests": float64(calls), "input_tokens": float64(inTok), "output_tokens": float64(outTok),
		"total_tokens": float64(totTok), "cost": float64(cost), "errors": float64(errors),
		"avg_duration_ms": 0,
	}
	if calls > 0 {
		out["avg_duration_ms"] = float64(dur) / float64(calls)
	}
	return out
}

func usageGroupColumn(groupBy string) string {
	switch strings.ToLower(strings.TrimSpace(groupBy)) {
	case "model":
		return "model"
	case "channel":
		return "channel"
	case "agent":
		return "agent_id"
	case "hour":
		return "bucket_hour"
	default:
		return "provider"
	}
}

func (s *SQLiteStore) UsageBreakdown(tenant, groupBy, from, to string) []map[string]any {
	w, args := usageWhere(tenant, from, to)
	col := usageGroupColumn(groupBy)
	rows, err := s.db.Query(`SELECT ` + col + `, coalesce(sum(call_count),0), coalesce(sum(input_tokens),0), coalesce(sum(output_tokens),0), coalesce(sum(error_count),0), coalesce(sum(duration_ms),0), coalesce(sum(cost_usd),0) FROM usage_events` + w + ` GROUP BY ` + col + ` ORDER BY 2 DESC LIMIT 50`, args...)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var key string
		var calls, inTok, outTok, errors, dur int64
		var cost float64
		if rows.Scan(&key, &calls, &inTok, &outTok, &errors, &dur, &cost) != nil {
			continue
		}
		avg := 0.0
		if calls > 0 {
			avg = float64(dur) / float64(calls)
		}
		out = append(out, map[string]any{
			"key": key, "request_count": calls, "llm_call_count": calls, "input_tokens": inTok,
			"output_tokens": outTok, "error_count": errors, "avg_duration_ms": avg, "total_cost": cost,
		})
	}
	return out
}

func (s *SQLiteStore) UsageSeries(tenant, groupBy, from, to string) []map[string]any {
	w, args := usageWhere(tenant, from, to)
	rows, err := s.db.Query(`SELECT bucket_hour, coalesce(sum(call_count),0), coalesce(sum(input_tokens),0), coalesce(sum(output_tokens),0), coalesce(sum(error_count),0), coalesce(sum(duration_ms),0), coalesce(sum(cost_usd),0) FROM usage_events` + w + ` GROUP BY bucket_hour ORDER BY bucket_hour LIMIT 500`, args...)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var bucket string
		var calls, inTok, outTok, errors, dur int64
		var cost float64
		if rows.Scan(&bucket, &calls, &inTok, &outTok, &errors, &dur, &cost) != nil {
			continue
		}
		avg := 0.0
		if calls > 0 {
			avg = float64(dur) / float64(calls)
		}
		out = append(out, map[string]any{
			"bucket_time": bucket, "input_tokens": inTok, "output_tokens": outTok,
			"cache_read_tokens": 0, "thinking_tokens": 0, "request_count": calls,
			"llm_call_count": calls, "tool_call_count": 0, "error_count": errors,
			"avg_duration_ms": avg, "unique_users": 0, "memory_docs": 0, "memory_chunks": 0,
			"kg_entities": 0, "kg_relations": 0, "total_cost": cost,
		})
	}
	return out
}

const usageCapColumns = `id, tenant_id, agent_id, provider_id, provider_type, model_id, window_key, max_tokens, max_cost_micros, enabled, priority, created_at, updated_at`

func (s *SQLiteStore) ListUsageCapPolicies(tenant string) []*UsageCapPolicy {
	rows, err := s.db.Query(`SELECT ` + usageCapColumns + ` FROM usage_cap_policies WHERE tenant_id=? ORDER BY priority, created_at`, NormalizeTenant(tenant))
	if err != nil {
		return []*UsageCapPolicy{}
	}
	defer rows.Close()
	out := []*UsageCapPolicy{}
	for rows.Next() {
		var p UsageCapPolicy
		var enabled int
		if rows.Scan(&p.ID, &p.TenantID, &p.AgentID, &p.ProviderID, &p.ProviderType, &p.ModelID,
			&p.Window, &p.MaxTokens, &p.MaxCostMicros, &enabled, &p.Priority, &p.CreatedAt, &p.UpdatedAt) != nil {
			continue
		}
		p.Enabled = enabled != 0
		out = append(out, &p)
	}
	return out
}

func (s *SQLiteStore) PutUsageCapPolicy(p UsageCapPolicy) (*UsageCapPolicy, error) {
	p.TenantID = NormalizeTenant(p.TenantID)
	if strings.TrimSpace(p.Window) == "" {
		p.Window = "day"
	}
	now := formatTime(time.Now().UTC())
	if p.ID == "" {
		p.ID = "ucp_" + newID()
		p.CreatedAt = now
		p.UpdatedAt = now
		_, err := s.db.Exec(`INSERT INTO usage_cap_policies(id, tenant_id, agent_id, provider_id, provider_type, model_id, window_key, max_tokens, max_cost_micros, enabled, priority, created_at, updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			p.ID, p.TenantID, p.AgentID, p.ProviderID, p.ProviderType, p.ModelID, p.Window,
			p.MaxTokens, p.MaxCostMicros, boolInt(p.Enabled), p.Priority, p.CreatedAt, p.UpdatedAt)
		if err != nil {
			return nil, err
		}
		return &p, nil
	}
	p.UpdatedAt = now
	res, err := s.db.Exec(`UPDATE usage_cap_policies SET agent_id=?, provider_id=?, provider_type=?, model_id=?, window_key=?, max_tokens=?, max_cost_micros=?, enabled=?, priority=?, updated_at=? WHERE id=? AND tenant_id=?`,
		p.AgentID, p.ProviderID, p.ProviderType, p.ModelID, p.Window, p.MaxTokens,
		p.MaxCostMicros, boolInt(p.Enabled), p.Priority, p.UpdatedAt, p.ID, p.TenantID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return &p, nil
}

func (s *SQLiteStore) DeleteUsageCapPolicy(tenant, id string) error {
	res, err := s.db.Exec(`DELETE FROM usage_cap_policies WHERE id=? AND tenant_id=?`, strings.TrimSpace(id), NormalizeTenant(tenant))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) ListPricingCatalog() []*PricingCatalogEntry {
	rows, err := s.db.Query(`SELECT id, model_id, canonical_model_id, pricing, synced_at FROM usage_pricing_catalog ORDER BY model_id`)
	if err != nil {
		return []*PricingCatalogEntry{}
	}
	defer rows.Close()
	out := []*PricingCatalogEntry{}
	for rows.Next() {
		var e PricingCatalogEntry
		if rows.Scan(&e.ID, &e.ModelID, &e.CanonicalModelID, &e.Pricing, &e.SyncedAt) != nil {
			continue
		}
		out = append(out, &e)
	}
	return out
}

// UsageCapUtilization reports spend per enabled policy over its window.
func (s *SQLiteStore) UsageCapUtilization(tenant string, now time.Time) []map[string]any {
	out := []map[string]any{}
	for _, p := range s.ListUsageCapPolicies(tenant) {
		if p == nil || !p.Enabled {
			continue
		}
		from := now.UTC().Add(-24 * time.Hour)
		switch p.Window {
		case "hour":
			from = now.UTC().Add(-time.Hour)
		case "week":
			from = now.UTC().Add(-7 * 24 * time.Hour)
		case "month":
			from = now.UTC().Add(-30 * 24 * time.Hour)
		}
		totals := s.UsageTotals(tenant, formatTime(from), formatTime(now.UTC()))
		out = append(out, map[string]any{
			"policy": p, "window_start": formatTime(from), "window_end": formatTime(now.UTC()),
			"used_tokens": int64(totals["total_tokens"]), "reserved_tokens": 0,
			"used_cost_micros": int64(totals["cost"] * 1e6), "reserved_cost_micros": 0,
		})
	}
	return out
}
