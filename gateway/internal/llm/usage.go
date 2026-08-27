// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"context"

	"github.com/mqglobal/goso/gateway/internal/billing"
)

// Usage is token counts for one Chat call.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	CacheReadTokens  int // prompt-cache reads; default 0
	Estimated        bool
}

// EstimateUsage uses billing.EstimateTokens (ceil(len/4)) for prompt + completion.
func EstimateUsage(messages []Message, completion string) Usage {
	prompt := 0
	for _, m := range messages {
		prompt += billing.EstimateTokens(m.Content)
	}
	return Usage{
		PromptTokens:     prompt,
		CompletionTokens: billing.EstimateTokens(completion),
		CacheReadTokens:  0,
		Estimated:        true,
	}
}

type usageProvider interface {
	ChatUsage(ctx context.Context, messages []Message) (string, Usage, error)
}

// ChatUsage calls the provider and returns token usage (actual or estimated).
func ChatUsage(ctx context.Context, p Provider, messages []Message) (string, Usage, error) {
	if p == nil {
		p = Echo{}
	}
	if u, ok := p.(usageProvider); ok {
		return u.ChatUsage(ctx, messages)
	}
	s, err := p.Chat(ctx, messages)
	if err != nil {
		return s, Usage{}, err
	}
	return s, EstimateUsage(messages, s), nil
}

func fallbackUsage(messages []Message, completion string, prompt, completionTok int) Usage {
	if prompt > 0 || completionTok > 0 {
		return Usage{PromptTokens: prompt, CompletionTokens: completionTok, Estimated: false}
	}
	return EstimateUsage(messages, completion)
}
