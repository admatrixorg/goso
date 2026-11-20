import assert from "node:assert/strict";
import test from "node:test";
import {
  BODY_CAP,
  capRows,
  classifyMemoryList,
  filterMemories,
  hasBothLanes,
  isEmbeddingConfigured,
  listTargetName,
  memoryCreateBlocked,
  memoryFilteredEmpty,
  memoryFormBlocked,
  memoryLane,
  memoryMutationsBlocked,
  memorySnippet,
  normalizeKind,
  plainMemoryBody,
} from "./memory-ops.ts";
import { classifyPageState, inventoryBlocksMutation } from "./page-state.ts";

const rows = [
  { id: "e1", session_id: "s1", agent_id: "a1", kind: "episodic", snippet: "session banana note" },
  { id: "d1", session_id: "s1", agent_id: "a1", kind: "durable", snippet: "playbook charter" },
  { id: "d2", session_id: "s2", agent_id: "a2", kind: "document", snippet: "other agent policy" },
];

test("normalizeKind maps document to durable", () => {
  assert.equal(normalizeKind(""), "episodic");
  assert.equal(normalizeKind("Episodic"), "episodic");
  assert.equal(normalizeKind("document"), "durable");
  assert.equal(normalizeKind("DURABLE"), "durable");
  assert.equal(normalizeKind("custom"), "custom");
});

test("memoryLane splits episodic vs durable", () => {
  assert.equal(memoryLane("episodic"), "episodic");
  assert.equal(memoryLane("message"), "episodic");
  assert.equal(memoryLane("durable"), "durable");
  assert.equal(memoryLane("document"), "durable");
  assert.equal(memoryLane("policy"), "durable");
});

test("filterMemories matches query, agent, session, lane", () => {
  assert.equal(filterMemories(rows, { query: "banana" }).map((r) => r.id).join(), "e1");
  assert.equal(filterMemories(rows, { agent: "a1" }).map((r) => r.id).join(), "e1,d1");
  assert.equal(filterMemories(rows, { session: "s2" }).map((r) => r.id).join(), "d2");
  assert.equal(filterMemories(rows, { lane: "durable" }).map((r) => r.id).join(), "d1,d2");
  assert.equal(filterMemories(rows, { lane: "episodic", agent: "a1" }).map((r) => r.id).join(), "e1");
  assert.equal(filterMemories(rows, { query: "play", lane: "episodic" }).length, 0);
});

test("hasBothLanes is true only when both exist", () => {
  assert.equal(hasBothLanes(rows), true);
  assert.equal(hasBothLanes(rows.filter((r) => memoryLane(r.kind) === "episodic")), false);
  assert.equal(hasBothLanes([]), false);
});

test("memorySnippet truncates and strips NUL", () => {
  assert.equal(memorySnippet("short"), "short");
  assert.equal(memorySnippet("x".repeat(90)).endsWith("…"), true);
  assert.equal(memorySnippet("x".repeat(90)).length, 81);
  assert.equal(memorySnippet("\u0000secret"), "secret");
});

test("plainMemoryBody stays plain text and caps length", () => {
  const inject = '<img src=x onerror="alert(1)"><script>alert(1)</script>';
  assert.equal(plainMemoryBody(inject), inject);
  assert.ok(plainMemoryBody("a".repeat(BODY_CAP + 10)).endsWith("…"));
});

test("listTargetName prefers snippet over id", () => {
  assert.equal(listTargetName({ snippet: "hello world", id: "abc" }), "hello world");
  assert.equal(listTargetName({ id: "abc" }), "abc");
});

test("isEmbeddingConfigured is false for missing or not_configured", () => {
  assert.equal(isEmbeddingConfigured(null), false);
  assert.equal(isEmbeddingConfigured({ embedding: "not_configured", embedding_configured: false }), false);
  assert.equal(isEmbeddingConfigured({ embedding: "openai", embedding_configured: true }), true);
  assert.equal(isEmbeddingConfigured({ embedding_configured: false, embedding: "" }), false);
});

test("capRows truncates at the cap", () => {
  const capped = capRows(rows, 2);
  assert.equal(capped.rows.length, 2);
  assert.equal(capped.truncated, true);
  assert.equal(capRows(rows, 10).truncated, false);
});

test("memory list permission is not true-empty and blocks mutations", () => {
  const perm = classifyMemoryList({
    loading: false,
    loaded: false,
    error: new Error("401 unauthorized"),
    itemCount: 0,
  });
  assert.equal(perm.kind, "permission");
  assert.equal(perm.showEmpty, false);
  assert.equal(perm.showItems, false);
  assert.equal(memoryMutationsBlocked(perm), true);
  assert.equal(inventoryBlocksMutation(perm.kind), true);
});

test("agent/session inventory failure is independent of memory-list empty", () => {
  const notesOk = classifyMemoryList({ loading: false, loaded: true, error: null, itemCount: 0 });
  const sessFail = classifyPageState({
    loading: false,
    loaded: false,
    error: new Error("403 forbidden"),
    itemCount: 0,
  });
  const sessOk = classifyPageState({ loading: false, loaded: true, error: null, itemCount: 2 });
  assert.equal(notesOk.kind, "empty");
  assert.equal(notesOk.showEmpty, true);
  assert.equal(memoryCreateBlocked(notesOk, sessFail, "s1"), true);
  assert.equal(memoryFormBlocked(notesOk, sessFail), true);
  assert.equal(memoryCreateBlocked(notesOk, sessOk, ""), true);
  assert.equal(memoryFormBlocked(notesOk, sessOk), false);
  assert.equal(memoryCreateBlocked(notesOk, sessOk, "s1"), false);
  assert.equal(memoryMutationsBlocked(notesOk), false);
});

test("filtered empty is distinct from true empty", () => {
  const ready = classifyMemoryList({ loading: false, loaded: true, error: null, itemCount: 3 });
  const empty = classifyMemoryList({ loading: false, loaded: true, error: null, itemCount: 0 });
  assert.equal(memoryFilteredEmpty(ready, 3, 0), true);
  assert.equal(memoryFilteredEmpty(ready, 3, 1), false);
  assert.equal(memoryFilteredEmpty(empty, 0, 0), false);
});

test("embedding not-configured is metadata, not vendor success or permission", () => {
  assert.equal(isEmbeddingConfigured({ embedding: "not_configured", embedding_configured: false }), false);
  const perm = classifyMemoryList({
    loading: false,
    loaded: false,
    error: new Error('401 {"error":"unauthorized"}'),
    itemCount: 0,
  });
  assert.equal(perm.kind, "permission");
  assert.notEqual(perm.kind, "empty");
});

test("named delete target prefers snippet", () => {
  assert.equal(listTargetName({ snippet: "session banana", id: "e1", kind: "episodic" }), "session banana");
  assert.equal(listTargetName({ id: "e1", kind: "episodic" }), "e1");
});
