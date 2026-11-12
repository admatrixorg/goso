// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package sessionimport

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	unresolvedPrefix = "import-unresolved."
	stubDisplay      = "Unresolved import"
	stubCreated      = "2026-01-01T00:00:00.000000000Z"
)

// duringTx runs inside the import transaction after message rows are inserted
// and their memory_fts copies are removed. Tests use it to prove the message
// index trigger is still installed. Production leaves it nil.
var duringTx func(tx *sql.Tx) error

type writer struct {
	db    *sql.DB
	pg    bool
	dest  string
	dry   bool
	links bool
}

func (w *writer) q(query string) string {
	if w == nil || !w.pg {
		return query
	}
	return pgSQL(query)
}

func (w *writer) ensureSchema() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS session_import_links (
			tenant_id TEXT NOT NULL,
			source_id TEXT NOT NULL,
			source_key TEXT NOT NULL,
			session_id TEXT NOT NULL,
			source_agent_id TEXT NOT NULL DEFAULT '',
			agent_id TEXT NOT NULL DEFAULT '',
			source_user_id TEXT NOT NULL DEFAULT '',
			contact_id TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (tenant_id, source_id)
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_session_import_links_key
			ON session_import_links(tenant_id, source_key) WHERE source_key <> ''`,
	}
	for _, stmt := range stmts {
		if _, err := w.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (w *writer) hasLinks() (bool, error) {
	var n int
	q := `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='session_import_links'`
	if w.pg {
		q = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'session_import_links'`
	}
	if err := w.db.QueryRow(q).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

func (w *writer) flush(rows []sessionRow, acc *accumulator) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]string, len(rows))
	keys := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.id
		keys[i] = row.key
	}
	haveID, err := w.selectSet(`SELECT id FROM sessions WHERE id IN `+placeholders(len(ids)), anys(ids))
	if err != nil {
		return err
	}
	haveKey := map[string]struct{}{}
	haveSrc := map[string]struct{}{}
	if w.links {
		keyArgs := append([]any{w.dest}, anys(keys)...)
		haveKey, err = w.selectSet(`SELECT source_key FROM session_import_links WHERE tenant_id=? AND source_key IN `+placeholders(len(keys)), keyArgs)
		if err != nil {
			return err
		}
		srcArgs := append([]any{w.dest}, anys(ids)...)
		haveSrc, err = w.selectSet(`SELECT source_id FROM session_import_links WHERE tenant_id=? AND source_id IN `+placeholders(len(ids)), srcArgs)
		if err != nil {
			return err
		}
	}
	var existIDs []string
	for _, row := range rows {
		if _, ok := haveID[row.id]; ok {
			existIDs = append(existIDs, row.id)
		}
	}
	snaps, err := w.loadSnaps(existIDs)
	if err != nil {
		return err
	}
	fresh := make([]sessionRow, 0, len(rows))
	for _, row := range rows {
		if _, ok := haveID[row.id]; ok {
			s := snaps[row.id]
			if s.ok && s.tenant != w.dest {
				w.countCollision(acc, row)
				continue
			}
			if s.matches(w.dest, row) {
				w.countSkip(acc, row)
			} else {
				w.countNative(acc, row)
			}
			continue
		}
		if _, ok := haveKey[row.key]; ok {
			w.countSkip(acc, row)
			continue
		}
		if _, ok := haveSrc[row.id]; ok {
			w.countSkip(acc, row)
			continue
		}
		fresh = append(fresh, row)
	}
	if w.dry || len(fresh) == 0 {
		for _, row := range fresh {
			acc.tenant(row.bucket).addNew(row)
		}
		return nil
	}
	if err := w.insertTx(fresh); err != nil {
		okN := 0
		var last error
		for _, row := range fresh {
			if err := w.insertTx([]sessionRow{row}); err != nil {
				last = err
				t := acc.tenant(row.bucket)
				t.Sessions++
				t.Invalid++
				continue
			}
			okN++
			acc.tenant(row.bucket).addNew(row)
		}
		if okN == 0 && last != nil {
			return last
		}
		return nil
	}
	for _, row := range fresh {
		acc.tenant(row.bucket).addNew(row)
	}
	return nil
}

func (w *writer) countSkip(acc *accumulator, row sessionRow) {
	t := acc.tenant(row.bucket)
	t.Sessions++
	t.Skipped++
}

