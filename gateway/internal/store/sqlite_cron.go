// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

func (s *SQLiteStore) CreateCronJob(j CronJob) (*CronJob, error) {
	j.Spec = strings.TrimSpace(j.Spec)
	j.SessionID = strings.TrimSpace(j.SessionID)
	j.Message = strings.TrimSpace(j.Message)
	if j.Spec == "" {
		return nil, errors.New("spec is required")
	}
	if j.SessionID == "" {
		return nil, errors.New("session_id is required")
	}
	if j.Message == "" {
		return nil, errors.New("message is required")
	}
	var sessTenant string
	err := s.db.QueryRow(`SELECT tenant_id FROM sessions WHERE id=?`, j.SessionID).Scan(&sessTenant)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("session not found")
	}
	if err != nil {
		return nil, err
	}
	// Session tenant wins. A caller-supplied TenantID is not stored.
	j.TenantID = NormalizeTenant(sessTenant)
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM cron_jobs WHERE tenant_id=?`, j.TenantID).Scan(&n); err != nil {
		return nil, err
	}
	if n >= CronJobCap {
		return nil, ErrCronCap
	}
	j.ID = newID()
	j.LastRun = nil
	j.LastError = ""
	enabled := 0
	if j.Enabled {
		enabled = 1
	}
	_, err = s.db.Exec(
		`INSERT INTO cron_jobs(id, spec, session_id, message, enabled, last_run, last_error, tenant_id) VALUES(?,?,?,?,?,?,?,?)`,
		j.ID, j.Spec, j.SessionID, j.Message, enabled, "", "", j.TenantID,
	)
	if err != nil {
		return nil, err
	}
	cp := j
	return cloneCronJob(&cp), nil
}

const cronJobSelect = `id, spec, session_id, message, enabled, last_run, last_error, tenant_id`

func (s *SQLiteStore) ListCronJobs() []*CronJob {
	return s.listCronJobsQuery(`SELECT ` + cronJobSelect + ` FROM cron_jobs ORDER BY id`)
}

func (s *SQLiteStore) ListCronJobsTenant(tid string) []*CronJob {
	return s.listCronJobsQuery(
		`SELECT `+cronJobSelect+` FROM cron_jobs WHERE tenant_id=? ORDER BY id`,
		NormalizeTenant(tid),
	)
}

func (s *SQLiteStore) listCronJobsQuery(q string, args ...any) []*CronJob {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return []*CronJob{}
	}
	defer rows.Close()
	out := make([]*CronJob, 0)
	for rows.Next() {
		j, err := scanCronJob(rows)
		if err != nil {
			continue
		}
		out = append(out, j)
	}
	return out
}

func (s *SQLiteStore) GetCronJob(id string) (*CronJob, error) {
	row := s.db.QueryRow(`SELECT `+cronJobSelect+` FROM cron_jobs WHERE id=?`, id)
	j, err := scanCronJob(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return j, nil
}

func (s *SQLiteStore) DeleteCronJob(id string) error {
	res, err := s.db.Exec(`DELETE FROM cron_jobs WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) MarkCronRun(id string, at time.Time) error {
	res, err := s.db.Exec(`UPDATE cron_jobs SET last_run=?, last_error='' WHERE id=?`, formatTime(at), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) SetCronEnabled(id string, enabled bool) error {
	en := 0
	if enabled {
		en = 1
	}
	res, err := s.db.Exec(`UPDATE cron_jobs SET enabled=? WHERE id=?`, en, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) MarkCronError(id, msg string) error {
	res, err := s.db.Exec(`UPDATE cron_jobs SET last_error=? WHERE id=?`, strings.TrimSpace(msg), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanCronJob(sc scanner) (*CronJob, error) {
	var j CronJob
	var enabled int
	var last sql.NullString
	var tenant string
	if err := sc.Scan(&j.ID, &j.Spec, &j.SessionID, &j.Message, &enabled, &last, &j.LastError, &tenant); err != nil {
		return nil, err
	}
	j.TenantID = NormalizeTenant(tenant)
	j.Enabled = enabled != 0
	if last.Valid && strings.TrimSpace(last.String) != "" {
		t := parseTime(last.String)
		if !t.IsZero() {
			j.LastRun = &t
		}
	}
	return &j, nil
}
