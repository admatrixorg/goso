import assert from "node:assert/strict";
import test from "node:test";
import {
  formatWhen,
  isEmbeddingConfigured,
  isInferred,
  kgSnippet,
  normalizeGraph,
  normalizeScope,
  plainKgBody,
  publicHasSecrets,
} from "./kg-ops.ts";

test("normalizeScope maps recorded/inferred aliases", () => {
  assert.equal(normalizeScope(""), "");
  assert.equal(normalizeScope("all"), "all");
  assert.equal(normalizeScope("posted"), "posted");
  assert.equal(normalizeScope("recorded"), "posted");
  assert.equal(normalizeScope("extracted"), "extracted");
  assert.equal(normalizeScope("inferred"), "extracted");
});

test("isInferred treats extracted source as non-fact", () => {
  assert.equal(isInferred({ inferred: true }), true);
  assert.equal(isInferred({ source: "extracted" }), true);
  assert.equal(isInferred({ source: "posted" }), false);
  assert.equal(isInferred({}), false);
});

test("kgSnippet and plainKgBody drop secret-shaped text", () => {
  assert.equal(kgSnippet("Acme Billing"), "Acme Billing");
  assert.equal(kgSnippet("sk-live-abcdefghijk"), "");
  assert.equal(plainKgBody("hello"), "hello");
  assert.equal(plainKgBody("Bearer abcdefghijk"), "");
});

test("isEmbeddingConfigured stays false for not_configured", () => {
  assert.equal(isEmbeddingConfigured(null), false);
  assert.equal(isEmbeddingConfigured({ embedding: "not_configured", embedding_configured: false }), false);
  assert.equal(isEmbeddingConfigured({ embedding_configured: true }), true);
});

test("normalizeGraph caps nodes/edges, strips secrets, keeps inferred_are_not_facts", () => {
  const g = normalizeGraph(
    {
      nodes: [
        { id: "n1", name: "Acme", kind: "org", snippet: "invoices", source: "posted", inferred: false },
        { id: "n2", name: "Zeta", snippet: "stock", source: "extracted", inferred: true },
        { id: "n3", name: "Leak", snippet: "ok", api_key: "sk-live-abcdefgh" },
        { id: "", name: "skip" },
      ],
      edges: [
        { id: "e1", from_id: "n1", to_id: "n2", rel: "ships_to", source: "extracted", inferred: true },
        { id: "e2", from_id: "n1", to_id: "n2", rel: "secret", token: "abc" },
      ],
      truncated: false,
      node_cap: 40,
      inferred_are_not_facts: true,
      embedding: "not_configured",
    },
    40,
  );
  assert.equal(g.nodes.length, 2);
  assert.equal(g.nodes[0].id, "n1");
  assert.equal(g.nodes[1].inferred, true);
  assert.equal(g.edges.length, 1);
  assert.equal(g.edges[0].inferred, true);
  assert.equal(g.inferred_are_not_facts, true);
  assert.equal(g.embedding_configured, false);
  assert.equal(publicHasSecrets(g.nodes[0]), false);
});

test("normalizeGraph truncates at cap", () => {
  const g = normalizeGraph(
    {
      nodes: [
        { id: "a", name: "A" },
        { id: "b", name: "B" },
      ],
      edges: [],
      node_cap: 1,
    },
    1,
  );
  assert.equal(g.nodes.length, 1);
  assert.equal(g.truncated, true);
  assert.equal(g.node_cap, 1);
});

test("formatWhen keeps empty fallback", () => {
  assert.equal(formatWhen("", "n/a"), "n/a");
  assert.equal(formatWhen("not-a-date", "n/a"), "not-a-date");
  const shown = formatWhen("2026-08-30T12:00:00Z", "n/a");
  assert.notEqual(shown, "n/a");
});