func (w *writer) countNative(acc *accumulator, row sessionRow) {
	t := acc.tenant(row.bucket)
	t.Sessions++
	t.NativeExists++
}

func (w *writer) countCollision(acc *accumulator, row sessionRow) {
	t := acc.tenant(row.bucket)
	t.Sessions++
	t.IDCollision++
}

type snapMsg struct {
	role, content, agent, mention, model, provider string
	at                                             time.Time
}

type snap struct {
	ok                                    bool
	tenant, agent, label, model, provider string
	at                                    time.Time
	msgs                                  []snapMsg
}

func (s snap) matches(dest string, row sessionRow) bool {
	if !s.ok || s.tenant != dest {
		return false
	}
	agent := row.agentID
	if agent == "" {
		agent = unresolvedAgentID(dest)
	}
	if s.agent != agent || s.label != row.label || s.model != row.model || s.provider != row.provider || !s.at.Equal(row.at.UTC()) {
		return false
	}
	if len(s.msgs) != len(row.msgs) {
		return false
	}
	for i := range s.msgs {
		got, want := s.msgs[i], row.msgs[i]
		if got.role != want.role || got.content != want.content || got.agent != want.agentID || got.mention != want.mentionID || got.model != want.model || got.provider != want.provider || !got.at.Equal(want.at.UTC()) {
			return false
		}
	}
	return true
}

func (w *writer) loadSnaps(ids []string) (map[string]snap, error) {
	out := map[string]snap{}
	if len(ids) == 0 {
		return out, nil
	}
	q := `SELECT id, tenant_id, agent_id, COALESCE(label,''), COALESCE(model_override,''), COALESCE(provider_override,''), created_at FROM sessions WHERE id IN ` + placeholders(len(ids))
	rows, err := w.db.Query(w.q(q), anys(ids)...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, tenant, agent, label, model, provider, at string
		if err := rows.Scan(&id, &tenant, &agent, &label, &model, &provider, &at); err != nil {
			rows.Close()
			return nil, err
		}
		out[id] = snap{ok: true, tenant: tenant, agent: agent, label: label, model: model, provider: provider, at: parseStamp(at)}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	mq := `SELECT session_id, role, content, created_at, COALESCE(agent_id,''), COALESCE(mention_agent_id,''), COALESCE(model,''), COALESCE(provider,'') FROM messages WHERE session_id IN ` + placeholders(len(ids)) + ` ORDER BY session_id, created_at, id`
	mrows, err := w.db.Query(w.q(mq), anys(ids)...)
	if err != nil {
		return nil, err
	}
	defer mrows.Close()
	for mrows.Next() {
		var sid, role, content, at, agent, mention, model, provider string
		if err := mrows.Scan(&sid, &role, &content, &at, &agent, &mention, &model, &provider); err != nil {
			return nil, err
		}
		s := out[sid]
		s.msgs = append(s.msgs, snapMsg{role: role, content: content, agent: agent, mention: mention, model: model, provider: provider, at: parseStamp(at)})
		out[sid] = s
	}
	return out, mrows.Err()
}

func parseStamp(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, err = time.Parse(time.RFC3339, s)
		if err != nil {
			return time.Time{}
		}
	}
	return t.UTC()
}

func (w *writer) selectSet(query string, args []any) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	if len(args) == 0 {
		return out, nil
	}
	rows, err := w.db.Query(w.q(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out[s] = struct{}{}
	}
	return out, rows.Err()
}

