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
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrNotFound     = errors.New("not found")
)

// Created is returned once on POST /api/webhooks. Secrets are never listed later.
type Created struct {
	ID          string `json:"id"`
	Token       string `json:"token"`
	TokenPrefix string `json:"token_prefix"`
	HMACKey     string `json:"hmac_key"`
}

// Public is the hashed-at-rest view (no secrets).
type Public struct {
	ID          string `json:"id"`
	TokenPrefix string `json:"token_prefix"`
}

type record struct {
	id          string
	tokenPrefix string
	tokenHash   [32]byte
	hmacKey     []byte
}

// Job is an async LLM webhook run.
type Job struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Reply  string `json:"reply,omitempty"`
}

// Registry stores webhook endpoints. Tokens are stored hashed; HMAC keys
// are held for verify and never returned after create.
type Registry struct {
	mu      sync.Mutex
	records []*record
	jobs    map[string]*Job
	seq     int64
}

func New() *Registry {
	return &Registry{jobs: make(map[string]*Job)}
}

func (r *Registry) Create() (*Created, error) {
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
	r.mu.Lock()
	id := r.allocIDLocked()
	r.records = append(r.records, &record{
		id:          id,
		tokenPrefix: prefix,
		tokenHash:   sha256.Sum256([]byte(token)),
		hmacKey:     []byte(hmacKey),
	})
	r.mu.Unlock()
	return &Created{ID: id, Token: token, TokenPrefix: prefix, HMACKey: hmacKey}, nil
}

func (r *Registry) List() []Public {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Public, 0, len(r.records))
	for _, rec := range r.records {
		out = append(out, Public{ID: rec.id, TokenPrefix: rec.tokenPrefix})
	}
	return out
}

// Authenticate accepts Bearer wh_… or X-Goso-Signature t=unix,v1=hex over t.body.
func (r *Registry) Authenticate(bearer, signature string, body []byte) error {
	if tok, ok := cutBearer(bearer); ok {
		if r.matchToken(tok) {
			return nil
		}
		return ErrUnauthorized
	}
	if signature != "" {
		if r.matchHMAC(signature, body) {
			return nil
		}
		return ErrUnauthorized
	}
	return ErrUnauthorized
}

func (r *Registry) matchToken(token string) bool {
	if !strings.HasPrefix(token, "wh_") {
		return false
	}
	sum := sha256.Sum256([]byte(token))
	r.mu.Lock()
	defer r.mu.Unlock()
	ok := false
	for _, rec := range r.records {
		if subtle.ConstantTimeCompare(sum[:], rec.tokenHash[:]) == 1 {
			ok = true
		}
	}
	return ok
}

func (r *Registry) matchHMAC(header string, body []byte) bool {
	ts, sig, err := parseSignature(header)
	if err != nil {
		return false
	}
	msg := []byte(ts + "." + string(body))
	r.mu.Lock()
	defer r.mu.Unlock()
	ok := false
	for _, rec := range r.records {
		mac := hmac.New(sha256.New, rec.hmacKey)
		_, _ = mac.Write(msg)
		want := mac.Sum(nil)
		if subtle.ConstantTimeCompare(want, sig) == 1 {
			ok = true
		}
	}
	return ok
}

func (r *Registry) NewJob() *Job {
	r.mu.Lock()
	j := &Job{ID: r.allocIDLocked(), Status: "accepted"}
	r.jobs[j.ID] = j
	r.mu.Unlock()
	return j
}

func (r *Registry) CompleteJob(id, reply string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if j, ok := r.jobs[id]; ok {
		j.Status = "done"
		j.Reply = reply
	}
}

func (r *Registry) GetJob(id string) (*Job, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	j, ok := r.jobs[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *j
	return &cp, nil
}

func (r *Registry) allocIDLocked() string {
	r.seq++
	return time.Now().UTC().Format("20060102") + "-wh-" + strconv.FormatInt(r.seq, 10)
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
