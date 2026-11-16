// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/vault"
)

// embeddingUsedFor names what the vector index serves. Memory and KG search
// stay lexical; their status reports the shared embedding state.
var embeddingUsedFor = []string{"vault"}

func embeddingState(r *http.Request) string {
	return vault.Status(requestTenant(r)).State
}

func embeddingConfigured(r *http.Request) bool {
	return embeddingState(r) == vault.EmbedReady
}

// vaultViewerFromRequest is the operator view of the tenant, or with
// ?agent_id= the view of that agent (tenant docs, its own, its teams'). An
// agent of another tenant is a 400.
func vaultViewerFromRequest(w http.ResponseWriter, r *http.Request, st store.StoreIface) (store.VaultViewer, bool) {
	ref := strings.TrimSpace(r.URL.Query().Get("agent_id"))
	if ref == "" {
		return store.OperatorView, true
	}
	tid := requestTenant(r)
	v := vault.ViewerForAgent(st, tid, ref, false, "")
	if v.AgentID == "" {
		writeErr(w, http.StatusBadRequest, "agent_id is not an agent of this tenant")
		return store.VaultViewer{}, false
	}
	return v, true
}

func vaultDocsVisible(list []*store.VaultDoc, v store.VaultViewer) []*store.VaultDoc {
	out := make([]*store.VaultDoc, 0, len(list))
	for _, d := range list {
		if v.Allows(d) {
			out = append(out, d)
		}
	}
	return out
}

// handleVaultEmbeddings reports the embedding state of the request tenant.
func handleVaultEmbeddings() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tid := requestTenant(r)
		writeJSON(w, http.StatusOK, map[string]any{
			"embedding": vault.Status(tid),
			"backfill":  vault.LastBackfill(tid),
		})
	}
}

// handleVaultBackfill embeds the request tenant's docs whose chunks are
// missing or stale. ?limit= caps docs (default 200, max 1000). By default it
// runs in the background (202); ?wait=1 runs it in the request (200).
func handleVaultBackfill() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ix := vault.ActiveIndexer()
		if ix == nil {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "embeddings not ready", "embedding": vault.Status(requestTenant(r))})
			return
		}
		limit := 200
		if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				writeErr(w, http.StatusBadRequest, "limit must be a positive integer")
				return
			}
			limit = n
		}
		if limit > vault.BackfillMax {
			limit = vault.BackfillMax
		}
		tid := requestTenant(r)
		if r.URL.Query().Get("wait") == "1" {
			res, err := ix.Backfill(r.Context(), tid, limit)
			status := http.StatusOK
			if err != nil {
				status = http.StatusBadGateway
			}
			writeJSON(w, status, map[string]any{"result": res, "embedding": vault.Status(tid)})
			return
		}
		if !ix.StartBackfill(tid, limit) {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "backfill already running", "backfill": vault.LastBackfill(tid)})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"started": true, "limit": limit})
	}
}