func (w *writer) insertTx(rows []sessionRow) error {
	tx, err := w.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	needStub := false
	for _, row := range rows {
		if row.agentID == "" {
			needStub = true
			break
		}
	}
	if needStub {
		if err := w.insertStub(tx); err != nil {
			return err
		}
	}
	sess := make([][]any, 0, len(rows))
	var msgs [][]any
	links := make([][]any, 0, len(rows))
	stub := unresolvedAgentID(w.dest)
	var msgIDs []string
	for _, row := range rows {
		agentCol := row.agentID
		if agentCol == "" {
			agentCol = stub
		}
		sess = append(sess, []any{
			row.id, agentCol, row.label, formatStamp(row.at), w.dest, "", row.model, row.provider, w.archArg(row.agentID == ""),
		})
		for _, m := range row.msgs {
			msgs = append(msgs, []any{
				m.id, row.id, m.role, m.content, formatStamp(m.at), m.agentID, m.mentionID, m.model, m.provider,
			})
			msgIDs = append(msgIDs, m.id)
		}
		links = append(links, []any{
			w.dest, row.id, row.key, row.id, row.agentSrc, row.agentID, row.userSrc, row.contactID,
		})
	}
	if err := insertChunks(tx, w.pg, `sessions`,
		[]string{"id", "agent_id", "label", "created_at", "tenant_id", "prompt_mode", "model_override", "provider_override", "archived"},
		sess); err != nil {
		return err
	}
	if err := insertChunks(tx, w.pg, `messages`,
		[]string{"id", "session_id", "role", "content", "created_at", "agent_id", "mention_agent_id", "model", "provider"},
		msgs); err != nil {
		return err
	}
	if err := w.dropImportedFTS(tx, msgIDs); err != nil {
		return err
	}
	if duringTx != nil {
		if err := duringTx(tx); err != nil {
			return err
		}
	}
	if err := insertChunks(tx, w.pg, `session_import_links`,
		[]string{"tenant_id", "source_id", "source_key", "session_id", "source_agent_id", "agent_id", "source_user_id", "contact_id"},
		links); err != nil {
		return err
	}
	return tx.Commit()
}

func (w *writer) insertStub(tx *sql.Tx) error {
	id := unresolvedAgentID(w.dest)
	stmt := `INSERT OR IGNORE INTO agents(id, agent_key, display_name, model, llm_provider, instructions, orchestration_mode, created_at, tenant_id, enabled, updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`
	if _, err := tx.Exec(pgMaybe(w.pg, stmt), id, id, stubDisplay, "", "", "", "", stubCreated, w.dest, 0, stubCreated); err != nil {
		return err
	}
	_, err := tx.Exec(pgMaybe(w.pg, `UPDATE agents SET enabled=0 WHERE id=? AND agent_key=?`), id, id)
	return err
}

func (w *writer) archArg(unmapped bool) any {
	if w.pg {
		return unmapped
	}
	if unmapped {
		return 1
	}
	return 0
}

