// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import "testing"

func TestRewriteLoopbackURL(t *testing.T) {
	t.Setenv("GOSO_LOOPBACK_HOST", "")
	if got := rewriteLoopbackURL("http://localhost:20128/v1"); got != "http://localhost:20128/v1" {
		t.Fatalf("empty env %s", got)
	}

	t.Setenv("GOSO_LOOPBACK_HOST", "host.docker.internal")
	cases := []struct {
		in, want string
	}{
		{"http://localhost:20128/v1", "http://host.docker.internal:20128/v1"},
		{"http://127.0.0.1:20128/v1", "http://host.docker.internal:20128/v1"},
		{"http://[::1]:20128/v1", "http://host.docker.internal:20128/v1"},
		{"https://api.openai.com/v1", "https://api.openai.com/v1"},
		{"http://9router:20128/v1", "http://9router:20128/v1"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := rewriteLoopbackURL(tc.in); got != tc.want {
			t.Fatalf("%s: got %s want %s", tc.in, got, tc.want)
		}
	}
}

func TestBuildRewritesLoopback(t *testing.T) {
	t.Setenv("GOSO_LOOPBACK_HOST", "host.docker.internal")
	p, err := Build("9ai", TypeOpenAICompat, "http://localhost:20128/v1", "gcli/grok-4.5", "k")
	if err != nil {
		t.Fatal(err)
	}
	o, ok := p.(*OpenAI)
	if !ok {
		t.Fatalf("type %T", p)
	}
	if o.BaseURL != "http://host.docker.internal:20128/v1" {
		t.Fatalf("base %s", o.BaseURL)
	}
}
