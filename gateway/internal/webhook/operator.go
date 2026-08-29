// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package webhook

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// ListOperator returns every webhook in the tenant-visible registry, including
// revoked rows, with last-delivery metadata and no secrets.
func (r *Registry) ListOperator() []Public {
	if r == nil {
		return []Public{}
	}
	rows := r.st.ListWebhooks()
	out := make([]Public, 0, len(rows))
	for _, rec := range rows {
		if rec == nil {
			continue
		}
		out = append(out, r.operatorOf(rec))
	}
	return out
}

// GetOperator is GET /api/webhooks/{id}: status, endpoint, last delivery, no secrets.
func (r *Registry) GetOperator(id string) (*Public, error) {
	rec, err := r.st.GetWebhook(strings.TrimSpace(id))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	p := r.operatorOf(rec)
	return &p, nil
}

func (r *Registry) operatorOf(rec *store.Webhook) Public {
	p := publicOf(rec)
	p.LastDelivery = r.lastDeliveryOf(rec.ID)
	return p
}

func (r *Registry) lastDeliveryOf(id string) *DeliveryPublic {
	j, err := r.st.LatestWebhookJob(id)
	if err != nil || j == nil {
		return nil
	}
	return deliveryOf(j, 0)
}

func deliveryOf(j *store.WebhookJob, httpStatus int) *DeliveryPublic {
	if j == nil {
		return nil
	}
	at := j.UpdatedAt
	if at.IsZero() {
		at = j.CreatedAt
	}
	d := &DeliveryPublic{
		ID:         j.ID,
		Status:     j.Status,
		HTTPStatus: httpStatus,
	}
	if !at.IsZero() {
		d.At = at.UTC().Format(time.RFC3339)
	}
	return d
}

// Test posts a signed {event:webhook.test} envelope to the stored endpoint.
// The body never includes token, hmac_key, input, or reply.
func (r *Registry) Test(id string) (*DeliveryPublic, error) {
	return r.deliverEvent(strings.TrimSpace(id), "", "webhook.test")
}

// Replay re-posts a redacted envelope for the latest (or named) job.
// Original input/reply/secrets are not included in the outbound body.
func (r *Registry) Replay(id, jobID string) (*DeliveryPublic, error) {
	return r.deliverEvent(strings.TrimSpace(id), strings.TrimSpace(jobID), "webhook.replay")
}

func (r *Registry) deliverEvent(id, jobID, event string) (*DeliveryPublic, error) {
	rec, err := r.st.GetWebhook(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if rec.Revoked {
		return nil, ErrNotFound
	}
	var src *store.WebhookJob
	if event == "webhook.replay" {
		if jobID != "" {
			src, err = r.st.GetWebhookJob(jobID)
			if err != nil || src == nil || src.WebhookID != id {
				return nil, ErrNotFound
			}
		} else {
			src, err = r.st.LatestWebhookJob(id)
			if err != nil || src == nil {
				return nil, ErrNotFound
			}
		}
	}
	endpoint := strings.TrimSpace(rec.Endpoint)
	if src != nil && strings.TrimSpace(src.CallbackURL) != "" {
		endpoint = strings.TrimSpace(src.CallbackURL)
	}
	if endpoint == "" || endpoint == InboundPath {
		return nil, ErrNoEndpoint
	}
	if err := r.CheckCallbackURL(endpoint); err != nil {
		return nil, err
	}

	r.mu.Lock()
	deliveryID := r.allocIDLocked()
	r.mu.Unlock()
	now := r.clock()
	payload := map[string]any{
		"event":      event,
		"id":         deliveryID,
		"webhook_id": id,
	}
	if src != nil {
		payload["original_id"] = src.ID
		payload["status"] = src.Status
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	code, postErr := r.postSigned(endpoint, deliveryID, id, body)
	status := store.WebhookDone
	errStr := ""
	if postErr != nil || code < 200 || code > 299 {
		status = store.WebhookFailed
		if postErr != nil {
			errStr = postErr.Error()
		} else {
			errStr = http.StatusText(code)
		}
	}
	row := store.WebhookJob{
		ID:          deliveryID,
		WebhookID:   id,
		Status:      status,
		CallbackURL: endpoint,
		Error:       errStr,
		Attempts:    1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	got, cerr := r.st.CreateWebhookJob(row)
	if cerr != nil {
		return nil, cerr
	}
	return deliveryOf(got, code), nil
}
