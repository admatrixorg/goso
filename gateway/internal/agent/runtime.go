// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/approval"
	"github.com/mqglobal/goso/gateway/internal/builtin"
	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/eventstore"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/observe"
	"github.com/mqglobal/goso/gateway/internal/pipeline"
	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/team"
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
	Reply     string         `json:"reply"`
	SessionID string         `json:"session_id"`
	Trace     []Trace        `json:"trace,omitempty"`
	Spans     []observe.Span `json:"spans,omitempty"`
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
	Observer  *observe.Observer
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
	rt := &Runtime{Store: st, Registry: reg, Gate: gate, Events: ev, LLM: provider, Hooks: pipeline.NewDispatcher()}
	rt.attachExecutor()
	return rt
}

type callMetaKey struct{}

type callMeta struct {
	AgentID   string
	SessionID string
	Requester string
}

// WithCallMeta attaches requester/agent/session for the approval inbox.
func WithCallMeta(ctx context.Context, agentID, sessionID, requester string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	cur := callMetaFrom(ctx)
	if strings.TrimSpace(agentID) == "" {
		agentID = cur.AgentID
	}
	if strings.TrimSpace(sessionID) == "" {
		sessionID = cur.SessionID
	}
	if strings.TrimSpace(requester) == "" {
		requester = cur.Requester
	}
	return context.WithValue(ctx, callMetaKey{}, callMeta{AgentID: agentID, SessionID: sessionID, Requester: requester})
}

func callMetaFrom(ctx context.Context) callMeta {
	if ctx == nil {
		return callMeta{}
	}
	if v, ok := ctx.Value(callMetaKey{}).(callMeta); ok {
		return v
	}
	return callMeta{}
}

func (rt *Runtime) attachExecutor() {
	if rt == nil || rt.Gate == nil {
		return
	}
	rt.Gate.Executor = func(ctx context.Context, req *approval.Request) error {
		if req == nil {
			return nil
		}
		if req.Connector != builtin.ConnectorName && !builtin.IsName(req.Tool) {
			return nil
		}
		enabled := false
		if rt.Store != nil {
			enabled = rt.Store.GetToolFlag(req.Tool)
			if req.AgentID != "" {
				if en, ok := rt.Store.GetAgentToolFlag(req.AgentID, req.Tool); ok {
					enabled = en
				}
			}
		}
		_, err := builtin.Invoke(ctx, req.Tool, req.Args, enabled)
		return err
	}
}

