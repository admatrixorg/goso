// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package health

import "testing"

func TestKind(t *testing.T) {
	cases := []struct {
		status int
		ok     bool
		want   string
	}{
		{200, true, "connected"},
		{200, false, "degraded"},
		{502, false, "degraded"},
		{500, true, "degraded"},
		{401, false, "unauthorized"},
		{403, true, "unauthorized"},
		{0, false, "offline"},
		{-1, false, "offline"},
	}
	for _, tc := range cases {
		got := Kind(tc.status, tc.ok)
		if got != tc.want {
			t.Fatalf("Kind(%d, %v) = %q, want %q", tc.status, tc.ok, got, tc.want)
		}
	}
}
