// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// Inbound is a normalized message before the LLM (SPEC 084 InboundMessage).
type Inbound struct {
	Channel  string
	SenderID string
	ChatID   string
	PeerKind string // direct | group
	Text     string
	Mention  bool
}

// Policy is the non-secret CheckPolicy input.
type Policy struct {
	DMPolicy       string
	GroupPolicy    string
	RequireMention bool
	AllowFrom      []string
}

// PolicyAction is the gate result.
type PolicyAction string

const (
	PolicyAccept          PolicyAction = "accept"
	PolicyReject          PolicyAction = "reject"
	PolicyNeedPairing     PolicyAction = "pairing"
	PolicyNeedMention     PolicyAction = "mention"
)

// DefaultPolicy returns SPEC §6 defaults for a catalog name.
func DefaultPolicy(name string) Policy {
	switch name {
	case "telegram":
		p := Policy{DMPolicy: "pairing", GroupPolicy: "allowlist", RequireMention: true}
		if demoEnv() {
			p.DMPolicy = "open"
		}
		return p
	case "zalo-oa":
		return Policy{DMPolicy: "pairing", GroupPolicy: "disabled"}
	case "zalo-personal":
		return Policy{DMPolicy: "allowlist", GroupPolicy: "allowlist"}
	default:
		return Policy{DMPolicy: "disabled", GroupPolicy: "disabled"}
	}
}

func demoEnv() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("GOSO_ENV")), "demo")
}

// MergePolicy overlays stored config on defaults. Empty policy strings inherit defaults.
func MergePolicy(name string, cfg *store.ChannelConfig) Policy {
	p := DefaultPolicy(name)
	if cfg == nil {
		return p
	}
	if cfg.DMPolicy != "" {
		p.DMPolicy = cfg.DMPolicy
	}
	if cfg.GroupPolicy != "" {
		p.GroupPolicy = cfg.GroupPolicy
	}
	p.RequireMention = cfg.RequireMention
	if name == "telegram" && !cfg.RequireMention && cfg.DMPolicy == "" && cfg.GroupPolicy == "" && cfg.AgentID == "" && len(cfg.AllowFrom) == 0 && !cfg.Enabled {
		p.RequireMention = true
	}
	p.AllowFrom = append([]string(nil), cfg.AllowFrom...)
	return p
}

// CheckPolicy decides whether inbound may reach the LLM.
func CheckPolicy(name string, p Policy, in Inbound, paired bool) PolicyAction {
	if in.PeerKind == "group" {
		switch p.GroupPolicy {
		case "disabled", "":
			if name == "zalo-oa" || p.GroupPolicy == "disabled" {
				return PolicyReject
			}
		case "allowlist":
			if !inAllow(p.AllowFrom, in.ChatID) && !inAllow(p.AllowFrom, in.SenderID) {
				return PolicyReject
			}
		case "open":
			// ok
		default:
			return PolicyReject
		}
		if p.RequireMention && !in.Mention {
			return PolicyNeedMention
		}
		return PolicyAccept
	}
	// direct / DM
	switch p.DMPolicy {
	case "disabled":
		return PolicyReject
	case "open":
		return PolicyAccept
	case "allowlist":
		if inAllow(p.AllowFrom, in.SenderID) {
			return PolicyAccept
		}
		return PolicyReject
	case "pairing":
		if paired || inAllow(p.AllowFrom, in.SenderID) {
			return PolicyAccept
		}
		return PolicyNeedPairing
	default:
		return PolicyReject
	}
}

func inAllow(list []string, id string) bool {
	if id == "" {
		return false
	}
	for _, a := range list {
		if a == id {
			return true
		}
	}
	return false
}

// PairingDebounce remembers last pairing instruction per sender+channel.
type PairingDebounce struct {
	mu   sync.Mutex
	last map[string]time.Time
	ttl  time.Duration
}

// NewPairingDebounce uses the SPEC 60s window (override in tests).
func NewPairingDebounce(ttl time.Duration) *PairingDebounce {
	if ttl <= 0 {
		ttl = pairingDebounce
	}
	return &PairingDebounce{last: map[string]time.Time{}, ttl: ttl}
}

// ShouldSend reports whether pairing instructions may be sent now.
func (d *PairingDebounce) ShouldSend(channel, sender string, now time.Time) bool {
	if d == nil {
		return true
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	key := channel + "\x00" + sender
	d.mu.Lock()
	defer d.mu.Unlock()
	prev, ok := d.last[key]
	if ok && now.Sub(prev) < d.ttl {
		return false
	}
	d.last[key] = now
	return true
}
