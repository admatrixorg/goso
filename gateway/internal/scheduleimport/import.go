// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Package scheduleimport loads cron_jobs and channel_instances from an offline
// GoClaw JSON or JSONL export into goso's existing cron jobs and channel
// config. It does not open a live database by itself and it does not copy
// credentials.
//
// Every imported cron job is stored with enabled false. Every imported channel
// is stored with enabled false. channel_instances.credentials is never written
// and never logged. Each channel is reported as needs credential.
//
// Goso cron_jobs has no tenant column. The job is attached to one session that
// already exists in the destination tenant. Goso channel_config is one global
// row per name, and the live adapters look up the seven catalog names
// (telegram, zalo-personal, …). An imported instance is stored as
// "<tenant>/<catalog>/<instance>" so it does not overwrite that catalog row
// and the live manager does not start it.
//
// A second run sees import_source_ids and does not insert again. Rows that
// were already in goso are not updated or deleted.
package scheduleimport

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/mqglobal/goso/gateway/internal/cron"
	"github.com/mqglobal/goso/gateway/internal/store"
)

const (
	kindCron    = "cron_job"
	kindChannel = "channel_instance"

	// ReasonNeedsDecision is the report text for a column or row that has no
	// home in the existing goso cron or channel models.
	ReasonNeedsDecision = "not imported, needs decision"
)

// Unmapped columns have no field on the goso cron job or channel config.
// enabled is not in this list: it is stored, and the stored value is false.
var (
	UnmappedCronColumns = []string{
		"user_id",
		"name",
		"timezone",
		"run_at",
		"delete_after_run",
		"next_run_at",
		"last_run_at",
		"last_status",
		"last_error",
		"created_at",
		"updated_at",
		"team_id",
		"stateless",
		"deliver",
		"deliver_channel",
		"deliver_to",
		"wake_heartbeat",
		"agent_id",
		"payload fields other than one message string",
	}
	UnmappedChannelColumns = []string{
		"credentials",
		"display_name",
		"created_by",
		"created_at",
		"updated_at",
		"config fields other than dm_policy, group_policy, require_mention, allow_from",
	}
)

// Options selects the single destination tenant for this run.
type Options struct {
	// SourceTenant is the export tenant_id to write. Other tenants are counted
	// and not written.
	SourceTenant string
	// DestTenant is the goso tenant. It is not defaulted.
	DestTenant string
	// DryRun reports per-tenant counts and writes nothing.
	DryRun bool
	// Log receives reason codes, tenant ids, and source ids. It must not be
	// given a credential, payload, or message.
	Log Logger
}

// Logger is the rejection log.
type Logger interface {
	Printf(format string, args ...any)
}

// ConfigError is a bad flag or a bad file, before any write.
type ConfigError struct{ msg string }

func (e *ConfigError) Error() string { return e.msg }

func errConfig(msg string) error { return &ConfigError{msg: msg} }

// Report is the run summary. NeedsCredential never carries secret bytes.
type Report struct {
	DryRun          bool                `json:"dry_run"`
	Tenants         []TenantCount       `json:"tenants"`
	NeedsCredential []CredentialNote    `json:"needs_credential"`
	NotImported     []Dropped           `json:"not_imported"`
	UnmappedColumns map[string][]string `json:"unmapped_columns"`
}

// TenantCount is one export tenant_id.
type TenantCount struct {
	TenantID         string    `json:"tenant_id"`
	Selected         bool      `json:"selected"`
	CronJobs         RowCounts `json:"cron_jobs"`
	ChannelInstances RowCounts `json:"channel_instances"`
}

// RowCounts balances as input = new + skipped + not_imported + held.
// Written is 0 on a dry-run and for a tenant that was not selected.
type RowCounts struct {
	Input       int `json:"input"`
	New         int `json:"new"`
	Skipped     int `json:"skipped"`
	NotImported int `json:"not_imported"`
	Held        int `json:"held"`
	Written     int `json:"written"`
}

// CredentialNote lists one channel instance whose credentials were not copied.
type CredentialNote struct {
	TenantID        string `json:"tenant_id"`
	SourceID        string `json:"source_id"`
	Name            string `json:"name"`
	ChannelType     string `json:"channel_type"`
	NeedsCredential bool   `json:"needs_credential"`
}

