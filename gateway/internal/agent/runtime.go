// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mqglobal/goso/gateway/internal/approval"
	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/pipeline"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// BoundTool is a tool advertised to the LLM, namespaced by connector.
type BoundTool struct {
	Connector string         `json:"connector"`
	Tool      connector.Tool `json:"tool"`
}

// Trace records one tool attempt.
type Trace struct {
	Connector  string `json:"connector"`
	Tool       string `json:"tool"`
	LatencyMS  int64  `json:"latency_ms"`
	Status     string `json:"status"`
	ApprovalID string `json:"approval_id,omitempty"`
	Error      string `json:"error,omitempty"`
}

// ChatResult is returned by Runtime.Chat.
type ChatResult struct {
	Reply     string  `json:"reply"`
	SessionID string  `json:"session_id"`
	Trace     []Trace `json:"trace,omitempty"`
}

// CallResult is a single tool-layer invocation (direct or from chat).
type CallResult struct {
	Result  *connector.InvokeResult `json:"result,omitempty"`
	Trace   Trace                   `json:"trace"`
	Pending bool                    `json:"pending,omitempty"`
}

// Runtime is the Tool Layer: connector tools + LLM + session messages.
type Runtime struct {
	Store     store.StoreIface
	Registry  *connector.Registry
	Gate      *approval.Gate
	Events    *eventstore.Store
	LLM       llm.Provider
	Hooks     *pipeline.Dispatcher
	Memory    pipeline.StageFunc
	Summarize pipeline.StageFunc
}

// New constructs a Runtime. Missing deps are allocated empty.
func New(st store.StoreIface, reg *connector.Registry, gate *approval.Gate, ev *eventstore.Store, provider llm.Provider) *Runtime {
	if reg == nil {
		reg = connector.NewRegistry()
	}
	if gate == nil {
		gate = approval.New(0)
	}
	if ev == nil {
		ev = eventstore.New(256)
	}
	if provider == nil {
		provider = llm.Echo{}
	}
	return &Runtime{Store: st, Registry: reg, Gate: gate, Events: ev, LLM: provider, Hooks: pipeline.NewDispatcher()}
}

// ListTools returns tools from connectors linked to the agent (or all if none linked).
func (rt *Runtime) ListTools(ctx context.Context, agentID string) ([]BoundTool, error) {
	names := rt.connectorNames(agentID)
	var out []BoundTool
	for _, name := range names {
		c, err := rt.Registry.Lookup(name)
		if err != nil {
			continue
		}
		tools, err := c.ListTools(ctx)
		if err != nil {
			continue
		}
		for _, t := range tools {
			out = append(out, BoundTool{Connector: name, Tool: t})
		}
	}
	return out, nil
}

func (rt *Runtime) connectorNames(agentID string) []string {
	if agentID != "" && rt.Store != nil {
		if linked, err := rt.Store.ListAgentConnectors(agentID); err == nil && len(linked) > 0 {
			return linked
		}
	}
	var names []string
	for _, c := range rt.Registry.List() {
		names = append(names, c.Name())
	}
	return names
}

// CallTool is the Tool Layer entry: approval gate first, then Invoke.
func (rt *Runtime) CallTool(ctx context.Context, connectorName, tool string, args map[string]any) (*CallResult, error) {
	traceID := eventstore.NewTraceID()
	start := time.Now()
	rt.Events.Append(eventstore.Event{
		TraceID:   traceID,
		Connector: connectorName,
		Tool:      tool,
		Kind:      eventstore.KindAttempt,
		Summary:   eventstore.SummarizeArgs(args),
	})

	c, err := rt.Registry.Lookup(connectorName)
	if err != nil {
		return rt.fail(traceID, connectorName, tool, start, err)
	}
	tools, err := c.ListTools(ctx)
	if err != nil {
		return rt.fail(traceID, connectorName, tool, start, err)
	}
	var meta connector.Tool
	found := false
	for _, t := range tools {
		if t.Name == tool {
			meta = t
			found = true
			break
		}
	}
	if !found {
		return rt.fail(traceID, connectorName, tool, start, fmt.Errorf("unknown tool %q", tool))
	}

	if meta.RequiresApproval {
		req := rt.Gate.Submit(connectorName, tool, args, map[string]any{
			"requires_approval": true,
			"description":       meta.Description,
		})
		lat := time.Since(start)
		rt.Events.Append(eventstore.Event{
			TraceID:   traceID,
			Connector: connectorName,
			Tool:      tool,
			Kind:      eventstore.KindPendingApproval,
			Summary:   fmt.Sprintf(`{"approval_id":%q,"status":"pending_approval"}`, req.ID),
		})
		return &CallResult{
			Pending: true,
			Result: &connector.InvokeResult{
				Tool:             tool,
				Connector:        connectorName,
				Status:           "pending_approval",
				ApprovalID:       req.ID,
				RequiresApproval: true,
				Content:          map[string]any{"pending_approval": true, "approval_id": req.ID},
				Latency:          lat,
				LatencyMS:        lat.Milliseconds(),
			},
			Trace: Trace{
				Connector:  connectorName,
				Tool:       tool,
				LatencyMS:  lat.Milliseconds(),
				Status:     "pending_approval",
				ApprovalID: req.ID,
			},
		}, nil
	}

	res, err := c.Invoke(ctx, tool, args)
	lat := time.Since(start)
	if err != nil {
		return rt.fail(traceID, connectorName, tool, start, err)
	}
	if res == nil {
		res = &connector.InvokeResult{Tool: tool, Connector: connectorName, Status: "ok"}
	}
	res.Latency = lat
	res.LatencyMS = lat.Milliseconds()
	if res.Status == "" {
		res.Status = "ok"
	}
	rt.Events.Append(eventstore.Event{
		TraceID:   traceID,
		Connector: connectorName,
		Tool:      tool,
		Kind:      eventstore.KindSuccess,
		Summary:   eventstore.Redact(asJSON(map[string]any{"status": res.Status, "latency_ms": lat.Milliseconds()})),
	})
	return &CallResult{
		Result: res,
		Trace: Trace{
			Connector: connectorName,
			Tool:      tool,
			LatencyMS: lat.Milliseconds(),
			Status:    res.Status,
		},
	}, nil
}

