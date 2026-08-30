import assert from "node:assert/strict";
import test from "node:test";
import { clampPageOffset, pageSlice } from "./page-state.ts";
import {
  SELECTED_SESSION_KEY,
  SESSION_PAGE_SIZE,
  agentLabel,
  clearSelectedSession,
  filterSessions,
  isGoneStatus,
  normalizePromptMode,
  parseSelectedSession,
  readSelectedSession,
  sessionActivityAt,
  sessionDisplayName,
  streamReconnectDelayMs,
  writeSelectedSession,
} from "./sessions.ts";

function memStore(init: Record<string, string> = {}) {
  const data = { ...init };
  return {
    getItem(key: string) {
      return Object.prototype.hasOwnProperty.call(data, key) ? data[key] : null;
    },
    setItem(key: string, value: string) {
      data[key] = value;
    },
    removeItem(key: string) {
      delete data[key];
    },
    data,
  };
}

const rows = [
  { id: "s1", agent_id: "a1", label: "Sales standup" },
  { id: "s2", agent_id: "a2", label: "Support" },
  { id: "abc-xyz", agent_id: "a1" },
];

test("filterSessions matches label, id, and agent", () => {
  assert.equal(filterSessions(rows, { query: "stand" }).map((s) => s.id).join(), "s1");
  assert.equal(filterSessions(rows, { query: "ABC" }).map((s) => s.id).join(), "abc-xyz");
  assert.equal(filterSessions(rows, { agentId: "a1" }).length, 2);
  assert.equal(filterSessions(rows, { query: "s1", agentId: "a2" }).length, 0);
  assert.equal(filterSessions(rows, { query: "  ", agentId: "" }).length, 3);
});

test("session pagination recovers when the list shrinks after delete", () => {
  const many = Array.from({ length: 45 }, (_, i) => ({ id: `s${i}`, agent_id: "a1" }));
  assert.equal(SESSION_PAGE_SIZE, 20);
  assert.equal(pageSlice(many, 40, SESSION_PAGE_SIZE).length, 5);
  const afterDelete = many.slice(0, 21);
  const off = clampPageOffset(afterDelete.length, 40, SESSION_PAGE_SIZE);
  assert.equal(off, 20);
  assert.equal(pageSlice(afterDelete, off, SESSION_PAGE_SIZE).map((s) => s.id).join(), "s20");
  assert.equal(filterSessions(many, { query: "no-such" }).length, 0);
});

test("sessionDisplayName prefers label then id", () => {
  assert.equal(sessionDisplayName({ id: "s1", label: " Sales " }), "Sales");
  assert.equal(sessionDisplayName({ id: "s1", label: "  " }), "s1");
});

test("sessionActivityAt uses created_at only", () => {
  assert.equal(sessionActivityAt({ created_at: " 2026-08-30T01:02:03Z " }), "2026-08-30T01:02:03Z");
  assert.equal(sessionActivityAt({}), "");
});

test("agentLabel uses display name then key", () => {
  const agents = [
    { id: "a1", display_name: "Sales bot", agent_key: "sales" },
    { id: "a2", display_name: "", agent_key: "support" },
  ];
  assert.equal(agentLabel(agents, "a1"), "Sales bot");
  assert.equal(agentLabel(agents, "a2"), "support");
  assert.equal(agentLabel(agents, "missing"), "missing");
});

test("selected session persists id+label and ignores junk", () => {
  const store = memStore();
  writeSelectedSession({ id: " s9 ", label: " Night " }, store);
  assert.equal(store.data[SELECTED_SESSION_KEY], JSON.stringify({ id: "s9", label: "Night" }));
  assert.deepEqual(readSelectedSession(store), { id: "s9", label: "Night" });
  assert.deepEqual(parseSelectedSession("bare-id"), { id: "bare-id", label: "bare-id" });
  assert.equal(parseSelectedSession("{"), null);
  assert.equal(parseSelectedSession('{"id":""}'), null);
  clearSelectedSession(store);
  assert.equal(readSelectedSession(store), null);
});

test("normalizePromptMode falls back to full", () => {
  assert.equal(normalizePromptMode("task"), "task");
  assert.equal(normalizePromptMode("TASK"), "task");
  assert.equal(normalizePromptMode("weird"), "full");
  assert.equal(normalizePromptMode(undefined), "full");
});

test("streamReconnectDelayMs backs off and caps", () => {
  assert.equal(streamReconnectDelayMs(0), 400);
  assert.equal(streamReconnectDelayMs(1), 800);
  assert.equal(streamReconnectDelayMs(10), 4000);
  assert.equal(streamReconnectDelayMs(-2), 400);
});

test("isGoneStatus detects 404", () => {
  assert.equal(isGoneStatus(new Error('404 {"error":"session not found"}')), true);
  assert.equal(isGoneStatus(new Error("502 upstream")), false);
});
