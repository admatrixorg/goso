// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
	"sort"
	"strings"
	"time"
)

func cloneWebhook(w *Webhook) *Webhook {
	if w == nil {
		return nil
	}
	cp := *w
	return &cp
}

func cloneWebhookJob(j *WebhookJob) *WebhookJob {
	if j == nil {
		return nil
	}
	cp := *j
	return &cp
}

func (s *Store) CreateWebhook(w Webhook) (*Webhook, error) {
	w.ID = strings.TrimSpace(w.ID)
	w.Kind = strings.TrimSpace(w.Kind)
	if w.Kind == "" {
		w.Kind = "llm"
	}
	w.TokenPrefix = strings.TrimSpace(w.TokenPrefix)
	w.TokenHash = strings.TrimSpace(w.TokenHash)
	if w.TokenHash == "" {
		return nil, errors.New("token_hash is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if w.ID == "" {
		w.ID = s.nextID()
	}
	if _, ok := s.webhooks[w.ID]; ok {
		return nil, ErrExists
	}
	if w.CreatedAt.IsZero() {
		w.CreatedAt = time.Now().UTC()
	} else {
		w.CreatedAt = w.CreatedAt.UTC()
	}
	cp := w
	s.webhooks[cp.ID] = &cp
	return cloneWebhook(&cp), nil
}

func (s *Store) ListWebhooks() []*Webhook {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Webhook, 0, len(s.webhooks))
	for _, v := range s.webhooks {
		out = append(out, cloneWebhook(v))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}

func (s *Store) GetWebhook(id string) (*Webhook, error) {
	id = strings.TrimSpace(id)
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.webhooks[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneWebhook(v), nil
}

func (s *Store) UpdateWebhook(w Webhook) (*Webhook, error) {
	w.ID = strings.TrimSpace(w.ID)
	if w.ID == "" {
		return nil, errors.New("id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.webhooks[w.ID]
	if !ok {
		return nil, ErrNotFound
	}
	if strings.TrimSpace(w.Kind) != "" {
		cur.Kind = strings.TrimSpace(w.Kind)
	}
	cur.Name = w.Name
	cur.AgentID = w.AgentID
	if strings.TrimSpace(w.TokenPrefix) != "" {
		cur.TokenPrefix = strings.TrimSpace(w.TokenPrefix)
	}
	if strings.TrimSpace(w.TokenHash) != "" {
		cur.TokenHash = strings.TrimSpace(w.TokenHash)
	}
	cur.HMACEnc = w.HMACEnc
	cur.RequireHMAC = w.RequireHMAC
	cur.Revoked = w.Revoked
	return cloneWebhook(cur), nil
}

func (s *Store) CreateWebhookJob(j WebhookJob) (*WebhookJob, error) {
	j.ID = strings.TrimSpace(j.ID)
	j.WebhookID = strings.TrimSpace(j.WebhookID)
	if j.WebhookID == "" {
		return nil, errors.New("webhook_id is required")
	}
	if strings.TrimSpace(j.Status) == "" {
		j.Status = WebhookQueued
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.webhooks[j.WebhookID]; !ok {
		return nil, errors.New("webhook not found")
	}
	if j.ID == "" {
		j.ID = s.nextID()
	}
	if _, ok := s.webhookJobs[j.ID]; ok {
		return nil, ErrExists
	}
	if j.CreatedAt.IsZero() {
		j.CreatedAt = now
	} else {
		j.CreatedAt = j.CreatedAt.UTC()
	}
	if j.UpdatedAt.IsZero() {
		j.UpdatedAt = now
	} else {
		j.UpdatedAt = j.UpdatedAt.UTC()
	}
	if j.NextAttemptAt.IsZero() {
		j.NextAttemptAt = now
	} else {
		j.NextAttemptAt = j.NextAttemptAt.UTC()
	}
	cp := j
	s.webhookJobs[cp.ID] = &cp
	return cloneWebhookJob(&cp), nil
}

func (s *Store) GetWebhookJob(id string) (*WebhookJob, error) {
	id = strings.TrimSpace(id)
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.webhookJobs[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneWebhookJob(v), nil
}

func (s *Store) GetWebhookJobByIdempotency(webhookID, key string) (*WebhookJob, error) {
	webhookID = strings.TrimSpace(webhookID)
	key = strings.TrimSpace(key)
	if webhookID == "" || key == "" {
		return nil, ErrNotFound
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var best *WebhookJob
	for _, v := range s.webhookJobs {
		if v == nil || v.WebhookID != webhookID || v.IdempotencyKey != key {
			continue
		}
		if best == nil || v.CreatedAt.After(best.CreatedAt) {
			best = v
		}
	}
	if best == nil {
		return nil, ErrNotFound
	}
	return cloneWebhookJob(best), nil
}

func (s *Store) UpdateWebhookJob(j WebhookJob) (*WebhookJob, error) {
	j.ID = strings.TrimSpace(j.ID)
	if j.ID == "" {
		return nil, errors.New("id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.webhookJobs[j.ID]
	if !ok {
		return nil, ErrNotFound
	}
	if strings.TrimSpace(j.Status) != "" {
		cur.Status = strings.TrimSpace(j.Status)
	}
	cur.Input = j.Input
	cur.Reply = j.Reply
	cur.Error = j.Error
	cur.CallbackURL = j.CallbackURL
	cur.Attempts = j.Attempts
	if !j.NextAttemptAt.IsZero() {
		cur.NextAttemptAt = j.NextAttemptAt.UTC()
	}
	cur.IdempotencyKey = j.IdempotencyKey
	cur.BodyHash = j.BodyHash
	cur.LeaseToken = j.LeaseToken
	cur.UpdatedAt = time.Now().UTC()
	return cloneWebhookJob(cur), nil
}

func (s *Store) ClaimWebhookJob(now time.Time, lease string) (*WebhookJob, error) {
	lease = strings.TrimSpace(lease)
	if lease == "" {
		return nil, errors.New("lease is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var pick *WebhookJob
	for _, v := range s.webhookJobs {
		if v == nil {
			continue
		}
		if v.Status != WebhookQueued {
			continue
		}
		if !v.NextAttemptAt.IsZero() && v.NextAttemptAt.After(now) {
			continue
		}
		if pick == nil || v.CreatedAt.Before(pick.CreatedAt) {
			pick = v
		}
	}
	if pick == nil {
		return nil, ErrNotFound
	}
	pick.Status = WebhookRunning
	pick.LeaseToken = lease
	pick.UpdatedAt = now
	return cloneWebhookJob(pick), nil
}
