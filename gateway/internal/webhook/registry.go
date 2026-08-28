// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package webhook

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("idempotency conflict")
	ErrStaleHMAC    = errors.New("hmac expired")
	ErrReplay       = errors.New("hmac replay")
	ErrNoMasterKey  = errors.New("master key required")
)

const (
	hmacSkew       = 300 * time.Second
	hmacReplay     = 320 * time.Second
	idempotencyTTL = 24 * time.Hour
)

// Created is returned once on POST /api/webhooks and rotate. Secrets are never listed later.
type Created struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Kind        string `json:"kind,omitempty"`
	AgentID     string `json:"agent_id,omitempty"`
	Token       string `json:"token"`
	TokenPrefix string `json:"token_prefix"`
	HMACKey     string `json:"hmac_key"`
	RequireHMAC bool   `json:"require_hmac"`
}

// Public is the hashed-at-rest view (no secrets).
type Public struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Kind        string `json:"kind,omitempty"`
	AgentID     string `json:"agent_id,omitempty"`
	TokenPrefix string `json:"token_prefix"`
	RequireHMAC bool   `json:"require_hmac"`
	Revoked     bool   `json:"revoked"`
}

// CreateOpts is the optional POST /api/webhooks JSON body.
type CreateOpts struct {
	Name        string
	Kind        string
	AgentID     string
	RequireHMAC bool
}

// Job is an async (or idempotent) LLM webhook run.
type Job struct {
	ID          string `json:"id"`
	WebhookID   string `json:"webhook_id,omitempty"`
	Status      string `json:"status"`
	Reply       string `json:"reply,omitempty"`
	Error       string `json:"error,omitempty"`
	Attempts    int    `json:"attempts,omitempty"`
	CallbackURL string `json:"-"`
	Input       string `json:"-"`
}

// Auth identifies the webhook that authenticated a request.
type Auth struct {
	ID          string
	AgentID     string
	RequireHMAC bool
}

// JobRunner runs chat for a claimed job. Nil uses a no-op reply.
type JobRunner func(job Job) (reply string, err error)

type nonce struct {
	until time.Time
}

// Registry is a store-backed webhook service. HMAC keys are encrypted at rest
// when GOSO_MASTER_KEY is set; otherwise HMAC material stays in-process.
type Registry struct {
	mu        sync.Mutex
	st        store.StoreIface
	hmacCache map[string]string // id -> hex HMAC key (in-process / hashed-only)
	nonces    map[string]nonce  // sha256(webhook_id|v1) -> expiry
	seq       int64
	retries   []time.Duration
	now       func() time.Time
	checkURL  func(string) error
	client    *http.Client
	run       JobRunner
	stop      chan struct{}
	startOnce sync.Once
}

func New() *Registry {
	return NewWithStore(store.New())
}

func NewWithStore(st store.StoreIface) *Registry {
	if st == nil {
		st = store.New()
	}
	cli := &http.Client{Timeout: 15 * time.Second}
	security.GuardClient(cli)
	return &Registry{
		st:        st,
		hmacCache: make(map[string]string),
		nonces:    make(map[string]nonce),
		retries:   retrySchedule(),
		now:       func() time.Time { return time.Now().UTC() },
		checkURL:  security.CheckURL,
		client:    cli,
	}
}

// SetRunner installs the chat callback used by the outbound worker.
func (r *Registry) SetRunner(fn JobRunner) {
	r.mu.Lock()
	r.run = fn
	r.mu.Unlock()
}

func (r *Registry) Create() (*Created, error) {
	return r.CreateOpts(CreateOpts{})
}

func (r *Registry) CreateOpts(opts CreateOpts) (*Created, error) {
	tokRaw := make([]byte, 24)
	keyRaw := make([]byte, 32)
	if _, err := rand.Read(tokRaw); err != nil {
		return nil, err
	}
	if _, err := rand.Read(keyRaw); err != nil {
		return nil, err
	}
	token := "wh_" + hex.EncodeToString(tokRaw)
	prefix := token
	if len(prefix) > 11 {
		prefix = prefix[:11]
	}
	hmacKey := hex.EncodeToString(keyRaw)
	kind := strings.TrimSpace(opts.Kind)
	if kind == "" {
		kind = "llm"
	}
	enc, err := sealHMAC(hmacKey)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(token))
	r.mu.Lock()
	id := r.allocIDLocked()
	r.hmacCache[id] = hmacKey
	r.mu.Unlock()
	row := store.Webhook{
		ID:          id,
		Name:        strings.TrimSpace(opts.Name),
		Kind:        kind,
		AgentID:     strings.TrimSpace(opts.AgentID),
		TokenPrefix: prefix,
		TokenHash:   hex.EncodeToString(sum[:]),
		HMACEnc:     enc,
		RequireHMAC: opts.RequireHMAC,
		CreatedAt:   r.clock(),
	}
	if _, err := r.st.CreateWebhook(row); err != nil {
		r.mu.Lock()
		delete(r.hmacCache, id)
		r.mu.Unlock()
		return nil, err
	}
	return &Created{
		ID:          id,
		Name:        row.Name,
		Kind:        row.Kind,
		AgentID:     row.AgentID,
		Token:       token,
		TokenPrefix: prefix,
		HMACKey:     hmacKey,
		RequireHMAC: row.RequireHMAC,
	}, nil
}