// Dropped is one column or row with no home in the existing models.
type Dropped struct {
	Kind     string `json:"kind"`
	TenantID string `json:"tenant_id,omitempty"`
	SourceID string `json:"source_id,omitempty"`
	Column   string `json:"column,omitempty"`
	Reason   string `json:"reason"`
	Detail   string `json:"detail,omitempty"`
}

// Import writes the selected tenant, or only counts when DryRun is set.
func Import(st store.StoreIface, ex *Export, opt Options) (*Report, error) {
	if st == nil {
		return nil, errConfig("store is required")
	}
	if ex == nil {
		return nil, errConfig("export is required")
	}
	dest := strings.TrimSpace(opt.DestTenant)
	source := strings.TrimSpace(opt.SourceTenant)
	if !store.TenantOK(dest) || !store.TenantOK(source) {
		return nil, errConfig("tenant id is required")
	}
	opt.DestTenant = dest
	opt.SourceTenant = source
	a := &acc{opt: opt, st: st, buckets: map[string]*bucket{}}
	a.existingCron = len(st.ListCronJobs())
	sessions := sessionsIn(st, dest)
	for _, row := range ex.CronJobs {
		if err := a.cron(row, sessions); err != nil {
			return nil, err
		}
	}
	for _, row := range ex.ChannelInstances {
		if err := a.channel(row); err != nil {
			return nil, err
		}
	}
	for _, o := range ex.otherTables {
		a.noteOther(o)
	}
	return a.finish(), nil
}

type bucket struct {
	cron RowCounts
	ch   RowCounts
}

type acc struct {
	opt          Options
	st           store.StoreIface
	buckets      map[string]*bucket
	drops        []Dropped
	creds        []CredentialNote
	seen         map[string]struct{}
	existingCron int
	plannedCron  int
}

func (a *acc) bucket(tenant string) *bucket {
	tenant = strings.TrimSpace(tenant)
	if tenant == "" {
		tenant = "(empty)"
	}
	b := a.buckets[tenant]
	if b == nil {
		b = &bucket{}
		a.buckets[tenant] = b
	}
	return b
}

func (a *acc) selected(tenant string) bool {
	return strings.EqualFold(strings.TrimSpace(tenant), a.opt.SourceTenant)
}

func (a *acc) logf(format string, args ...any) {
	if a.opt.Log != nil {
		a.opt.Log.Printf(format, args...)
	}
}

func (a *acc) drop(kind, tenant, source, column, detail string) {
	if len(a.drops) >= 500 {
		return
	}
	a.drops = append(a.drops, Dropped{
		Kind: kind, TenantID: strings.TrimSpace(tenant), SourceID: strings.TrimSpace(source),
		Column: column, Reason: ReasonNeedsDecision, Detail: detail,
	})
	a.logf("scheduleimport: %s tenant=%s source=%s column=%s detail=%s", kind, strings.TrimSpace(tenant), strings.TrimSpace(source), column, detail)
}

