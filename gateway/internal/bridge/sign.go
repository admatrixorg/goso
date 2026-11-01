// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package bridge

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"time"
)

const (
	headerTenant    = "x-goclaw-tenant"
	headerAgent     = "x-goclaw-agent"
	headerTimestamp = "x-goclaw-timestamp"
	headerSignature = "x-goclaw-signature"
	headerNonce     = "x-goclaw-nonce"

	defaultSkew = 5 * time.Minute
)

var (
	errMissingSig = &Error{Status: http.StatusUnauthorized, Msg: "missing signature"}
	errBadSig     = &Error{Status: http.StatusUnauthorized, Msg: "invalid signature"}
	errExpired    = &Error{Status: http.StatusUnauthorized, Msg: "timestamp expired"}
	errBadTime    = &Error{Status: http.StatusUnauthorized, Msg: "invalid timestamp"}
)

// verifySignature checks the Omni-Core x-goclaw-* HMAC.
// The primary payload is timestamp bytes concatenated with payload (raw HTTP
// body, or the prompt string for gRPC). When x-goclaw-nonce is set, the
// dotted form timestamp.nonce.payload is also accepted.
// An empty secret fails closed.
func verifySignature(secret, ts, sig, nonce string, payload []byte, now time.Time, skew time.Duration) error {
	if secret == "" || ts == "" || sig == "" {
		return errMissingSig
	}
	if skew <= 0 {
		skew = defaultSkew
	}
	sec, err := parseUnix(ts)
	if err != nil {
		return errBadTime
	}
	delta := now.Unix() - sec
	if delta < 0 {
		delta = -delta
	}
	if delta > int64(skew/time.Second) {
		return errExpired
	}
	got, err := hex.DecodeString(sig)
	if err != nil || len(got) != sha256.Size {
		return errBadSig
	}
	if macEqual(secret, append([]byte(ts), payload...), got) {
		return nil
	}
	if nonce != "" {
		dotted := ts + "." + nonce + "." + string(payload)
		if macEqual(secret, []byte(dotted), got) {
			return nil
		}
	}
	return errBadSig
}

func parseUnix(ts string) (int64, error) {
	n, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return 0, err
	}
	if n > 1e11 {
		n /= 1000
	}
	if n <= 0 {
		return 0, errors.New("timestamp")
	}
	return n, nil
}

func macEqual(secret string, payload, got []byte) bool {
	m := hmac.New(sha256.New, []byte(secret))
	_, _ = m.Write(payload)
	return hmac.Equal(m.Sum(nil), got)
}

func sign(secret, ts string, payload []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	_, _ = m.Write([]byte(ts))
	_, _ = m.Write(payload)
	return hex.EncodeToString(m.Sum(nil))
}