func (r *Registry) List() []Public {
	rows := r.st.ListWebhooks()
	out := make([]Public, 0, len(rows))
	for _, rec := range rows {
		if rec == nil || rec.Revoked {
			continue
		}
		out = append(out, publicOf(rec))
	}
	return out
}

func (r *Registry) Get(id string) (*Public, error) {
	rec, err := r.st.GetWebhook(strings.TrimSpace(id))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	p := publicOf(rec)
	return &p, nil
}

func (r *Registry) Rotate(id string) (*Created, error) {
	id = strings.TrimSpace(id)
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
	tokRaw := make([]byte, 24)
	keyRaw := make([]byte, 32)
	if _, err := rand.Read(tokRaw); err != nil {
		return nil, err
	}
	if _, err := rand.Read(keyRaw); err != nil {
		return nil, err
	}
	token := "wh_" + hex.EncodeToString(tokRaw)
	prefix := token
	if len(prefix) > 11 {
		prefix = prefix[:11]
	}
	hmacKey := hex.EncodeToString(keyRaw)
	enc, err := sealHMAC(hmacKey)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(token))
	rec.TokenPrefix = prefix
	rec.TokenHash = hex.EncodeToString(sum[:])
	rec.HMACEnc = enc
	if _, err := r.st.UpdateWebhook(*rec); err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.hmacCache[id] = hmacKey
	r.mu.Unlock()
	return &Created{
		ID:          rec.ID,
		Name:        rec.Name,
		Kind:        rec.Kind,
		AgentID:     rec.AgentID,
		Token:       token,
		TokenPrefix: prefix,
		HMACKey:     hmacKey,
		RequireHMAC: rec.RequireHMAC,
	}, nil
}

func (r *Registry) Revoke(id string) error {
	id = strings.TrimSpace(id)
	rec, err := r.st.GetWebhook(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	rec.Revoked = true
	if _, err := r.st.UpdateWebhook(*rec); err != nil {
		return err
	}
	r.mu.Lock()
	delete(r.hmacCache, id)
	r.mu.Unlock()
	return nil
}

// Authenticate accepts Bearer wh_… or X-Goso-Signature t=unix,v1=hex over ts.body.
func (r *Registry) Authenticate(bearer, signature string, body []byte) error {
	_, err := r.AuthenticateRecord(bearer, signature, body)
	return err
}

func (r *Registry) AuthenticateRecord(bearer, signature string, body []byte) (*Auth, error) {
	if tok, ok := cutBearer(bearer); ok {
		return r.matchToken(tok)
	}
	if signature != "" {
		return r.matchHMAC(signature, body)
	}
	return nil, ErrUnauthorized
}

func (r *Registry) matchToken(token string) (*Auth, error) {
	if !strings.HasPrefix(token, "wh_") {
		return nil, ErrUnauthorized
	}
	sum := sha256.Sum256([]byte(token))
	want := hex.EncodeToString(sum[:])
	var found *store.Webhook
	for _, rec := range r.st.ListWebhooks() {
		if rec == nil || rec.Revoked {
			continue
		}
		if rec.RequireHMAC {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(rec.TokenHash)) == 1 {
			found = rec
		}
	}
	if found == nil {
		return nil, ErrUnauthorized
	}
	return &Auth{ID: found.ID, AgentID: found.AgentID, RequireHMAC: found.RequireHMAC}, nil
}

func (r *Registry) matchHMAC(header string, body []byte) (*Auth, error) {
	ts, sig, err := parseSignature(header)
	if err != nil {
		return nil, ErrUnauthorized
	}
	unix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return nil, ErrUnauthorized
	}
	now := r.clock()
	skew := now.Sub(time.Unix(unix, 0).UTC())
	if skew < 0 {
		skew = -skew
	}
	if skew > hmacSkew {
		return nil, ErrStaleHMAC
	}
	msg := []byte(ts + "." + string(body))
	var found *store.Webhook
	var v1hex string
	v1hex = hex.EncodeToString(sig)
	for _, rec := range r.st.ListWebhooks() {
		if rec == nil || rec.Revoked {
			continue
		}
		key := r.hmacKeyOf(rec)
		if key == "" {
			continue
		}
		mac := hmac.New(sha256.New, []byte(key))
		_, _ = mac.Write(msg)
		want := mac.Sum(nil)
		if subtle.ConstantTimeCompare(want, sig) == 1 {
			found = rec
		}
	}
	if found == nil {
		return nil, ErrUnauthorized
	}
	nonceKey := replayKey(found.ID, v1hex)
	r.mu.Lock()
	r.pruneNoncesLocked(now)
	if n, ok := r.nonces[nonceKey]; ok && n.until.After(now) {
		r.mu.Unlock()
		return nil, ErrReplay
	}
	r.nonces[nonceKey] = nonce{until: now.Add(hmacReplay)}
	r.mu.Unlock()
	return &Auth{ID: found.ID, AgentID: found.AgentID, RequireHMAC: found.RequireHMAC}, nil
}

