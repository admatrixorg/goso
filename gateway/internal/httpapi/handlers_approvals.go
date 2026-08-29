// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/approval"
	"github.com/mqglobal/goso/gateway/internal/auditlog"
	"github.com/mqglobal/goso/gateway/internal/eventstore"
)

func registerApprovalRoutes(mux *http.ServeMux, opt Options) {
	aliasAPI(mux, "GET /api/approvals", handleListApprovals(opt))
	aliasAPI(mux, "GET /api/approvals/{id}", handleGetApproval(opt))
	aliasAPI(mux, "POST /api/approvals/{id}/decision", handleApprovalDecision(opt))
}

func handleListApprovals(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
		rows := opt.Gate.List(status)
		pub := approval.PublicList(rows)
		pending := 0
		for _, row := range pub {
			if row.Status == approval.StatusPending {
				pending++
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"approvals":    pub,
			"pending":      pending,
			"generated_at": time.Now().UTC(),
		})
	}
}

func handleGetApproval(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, err := opt.Gate.Get(r.PathValue("id"))
		if err != nil && !errors.Is(err, approval.ErrExpired) {
			status := http.StatusNotFound
			if errors.Is(err, approval.ErrNotPending) {
				status = http.StatusConflict
			}
			writeErr(w, status, err.Error())
			return
		}
		if req == nil {
			writeErr(w, http.StatusNotFound, approval.ErrNotFound.Error())
			return
		}
		writeJSON(w, http.StatusOK, approval.Public(req))
	}
}

func handleApprovalDecision(opt Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body struct {
			Decision string `json:"decision"`
			Reason   string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid json")
			return
		}
		decision := approval.NormalizeDecision(body.Decision)
		reason := strings.TrimSpace(body.Reason)
		if decision == approval.DecisionReject && reason == "" {
			writeErr(w, http.StatusBadRequest, approval.ErrReasonRequired.Error())
			return
		}
		req, err := opt.Gate.DecideReason(r.Context(), id, decision, reason)
		if err != nil {
			status := http.StatusBadRequest
			switch {
			case errors.Is(err, approval.ErrNotFound):
				status = http.StatusNotFound
			case errors.Is(err, approval.ErrNotPending):
				status = http.StatusConflict
			case errors.Is(err, approval.ErrExpired):
				status = http.StatusGone
			}
			writeErr(w, status, err.Error())
			return
		}
		if opt.Events != nil {
			opt.Events.Append(eventstore.Event{
				Connector: req.Connector,
				Tool:      req.Tool,
				Kind:      eventstore.KindHumanFeedback,
				Actor:     operatorActor(r),
				AgentID:   req.AgentID,
				Summary: eventstore.SummarizeArgs(map[string]any{
					"approval_id": req.ID,
					"decision":    req.Decision,
					"status":      req.Status,
				}),
			})
		}
		action := "approve"
		if req.Decision == approval.DecisionReject {
			action = "deny"
		}
		recordAudit(opt.Audit, r, auditlog.Record{
			Action:   action,
			Entity:   "approval",
			EntityID: req.ID,
			After: auditMeta(true, map[string]any{
				"status":    req.Status,
				"tool":      req.Tool,
				"connector": req.Connector,
				"agent_id":  req.AgentID,
				"requester": req.Requester,
				"risk":      req.Risk,
				"reason":    req.Reason,
			}),
		})
		writeJSON(w, http.StatusOK, approval.Public(req))
	}
}
