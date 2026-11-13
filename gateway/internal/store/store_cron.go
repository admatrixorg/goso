// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
	"sort"
	"strings"
	"time"
)

func cloneCronJob(j *CronJob) *CronJob {
	if j == nil {
		return nil
	}
	cp := *j
	if j.LastRun != nil {
		t := j.LastRun.UTC()
		cp.LastRun = &t
	}
	return &cp
}

func (s *Store) CreateCronJob(j CronJob) (*CronJob, error) {
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
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[j.SessionID]
	if !ok || sess == nil {
		return nil, errors.New("session not found")
	}
	// Session tenant wins. A caller-supplied TenantID is not stored.
	j.TenantID = NormalizeTenant(sess.TenantID)
	if s.cronCountTenantLocked(j.TenantID) >= CronJobCap {
		return nil, ErrCronCap
	}
	j.ID = s.nextID()
	j.LastRun = nil
	j.LastError = ""
	cp := j
	s.cronJobs[cp.ID] = &cp
	return cloneCronJob(&cp), nil
}

func (s *Store) cronCountTenantLocked(tid string) int {
	n := 0
	for _, j := range s.cronJobs {
		if j != nil && SameTenant(j.TenantID, tid) {
			n++
		}
	}
	return n
}

func (s *Store) deleteSessionCronLocked(sessionID string) {
	for id, j := range s.cronJobs {
		if j != nil && j.SessionID == sessionID {
			delete(s.cronJobs, id)
		}
	}
}

func (s *Store) ListCronJobs() []*CronJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*CronJob, 0, len(s.cronJobs))
	for _, v := range s.cronJobs {
		out = append(out, cloneCronJob(v))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ListCronJobsTenant is the HTTP listing. ListCronJobs stays global for cron.Tick.
func (s *Store) ListCronJobsTenant(tid string) []*CronJob {
	all := s.ListCronJobs()
	out := make([]*CronJob, 0)
	for _, j := range all {
		if j != nil && SameTenant(j.TenantID, tid) {
			out = append(out, j)
		}
	}
	return out
}

func (s *Store) GetCronJob(id string) (*CronJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.cronJobs[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneCronJob(v), nil
}

func (s *Store) DeleteCronJob(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.cronJobs[id]; !ok {
		return ErrNotFound
	}
	delete(s.cronJobs, id)
	return nil
}

func (s *Store) MarkCronRun(id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.cronJobs[id]
	if !ok {
		return ErrNotFound
	}
	t := at.UTC()
	v.LastRun = &t
	v.LastError = ""
	return nil
}

func (s *Store) SetCronEnabled(id string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.cronJobs[id]
	if !ok {
		return ErrNotFound
	}
	v.Enabled = enabled
	return nil
}

func (s *Store) MarkCronError(id, msg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.cronJobs[id]
	if !ok {
		return ErrNotFound
	}
	v.LastError = strings.TrimSpace(msg)
	return nil
}