func (a *acc) cron(row CronRow, sessions []*store.Session) error {
	tenant := strings.TrimSpace(row.TenantID)
	b := a.bucket(tenant)
	b.cron.Input++
	if !a.selected(tenant) {
		b.cron.Held++
		return nil
	}
	sourceID := strings.TrimSpace(row.ID)
	if !idOK(sourceID) {
		b.cron.NotImported++
		a.drop(kindCron, tenant, sourceID, "id", "bad-id")
		return nil
	}
	key := tenant + "\x00" + kindCron + "\x00" + sourceID
	if a.seen == nil {
		a.seen = map[string]struct{}{}
	}
	if _, ok := a.seen[key]; ok {
		b.cron.Skipped++
		return nil
	}
	if gosoID, ok, err := a.st.GetImportSource(a.opt.DestTenant, kindCron, sourceID); err != nil {
		return err
	} else if ok && gosoID != "" {
		a.seen[key] = struct{}{}
		b.cron.Skipped++
		return nil
	}
	blobs := secretBlobs(nil, row.Payload)
	spec, specDetail := cronSpec(row)
	message, extra := messageFrom(row.Payload)
	if spec == "" {
		b.cron.NotImported++
		a.seen[key] = struct{}{}
		a.drop(kindCron, tenant, sourceID, "schedule_kind", specDetail)
		return nil
	}
	if message == "" || containsSecret(message, blobs) || containsSecret(spec, blobs) {
		b.cron.NotImported++
		a.seen[key] = struct{}{}
		detail := "no-message"
		if message != "" {
			detail = "secret-in-message"
		}
		a.drop(kindCron, tenant, sourceID, "payload", detail)
		return nil
	}
	agentID, where := resolveAgent(a.st, a.opt.DestTenant, row.AgentID)
	sessionID, sessDetail := pickSession(sessions, agentID, where)
	if sessionID == "" {
		b.cron.NotImported++
		a.seen[key] = struct{}{}
		a.drop(kindCron, tenant, sourceID, "session_id", sessDetail)
		return nil
	}
	for _, k := range extra {
		if containsSecret(k, blobs) {
			continue
		}
		a.drop(kindCron, tenant, sourceID, "payload."+k, "no-column")
	}
	if a.opt.DryRun {
		if a.existingCron+a.plannedCron >= store.CronJobCap {
			b.cron.NotImported++
			a.seen[key] = struct{}{}
			a.drop(kindCron, tenant, sourceID, "cron_jobs", "cron-cap")
			return nil
		}
		a.plannedCron++
		a.seen[key] = struct{}{}
		b.cron.New++
		return nil
	}
	job, err := a.st.CreateCronJob(store.CronJob{
		Spec: spec, SessionID: sessionID, Message: message, Enabled: false,
	})
	if errors.Is(err, store.ErrCronCap) {
		b.cron.NotImported++
		a.seen[key] = struct{}{}
		a.drop(kindCron, tenant, sourceID, "cron_jobs", "cron-cap")
		return nil
	}
	if err != nil {
		return err
	}
	if job == nil || job.Enabled {
		if job != nil {
			_ = a.st.DeleteCronJob(job.ID)
		}
		return errConfig("refusing to leave an enabled cron job")
	}
	if err := a.st.PutImportSource(a.opt.DestTenant, kindCron, sourceID, job.ID); err != nil {
		_ = a.st.DeleteCronJob(job.ID)
		if errors.Is(err, store.ErrExists) {
			a.seen[key] = struct{}{}
			b.cron.Skipped++
			return nil
		}
		return err
	}
	a.seen[key] = struct{}{}
	b.cron.New++
	b.cron.Written++
	return nil
}

