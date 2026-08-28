// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package health

// Kind maps an HTTP /healthz probe to control-plane chrome state.
// status <= 0 means the request never completed (network or timeout).
func Kind(status int, ok bool) string {
	switch {
	case status == 401 || status == 403:
		return "unauthorized"
	case status <= 0:
		return "offline"
	case status == 200 && ok:
		return "connected"
	default:
		return "degraded"
	}
}
