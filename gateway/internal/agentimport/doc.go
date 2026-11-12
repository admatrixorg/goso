// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

// Package agentimport loads one tenant from an offline GoClaw agent export
// into the existing goso agent, team, and link records.
//
// The input is a JSON object or JSONL file whose keys are GoClaw column
// names. This package never opens a GoClaw database. It does not create
// tables for shares, per-user overrides, profiles, or config permissions.
// Rows from those tables are counted and not written.
//
// Writes are add-only. A source id that is already mapped is left as it
// was, including when the export text differs. A goso agent_key that
// already exists is not overwritten and is not adopted into the map.
// The map table goclaw_agent_import_map is created only when a run writes.
package agentimport
