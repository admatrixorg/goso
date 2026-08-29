// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package webhook

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

const maxRetryAfter = 6 * time.Hour

// Start launches the in-process job poller once.
func (r *Registry) Start() {
	if r == nil {
		return
	}
	r.startOnce.Do(func() {
		r.mu.Lock()
		if r.stop == nil {
			r.stop = make(chan struct{})
		}
		stop := r.stop
		r.mu.Unlock()
		go r.loop(stop)
	})
}

// Stop ends the poller. Safe to call more than once.
func (r *Registry) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	stop := r.stop
	r.stop = nil
	r.mu.Unlock()
	if stop != nil {
		select {
		case <-stop:
		default:
			close(stop)
		}
	}
}

func (r *Registry) loop(stop <-chan struct{}) {
	t := time.NewTicker(r.pollEvery())
	defer t.Stop()
	r.tick()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			r.tick()
		}
	}
}

func (r *Registry) pollEvery() time.Duration {
	if strings.TrimSpace(os.Getenv("GOSO_WEBHOOK_RETRY_MS")) != "" || testing.Testing() {
		return 15 * time.Millisecond
	}
	return 500 * time.Millisecond
}

func (r *Registry) tick() {
	lease := newLease()
	job, err := r.st.ClaimWebhookJob(r.clock(), lease)
	if err != nil || job == nil {
		return
	}
	go r.process(job)
}

func (r *Registry) process(job *store.WebhookJob) {
	if job == nil {
		return
	}
	defer func() { _ = recover() }()

	runner := r.runner()
	reply, runErr := "", error(nil)
	if runner != nil {
		reply, runErr = runner(*jobOf(job))
	}
	if runErr != nil && strings.TrimSpace(reply) == "" {
		job.Error = runErr.Error()
		job.Reply = ""
	} else {
		job.Reply = reply
		if runErr != nil {
			job.Error = runErr.Error()
		} else {
			job.Error = ""
		}
	}

	if strings.TrimSpace(job.CallbackURL) == "" {
		if job.Error != "" {
			job.Status = store.WebhookFailed
		} else {
			job.Status = store.WebhookDone
		}
		job.UpdatedAt = r.clock()
		_, _ = r.st.UpdateWebhookJob(*job)
		return
	}

	status := store.WebhookDone
	if job.Error != "" {
		status = store.WebhookFailed
	}
	code, postErr := r.postCallback(job, status)
	job.Attempts++
	job.UpdatedAt = r.clock()

	switch {
	case postErr == nil && code >= 200 && code <= 299:
		job.Status = store.WebhookDone
		_, _ = r.st.UpdateWebhookJob(*job)
	case code == http.StatusTooManyRequests:
		r.scheduleRetry(job, retryAfterDelay(postErr, r.clock()))
	case code >= 400 && code < 500:
		job.Status = store.WebhookDead
		_, _ = r.st.UpdateWebhookJob(*job)
	default:
		r.scheduleRetry(job, 0)
	}
}

func (r *Registry) scheduleRetry(job *store.WebhookJob, override time.Duration) {
	if job.Attempts >= len(r.retries) {
		job.Status = store.WebhookDead
		job.UpdatedAt = r.clock()
		_, _ = r.st.UpdateWebhookJob(*job)
		return
	}
	delay := r.retries[job.Attempts-1]
	if override > 0 {
		delay = override
	}
	delay = jitter(delay)
	job.Status = store.WebhookQueued
	job.NextAttemptAt = r.clock().Add(delay)
	job.UpdatedAt = r.clock()
	_, _ = r.st.UpdateWebhookJob(*job)
}

func (r *Registry) postCallback(job *store.WebhookJob, status string) (int, error) {
	payload := map[string]any{
		"id":     job.ID,
		"status": status,
		"reply":  job.Reply,
		"error":  job.Error,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	return r.postSigned(job.CallbackURL, job.ID, job.WebhookID, body)
}

func (r *Registry) postSigned(rawURL, deliveryID, webhookID string, body []byte) (int, error) {
	if err := r.CheckCallbackURL(rawURL); err != nil {
		return http.StatusForbidden, err
	}
	req, err := http.NewRequest(http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goso-Delivery-Id", deliveryID)
	req.Header.Set("User-Agent", "goso-webhook/1")
	wh, _ := r.st.GetWebhook(webhookID)
	if key := r.hmacKeyOf(wh); key != "" {
		req.Header.Set("X-Goso-Signature", Sign(key, r.clock(), body))
	}
	cli := r.client
	if cli == nil {
		cli = http.DefaultClient
	}
	resp, err := cli.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests {
		return resp.StatusCode, errRetryAfter{d: parseRetryAfter(resp.Header.Get("Retry-After"))}
	}
	return resp.StatusCode, nil
}

type errRetryAfter struct{ d time.Duration }

func (e errRetryAfter) Error() string { return "retry-after" }

func retryAfterDelay(err error, now time.Time) time.Duration {
	if e, ok := err.(errRetryAfter); ok && e.d > 0 {
		if e.d > maxRetryAfter {
			return maxRetryAfter
		}
		return e.d
	}
	_ = now
	return 0
}

func parseRetryAfter(h string) time.Duration {
	h = strings.TrimSpace(h)
	if h == "" {
		return 0
	}
	if n, err := strconv.Atoi(h); err == nil && n > 0 {
		d := time.Duration(n) * time.Second
		if d > maxRetryAfter {
			return maxRetryAfter
		}
		return d
	}
	if t, err := http.ParseTime(h); err == nil {
		d := time.Until(t)
		if d < 0 {
			return 0
		}
		if d > maxRetryAfter {
			return maxRetryAfter
		}
		return d
	}
	return 0
}

func (r *Registry) runner() JobRunner {
	r.mu.Lock()
	fn := r.run
	r.mu.Unlock()
	return fn
}

func newLease() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		binary.BigEndian.PutUint64(b[:8], uint64(time.Now().UnixNano()))
	}
	return hex.EncodeToString(b[:])
}

func jitter(d time.Duration) time.Duration {
	if d <= 10*time.Millisecond {
		return d
	}
	span := d / 10
	if span <= 0 {
		return d
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return d
	}
	n := int64(binary.BigEndian.Uint64(b[:]) % uint64(span*2+1))
	return d - span + time.Duration(n)
}
