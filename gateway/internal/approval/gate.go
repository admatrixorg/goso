// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package approval

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
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
	DecisionDeny    = "deny"
	KindExecution   = "execution"
	RiskLow         = "low"
	RiskMedium      = "medium"
	RiskHigh        = "high"
	MaxPreviewBytes = 240
	MaxReasonBytes  = 400
	StaleWindow     = time.Minute
)

var (
	ErrNotFound       = errors.New("approval not found")
	ErrNotPending     = errors.New("approval is not pending")
	ErrExpired        = errors.New("approval expired")
	ErrBadDecision    = errors.New("decision must be approve or reject")
	ErrReasonRequired = errors.New("denial reason is required")
)

// Request is a pending mutation that the Tool Layer must not Invoke until decided.
type Request struct {
	ID          string         `json:"approval_id"`
	Kind        string         `json:"kind"`
	Requester   string         `json:"requester,omitempty"`
	AgentID     string         `json:"agent_id,omitempty"`
	SessionID   string         `json:"session_id,omitempty"`
	Connector   string         `json:"connector"`
	Tool        string         `json:"tool"`
	Args        map[string]any `json:"-"`
	PolicyProof map[string]any `json:"-"`
	ArgPreview  string         `json:"arg_preview"`
	Risk        string         `json:"risk"`
	Status      string         `json:"status"`
	ExpiresAt   time.Time      `json:"expires_at"`
	CreatedAt   time.Time      `json:"created_at"`
	DecidedAt   *time.Time     `json:"decided_at,omitempty"`
	Decision    string         `json:"decision,omitempty"`
	Reason      string         `json:"reason,omitempty"`
	Stale       bool           `json:"stale"`
	RelayError  string         `json:"relay_error,omitempty"`
}

// SubmitIn is the metadata captured when a tool is gated.
type SubmitIn struct {
	Connector string
	Tool      string
	Args      map[string]any
	Proof     map[string]any
	Requester string
	AgentID   string
	SessionID string
	Risk      string
}

// Relayer optionally forwards a human decision to the remote owner (CRM).
// goso does not execute remote connector mutations itself.
type Relayer func(ctx context.Context, req *Request, decision string) error

// Executor runs a local (builtin) tool after approve. Nil for connector tools.
type Executor func(ctx context.Context, req *Request) error

// Gate stores pending approvals in memory.
type Gate struct {
	mu       sync.Mutex
	items    map[string]*Request
	ttl      time.Duration
	Relayer  Relayer
	Executor Executor
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
	return g.SubmitMeta(SubmitIn{Connector: connector, Tool: tool, Args: args, Proof: proof})
}

// SubmitMeta records a pending execution approval with requester/agent metadata.
func (g *Gate) SubmitMeta(in SubmitIn) *Request {
	now := time.Now().UTC()
	risk := strings.ToLower(strings.TrimSpace(in.Risk))
	if risk == "" {
		risk = ClassifyRisk(in.Tool)
	}
	req := &Request{
		ID:          newID(),
		Kind:        KindExecution,
		Requester:   clip(strings.TrimSpace(in.Requester), 120),
		AgentID:     clip(strings.TrimSpace(in.AgentID), 80),
		SessionID:   clip(strings.TrimSpace(in.SessionID), 80),
		Connector:   strings.TrimSpace(in.Connector),
		Tool:        strings.TrimSpace(in.Tool),
		Args:        cloneMap(in.Args),
		PolicyProof: cloneMap(in.Proof),
		ArgPreview:  ArgPreview(in.Args),
		Risk:        risk,
		Status:      StatusPending,
		ExpiresAt:   now.Add(g.ttl),
		CreatedAt:   now,
	}
	g.mu.Lock()
	g.items[req.ID] = req
	g.mu.Unlock()
	return copyReq(req)
}

// Get returns a copy of the request (expired pending rows are marked expired).
func (g *Gate) Get(id string) (*Request, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	req, err := g.getLocked(id)
	if err != nil {
		if errors.Is(err, ErrExpired) && req != nil {
			return copyReq(req), ErrExpired
		}
		return nil, err
	}
	return copyReq(req), nil
}

