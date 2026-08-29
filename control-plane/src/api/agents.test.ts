import assert from "node:assert/strict";
import test from "node:test";
import {
  agentConflictKind,
  agentDisplayName,
  filterAgents,
  isAgentActive,
  isConflictStatus,
  uniqueProviders,
  validateAgentKey,
} from "./agents.ts";

const rows = [
  { id: "a1", agent_key: "sales", display_name: "Sales bot", model: "flash", llm_provider: "groq", enabled: true },
  { id: "a2", agent_key: "support", display_name: "Support", model: "echo", llm_provider: "groq", enabled: false },
  { id: "abc-xyz", agent_key: "ops", display_name: "", model: "", llm_provider: "", enabled: true },
];

test("filterAgents matches key, name, id, model, provider", () => {
  assert.equal(filterAgents(rows, { query: "sales" }).map((a) => a.id).join(), "a1");
  assert.equal(filterAgents(rows, { query: "ABC" }).map((a) => a.id).join(), "abc-xyz");
  assert.equal(filterAgents(rows, { query: "flash" }).map((a) => a.id).join(), "a1");
  assert.equal(filterAgents(rows, { provider: "groq" }).length, 2);
  assert.equal(filterAgents(rows, { query: "sales", provider: "groq" }).length, 1);
  assert.equal(filterAgents(rows, { query: "  ", status: "", provider: "" }).length, 3);
});

test("filterAgents status uses enabled flag", () => {
  assert.equal(filterAgents(rows, { status: "active" }).map((a) => a.id).join(), "a1,abc-xyz");
  assert.equal(filterAgents(rows, { status: "inactive" }).map((a) => a.id).join(), "a2");
  assert.equal(isAgentActive({ enabled: undefined }), true);
  assert.equal(isAgentActive({ enabled: false }), false);
});

test("agentDisplayName prefers display name then key", () => {
  assert.equal(agentDisplayName({ id: "a1", display_name: " Sales ", agent_key: "sales" }), "Sales");
  assert.equal(agentDisplayName({ id: "a2", display_name: "  ", agent_key: "support" }), "support");
  assert.equal(agentDisplayName({ id: "missing" }), "missing");
});

test("uniqueProviders skips blanks", () => {
  assert.deepEqual(uniqueProviders(rows), ["groq"]);
});

test("validateAgentKey only on create", () => {
  assert.equal(validateAgentKey("  ", false), "agents.needKey");
  assert.equal(validateAgentKey("sales", false), null);
  assert.equal(validateAgentKey("", true), null);
});

test("isConflictStatus and agentConflictKind", () => {
  assert.equal(isConflictStatus(new Error('409 {"error":"agent was modified"}')), true);
  assert.equal(agentConflictKind(new Error('409 {"error":"agent was modified"}')), "conflict");
  assert.equal(agentConflictKind(new Error('409 {"error":"agent is team lead"}')), "lead");
  assert.equal(agentConflictKind(new Error('409 {"error":"agent is inactive"}')), "inactive");
  assert.equal(agentConflictKind(new Error("502 upstream")), null);
});
