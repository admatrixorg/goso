// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package approval

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	StatusPending   = "pending"
	StatusApproved  = "approved"
	StatusRejected  = "rejected"
	StatusExpired   = "expired"
	DecisionApprove = "approve"
	DecisionReject  = "reject"
)

var (
	ErrNotFound    = errors.New("approval not found")
	ErrNotPending  = errors.New("approval is not pending")
	ErrExpired     = errors.New("approval expired")
	ErrBadDecision = errors.New("decision must be approve or reject")
)

// Request is a pending mutation that the Tool Layer must not Invoke.
type Request struct {
	ID          string         `json:"approval_id"`
	Connector   string         `json:"connector"`
	Tool        string         `json:"tool"`
	Args        map[string]any `json:"args,omitempty"`
	PolicyProof map[string]any `json:"policy_proof,omitempty"`
	Status      string         `json:"status"`
	ExpiresAt   time.Time      `json:"expires_at"`
	CreatedAt   time.Time      `json:"created_at"`
	DecidedAt   *time.Time     `json:"decided_at,omitempty"`
	Decision    string         `json:"decision,omitempty"`
	RelayError  string         `json:"relay_error,omitempty"`
}

// Relayer optionally forwards a human decision to the remote owner (CRM).
// goso does not execute the mutation itself.
type Relayer func(ctx context.Context, req *Request, decision string) error

// Gate stores pending approvals in memory.
type Gate struct {
	mu      sync.Mutex
	items   map[string]*Request
	ttl     time.Duration
	Relayer Relayer
}

// New returns a Gate. ttl=0 defaults to 15 minutes.
func New(ttl time.Duration) *Gate {
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	return &Gate{items: make(map[string]*Request), ttl: ttl}
}

// Submit records a pending approval. The caller must NOT Invoke the tool.
func (g *Gate) Submit(connector, tool string, args map[string]any, proof map[string]any) *Request {
	now := time.Now().UTC()
	req := &Request{
		ID:          newID(),
		Connector:   connector,
		Tool:        tool,
		Args:        args,
		PolicyProof: proof,
		Status:      StatusPending,
		ExpiresAt:   now.Add(g.ttl),
		CreatedAt:   now,
	}
	g.mu.Lock()
	g.items[req.ID] = req
	g.mu.Unlock()
	return copyReq(req)
}

// Get returns a copy of the request.
func (g *Gate) Get(id string) (*Request, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	req, err := g.getLocked(id)
	if err != nil {
		return nil, err
	}
	return copyReq(req), nil
}

// Decide records approve|reject and optionally relays to the remote owner.
func (g *Gate) Decide(ctx context.Context, id, decision string) (*Request, error) {
	decision = strings.ToLower(strings.TrimSpace(decision))
	if decision != DecisionApprove && decision != DecisionReject {
		return nil, ErrBadDecision
	}
	g.mu.Lock()
	req, err := g.getLocked(id)
	if err != nil {
		g.mu.Unlock()
		return nil, err
	}
	if req.Status != StatusPending {
		g.mu.Unlock()
		return nil, ErrNotPending
	}
	now := time.Now().UTC()
	req.Status = StatusApproved
	if decision == DecisionReject {
		req.Status = StatusRejected
	}
	req.Decision = decision
	req.DecidedAt = &now
	relayer := g.Relayer
	g.mu.Unlock()

	if relayer != nil {
		if rerr := relayer(ctx, copyReq(req), decision); rerr != nil {
			g.mu.Lock()
			if cur, ok := g.items[id]; ok {
				cur.RelayError = rerr.Error()
			}
			g.mu.Unlock()
		}
	}
	return g.Get(id)
}

func (g *Gate) getLocked(id string) (*Request, error) {
	req, ok := g.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	if req.Status == StatusPending && time.Now().UTC().After(req.ExpiresAt) {
		req.Status = StatusExpired
		return req, ErrExpired
	}
	return req, nil
}

func copyReq(r *Request) *Request {
	if r == nil {
		return nil
	}
	cp := *r
	if r.DecidedAt != nil {
		t := *r.DecidedAt
		cp.DecidedAt = &t
	}
	return &cp
}

func newID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return "appr-" + hex.EncodeToString(b[:])
}