func (w *writer) dropImportedFTS(tx *sql.Tx, ids []string) error {
	if w.pg || len(ids) == 0 {
		return nil
	}
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='memory_fts'`).Scan(&n); err != nil || n == 0 {
		return err
	}
	q := `DELETE FROM memory_fts WHERE id IN ` + placeholders(len(ids))
	_, err := tx.Exec(q, anys(ids)...)
	return err
}

func unresolvedAgentID(tenant string) string {
	return unresolvedPrefix + tenant
}

func insertChunks(tx *sql.Tx, pg bool, table string, cols []string, rows [][]any) error {
	if len(rows) == 0 {
		return nil
	}
	const chunk = 40
	width := len(cols)
	for start := 0; start < len(rows); start += chunk {
		end := start + chunk
		if end > len(rows) {
			end = len(rows)
		}
		part := rows[start:end]
		var b strings.Builder
		b.WriteString("INSERT INTO ")
		b.WriteString(table)
		b.WriteString("(")
		b.WriteString(strings.Join(cols, ","))
		b.WriteString(") VALUES ")
		args := make([]any, 0, len(part)*width)
		for i, row := range part {
			if len(row) != width {
				return fmt.Errorf("insert width")
			}
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteByte('(')
			for j := 0; j < width; j++ {
				if j > 0 {
					b.WriteByte(',')
				}
				b.WriteByte('?')
			}
			b.WriteByte(')')
			args = append(args, row...)
		}
		if _, err := tx.Exec(pgMaybe(pg, b.String()), args...); err != nil {
			return err
		}
	}
	return nil
}

func placeholders(n int) string {
	var b strings.Builder
	b.WriteByte('(')
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('?')
	}
	b.WriteByte(')')
	return b.String()
}

func anys(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func pgMaybe(pg bool, query string) string {
	if !pg {
		return query
	}
	return pgSQL(query)
}

func pgSQL(q string) string {
	q = rewriteInsertOrIgnore(q)
	return rewritePlaceholders(q)
}

func rewriteInsertOrIgnore(q string) string {
	upper := strings.ToUpper(q)
	const needle = "INSERT OR IGNORE INTO"
	idx := strings.Index(upper, needle)
	if idx < 0 {
		return q
	}
	out := q[:idx] + "INSERT INTO" + q[idx+len(needle):]
	if strings.Contains(strings.ToUpper(out), "ON CONFLICT") {
		return out
	}
	return strings.TrimRight(out, " \t\n;") + " ON CONFLICT DO NOTHING"
}

func rewritePlaceholders(q string) string {
	var b strings.Builder
	b.Grow(len(q) + 8)
	n := 0
	for i := 0; i < len(q); i++ {
		if q[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(q[i])
	}
	return b.String()
}

type contactRec struct {
	merged bool
	into   string
}

type resolver struct {
	byAgentID  map[string]string
	byAgentKey map[string]string
	contacts   map[string]contactRec
	byPair     map[string]string
	byDest     map[string]map[string]struct{}
}

func loadResolver(w *writer) (resolver, error) {
	res := resolver{
		byAgentID:  map[string]string{},
		byAgentKey: map[string]string{},
		contacts:   map[string]contactRec{},
		byPair:     map[string]string{},
		byDest:     map[string]map[string]struct{}{},
	}
	rows, err := w.db.Query(w.q(`SELECT id, agent_key FROM agents WHERE tenant_id=?`), w.dest)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var id, key string
		if err := rows.Scan(&id, &key); err != nil {
			rows.Close()
			return res, err
		}
		if strings.HasPrefix(id, unresolvedPrefix) || strings.HasPrefix(key, unresolvedPrefix) {
			continue
		}
		res.byAgentID[id] = id
		if key != "" {
			res.byAgentKey[key] = id
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return res, err
	}
	rows.Close()

	crows, err := w.db.Query(w.q(`SELECT id, merged, merged_into FROM contacts WHERE tenant_id=?`), w.dest)
	if err != nil {
		return res, err
	}
	for crows.Next() {
		var id, into string
		var merged int
		if err := crows.Scan(&id, &merged, &into); err != nil {
			crows.Close()
			return res, err
		}
		res.contacts[id] = contactRec{merged: merged != 0, into: into}
	}
	if err := crows.Err(); err != nil {
		crows.Close()
		return res, err
	}
	crows.Close()

	irows, err := w.db.Query(w.q(`SELECT channel, dest, contact_id FROM contact_identifiers WHERE tenant_id=?`), w.dest)
	if err != nil {
		return res, err
	}
	defer irows.Close()
	for irows.Next() {
		var channel, dest, contactID string
		if err := irows.Scan(&channel, &dest, &contactID); err != nil {
			return res, err
		}
		canon, ok := res.canonical(contactID)
		if !ok || dest == "" {
			continue
		}
		res.byPair[channel+"\x00"+dest] = canon
		set := res.byDest[dest]
		if set == nil {
			set = map[string]struct{}{}
			res.byDest[dest] = set
		}
		set[canon] = struct{}{}
	}
	return res, irows.Err()
}

func (r resolver) agent(src string) (string, bool) {
	src = strings.TrimSpace(src)
	if src == "" {
		return "", false
	}
	if id, ok := r.byAgentID[src]; ok {
		return id, true
	}
	if id, ok := r.byAgentKey[src]; ok {
		return id, true
	}
	return "", false
}

func (r resolver) contact(userID, channel string) (string, bool) {
	userID = strings.TrimSpace(userID)
	channel = strings.TrimSpace(channel)
	if userID == "" {
		return "", false
	}
	if id, ok := r.canonical(userID); ok {
		return id, true
	}
	if _, known := r.contacts[userID]; known {
		return "", true
	}
	if channel != "" {
		if id, ok := r.byPair[channel+"\x00"+userID]; ok {
			if c, ok := r.canonical(id); ok {
				return c, true
			}
			return "", true
		}
	}
	if set := r.byDest[userID]; len(set) == 1 {
		for id := range set {
			if c, ok := r.canonical(id); ok {
				return c, true
			}
		}
		return "", true
	}
	return "", true
}

func (r resolver) canonical(id string) (string, bool) {
	c, ok := r.contacts[id]
	if !ok {
		return "", false
	}
	if !c.merged {
		return id, true
	}
	if c.into == "" || c.into == id {
		return "", false
	}
	nxt, ok := r.contacts[c.into]
	if !ok || nxt.merged {
		return "", false
	}
	return c.into, true
}
