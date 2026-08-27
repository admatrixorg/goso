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
	if _, ok := s.sessions[j.SessionID]; !ok {
		return nil, errors.New("session not found")
	}
	if len(s.cronJobs) >= CronJobCap {
		return nil, ErrCronCap
	}
	j.ID = s.nextID()
	j.LastRun = nil
	cp := j
	s.cronJobs[cp.ID] = &cp
	return cloneCronJob(&cp), nil
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
	return nil
}
