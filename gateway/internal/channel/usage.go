// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"github.com/mqglobal/goso/gateway/internal/billing"
	"github.com/mqglobal/goso/gateway/internal/llm"
)

func trackUsage(meter *billing.Store, agentID, provider string, u llm.Usage) {
	if meter == nil {
		return
	}
	meter.AddCall(agentID, provider, u.PromptTokens, u.CompletionTokens, u.Estimated)
}
