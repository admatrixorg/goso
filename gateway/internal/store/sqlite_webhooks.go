// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

func (s *SQLiteStore) CreateWebhook(w Webhook) (*Webhook, error) {
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
	w.TenantID = NormalizeTenant(w.TenantID)
	if w.ID == "" {
		w.ID = newID()
	}
	if w.CreatedAt.IsZero() {
		w.CreatedAt = time.Now().UTC()
	} else {
		w.CreatedAt = w.CreatedAt.UTC()
	}
	req, rev := 0, 0
	if w.RequireHMAC {
		req = 1
	}
	if w.Revoked {
		rev = 1
	}
	w.Endpoint = strings.TrimSpace(w.Endpoint)
	_, err := s.db.Exec(
		`INSERT INTO webhooks(id, name, kind, agent_id, token_prefix, token_hash, hmac_enc, require_hmac, revoked, created_at, tenant_id, endpoint)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		w.ID, w.Name, w.Kind, w.AgentID, w.TokenPrefix, w.TokenHash, w.HMACEnc, req, rev, formatTime(w.CreatedAt), w.TenantID, w.Endpoint,
	)
	if err != nil {
		return nil, err
	}
	cp := w
	return cloneWebhook(&cp), nil
}

func (s *SQLiteStore) ListWebhooks() []*Webhook {
	rows, err := s.db.Query(`SELECT id, name, kind, agent_id, token_prefix, token_hash, hmac_enc, require_hmac, revoked, created_at, tenant_id, endpoint FROM webhooks ORDER BY created_at`)
	if err != nil {
		return []*Webhook{}
	}
	defer rows.Close()
	out := make([]*Webhook, 0)
	for rows.Next() {
		w, err := scanWebhook(rows)
		if err != nil {
			continue
		}
		out = append(out, w)
	}
	return out
}

func (s *SQLiteStore) GetWebhook(id string) (*Webhook, error) {
	row := s.db.QueryRow(`SELECT id, name, kind, agent_id, token_prefix, token_hash, hmac_enc, require_hmac, revoked, created_at, tenant_id, endpoint FROM webhooks WHERE id=?`, strings.TrimSpace(id))
	w, err := scanWebhook(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return w, nil
}

func (s *SQLiteStore) UpdateWebhook(w Webhook) (*Webhook, error) {
	w.ID = strings.TrimSpace(w.ID)
	if w.ID == "" {
		return nil, errors.New("id is required")
	}
	cur, err := s.GetWebhook(w.ID)
	if err != nil {
		return nil, err
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
	cur.Endpoint = strings.TrimSpace(w.Endpoint)
	req, rev := 0, 0
	if cur.RequireHMAC {
		req = 1
	}
	if cur.Revoked {
		rev = 1
	}
	res, err := s.db.Exec(
		`UPDATE webhooks SET name=?, kind=?, agent_id=?, token_prefix=?, token_hash=?, hmac_enc=?, require_hmac=?, revoked=?, endpoint=? WHERE id=?`,
		cur.Name, cur.Kind, cur.AgentID, cur.TokenPrefix, cur.TokenHash, cur.HMACEnc, req, rev, cur.Endpoint, cur.ID,
	)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, ErrNotFound
	}
	return cloneWebhook(cur), nil
}

func scanWebhook(sc scanner) (*Webhook, error) {
	var w Webhook
	var name, kind, agentID, hmacEnc, tenant, endpoint sql.NullString
	var req, rev int
	var ts string
	if err := sc.Scan(&w.ID, &name, &kind, &agentID, &w.TokenPrefix, &w.TokenHash, &hmacEnc, &req, &rev, &ts, &tenant, &endpoint); err != nil {
		return nil, err
	}
	w.Name = name.String
	w.Kind = kind.String
	w.AgentID = agentID.String
	w.HMACEnc = hmacEnc.String
	w.TenantID = NormalizeTenant(tenant.String)
	w.Endpoint = strings.TrimSpace(endpoint.String)
	w.RequireHMAC = req != 0
	w.Revoked = rev != 0
	w.CreatedAt = parseTime(ts)
	return &w, nil
}

func (s *SQLiteStore) CreateWebhookJob(j WebhookJob) (*WebhookJob, error) {
	j.ID = strings.TrimSpace(j.ID)
	j.WebhookID = strings.TrimSpace(j.WebhookID)
	if j.WebhookID == "" {
		return nil, errors.New("webhook_id is required")
	}
	if strings.TrimSpace(j.Status) == "" {
		j.Status = WebhookQueued
	}
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM webhooks WHERE id=?`, j.WebhookID).Scan(&n); err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errors.New("webhook not found")
	}
	if j.ID == "" {
		j.ID = newID()
	}
	now := time.Now().UTC()
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
	_, err := s.db.Exec(
		`INSERT INTO webhook_jobs(id, webhook_id, status, input, reply, error, callback_url, attempts, next_attempt_at, idempotency_key, body_hash, lease_token, created_at, updated_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		j.ID, j.WebhookID, j.Status, j.Input, j.Reply, j.Error, j.CallbackURL, j.Attempts,
		formatTime(j.NextAttemptAt), j.IdempotencyKey, j.BodyHash, j.LeaseToken,
		formatTime(j.CreatedAt), formatTime(j.UpdatedAt),
	)
	if err != nil {
		return nil, err
	}
	cp := j
	return cloneWebhookJob(&cp), nil
}

func (s *SQLiteStore) GetWebhookJob(id string) (*WebhookJob, error) {
	row := s.db.QueryRow(
		`SELECT id, webhook_id, status, input, reply, error, callback_url, attempts, next_attempt_at, idempotency_key, body_hash, lease_token, created_at, updated_at
		 FROM webhook_jobs WHERE id=?`, strings.TrimSpace(id),
	)
	j, err := scanWebhookJob(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return j, nil
}

func (s *SQLiteStore) LatestWebhookJob(webhookID string) (*WebhookJob, error) {
	webhookID = strings.TrimSpace(webhookID)
	if webhookID == "" {
		return nil, ErrNotFound
	}
	row := s.db.QueryRow(
		`SELECT id, webhook_id, status, input, reply, error, callback_url, attempts, next_attempt_at, idempotency_key, body_hash, lease_token, created_at, updated_at
		 FROM webhook_jobs WHERE webhook_id=? ORDER BY updated_at DESC, created_at DESC LIMIT 1`,
		webhookID,
	)
	j, err := scanWebhookJob(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return j, nil
}

func (s *SQLiteStore) GetWebhookJobByIdempotency(webhookID, key string) (*WebhookJob, error) {
	webhookID = strings.TrimSpace(webhookID)
	key = strings.TrimSpace(key)
	if webhookID == "" || key == "" {
		return nil, ErrNotFound
	}
	row := s.db.QueryRow(
		`SELECT id, webhook_id, status, input, reply, error, callback_url, attempts, next_attempt_at, idempotency_key, body_hash, lease_token, created_at, updated_at
		 FROM webhook_jobs WHERE webhook_id=? AND idempotency_key=? ORDER BY created_at DESC LIMIT 1`,
		webhookID, key,
	)
	j, err := scanWebhookJob(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return j, nil
}

func (s *SQLiteStore) UpdateWebhookJob(j WebhookJob) (*WebhookJob, error) {
	j.ID = strings.TrimSpace(j.ID)
	if j.ID == "" {
		return nil, errors.New("id is required")
	}
	cur, err := s.GetWebhookJob(j.ID)
	if err != nil {
		return nil, err
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
	res, err := s.db.Exec(
		`UPDATE webhook_jobs SET status=?, input=?, reply=?, error=?, callback_url=?, attempts=?, next_attempt_at=?, idempotency_key=?, body_hash=?, lease_token=?, updated_at=? WHERE id=?`,
		cur.Status, cur.Input, cur.Reply, cur.Error, cur.CallbackURL, cur.Attempts,
		formatTime(cur.NextAttemptAt), cur.IdempotencyKey, cur.BodyHash, cur.LeaseToken, formatTime(cur.UpdatedAt), cur.ID,
	)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, ErrNotFound
	}
	return cloneWebhookJob(cur), nil
}

func (s *SQLiteStore) ClaimWebhookJob(now time.Time, lease string) (*WebhookJob, error) {
	lease = strings.TrimSpace(lease)
	if lease == "" {
		return nil, errors.New("lease is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	row := tx.QueryRow(
		`SELECT id, webhook_id, status, input, reply, error, callback_url, attempts, next_attempt_at, idempotency_key, body_hash, lease_token, created_at, updated_at
		 FROM webhook_jobs
		 WHERE status=? AND (next_attempt_at IS NULL OR next_attempt_at='' OR next_attempt_at<=?)
		 ORDER BY created_at LIMIT 1`,
		WebhookQueued, formatTime(now),
	)
	j, err := scanWebhookJob(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	j.Status = WebhookRunning
	j.LeaseToken = lease
	j.UpdatedAt = now
	res, err := tx.Exec(
		`UPDATE webhook_jobs SET status=?, lease_token=?, updated_at=? WHERE id=? AND status=?`,
		j.Status, j.LeaseToken, formatTime(j.UpdatedAt), j.ID, WebhookQueued,
	)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return j, nil
}

func scanWebhookJob(sc scanner) (*WebhookJob, error) {
	var j WebhookJob
	var input, reply, errStr, cb, next, idem, hash, lease, created, updated sql.NullString
	if err := sc.Scan(&j.ID, &j.WebhookID, &j.Status, &input, &reply, &errStr, &cb, &j.Attempts, &next, &idem, &hash, &lease, &created, &updated); err != nil {
		return nil, err
	}
	j.Input = input.String
	j.Reply = reply.String
	j.Error = errStr.String
	j.CallbackURL = cb.String
	j.IdempotencyKey = idem.String
	j.BodyHash = hash.String
	j.LeaseToken = lease.String
	if strings.TrimSpace(next.String) != "" {
		j.NextAttemptAt = parseTime(next.String)
	}
	j.CreatedAt = parseTime(created.String)
	j.UpdatedAt = parseTime(updated.String)
	return &j, nil
}