func (r *Registry) hmacKeyOf(rec *store.Webhook) string {
	if rec == nil {
		return ""
	}
	r.mu.Lock()
	cached := r.hmacCache[rec.ID]
	r.mu.Unlock()
	if cached != "" {
		return cached
	}
	key, err := openHMAC(rec.HMACEnc)
	if err != nil || key == "" {
		return ""
	}
	r.mu.Lock()
	r.hmacCache[rec.ID] = key
	r.mu.Unlock()
	return key
}

func (r *Registry) pruneNoncesLocked(now time.Time) {
	for k, n := range r.nonces {
		if !n.until.After(now) {
			delete(r.nonces, k)
		}
	}
}

func (r *Registry) CheckCallbackURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if r.checkURL == nil {
		return nil
	}
	return r.checkURL(raw)
}

func (r *Registry) LookupIdempotency(webhookID, key, hash string) (*Job, error) {
	webhookID = strings.TrimSpace(webhookID)
	key = strings.TrimSpace(key)
	if webhookID == "" || key == "" {
		return nil, ErrNotFound
	}
	existing, err := r.st.GetWebhookJobByIdempotency(webhookID, key)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if existing == nil {
		return nil, ErrNotFound
	}
	if !existing.CreatedAt.IsZero() && r.clock().Sub(existing.CreatedAt) > idempotencyTTL {
		return nil, ErrNotFound
	}
	if existing.BodyHash != hash {
		return nil, ErrConflict
	}
	return jobOf(existing), nil
}