// List copies inbox rows. status="" means pending+expired (operator inbox).
func (g *Gate) List(status string) []*Request {
	status = strings.ToLower(strings.TrimSpace(status))
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now().UTC()
	out := make([]*Request, 0, len(g.items))
	for _, req := range g.items {
		expirePending(req, now)
		if status == "" || status == "inbox" {
			if req.Status != StatusPending && req.Status != StatusExpired {
				continue
			}
		} else if status != "all" && req.Status != status {
			continue
		}
		out = append(out, copyReq(req))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

// Decide records approve|reject and optionally relays to the remote owner.
func (g *Gate) Decide(ctx context.Context, id, decision string) (*Request, error) {
	return g.DecideReason(ctx, id, decision, "")
}

// DecideReason records approve|reject with an optional denial reason.
func (g *Gate) DecideReason(ctx context.Context, id, decision, reason string) (*Request, error) {
	decision = NormalizeDecision(decision)
	if decision != DecisionApprove && decision != DecisionReject {
		return nil, ErrBadDecision
	}
	reason = clip(strings.TrimSpace(reason), MaxReasonBytes)
	g.mu.Lock()
	req, err := g.getLocked(id)
	if err != nil {
		g.mu.Unlock()
		if errors.Is(err, ErrExpired) {
			return nil, ErrExpired
		}
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
	req.Reason = reason
	req.DecidedAt = &now
	req.Stale = false
	relayer := g.Relayer
	executor := g.Executor
	g.mu.Unlock()

	if decision == DecisionApprove && executor != nil {
		if rerr := executor(ctx, copyReq(req)); rerr != nil {
			g.mu.Lock()
			if cur, ok := g.items[id]; ok {
				cur.RelayError = clip(rerr.Error(), 240)
			}
			g.mu.Unlock()
		}
	}
	if relayer != nil {
		if rerr := relayer(ctx, copyReq(req), decision); rerr != nil {
			g.mu.Lock()
			if cur, ok := g.items[id]; ok {
				if cur.RelayError == "" {
					cur.RelayError = clip(rerr.Error(), 240)
				}
			}
			g.mu.Unlock()
		}
	}
	got, gerr := g.Get(id)
	if errors.Is(gerr, ErrExpired) {
		return got, nil
	}
	return got, gerr
}

func (g *Gate) getLocked(id string) (*Request, error) {
	req, ok := g.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	expirePending(req, time.Now().UTC())
	if req.Status == StatusExpired && req.Decision == "" {
		return req, ErrExpired
	}
	return req, nil
}

func expirePending(req *Request, now time.Time) {
	if req == nil || req.Status != StatusPending {
		if req != nil {
			req.Stale = req.Status == StatusExpired
		}
		return
	}
	if now.After(req.ExpiresAt) {
		req.Status = StatusExpired
		req.Stale = true
		return
	}
	req.Stale = now.Add(StaleWindow).After(req.ExpiresAt)
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
	cp.Args = cloneMap(r.Args)
	cp.PolicyProof = cloneMap(r.PolicyProof)
	return &cp
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func newID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return "appr-" + hex.EncodeToString(b[:])
}

// NormalizeDecision maps deny → reject and lowercases.
func NormalizeDecision(decision string) string {
	d := strings.ToLower(strings.TrimSpace(decision))
	if d == DecisionDeny {
		return DecisionReject
	}
	return d
}

// ClassifyRisk scores a tool name for the operator inbox.
func ClassifyRisk(tool string) string {
	t := strings.ToLower(strings.TrimSpace(tool))
	switch t {
	case "write_file", "edit", "sandbox", "message_send", "price_change", "browser":
		return RiskHigh
	case "media", "image_gen", "tts":
		return RiskMedium
	}
	if strings.Contains(t, "delete") || strings.Contains(t, "exec") || strings.Contains(t, "send") {
		return RiskHigh
	}
	if t == "" {
		return RiskMedium
	}
	return RiskMedium
}

var (
	secretKeys = []string{
		"token", "password", "secret", "authorization", "api_key", "apikey", "bearer",
		"credential", "hmac", "private_key", "bot_token", "access_token", "hmac_key",
		"content", "body", "text", "arguments", "args", "prompt", "message",
	}
	tokenShape = regexp.MustCompile(`(?i)(sk-[A-Za-z0-9_-]{8,}|gsk_[A-Za-z0-9]+|xai-[A-Za-z0-9]+|gk_[0-9a-f]{16,}|Bearer\s+[A-Za-z0-9._\-+=/]{8,}|ghp_[A-Za-z0-9]+)`)
)

func hidePreviewKey(lk string) bool {
	for _, sk := range secretKeys {
		if lk == sk || strings.Contains(lk, sk) {
			return true
		}
	}
	return false
}

// ArgPreview is a bounded, redacted JSON preview. Never includes secret values.
func ArgPreview(args map[string]any) string {
	if len(args) == 0 {
		return "{}"
	}
	safe := make(map[string]any, len(args))
	n := 0
	for k, v := range args {
		if n >= 8 {
			break
		}
		lk := strings.ToLower(strings.TrimSpace(k))
		if lk == "" || hidePreviewKey(lk) {
			continue
		}
		safe[k] = previewValue(v)
		n++
	}
	if len(safe) == 0 {
		return "{}"
	}
	b, err := json.Marshal(safe)
	if err != nil {
		return "{}"
	}
	s := tokenShape.ReplaceAllString(string(b), "[redacted]")
	if len(s) > MaxPreviewBytes {
		return s[:MaxPreviewBytes] + "…"
	}
	return s
}

func previewValue(v any) any {
	switch t := v.(type) {
	case string:
		s := t
		if tokenShape.MatchString(s) {
			return "[redacted]"
		}
		if len(s) > 80 {
			return s[:80] + "…"
		}
		return s
	case map[string]any:
		return "[object]"
	case []any:
		return "[list]"
	default:
		return t
	}
}

func clip(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n]
}

// PublicJSON is the GET-safe envelope. Args are never included.
type PublicJSON struct {
	ID         string     `json:"id"`
	ApprovalID string     `json:"approval_id"`
	Kind       string     `json:"kind"`
	Requester  string     `json:"requester"`
	AgentID    string     `json:"agent_id,omitempty"`
	SessionID  string     `json:"session_id,omitempty"`
	Connector  string     `json:"connector"`
	Tool       string     `json:"tool"`
	ArgPreview string     `json:"arg_preview"`
	Risk       string     `json:"risk"`
	Status     string     `json:"status"`
	ExpiresAt  time.Time  `json:"expires_at"`
	CreatedAt  time.Time  `json:"created_at"`
	DecidedAt  *time.Time `json:"decided_at,omitempty"`
	Decision   string     `json:"decision,omitempty"`
	Reason     string     `json:"reason,omitempty"`
	Stale      bool       `json:"stale"`
	RelayError string     `json:"relay_error,omitempty"`
}

// Public returns a GET-safe copy: no args, no secrets, bounded preview.
func Public(r *Request) PublicJSON {
	if r == nil {
		return PublicJSON{Kind: KindExecution}
	}
	out := PublicJSON{
		ID:         r.ID,
		ApprovalID: r.ID,
		Kind:       KindExecution,
		Requester:  r.Requester,
		AgentID:    r.AgentID,
		SessionID:  r.SessionID,
		Connector:  r.Connector,
		Tool:       r.Tool,
		ArgPreview: r.ArgPreview,
		Risk:       r.Risk,
		Status:     r.Status,
		ExpiresAt:  r.ExpiresAt,
		CreatedAt:  r.CreatedAt,
		DecidedAt:  r.DecidedAt,
		Decision:   r.Decision,
		Reason:     r.Reason,
		Stale:      r.Stale,
		RelayError: r.RelayError,
	}
	if out.Kind == "" {
		out.Kind = KindExecution
	}
	if out.ArgPreview == "" {
		out.ArgPreview = "{}"
	}
	if tokenShape.MatchString(out.ArgPreview) {
		out.ArgPreview = tokenShape.ReplaceAllString(out.ArgPreview, "[redacted]")
	}
	if tokenShape.MatchString(out.Reason) {
		out.Reason = tokenShape.ReplaceAllString(out.Reason, "[redacted]")
	}
	if tokenShape.MatchString(out.RelayError) {
		out.RelayError = tokenShape.ReplaceAllString(out.RelayError, "[redacted]")
	}
	return out
}

// PublicList maps requests to GET-safe rows.
func PublicList(rows []*Request) []PublicJSON {
	out := make([]PublicJSON, 0, len(rows))
	for _, r := range rows {
		out = append(out, Public(r))
	}
	if out == nil {
		out = []PublicJSON{}
	}
	return out
}