func (a *acc) channel(row ChannelRow) error {
	tenant := strings.TrimSpace(row.TenantID)
	b := a.bucket(tenant)
	b.ch.Input++
	blobs := secretBlobs(row.Credentials, row.Config)
	a.creds = append(a.creds, credentialNote(tenant, row, blobs))
	if !a.selected(tenant) {
		b.ch.Held++
		return nil
	}
	sourceID := strings.TrimSpace(row.ID)
	if !idOK(sourceID) {
		b.ch.NotImported++
		a.drop(kindChannel, tenant, sourceID, "id", "bad-id")
		return nil
	}
	key := tenant + "\x00" + kindChannel + "\x00" + sourceID
	if a.seen == nil {
		a.seen = map[string]struct{}{}
	}
	if _, ok := a.seen[key]; ok {
		b.ch.Skipped++
		return nil
	}
	if gosoID, ok, err := a.st.GetImportSource(a.opt.DestTenant, kindChannel, sourceID); err != nil {
		return err
	} else if ok && gosoID != "" {
		a.seen[key] = struct{}{}
		b.ch.Skipped++
		return nil
	}
	catalog, ok := catalogOf(row.ChannelType)
	if !ok {
		b.ch.NotImported++
		a.seen[key] = struct{}{}
		a.drop(kindChannel, tenant, sourceID, "channel_type", "unknown-channel")
		return nil
	}
	instance := strings.TrimSpace(row.Name)
	if !instanceOK(instance) {
		b.ch.NotImported++
		a.seen[key] = struct{}{}
		a.drop(kindChannel, tenant, sourceID, "name", "bad-name")
		return nil
	}
	if containsSecret(instance, blobs) || containsSecret(catalog, blobs) {
		b.ch.NotImported++
		a.seen[key] = struct{}{}
		a.drop(kindChannel, tenant, sourceID, "name", "secret-in-name")
		return nil
	}
	cfg, extra := policyFrom(row.Config, blobs)
	agentID, where := resolveAgent(a.st, a.opt.DestTenant, row.AgentID)
	switch where {
	case "dest":
		cfg.AgentID = agentID
	case "other":
		a.drop(kindChannel, tenant, sourceID, "agent_id", "agent-other-tenant")
	case "missing":
		if strings.TrimSpace(row.AgentID) != "" {
			a.drop(kindChannel, tenant, sourceID, "agent_id", "agent-unresolved")
		}
	}
	for _, k := range extra {
		if containsSecret(k, blobs) {
			continue
		}
		a.drop(kindChannel, tenant, sourceID, "config."+k, "no-column")
	}
	name := a.opt.DestTenant + "/" + catalog + "/" + instance
	if _, err := a.st.GetChannelConfig(name); err == nil {
		b.ch.NotImported++
		a.seen[key] = struct{}{}
		a.drop(kindChannel, tenant, sourceID, "name", "name-taken")
		return nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if a.opt.DryRun {
		a.seen[key] = struct{}{}
		b.ch.New++
		return nil
	}
	cfg.Name = name
	cfg.Enabled = false
	if err := a.st.PutChannelConfig(cfg); err != nil {
		return err
	}
	if err := a.st.PutImportSource(a.opt.DestTenant, kindChannel, sourceID, name); err != nil {
		_ = a.st.DeleteChannelConfig(name)
		if errors.Is(err, store.ErrExists) {
			a.seen[key] = struct{}{}
			b.ch.Skipped++
			return nil
		}
		return err
	}
	got, err := a.st.GetChannelConfig(name)
	if err != nil || got == nil || got.Enabled {
		_ = a.st.DeleteChannelConfig(name)
		return errConfig("refusing to leave an enabled channel")
	}
	a.seen[key] = struct{}{}
	b.ch.New++
	b.ch.Written++
	return nil
}

func (a *acc) noteOther(o otherTable) {
	tenant := strings.TrimSpace(o.TenantID)
	b := a.bucket(tenant)
	b.ch.Input++
	if !a.selected(tenant) {
		b.ch.Held++
		return
	}
	b.ch.NotImported++
	a.drop("table", tenant, strings.TrimSpace(o.ID), o.Table, "other-table")
}

func (a *acc) finish() *Report {
	rep := &Report{
		DryRun:          a.opt.DryRun,
		Tenants:         []TenantCount{},
		NeedsCredential: append([]CredentialNote(nil), a.creds...),
		NotImported:     append([]Dropped(nil), a.drops...),
		UnmappedColumns: map[string][]string{
			"cron_jobs":         append([]string(nil), UnmappedCronColumns...),
			"channel_instances": append([]string(nil), UnmappedChannelColumns...),
		},
	}
	ids := make([]string, 0, len(a.buckets))
	for id := range a.buckets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		b := a.buckets[id]
		rep.Tenants = append(rep.Tenants, TenantCount{
			TenantID: id, Selected: a.selected(id),
			CronJobs: b.cron, ChannelInstances: b.ch,
		})
	}
	sort.Slice(rep.NeedsCredential, func(i, j int) bool {
		if rep.NeedsCredential[i].TenantID != rep.NeedsCredential[j].TenantID {
			return rep.NeedsCredential[i].TenantID < rep.NeedsCredential[j].TenantID
		}
		return rep.NeedsCredential[i].SourceID < rep.NeedsCredential[j].SourceID
	})
	sort.Slice(rep.NotImported, func(i, j int) bool {
		if rep.NotImported[i].SourceID != rep.NotImported[j].SourceID {
			return rep.NotImported[i].SourceID < rep.NotImported[j].SourceID
		}
		if rep.NotImported[i].Column != rep.NotImported[j].Column {
			return rep.NotImported[i].Column < rep.NotImported[j].Column
		}
		return rep.NotImported[i].Detail < rep.NotImported[j].Detail
	})
	return rep
}