func (rt *Runtime) fail(traceID, connectorName, tool string, start time.Time, err error) (*CallResult, error) {
	lat := time.Since(start)
	kind := eventstore.KindError
	if errors.Is(err, connector.ErrUnavailable) {
		kind = eventstore.KindUnavailable
	}
	rt.Events.Append(eventstore.Event{
		TraceID:   traceID,
		Connector: connectorName,
		Tool:      tool,
		Kind:      kind,
		Summary:   eventstore.Redact(err.Error()),
	})
	tr := Trace{
		Connector: connectorName,
		Tool:      tool,
		LatencyMS: lat.Milliseconds(),
		Status:    "error",
		Error:     err.Error(),
	}
	if errors.Is(err, connector.ErrUnavailable) {
		tr.Status = "connector_unavailable"
	}
	return &CallResult{Trace: tr}, err
}

// Chat runs the 8-stage pipeline with the default prompt mode (full).
func (rt *Runtime) Chat(ctx context.Context, sessionID, message string) (*ChatResult, error) {
	return rt.ChatWithMode(ctx, sessionID, message, "")
}

// ChatWithMode runs the pipeline with an explicit prompt_mode (empty = full).
func (rt *Runtime) ChatWithMode(ctx context.Context, sessionID, message, promptMode string) (*ChatResult, error) {
	return rt.ChatOpts(ctx, sessionID, message, promptMode, false)
}

// ChatOpts is ChatWithMode plus a summarize flag (summarize=1).
func (rt *Runtime) ChatOpts(ctx context.Context, sessionID, message, promptMode string, summarize bool) (*ChatResult, error) {
	if rt.Store == nil {
		return nil, errors.New("store required")
	}
	mode, err := pipeline.ParseMode(promptMode)
	if err != nil {
		return nil, err
	}
	hooks := rt.Hooks
	if hooks == nil {
		hooks = pipeline.NewDispatcher()
	}
	runner := pipeline.NewRunner(rt.Store, runtimeTools{rt: rt}, rt.LLM, hooks)
	if rt.Memory != nil {
		runner.Memory = rt.Memory
	}
	if rt.Summarize != nil {
		runner.Summarize = rt.Summarize
	}
	runner.ForceSummarize = summarize
	out, err := runner.Run(ctx, sessionID, message, mode)
	if err != nil {
		return nil, err
	}
	res := &ChatResult{SessionID: sessionID}
	if out != nil {
		res.Reply = out.Reply
		res.Trace = toAgentTraces(out.Trace)
	}
	return res, nil
}

type runtimeTools struct{ rt *Runtime }

func (a runtimeTools) List(ctx context.Context, agentID string) ([]llm.ToolSpec, error) {
	tools, err := a.rt.ListTools(ctx, agentID)
	if err != nil {
		return nil, err
	}
	out := make([]llm.ToolSpec, 0, len(tools))
	for _, bt := range tools {
		out = append(out, llm.ToolSpec{
			Name:        pipeline.AdvertiseName(bt.Connector, bt.Tool.Name),
			Description: bt.Tool.Description,
			Connector:   bt.Connector,
			InputSchema: bt.Tool.InputSchema,
		})
	}
	return out, nil
}

func (a runtimeTools) Call(ctx context.Context, call llm.ToolCall) (pipeline.CallOutcome, error) {
	conn, tool := pipeline.ResolveCall(call)
	cr, err := a.rt.CallTool(ctx, conn, tool, call.Arguments)
	out := pipeline.CallOutcome{}
	if cr != nil {
		out.Pending = cr.Pending
		out.Trace = pipeline.ToolTrace{
			Connector:  cr.Trace.Connector,
			Tool:       cr.Trace.Tool,
			LatencyMS:  cr.Trace.LatencyMS,
			Status:     cr.Trace.Status,
			ApprovalID: cr.Trace.ApprovalID,
			Error:      cr.Trace.Error,
		}
		if err != nil {
			out.Content = asJSON(map[string]any{"error": err.Error(), "trace": cr.Trace})
		} else if cr.Result != nil {
			out.Content = asJSON(cr.Result)
		}
	} else if err != nil {
		out.Trace = pipeline.ToolTrace{Connector: conn, Tool: tool, Status: "error", Error: err.Error()}
		out.Content = err.Error()
	}
	return out, err
}

func toAgentTraces(in []pipeline.ToolTrace) []Trace {
	if len(in) == 0 {
		return nil
	}
	out := make([]Trace, len(in))
	for i, t := range in {
		out[i] = Trace{
			Connector:  t.Connector,
			Tool:       t.Tool,
			LatencyMS:  t.LatencyMS,
			Status:     t.Status,
			ApprovalID: t.ApprovalID,
			Error:      t.Error,
		}
	}
	return out
}

func asJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
