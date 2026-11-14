// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package scheduleimport

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

const maxExportBytes = 64 << 20

// Export is an offline dump. Column names match the GoClaw tables cron_jobs
// and channel_instances. credentials is accepted only so the reader can see
// that a secret was present; callers must not print the raw value.
type Export struct {
	CronJobs         []CronRow    `json:"cron_jobs"`
	ChannelInstances []ChannelRow `json:"channel_instances"`
	// otherTables are lines whose table is neither cron_jobs nor
	// channel_instances. They are not written.
	otherTables []otherTable
}

// CronRow is one cron_jobs object. Fields that goso does not store stay on
// the struct so the decoder keeps them out of the leftover map, and the
// importer lists them as not imported.
type CronRow struct {
	Table          string          `json:"table,omitempty"`
	ID             string          `json:"id"`
	AgentID        string          `json:"agent_id"`
	UserID         string          `json:"user_id"`
	Name           string          `json:"name"`
	Enabled        *bool           `json:"enabled"`
	ScheduleKind   string          `json:"schedule_kind"`
	CronExpression string          `json:"cron_expression"`
	IntervalMS     *int64          `json:"interval_ms"`
	RunAt          string          `json:"run_at"`
	Timezone       string          `json:"timezone"`
	Payload        json.RawMessage `json:"payload"`
	DeleteAfterRun *bool           `json:"delete_after_run"`
	NextRunAt      string          `json:"next_run_at"`
	LastRunAt      string          `json:"last_run_at"`
	LastStatus     string          `json:"last_status"`
	LastError      string          `json:"last_error"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
	TeamID         string          `json:"team_id"`
	TenantID       string          `json:"tenant_id"`
	Stateless      *bool           `json:"stateless"`
	Deliver        *bool           `json:"deliver"`
	DeliverChannel string          `json:"deliver_channel"`
	DeliverTo      string          `json:"deliver_to"`
	WakeHeartbeat  *bool           `json:"wake_heartbeat"`
}

// ChannelRow is one channel_instances object. Credentials is raw JSON and
// must not be logged or copied into goso.
type ChannelRow struct {
	Table       string          `json:"table,omitempty"`
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	DisplayName string          `json:"display_name"`
	ChannelType string          `json:"channel_type"`
	AgentID     string          `json:"agent_id"`
	Credentials json.RawMessage `json:"credentials"`
	Config      json.RawMessage `json:"config"`
	Enabled     *bool           `json:"enabled"`
	CreatedBy   string          `json:"created_by"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
	TenantID    string          `json:"tenant_id"`
}

// ReadExport reads one JSON document or JSONL file. It does not open a database.
func ReadExport(r io.Reader) (*Export, error) {
	if r == nil {
		return nil, errConfig("export is required")
	}
	b, err := io.ReadAll(io.LimitReader(r, maxExportBytes+1))
	if err != nil {
		return nil, errConfig("read export failed")
	}
	if len(b) > maxExportBytes {
		return nil, errConfig("export is too large")
	}
	return parseExport(b)
}

func parseExport(b []byte) (*Export, error) {
	trim := bytes.TrimSpace(b)
	if len(trim) == 0 {
		return &Export{}, nil
	}
	if trim[0] == '{' && isEnvelope(trim) {
		var ex Export
		if err := json.Unmarshal(trim, &ex); err != nil {
			return nil, errConfig("export is not json")
		}
		return &ex, nil
	}
	if trim[0] == '[' {
		var rows []json.RawMessage
		if err := json.Unmarshal(trim, &rows); err != nil {
			return nil, errConfig("export is not json")
		}
		ex := &Export{}
		for _, raw := range rows {
			if err := appendRaw(ex, raw); err != nil {
				return nil, err
			}
		}
		return ex, nil
	}
	return parseJSONL(trim)
}

func isEnvelope(b []byte) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return false
	}
	_, cron := m["cron_jobs"]
	_, ch := m["channel_instances"]
	return cron || ch
}

func parseJSONL(b []byte) (*Export, error) {
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 64*1024), 4<<20)
	ex := &Export{}
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		if err := appendRaw(ex, line); err != nil {
			return nil, err
		}
	}
	if err := sc.Err(); err != nil {
		return nil, errConfig("export is not json")
	}
	return ex, nil
}

func appendRaw(ex *Export, raw []byte) error {
	var probe struct {
		Table        string `json:"table"`
		ScheduleKind string `json:"schedule_kind"`
		ChannelType  string `json:"channel_type"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return errConfig("export is not json")
	}
	table := strings.ToLower(strings.TrimSpace(probe.Table))
	switch {
	case table == "cron_jobs" || (table == "" && probe.ScheduleKind != "" && probe.ChannelType == ""):
		var row CronRow
		if err := json.Unmarshal(raw, &row); err != nil {
			return errConfig("export is not json")
		}
		ex.CronJobs = append(ex.CronJobs, row)
	case table == "channel_instances" || (table == "" && probe.ChannelType != ""):
		var row ChannelRow
		if err := json.Unmarshal(raw, &row); err != nil {
			return errConfig("export is not json")
		}
		ex.ChannelInstances = append(ex.ChannelInstances, row)
	case table == "":
		return errConfig("export row has no table")
	default:
		var idProbe struct {
			ID       string `json:"id"`
			TenantID string `json:"tenant_id"`
		}
		_ = json.Unmarshal(raw, &idProbe)
		ex.otherTables = append(ex.otherTables, otherTable{
			Table:    probe.Table,
			ID:       idProbe.ID,
			TenantID: idProbe.TenantID,
		})
	}
	return nil
}

type otherTable struct {
	Table    string
	ID       string
	TenantID string
}