func credentialNote(tenant string, row ChannelRow, blobs []string) CredentialNote {
	note := CredentialNote{NeedsCredential: true}
	if !containsSecret(tenant, blobs) {
		note.TenantID = tenant
	}
	if id := strings.TrimSpace(row.ID); !containsSecret(id, blobs) {
		note.SourceID = id
	}
	if name := strings.TrimSpace(row.Name); !containsSecret(name, blobs) {
		note.Name = name
	}
	if kind := strings.TrimSpace(row.ChannelType); !containsSecret(kind, blobs) {
		note.ChannelType = kind
	}
	return note
}

func sessionsIn(st store.StoreIface, dest string) []*store.Session {
	var out []*store.Session
	for _, s := range st.ListSessions() {
		if s != nil && store.SameTenant(s.TenantID, dest) {
			out = append(out, s)
		}
	}
	return out
}

func resolveAgent(st store.StoreIface, dest, ref string) (string, string) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "none"
	}
	other := false
	for _, a := range st.ListAgents() {
		if a == nil || (a.ID != ref && a.AgentKey != ref) {
			continue
		}
		if store.SameTenant(a.TenantID, dest) {
			return a.ID, "dest"
		}
		other = true
	}
	if other {
		return "", "other"
	}
	return "", "missing"
}

func pickSession(sessions []*store.Session, agentID, where string) (string, string) {
	if where == "other" {
		return "", "agent-other-tenant"
	}
	if where == "dest" {
		var ids []string
		for _, s := range sessions {
			if s != nil && s.AgentID == agentID {
				ids = append(ids, s.ID)
			}
		}
		switch len(ids) {
		case 1:
			return ids[0], ""
		case 0:
			return "", "no-session"
		default:
			return "", "ambiguous-session"
		}
	}
	if len(sessions) == 1 && sessions[0] != nil {
		return sessions[0].ID, ""
	}
	if len(sessions) == 0 {
		return "", "no-session"
	}
	return "", "ambiguous-session"
}

func catalogOf(channelType string) (string, bool) {
	s := strings.ToLower(strings.TrimSpace(channelType))
	s = strings.ReplaceAll(s, "_", "-")
	switch s {
	case "telegram", "zalo-personal", "zalo-oa", "discord", "slack", "feishu", "whatsapp":
		return s, true
	default:
		return "", false
	}
}

func cronSpec(row CronRow) (string, string) {
	kind := strings.ToLower(strings.TrimSpace(row.ScheduleKind))
	switch kind {
	case "cron":
		spec := strings.TrimSpace(row.CronExpression)
		if _, err := cron.ParseSpec(spec); err != nil {
			return "", "schedule-unsupported"
		}
		return spec, ""
	case "every", "interval":
		if row.IntervalMS == nil {
			return "", "schedule-unsupported"
		}
		return intervalSpec(*row.IntervalMS)
	default:
		return "", "schedule-unsupported"
	}
}

func intervalSpec(ms int64) (string, string) {
	if ms <= 0 {
		return "", "schedule-unsupported"
	}
	if ms%3_600_000 == 0 {
		h := ms / 3_600_000
		if h >= 1 {
			return fmt.Sprintf("every:%dh", h), ""
		}
	}
	if ms%60_000 == 0 {
		m := ms / 60_000
		if m >= 1 {
			return fmt.Sprintf("every:%dm", m), ""
		}
	}
	return "", "schedule-unsupported"
}

func messageFrom(payload json.RawMessage) (string, []string) {
	raw := bytes.TrimSpace(payload)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", nil
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", nil
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return "", nil
		}
		return s, nil
	}
	if raw[0] != '{' {
		return "", nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", nil
	}
	var message string
	used := ""
	for _, key := range []string{"message", "text", "prompt", "content", "body"} {
		v, ok := obj[key]
		if !ok {
			continue
		}
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			continue
		}
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		message = s
		used = key
		break
	}
	var extra []string
	for k := range obj {
		if k == used {
			continue
		}
		extra = append(extra, k)
	}
	sort.Strings(extra)
	return message, extra
}

