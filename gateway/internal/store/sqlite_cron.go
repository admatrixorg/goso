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
	var nSess int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id=?`, j.SessionID).Scan(&nSess); err != nil {
		return nil, err
	}
	if nSess == 0 {
		return nil, errors.New("session not found")
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM cron_jobs`).Scan(&n); err != nil {
		return nil, err
	}
	if n >= CronJobCap {
		return nil, ErrCronCap
	}
	j.ID = newID()
	j.LastRun = nil
	enabled := 0
	if j.Enabled {
		enabled = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO cron_jobs(id, spec, session_id, message, enabled, last_run) VALUES(?,?,?,?,?,?)`,
		j.ID, j.Spec, j.SessionID, j.Message, enabled, "",
	)
	if err != nil {
		return nil, err
	}
	cp := j
	return cloneCronJob(&cp), nil
}

func (s *SQLiteStore) ListCronJobs() []*CronJob {
	rows, err := s.db.Query(`SELECT id, spec, session_id, message, enabled, last_run FROM cron_jobs ORDER BY id`)
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
	row := s.db.QueryRow(`SELECT id, spec, session_id, message, enabled, last_run FROM cron_jobs WHERE id=?`, id)
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
	res, err := s.db.Exec(`UPDATE cron_jobs SET last_run=? WHERE id=?`, formatTime(at), id)
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
	var last string
	if err := sc.Scan(&j.ID, &j.Spec, &j.SessionID, &j.Message, &enabled, &last); err != nil {
		return nil, err
	}
	j.Enabled = enabled != 0
	if strings.TrimSpace(last) != "" {
		t := parseTime(last)
		if !t.IsZero() {
			j.LastRun = &t
		}
	}
	return &j, nil
}