// ListTools returns tools from connectors linked to the agent (or all if none linked).
func (rt *Runtime) ListTools(ctx context.Context, agentID string) ([]BoundTool, error) {
	names := rt.connectorNames(agentID)
	var out []BoundTool
	for _, name := range names {
		c, err := peekConnector(rt.Registry, name)
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
	for _, t := range builtin.Tools() {
		out = append(out, BoundTool{Connector: builtin.ConnectorName, Tool: t})
	}
	return out, nil
}

func peekConnector(reg *connector.Registry, name string) (connector.Connector, error) {
	if reg == nil {
		return nil, connector.ErrNotFound
	}
	c, _, err := reg.Peek(name)
	return c, err
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

	if err := security.RejectPathArgs(args); err != nil {
		return rt.fail(traceID, connectorName, tool, start, err)
	}

	if connectorName == builtin.ConnectorName || builtin.IsName(tool) && connectorName == "" {
		return rt.callBuiltin(ctx, traceID, tool, args, start)
	}

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
		return rt.pendingApproval(ctx, traceID, connectorName, tool, args, map[string]any{
			"requires_approval": true,
			"description":       meta.Description,
		}, start), nil
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

func (rt *Runtime) callBuiltin(ctx context.Context, traceID, tool string, args map[string]any, start time.Time) (*CallResult, error) {
	if builtin.RequiresApproval(tool) {
		return rt.pendingApproval(ctx, traceID, builtin.ConnectorName, tool, args, map[string]any{
			"requires_approval": true,
		}, start), nil
	}
	enabled := false
	if rt.Store != nil {
		enabled = rt.Store.GetToolFlag(tool)
	}
	res, err := builtin.Invoke(ctx, tool, args, enabled)
	lat := time.Since(start)
	if err != nil {
		return rt.fail(traceID, builtin.ConnectorName, tool, start, err)
	}
	if res == nil {
		res = notConfiguredResult(tool)
	}
	res.Latency = lat
	res.LatencyMS = lat.Milliseconds()
	kind := eventstore.KindSuccess
	if res.Status != "ok" {
		kind = eventstore.KindError
	}
	rt.Events.Append(eventstore.Event{
		TraceID:   traceID,
		Connector: builtin.ConnectorName,
		Tool:      tool,
		Kind:      kind,
		Summary:   eventstore.Redact(asJSON(map[string]any{"status": res.Status, "latency_ms": lat.Milliseconds()})),
	})
	return &CallResult{
		Result: res,
		Trace: Trace{
			Connector: builtin.ConnectorName,
			Tool:      tool,
			LatencyMS: lat.Milliseconds(),
			Status:    res.Status,
		},
	}, nil
}

func (rt *Runtime) pendingApproval(ctx context.Context, traceID, connectorName, tool string, args map[string]any, proof map[string]any, start time.Time) *CallResult {
	meta := callMetaFrom(ctx)
	requester := meta.Requester
	if requester == "" && meta.AgentID != "" {
		requester = "agent:" + meta.AgentID
	}
	if requester == "" && meta.SessionID != "" {
		requester = "session:" + meta.SessionID
	}
	req := rt.Gate.SubmitMeta(approval.SubmitIn{
		Connector: connectorName,
		Tool:      tool,
		Args:      args,
		Proof:     proof,
		Requester: requester,
		AgentID:   meta.AgentID,
		SessionID: meta.SessionID,
	})
	lat := time.Since(start)
	rt.Events.Append(eventstore.Event{
		TraceID:   traceID,
		Connector: connectorName,
		Tool:      tool,
		Kind:      eventstore.KindPendingApproval,
		Summary:   fmt.Sprintf(`{"approval_id":%q,"status":"pending_approval"}`, req.ID),
		AgentID:   meta.AgentID,
		Actor:     requester,
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
	}
}

func notConfiguredResult(tool string) *connector.InvokeResult {
	return &connector.InvokeResult{
		Tool:      tool,
		Connector: builtin.ConnectorName,
		Status:    "not_configured",
		Content:   map[string]any{"error": "not_configured"},
	}
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

// Chat runs the 8-stage pipeline. Empty request mode uses the session, else full.
func (rt *Runtime) Chat(ctx context.Context, sessionID, message string) (*ChatResult, error) {
	return rt.ChatWithMode(ctx, sessionID, message, "")
}

// ChatWithMode runs the pipeline with an explicit prompt_mode (empty = session, else full).
func (rt *Runtime) ChatWithMode(ctx context.Context, sessionID, message, promptMode string) (*ChatResult, error) {
	return rt.ChatOpts(ctx, sessionID, message, promptMode, false)
}

// ChatOpts is ChatWithMode plus a summarize flag (summarize=1).
func (rt *Runtime) ChatOpts(ctx context.Context, sessionID, message, promptMode string, summarize bool) (*ChatResult, error) {
	return rt.ChatOptsStream(ctx, sessionID, message, promptMode, summarize, nil)
}

// ChatOptsStream is ChatOpts with a per-delta callback (nil = non-stream).
func (rt *Runtime) ChatOptsStream(ctx context.Context, sessionID, message, promptMode string, summarize bool, onDelta llm.StreamHandler) (*ChatResult, error) {
	if rt.Store == nil {
		return nil, errors.New("store required")
	}
	if strings.TrimSpace(promptMode) != "" {
		if _, err := pipeline.ParseMode(promptMode); err != nil {
			return nil, err
		}
	}
	hooks := rt.Hooks
	if hooks == nil {
		hooks = pipeline.NewDispatcher()
	}
	provider := rt.LLM
	sessionMode := ""
	if sess, e := rt.Store.GetSession(sessionID); e == nil && sess != nil {
		sessionMode = sess.PromptMode
		ctx = WithCallMeta(ctx, sess.AgentID, sess.ID, "agent:"+sess.AgentID)
		if a, e := rt.Store.GetAgent(sess.AgentID); e == nil && a != nil {
			p, rerr := llm.Resolve(rt.Store, a.LLMProvider, a.Model, rt.LLM)
			if rerr != nil {
				return nil, rerr
			}
			provider = p
		}
	}
	mode, err := pipeline.ResolvePromptMode(promptMode, sessionMode)
	if err != nil {
		return nil, err
	}
	runner := pipeline.NewRunner(rt.Store, runtimeTools{rt: rt}, provider, hooks)
	if rt.Memory != nil {
		runner.Memory = rt.Memory
	}
	if rt.Summarize != nil {
		runner.Summarize = rt.Summarize
	}
	runner.ForceSummarize = summarize
	runner.OnDelta = onDelta
	out, err := runner.Run(ctx, sessionID, message, mode)
	if out != nil && rt.Observer != nil {
		rt.Observer.RecordSpans(out.Spans)
	}
	if err != nil {
		return nil, err
	}
	if sess, e := rt.Store.GetSession(sessionID); e == nil && sess != nil {
		rt.Store.RecordChatRun(sess.AgentID)
	}
	res := &ChatResult{SessionID: sessionID}
	if out != nil {
		res.Reply = out.Reply
		res.Trace = toAgentTraces(out.Trace)
		res.Spans = out.Spans
	}
	return res, nil
}

type runtimeTools struct{ rt *Runtime }

func (a runtimeTools) List(ctx context.Context, agentID string) ([]llm.ToolSpec, error) {
	tools, err := a.rt.ListTools(ctx, agentID)
	if err != nil {
		return nil, err
	}
	out := make([]llm.ToolSpec, 0, len(tools)+7)
	for _, bt := range tools {
		out = append(out, llm.ToolSpec{
			Name:        pipeline.AdvertiseName(bt.Connector, bt.Tool.Name),
			Description: bt.Tool.Description,
			Connector:   bt.Connector,
			InputSchema: bt.Tool.InputSchema,
		})
	}
	mode := team.ResolveMode(a.rt.Store, agentID)
	out = append(out, team.ToolSpecs(mode)...)
	out = append(out, pipeline.MemoryToolSpecs()...)
	out = append(out, pipeline.SessionToolSpecs()...)
	if a.rt.Store != nil && agentID != "" {
		names := make([]string, 0, len(out))
		for _, t := range out {
			names = append(names, t.Name)
		}
		a.rt.Store.RecordAdvertisedTools(agentID, names)
	}
	return out, nil
}

func (a runtimeTools) Call(ctx context.Context, agentID string, call llm.ToolCall) (pipeline.CallOutcome, error) {
	if pipeline.IsMemoryTool(call.Name) || pipeline.IsSessionTool(call.Name) {
		tid := store.DefaultTenant
		if a.rt.Store != nil && agentID != "" {
			if ag, err := a.rt.Store.GetAgent(agentID); err == nil && ag != nil {
				tid = store.NormalizeTenant(ag.TenantID)
			}
		}
		var body string
		var err error
		if pipeline.IsMemoryTool(call.Name) {
			body, err = pipeline.DispatchMemoryTool(a.rt.Store, tid, call)
		} else {
			body, err = pipeline.DispatchSessionTool(a.rt.Store, tid, call)
		}
		failed := err != nil
		if a.rt.Store != nil {
			a.rt.Store.RecordToolUse(agentID, call.Name, failed)
		}
		out := pipeline.CallOutcome{Content: body}
		out.Trace.Tool = call.Name
		if failed {
			out.Trace.Status = "error"
			out.Trace.Error = err.Error()
			if out.Content == "" {
				out.Content = err.Error()
			}
			return out, err
		}
		out.Trace.Status = "ok"
		return out, nil
	}
	if pipeline.IsOrchestrationTool(call.Name) {
		svc := &team.Service{Store: a.rt.Store, Chat: a.rt.chatText}
		body, err := svc.Dispatch(ctx, agentID, call)
		failed := err != nil
		if a.rt.Store != nil {
			a.rt.Store.RecordToolUse(agentID, call.Name, failed)
		}
		out := pipeline.CallOutcome{Content: body}
		out.Trace.Tool = call.Name
		if failed {
			out.Trace.Status = "error"
			out.Trace.Error = err.Error()
			if out.Content == "" {
				out.Content = err.Error()
			}
			return out, err
		}
		out.Trace.Status = "ok"
		return out, nil
	}
	conn, tool := pipeline.ResolveCall(call)
	ctx = WithCallMeta(ctx, agentID, "", "agent:"+agentID)
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
	if a.rt.Store != nil {
		failed := err != nil || out.Trace.Status == "error" || out.Trace.Status == "connector_unavailable"
		a.rt.Store.RecordToolUse(agentID, pipeline.AdvertiseName(conn, tool), failed)
	}
	return out, err
}

func (rt *Runtime) chatText(ctx context.Context, sessionID, message string) (string, error) {
	out, err := rt.Chat(ctx, sessionID, message)
	if err != nil {
		return "", err
	}
	if out == nil {
		return "", nil
	}
	return out.Reply, nil
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
