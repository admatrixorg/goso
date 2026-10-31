// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package bridge

import "net/http"

// Error is a bridge failure with an HTTP status. Messages never include secrets.
// Code is a stable machine token (insufficient_funds, rate_limited, ...).
// Upstream is the provider HTTP status when the failure came from the LLM.
type Error struct {
	Status   int
	Code     string
	Upstream int
	Msg      string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Msg
}

func errStatus(status int, msg string) *Error {
	return &Error{Status: status, Msg: msg}
}

func badRequest(msg string) *Error { return errStatus(http.StatusBadRequest, msg) }
func unauthorized(msg string) *Error {
	return errStatus(http.StatusUnauthorized, msg)
}
func notFound(msg string) *Error { return errStatus(http.StatusNotFound, msg) }
func unavailable(msg string) *Error {
	return errStatus(http.StatusServiceUnavailable, msg)
}
