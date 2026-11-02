// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package bridge

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/jev"
)

// knowledgeBlock is the system-prompt knowledge section. With Jev off, or
// with no API key, it is knowledgePrompt unchanged. With Jev on, only the
// chunks that already fit the cap are scored, and a chunk is removed only
// when Jev's noul is at or below the configured no threshold. Any error,
// timeout, uncertain score, or call cap keeps that chunk.
func knowledgeBlock(ctx context.Context, tenant, agentID, prompt string, ag Agent, docs []KnowledgeDoc) string {
	capN := knowledgeMaxBytes(ag)
	if !jev.Active() {
		return knowledgePrompt(docs, capN)
	}
	included, _ := walkKnowledge(docs, capN)
	if len(included) == 0 {
		return knowledgePrompt(docs, capN)
	}
	passages := make([]jev.Passage, len(included))
	for i, d := range included {
		passages[i] = jev.Passage{ID: passageKey(i, d), Name: d.FileName, Content: d.Content}
	}
	log.Printf("goso jev: knowledge tenant=%s agent=%s candidates=%d", tenant, agentID, len(passages))
	kept := jev.FilterPassages(ctx, prompt, passages)
	if samePassageKeys(passages, kept) {
		return knowledgePrompt(docs, capN)
	}
	keep := map[string]struct{}{}
	for _, p := range kept {
		keep[p.ID] = struct{}{}
	}
	out := make([]KnowledgeDoc, 0, len(kept))
	for i, d := range included {
		if _, ok := keep[passageKey(i, d)]; ok {
			out = append(out, d)
		}
	}
	return knowledgePrompt(out, capN)
}

func passageKey(i int, d KnowledgeDoc) string {
	id := strings.TrimSpace(d.ID)
	if id == "" {
		id = d.FileName
	}
	return fmt.Sprintf("%d:%s", i, id)
}

func samePassageKeys(in, kept []jev.Passage) bool {
	if len(in) != len(kept) {
		return false
	}
	for i := range in {
		if in[i].ID != kept[i].ID {
			return false
		}
	}
	return true
}
