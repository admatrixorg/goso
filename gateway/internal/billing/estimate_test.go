// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package billing

import (
	"strings"
	"testing"
)

func TestEstimateTokens(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"a", 1},
		{"abcd", 1},
		{"abcde", 2},
		{"abcdefgh", 2},
		{strings.Repeat("x", 100), 25},
		{strings.Repeat("x", 101), 26},
	}
	for _, tc := range cases {
		got := EstimateTokens(tc.in)
		if got != tc.want {
			t.Fatalf("EstimateTokens(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestEstimateTokens_CeilBytes(t *testing.T) {
	// 3 bytes -> ceil(3/4)=1; 7 bytes -> 2.
	if EstimateTokens("hi!") != 1 {
		t.Fatalf("got %d", EstimateTokens("hi!"))
	}
	if EstimateTokens("hello!!") != 2 {
		t.Fatalf("got %d", EstimateTokens("hello!!"))
	}
}