func (r *Registry) FinishSync(auth *Auth, input, idemKey, bodyHash, reply, errStr string) (*Job, error) {
	if auth == nil {
		return nil, ErrUnauthorized
	}
	r.mu.Lock()
	id := r.allocIDLocked()
	r.mu.Unlock()
	now := r.clock()
	status := store.WebhookDone
	if strings.TrimSpace(errStr) != "" {
		status = store.WebhookFailed
	}
	row := store.WebhookJob{
		ID:             id,
		WebhookID:      auth.ID,
		Status:         status,
		Input:          strings.TrimSpace(input),
		Reply:          reply,
		Error:          errStr,
		IdempotencyKey: strings.TrimSpace(idemKey),
		BodyHash:       bodyHash,
		NextAttemptAt:  now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	got, err := r.st.CreateWebhookJob(row)
	if err != nil {
		return nil, err
	}
	return jobOf(got), nil
}

func (r *Registry) Enqueue(auth *Auth, input, callbackURL, idemKey, bodyHash string) (*Job, error) {
	if auth == nil {
		return nil, ErrUnauthorized
	}
	input = strings.TrimSpace(input)
	callbackURL = strings.TrimSpace(callbackURL)
	idemKey = strings.TrimSpace(idemKey)
	if callbackURL != "" {
		if err := r.CheckCallbackURL(callbackURL); err != nil {
			return nil, err
		}
	}
	if idemKey != "" {
		if existing, err := r.st.GetWebhookJobByIdempotency(auth.ID, idemKey); err == nil && existing != nil {
			if time.Since(existing.CreatedAt) <= idempotencyTTL {
				if existing.BodyHash != bodyHash {
					return nil, ErrConflict
				}
				return jobOf(existing), nil
			}
		}
	}
	r.mu.Lock()
	id := r.allocIDLocked()
	r.mu.Unlock()
	now := r.clock()
	row := store.WebhookJob{
		ID:             id,
		WebhookID:      auth.ID,
		Status:         store.WebhookQueued,
		Input:          input,
		CallbackURL:    callbackURL,
		IdempotencyKey: idemKey,
		BodyHash:       bodyHash,
		NextAttemptAt:  now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	got, err := r.st.CreateWebhookJob(row)
	if err != nil {
		return nil, err
	}
	r.Start()
	return jobOf(got), nil
}

func (r *Registry) CompleteJob(id, reply string) {
	j, err := r.st.GetWebhookJob(id)
	if err != nil || j == nil {
		return
	}
	j.Status = store.WebhookDone
	j.Reply = reply
	j.Error = ""
	j.UpdatedAt = r.clock()
	_, _ = r.st.UpdateWebhookJob(*j)
}

func (r *Registry) GetJob(id string) (*Job, error) {
	j, err := r.st.GetWebhookJob(strings.TrimSpace(id))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return jobOf(j), nil
}

func (r *Registry) allocIDLocked() string {
	r.seq++
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().UTC().Format("20060102") + "-wh-" + strconv.FormatInt(r.seq, 10)
	}
	return time.Now().UTC().Format("20060102") + "-wh-" + hex.EncodeToString(b[:])
}

func (r *Registry) clock() time.Time {
	if r.now != nil {
		return r.now().UTC()
	}
	return time.Now().UTC()
}

func publicOf(rec *store.Webhook) Public {
	if rec == nil {
		return Public{}
	}
	return Public{
		ID:          rec.ID,
		Name:        rec.Name,
		Kind:        rec.Kind,
		AgentID:     rec.AgentID,
		TokenPrefix: rec.TokenPrefix,
		RequireHMAC: rec.RequireHMAC,
		Revoked:     rec.Revoked,
	}
}

func jobOf(j *store.WebhookJob) *Job {
	if j == nil {
		return nil
	}
	return &Job{
		ID:          j.ID,
		WebhookID:   j.WebhookID,
		Status:      j.Status,
		Reply:       j.Reply,
		Error:       j.Error,
		Attempts:    j.Attempts,
		CallbackURL: j.CallbackURL,
		Input:       j.Input,
	}
}

func cutBearer(h string) (string, bool) {
	h = strings.TrimSpace(h)
	if !strings.HasPrefix(h, "Bearer ") {
		return "", false
	}
	tok := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	if tok == "" {
		return "", false
	}
	return tok, true
}

func parseSignature(h string) (ts string, sig []byte, err error) {
	var v1 string
	for _, part := range strings.Split(h, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			ts = kv[1]
		case "v1":
			v1 = kv[1]
		}
	}
	if ts == "" || v1 == "" {
		return "", nil, errors.New("bad signature header")
	}
	if _, err := strconv.ParseInt(ts, 10, 64); err != nil {
		return "", nil, err
	}
	raw, err := hex.DecodeString(v1)
	if err != nil {
		return "", nil, err
	}
	return ts, raw, nil
}

func Sign(hmacKey string, t time.Time, body []byte) string {
	ts := strconv.FormatInt(t.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(hmacKey))
	_, _ = mac.Write([]byte(ts + "." + string(body)))
	return fmt.Sprintf("t=%s,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

func replayKey(webhookID, v1hex string) string {
	sum := sha256.Sum256([]byte(webhookID + "|" + v1hex))
	return hex.EncodeToString(sum[:])
}

func sealHMAC(key string) (string, error) {
	nonce, ct, err := secrets.Encrypt([]byte(key))
	if err != nil {
		if errors.Is(err, secrets.ErrNoMasterKey) {
			return "", nil
		}
		return "", err
	}
	return hex.EncodeToString(nonce) + ":" + hex.EncodeToString(ct), nil
}

func openHMAC(enc string) (string, error) {
	enc = strings.TrimSpace(enc)
	if enc == "" {
		return "", nil
	}
	nonceHex, ctHex, ok := strings.Cut(enc, ":")
	if !ok {
		return "", errors.New("bad hmac_enc")
	}
	nonce, err := hex.DecodeString(nonceHex)
	if err != nil {
		return "", err
	}
	ct, err := hex.DecodeString(ctHex)
	if err != nil {
		return "", err
	}
	pt, err := secrets.Decrypt(nonce, ct)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

func retrySchedule() []time.Duration {
	raw := strings.TrimSpace(os.Getenv("GOSO_WEBHOOK_RETRY_MS"))
	if raw != "" {
		var out []time.Duration
		for _, p := range strings.Split(raw, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil || n < 0 {
				continue
			}
			out = append(out, time.Duration(n)*time.Millisecond)
		}
		if len(out) > 0 {
			return out
		}
	}
	return []time.Duration{
		30 * time.Second,
		2 * time.Minute,
		10 * time.Minute,
		time.Hour,
		6 * time.Hour,
	}
}

func bodyHash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// BodyHash is the SHA-256 hex of the raw request body (idempotency).
func BodyHash(raw []byte) string { return bodyHash(raw) }
