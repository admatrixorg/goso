// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package pipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mqglobal/goso/gateway/internal/cron"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/store"
)

// Agent self-management tools: cron, session_status, sessions_send. Each runs
// for one calling agent inside its tenant. A customer thread (a channel
// session) may read its own status but cannot schedule jobs or post into
// other sessions: a customer must not be able to talk the agent into it.
const (
	ToolCron          = "cron"
	ToolSessionStatus = "session_status"
	ToolSessionsSend  = "sessions_send"

	// CronMessageMaxRunes caps the message a scheduled job posts.
	CronMessageMaxRunes = 2000
	// CronMinEvery is the shortest schedule an agent may create (the HTTP
	// API, used by operators, allows every:1m).
	CronMinEvery = 5 * time.Minute
	// SessionsSendMaxRunes caps one sessions_send message.
	SessionsSendMaxRunes = 4000
)

// OpsEnv is what the ops tools need from the runtime for one call.
type OpsEnv struct {
	Store           store.StoreIface
	Tenant          string
	AgentID         string
	CallerSessionID string
	// Send starts a turn in another session (the runtime runs it in the
	// background). Nil leaves sessions_send not_configured.
	Send func(sessionID, message string) error
	// Nested is true when this turn was itself started by sessions_send. Such
	// a turn may not send again, so two sessions cannot ping-pong forever.
	Nested bool
	// Now is the clock (tests); nil means time.Now.
	Now func() time.Time
}

// OpsToolSpecs are always advertised.
func OpsToolSpecs() []llm.ToolSpec {
	return []llm.ToolSpec{
		{
			Name: ToolCron,
			Description: "Schedule messages to yourself. action=list shows this agent's jobs; action=add {spec, message, session_id?} creates a job that posts message into a session of yours (default: this one) on schedule; action=remove {job_id}. " +
				"spec is every:Nm|every:Nh or a 5-field cron 'min hour dom month dow' evaluated in UTC (Asia/Ho_Chi_Minh is UTC+7, so 09:00 there is '0 2 * * *'). Fields take *, a number or */n. Shortest schedule 5 minutes; max 20 jobs per tenant. Not available in customer channel conversations.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"action":{"type":"string","enum":["list","add","remove"]},"spec":{"type":"string"},"message":{"type":"string"},"session_id":{"type":"string"},"job_id":{"type":"string"}},"required":["action"]}`),
		},
		{
			Name:        ToolSessionStatus,
			Description: "Status of the current session (or session_id of your tenant): agent, channel and chat, message count, last activity, model override. Read-only.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"session_id":{"type":"string"}}}`),
		},
		{
			Name:        ToolSessionsSend,
			Description: "Post a message into another session of yours (same agent, same tenant); that session then answers it in the background. Not your current session, not archived sessions, not from a customer channel conversation, and not from a turn that was itself started by sessions_send. Args: session_id, message.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"session_id":{"type":"string"},"message":{"type":"string"}},"required":["session_id","message"]}`),
		},
	}
}

// IsOpsTool reports cron / session_status / sessions_send. Only the bare
// advertised name matches, so a connector tool that happens to be called
// "cron" (conn__cron) still goes to its connector.
func IsOpsTool(name string) bool {
	switch strings.TrimSpace(name) {
	case ToolCron, ToolSessionStatus, ToolSessionsSend:
		return true
	default:
		return false
	}
}

// isCustomerThread reports a session that belongs to an external chat: an old
// "<channel>:<id>" label, or a tenant channel instance "<t>/<type>/<name>:<id>".
func isCustomerThread(sess *store.Session) bool {
	if sess == nil {
		return false
	}
	if IsChannelSession(sess) {
		return true
	}
	_, _, ok := splitInstanceLabel(sess.Label)
	return ok
}

// splitInstanceLabel parses "<tenant>/<type>/<name>:<chat>". The instance part
// is everything before the first ':' after the last '/'.
func splitInstanceLabel(label string) (instance, chat string, ok bool) {
	slash := strings.LastIndex(label, "/")
	if slash < 0 {
		return "", "", false
	}
	colon := strings.Index(label[slash:], ":")
	if colon < 0 {
		return "", "", false
	}
	instance, chat = label[:slash+colon], label[slash+colon+1:]
	parts := strings.Split(instance, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" || chat == "" {
		return "", "", false
	}
	return instance, chat, true
}

// channelOf names the channel and chat of a session, or "" for web/API.
func channelOf(sess *store.Session) (channel, chat string) {
	if sess == nil {
		return "", ""
	}
	if inst, c, ok := splitInstanceLabel(sess.Label); ok {
		return inst, c
	}
	for _, p := range store.ChannelLabelPrefixes {
		if strings.HasPrefix(sess.Label, p) && len(sess.Label) > len(p) {
			return strings.TrimSuffix(p, ":"), sess.Label[len(p):]
		}
	}
	return "", ""
}

func opsErr(msg string) (string, error) {
	b, _ := json.Marshal(map[string]string{"error": msg})
	return string(b), errors.New(msg)
}

func opsJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"error":"encode"}`, err
	}
	return string(b), nil
}

func (env OpsEnv) now() time.Time {
	if env.Now != nil {
		return env.Now()
	}
	return time.Now()
}

func (env OpsEnv) caller() *store.Session {
	if strings.TrimSpace(env.CallerSessionID) == "" {
		return nil
	}
	sess, err := env.Store.GetSession(env.CallerSessionID)
	if err != nil || sess == nil || !store.SameTenant(sess.TenantID, env.Tenant) {
		return nil
	}
	return sess
}

// ownSession returns a session of this tenant and this agent, or nil.
func (env OpsEnv) ownSession(id string) *store.Session {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	sess, err := env.Store.GetSession(id)
	if err != nil || sess == nil || !store.SameTenant(sess.TenantID, env.Tenant) || sess.AgentID != env.AgentID {
		return nil
	}
	return sess
}

// DispatchOpsTool runs cron / session_status / sessions_send for env.AgentID.
func DispatchOpsTool(env OpsEnv, call llm.ToolCall) (string, error) {
	if env.Store == nil {
		return opsErr("store required")
	}
	env.Tenant = store.NormalizeTenant(env.Tenant)
	if strings.TrimSpace(env.AgentID) == "" {
		return opsErr("agent context required")
	}
	if ag, err := env.Store.GetAgent(env.AgentID); err != nil || ag == nil || !store.SameTenant(ag.TenantID, env.Tenant) {
		return opsErr("agent context required")
	}
	switch strings.TrimSpace(call.Name) {
	case ToolCron:
		return dispatchCron(env, call.Arguments)
	case ToolSessionStatus:
		return dispatchSessionStatus(env, call.Arguments)
	case ToolSessionsSend:
		return dispatchSessionsSend(env, call.Arguments)
	default:
		return opsErr("unknown tool")
	}
}

// --- cron ---

func dispatchCron(env OpsEnv, args map[string]any) (string, error) {
	action := strings.ToLower(argString(args, "action"))
	if action != "list" && action != "" && isCustomerThread(env.caller()) {
		return opsErr("cron changes are not allowed from a customer channel conversation")
	}
	switch action {
	case "list", "":
		return cronList(env)
	case "add":
		return cronAdd(env, args)
	case "remove", "delete":
		return cronRemove(env, args)
	default:
		return opsErr("action must be list, add or remove")
	}
}

type cronItem struct {
	ID         string `json:"id"`
	Spec       string `json:"spec"`
	SessionID  string `json:"session_id"`
	Message    string `json:"message"`
	Enabled    bool   `json:"enabled"`
	LastRun    string `json:"last_run,omitempty"`
	LastFailed bool   `json:"last_failed,omitempty"`
}

func toCronItem(j *store.CronJob) cronItem {
	it := cronItem{ID: j.ID, Spec: j.Spec, SessionID: j.SessionID, Message: j.Message, Enabled: j.Enabled}
	if j.LastRun != nil {
		it.LastRun = j.LastRun.UTC().Format(time.RFC3339)
	}
	// The error text can carry provider details; only say that it failed.
	it.LastFailed = strings.TrimSpace(j.LastError) != ""
	return it
}

// ownJob returns a job of this tenant whose session belongs to this agent
// (and, for a customer thread, is that very thread).
func (env OpsEnv) ownJob(j *store.CronJob) bool {
	if j == nil || !store.SameTenant(j.TenantID, env.Tenant) {
		return false
	}
	sess := env.ownSession(j.SessionID)
	if sess == nil {
		return false
	}
	if c := env.caller(); isCustomerThread(c) {
		return sess.ID == c.ID
	}
	return true
}

func cronList(env OpsEnv) (string, error) {
	items := []cronItem{}
	for _, j := range env.Store.ListCronJobsTenant(env.Tenant) {
		if env.ownJob(j) {
			items = append(items, toCronItem(j))
		}
	}
	return opsJSON(map[string]any{"jobs": items, "now_utc": env.now().UTC().Format(time.RFC3339)})
}

// checkCronSpec applies the HTTP API's spec validation plus the agent floor.
func checkCronSpec(spec string) error {
	p, err := cron.ParseSpec(spec)
	if err != nil {
		return errors.New("invalid spec: use every:Nm, every:Nh or a 5-field cron 'min hour dom month dow'")
	}
	if p.Interval > 0 && p.Interval < CronMinEvery {
		return fmt.Errorf("spec runs more often than every %s", CronMinEvery)
	}
	if p.Five && p.Fields[0].Any && p.Fields[0].Step < int(CronMinEvery/time.Minute) {
		return fmt.Errorf("spec runs more often than every %s", CronMinEvery)
	}
	return nil
}

func cronAdd(env OpsEnv, args map[string]any) (string, error) {
	spec := argString(args, "spec", "schedule")
	if err := checkCronSpec(spec); err != nil {
		return opsErr(err.Error())
	}
	msg := argString(args, "message", "text")
	if msg == "" {
		return opsErr("message is required")
	}
	if len([]rune(msg)) > CronMessageMaxRunes {
		return opsErr(fmt.Sprintf("message is longer than %d characters", CronMessageMaxRunes))
	}
	if _, block := security.InspectChat(msg); block {
		return opsErr("message refused by the injection filter")
	}
	sid := argString(args, "session_id")
	if sid == "" {
		sid = env.CallerSessionID
	}
	if strings.TrimSpace(sid) == "" {
		return opsErr("session_id is required")
	}
	sess := env.ownSession(sid)
	if sess == nil {
		return opsErr("session not found")
	}
	if sess.Archived {
		return opsErr("session is archived")
	}
	if isCustomerThread(sess) {
		return opsErr("cron jobs cannot post into a customer channel conversation")
	}
	job, err := env.Store.CreateCronJob(store.CronJob{Spec: spec, SessionID: sess.ID, Message: msg, Enabled: true})
	if err != nil {
		if errors.Is(err, store.ErrCronCap) {
			return opsErr(fmt.Sprintf("cron cap: max %d jobs per tenant", store.CronJobCap))
		}
		return opsErr("could not create the job")
	}
	return opsJSON(map[string]any{"created": toCronItem(job)})
}

func cronRemove(env OpsEnv, args map[string]any) (string, error) {
	id := argString(args, "job_id", "id")
	if id == "" {
		return opsErr("job_id is required")
	}
	job, err := env.Store.GetCronJob(id)
	if err != nil || !env.ownJob(job) {
		return opsErr("job not found")
	}
	if err := env.Store.DeleteCronJob(job.ID); err != nil {
		return opsErr("job not found")
	}
	return opsJSON(map[string]any{"removed": job.ID})
}

// --- session_status ---

func dispatchSessionStatus(env OpsEnv, args map[string]any) (string, error) {
	caller := env.caller()
	sid := argString(args, "session_id")
	if sid == "" {
		if caller == nil {
			return opsErr("no current session: pass session_id")
		}
		sid = caller.ID
	}
	sess, err := env.Store.GetSession(sid)
	if err != nil || sess == nil || !store.SameTenant(sess.TenantID, env.Tenant) {
		return opsErr("session not found")
	}
	// A customer thread sees only itself; web callers see their tenant, like
	// sessions_history.
	if isCustomerThread(caller) && sess.ID != caller.ID {
		return opsErr("session not found")
	}
	msgs, _ := env.Store.ListMessages(sess.ID)
	count := 0
	last := sess.CreatedAt
	for _, m := range msgs {
		if m == nil || m.Role == "event" {
			continue
		}
		count++
		if m.CreatedAt.After(last) {
			last = m.CreatedAt
		}
	}
	out := map[string]any{
		"session_id":    sess.ID,
		"tenant_id":     store.NormalizeTenant(sess.TenantID),
		"agent_id":      sess.AgentID,
		"label":         sess.Label,
		"prompt_mode":   sess.PromptMode,
		"archived":      sess.Archived,
		"message_count": count,
		"created_at":    sess.CreatedAt.UTC().Format(time.RFC3339),
		"last_activity": last.UTC().Format(time.RFC3339),
		"current":       caller != nil && caller.ID == sess.ID,
		"now_utc":       env.now().UTC().Format(time.RFC3339),
	}
	if ag, err := env.Store.GetAgent(sess.AgentID); err == nil && ag != nil {
		out["agent_key"] = ag.AgentKey
		out["agent_name"] = ag.DisplayName
	}
	if ch, chat := channelOf(sess); ch != "" {
		out["channel"] = ch
		out["chat_id"] = chat
	} else {
		out["channel"] = "web"
	}
	if m := strings.TrimSpace(sess.ModelOverride); m != "" {
		out["model_override"] = m
	}
	return opsJSON(out)
}

// --- sessions_send ---

func dispatchSessionsSend(env OpsEnv, args map[string]any) (string, error) {
	if env.Send == nil {
		return opsErr("not_configured")
	}
	if env.Nested {
		return opsErr("a turn started by sessions_send cannot send again")
	}
	caller := env.caller()
	if isCustomerThread(caller) {
		return opsErr("sessions_send is not allowed from a customer channel conversation")
	}
	msg := argString(args, "message", "text")
	if msg == "" {
		return opsErr("message is required")
	}
	if len([]rune(msg)) > SessionsSendMaxRunes {
		return opsErr(fmt.Sprintf("message is longer than %d characters", SessionsSendMaxRunes))
	}
	if _, block := security.InspectChat(msg); block {
		return opsErr("message refused by the injection filter")
	}
	sid := argString(args, "session_id")
	if sid == "" {
		return opsErr("session_id is required")
	}
	// Same tenant and same agent only; another agent's session reads as missing.
	target := env.ownSession(sid)
	if target == nil {
		return opsErr("session not found")
	}
	if caller != nil && target.ID == caller.ID {
		return opsErr("cannot send to your current session: your reply already goes there")
	}
	if target.Archived {
		return opsErr("session is archived")
	}
	if err := env.Send(target.ID, msg); err != nil {
		return opsErr("could not start the turn")
	}
	return opsJSON(map[string]any{"status": "accepted", "session_id": target.ID})
}