func policyFrom(raw json.RawMessage, blobs []string) (store.ChannelConfig, []string) {
	var cfg store.ChannelConfig
	trim := bytes.TrimSpace(raw)
	if len(trim) == 0 || bytes.Equal(trim, []byte("null")) {
		return cfg, nil
	}
	if trim[0] != '{' {
		return cfg, []string{"config"}
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(trim, &obj); err != nil {
		return cfg, []string{"config"}
	}
	known := map[string]bool{}
	if v, ok := obj["dm_policy"]; ok {
		known["dm_policy"] = true
		var s string
		if err := json.Unmarshal(v, &s); err == nil && policyDM(s) && !containsSecret(s, blobs) {
			cfg.DMPolicy = s
		}
	}
	if v, ok := obj["group_policy"]; ok {
		known["group_policy"] = true
		var s string
		if err := json.Unmarshal(v, &s); err == nil && policyGroup(s) && !containsSecret(s, blobs) {
			cfg.GroupPolicy = s
		}
	}
	if v, ok := obj["require_mention"]; ok {
		known["require_mention"] = true
		var b bool
		if err := json.Unmarshal(v, &b); err == nil {
			cfg.RequireMention = b
		}
	}
	if v, ok := obj["allow_from"]; ok {
		known["allow_from"] = true
		var list []string
		if err := json.Unmarshal(v, &list); err == nil && allowFromOK(list, blobs) {
			cfg.AllowFrom = list
		}
	}
	var extra []string
	for k := range obj {
		if known[k] || secretKey(k) {
			if !known[k] {
				extra = append(extra, k)
			}
			continue
		}
		extra = append(extra, k)
	}
	sort.Strings(extra)
	return cfg, extra
}

func policyDM(s string) bool {
	switch strings.TrimSpace(s) {
	case "pairing", "allowlist", "open", "disabled":
		return true
	default:
		return false
	}
}

func policyGroup(s string) bool {
	switch strings.TrimSpace(s) {
	case "open", "allowlist", "disabled":
		return true
	default:
		return false
	}
}

func allowFromOK(list []string, blobs []string) bool {
	if len(list) > 32 {
		return false
	}
	for _, item := range list {
		if !instanceOK(item) || containsSecret(item, blobs) {
			return false
		}
	}
	return true
}

func idOK(id string) bool {
	if id == "" || len(id) > 80 {
		return false
	}
	for _, r := range id {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' || r == ':' {
			continue
		}
		return false
	}
	return true
}

func instanceOK(name string) bool {
	if name == "" || len(name) > 100 {
		return false
	}
	for _, r := range name {
		if r == '/' || r == '\\' || unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

func secretKey(name string) bool {
	n := strings.ToLower(name)
	n = strings.ReplaceAll(n, "_", "")
	n = strings.ReplaceAll(n, "-", "")
	switch {
	case strings.Contains(n, "token"),
		strings.Contains(n, "secret"),
		strings.Contains(n, "password"),
		strings.Contains(n, "credential"),
		strings.Contains(n, "cookie"),
		strings.Contains(n, "imei"),
		strings.Contains(n, "authorization"),
		strings.Contains(n, "apikey"),
		strings.Contains(n, "privatekey"):
		return true
	default:
		return false
	}
}

func secretBlobs(cred, extra json.RawMessage) []string {
	var out []string
	collectSecrets(cred, true, &out)
	collectSecrets(extra, false, &out)
	return out
}

func collectSecrets(raw json.RawMessage, all bool, out *[]string) {
	trim := bytes.TrimSpace(raw)
	if len(trim) == 0 || bytes.Equal(trim, []byte("null")) {
		return
	}
	if trim[0] == '"' {
		var s string
		if err := json.Unmarshal(trim, &s); err == nil {
			addBlob(out, s)
		}
		return
	}
	if trim[0] == '{' {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(trim, &obj); err != nil {
			return
		}
		for k, v := range obj {
			if all || secretKey(k) {
				collectSecrets(v, true, out)
			}
		}
		return
	}
	if trim[0] == '[' {
		var arr []json.RawMessage
		if err := json.Unmarshal(trim, &arr); err != nil {
			return
		}
		for _, v := range arr {
			collectSecrets(v, all, out)
		}
	}
}

func addBlob(out *[]string, s string) {
	s = strings.TrimSpace(s)
	if len(s) < 8 {
		return
	}
	*out = append(*out, s)
}

func containsSecret(s string, blobs []string) bool {
	if s == "" || len(blobs) == 0 {
		return false
	}
	for _, b := range blobs {
		if b != "" && strings.Contains(s, b) {
			return true
		}
	}
	return false
}
