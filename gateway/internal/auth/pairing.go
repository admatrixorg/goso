// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	// PairingTTL is the one-time code lifetime.
	PairingTTL = 10 * time.Minute
	codeLen    = 8
	// 32-char alphabet; 32 divides 256 so rand-byte modulo is unbiased.
	codeAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
)

var (
	ErrPairingInvalid = errors.New("invalid pairing code")
	ErrPairingExpired = errors.New("pairing code expired")
)

// Pairing holds one-time codes bound to GOSO_VIEW_TOKEN or a minted view grant.
type Pairing struct {
	mu     sync.Mutex
	now    func() time.Time
	codes  map[string]*codeEntry
	grants map[string]time.Time // sha256(token) → expiry for minted grants
}

type codeEntry struct {
	grant     string
	expiresAt time.Time
	used      bool
	minted    bool
}

// Issued is the show-once create payload. The code is never listed later.
type Issued struct {
	Code       string
	ExpiresAt  time.Time
	TTLSeconds int
	Role       string
}

// Exchanged is the one-time swap result: the view-token grant (or minted equivalent).
type Exchanged struct {
	Token     string
	Role      string
	ExpiresAt time.Time
	Minted    bool
}

// NewPairing returns an empty in-memory pairing store.
func NewPairing() *Pairing {
	return &Pairing{
		now:    time.Now,
		codes:  map[string]*codeEntry{},
		grants: map[string]time.Time{},
	}
}

// Issue mints an 8-character code. viewToken, when set, is the grant returned on
// exchange; otherwise a short-lived view grant is minted (same 10-minute TTL).
func (p *Pairing) Issue(viewToken string) (Issued, error) {
	if p == nil {
		return Issued{}, ErrPairingInvalid
	}
	code, err := randomCode()
	if err != nil {
		return Issued{}, err
	}
	now := p.clock()
	expires := now.Add(PairingTTL)
	grant := strings.TrimSpace(viewToken)
	minted := false
	if grant == "" {
		g, err := randomGrant()
		if err != nil {
			return Issued{}, err
		}
		grant = g
		minted = true
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.gcLocked(now)
	p.codes[hashSecret(code)] = &codeEntry{
		grant:     grant,
		expiresAt: expires,
		minted:    minted,
	}
	return Issued{
		Code:       code,
		ExpiresAt:  expires.UTC(),
		TTLSeconds: int(PairingTTL.Seconds()),
		Role:       "view",
	}, nil
}

// Exchange consumes a code once and returns the bound view grant.
func (p *Pairing) Exchange(code string) (Exchanged, error) {
	if p == nil {
		return Exchanged{}, ErrPairingInvalid
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return Exchanged{}, ErrPairingInvalid
	}
	now := p.clock()
	key := hashSecret(code)
	p.mu.Lock()
	defer p.mu.Unlock()
	ent := p.codes[key]
	if ent == nil || ent.used {
		return Exchanged{}, ErrPairingInvalid
	}
	if !now.Before(ent.expiresAt) {
		ent.used = true
		ent.grant = ""
		return Exchanged{}, ErrPairingExpired
	}
	ent.used = true
	token := ent.grant
	minted := ent.minted
	exp := ent.expiresAt
	ent.grant = ""
	if minted {
		p.grants[hashSecret(token)] = exp
	}
	return Exchanged{
		Token:     token,
		Role:      "view",
		ExpiresAt: exp.UTC(),
		Minted:    minted,
	}, nil
}

// Accepts reports whether token is a still-valid minted view grant.
func (p *Pairing) Accepts(token string) bool {
	if p == nil {
		return false
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	now := p.clock()
	p.mu.Lock()
	defer p.mu.Unlock()
	exp, ok := p.grants[hashSecret(token)]
	if !ok {
		return false
	}
	return now.Before(exp)
}

func (p *Pairing) clock() time.Time {
	if p.now != nil {
		return p.now()
	}
	return time.Now()
}

func (p *Pairing) gcLocked(now time.Time) {
	for k, ent := range p.codes {
		if ent == nil || !now.Before(ent.expiresAt) || ent.used {
			delete(p.codes, k)
		}
	}
	for k, exp := range p.grants {
		if !now.Before(exp) {
			delete(p.grants, k)
		}
	}
}

func hashSecret(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func randomCode() (string, error) {
	buf := make([]byte, codeLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, codeLen)
	n := byte(len(codeAlphabet))
	for i, b := range buf {
		out[i] = codeAlphabet[int(b%n)]
	}
	return string(out), nil
}

func randomGrant() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "gv_" + hex.EncodeToString(raw[:]), nil
}
