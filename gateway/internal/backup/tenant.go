// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package backup

import "github.com/mqglobal/goso/gateway/internal/store"

// PruneTenant drops rows that do not belong to tenant from a snapshot copy.
func PruneTenant(path, tenantID string) error {
	tenantID = store.NormalizeTenant(tenantID)
	db, err := openSQLite(path, false)
	if err != nil {
		return err
	}
	defer db.Close()
	stmts := []struct {
		q    string
		args []any
	}{
		{`DELETE FROM agents WHERE tenant_id != ?`, []any{tenantID}},
		{`DELETE FROM sessions WHERE tenant_id != ?`, []any{tenantID}},
		{`DELETE FROM sessions WHERE agent_id NOT IN (SELECT id FROM agents)`, nil},
		{`DELETE FROM messages WHERE session_id NOT IN (SELECT id FROM sessions)`, nil},
		{`DELETE FROM memories WHERE tenant_id != ?`, []any{tenantID}},
		{`DELETE FROM memories WHERE session_id NOT IN (SELECT id FROM sessions)`, nil},
		{`DELETE FROM vault_docs WHERE tenant_id != ?`, []any{tenantID}},
		{`DELETE FROM vault_links WHERE from_id NOT IN (SELECT id FROM vault_docs)`, nil},
		{`DELETE FROM teams WHERE tenant_id != ?`, []any{tenantID}},
		{`DELETE FROM team_members WHERE team_id NOT IN (SELECT id FROM teams)`, nil},
		{`DELETE FROM team_tasks WHERE team_id NOT IN (SELECT id FROM teams)`, nil},
		{`DELETE FROM team_messages WHERE team_id NOT IN (SELECT id FROM teams)`, nil},
		{`DELETE FROM agent_connectors WHERE agent_id NOT IN (SELECT id FROM agents)`, nil},
		{`DELETE FROM agent_links WHERE from_agent_id NOT IN (SELECT id FROM agents) OR to_agent_id NOT IN (SELECT id FROM agents)`, nil},
		{`DELETE FROM agent_metrics WHERE agent_id NOT IN (SELECT id FROM agents)`, nil},
		{`DELETE FROM agent_tool_flags WHERE agent_id NOT IN (SELECT id FROM agents)`, nil},
		{`DELETE FROM evolution_applies WHERE agent_id NOT IN (SELECT id FROM agents)`, nil},
		{`DELETE FROM evolution_guardrails WHERE agent_id NOT IN (SELECT id FROM agents)`, nil},
		{`DELETE FROM llm_providers WHERE tenant_id != ?`, []any{tenantID}},
		{`DELETE FROM webhooks WHERE tenant_id != ?`, []any{tenantID}},
		{`DELETE FROM webhook_jobs WHERE webhook_id NOT IN (SELECT id FROM webhooks)`, nil},
		{`DELETE FROM kg_entities WHERE tenant_id != ?`, []any{tenantID}},
		{`DELETE FROM kg_relations WHERE tenant_id != ?`, []any{tenantID}},
		{`DELETE FROM cron_jobs WHERE session_id NOT IN (SELECT id FROM sessions)`, nil},
		{`DELETE FROM connectors`, nil},
		{`DELETE FROM agent_connectors`, nil},
		{`DELETE FROM channel_config`, nil},
		{`DELETE FROM channel_pairing`, nil},
		{`DELETE FROM tool_flags`, nil},
	}
	for _, s := range stmts {
		if err := execIgnoreMissing(db, s.q, s.args...); err != nil {
			return err
		}
	}
	return nil
}
