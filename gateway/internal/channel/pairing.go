// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// PairingAlphabet excludes ambiguous 0 O 1 I L (SPEC 084).
const PairingAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

const (
	pairingTTL      = 60 * time.Minute
	pairingMaxPend  = 3
	pairingCodeLen  = 8
	pairingDebounce = 60 * time.Second
)

var (
	ErrPairingCap     = errors.New("too many pending pairing codes")
	ErrPairingGone    = errors.New("pairing not found")
	ErrPairingExpired = errors.New("pairing expired")
	ErrPairingStatus  = errors.New("pairing not pending")
)

// IssuedPairing is the one-time plaintext code for the end-user.
type IssuedPairing struct {
	ID        string
	Code      string
	ExpiresAt time.Time
}

// IssuePairing mints a hashed pending code. Plaintext is returned once.
func IssuePairing(st store.StoreIface, channel, sender string, now time.Time) (*IssuedPairing, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if st.CountPendingChannelPairings(channel, sender, now) >= pairingMaxPend {
		return nil, ErrPairingCap
	}
	code, err := randomPairingCode()
	if err != nil {
		return nil, err
	}
	exp := now.Add(pairingTTL)
	row, err := st.CreateChannelPairing(store.ChannelPairing{
		Channel:   channel,
		SenderID:  sender,
		CodeHash:  hashPairingCode(code),
		Status:    "pending",
		ExpiresAt: exp,
		CreatedAt: now,
	})
	if err != nil {
		return nil, err
	}
	return &IssuedPairing{ID: row.ID, Code: code, ExpiresAt: exp}, nil
}

// ApprovePairing marks a pending unexpired row approved.
func ApprovePairing(st store.StoreIface, id string, now time.Time) error {
	return setPairingStatus(st, id, "approved", now)
}

// DenyPairing marks a pending unexpired row denied.
func DenyPairing(st store.StoreIface, id string, now time.Time) error {
	return setPairingStatus(st, id, "denied", now)
}

func setPairingStatus(st store.StoreIface, id, status string, now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	row, err := st.GetChannelPairing(id)
	if err != nil {
		return ErrPairingGone
	}
	if row.Status != "pending" {
		return ErrPairingStatus
	}
	if !row.ExpiresAt.IsZero() && !row.ExpiresAt.After(now) {
		row.Status = "expired"
		_, _ = st.UpdateChannelPairing(*row)
		return ErrPairingExpired
	}
	row.Status = status
	if status == "approved" {
		row.ApprovedAt = now
	}
	_, err = st.UpdateChannelPairing(*row)
	return err
}

// SenderPaired reports an approved pairing (or allowlist is checked by policy).
func SenderPaired(st store.StoreIface, channel, sender string, now time.Time) bool {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	for _, p := range st.ListChannelPairings() {
		if p.Channel != channel || p.SenderID != sender {
			continue
		}
		if p.Status == "approved" {
			return true
		}
	}
	return false
}

func randomPairingCode() (string, error) {
	buf := make([]byte, pairingCodeLen)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		return "", fmt.Errorf("pairing code: %w", err)
	}
	out := make([]byte, pairingCodeLen)
	alpha := PairingAlphabet
	for i, b := range buf {
		out[i] = alpha[int(b)%len(alpha)]
	}
	return string(out), nil
}

func hashPairingCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}
