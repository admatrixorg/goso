import assert from "node:assert/strict";
import test from "node:test";
import {
  BODY_CAP,
  boundNeighborhood,
  capRows,
  classifyDoc,
  filterVaultDocs,
  formatMtime,
  GRAPH_NODE_CAP,
  isStaleHealth,
  LIST_CAP,
  normalizeGraph,
  parseVaultFrontmatter,
  plainVaultBody,
  shortHash,
  uniqueField,
} from "./vault-ops.ts";

const docs = [
  {
    id: "d1",
    title: "Playbook",
    path: "agents/sales/playbook.md",
    body: "# Playbook\nsee [[Charter]]",
  },
  {
    id: "d2",
    title: "Charter",
    path: "teams/ops/charter.md",
    body: "---\ntype: policy\nagent: desk\nteam: support\n---\n# Charter",
  },
  {
    id: "d3",
    title: "TEAM",
    path: "TEAM.md",
    body: "members",
  },
  {
    id: "d4",
    title: "Scratch",
    path: "scratch.txt",
    body: "plain",
  },
];

test("classifyDoc uses path prefixes and TEAM.md", () => {
  assert.deepEqual(classifyDoc(docs[0]), { type: "agent", agent: "sales", team: "" });
  assert.deepEqual(classifyDoc(docs[2]), { type: "team", agent: "", team: "team" });
  assert.deepEqual(classifyDoc(docs[3]), { type: "text", agent: "", team: "" });
  assert.equal(classifyDoc({ id: "x", path: "note.md" }).type, "markdown");
});

test("frontmatter overrides path class", () => {
  const fm = parseVaultFrontmatter(docs[1].body);
  assert.equal(fm.type, "policy");
  assert.equal(fm.agent, "desk");
  assert.equal(fm.team, "support");
  assert.deepEqual(classifyDoc(docs[1]), { type: "policy", agent: "desk", team: "support" });
});

test("filterVaultDocs matches query, type, agent, team", () => {
  assert.equal(filterVaultDocs(docs, { query: "play" }).map((d) => d.id).join(), "d1");
  assert.equal(filterVaultDocs(docs, { query: "D3" }).map((d) => d.id).join(), "d3");
  assert.equal(filterVaultDocs(docs, { type: "agent" }).map((d) => d.id).join(), "d1");
  assert.equal(filterVaultDocs(docs, { agent: "desk" }).map((d) => d.id).join(), "d2");
  assert.equal(filterVaultDocs(docs, { team: "support" }).map((d) => d.id).join(), "d2");
  assert.equal(filterVaultDocs(docs, { query: "  ", type: "", agent: "", team: "" }).length, 4);
  assert.equal(filterVaultDocs(docs, { query: "play", type: "text" }).length, 0);
});

test("uniqueField lists distinct class values", () => {
  assert.deepEqual(uniqueField(docs, "type"), ["agent", "policy", "team", "text"]);
  assert.deepEqual(uniqueField(docs, "agent"), ["desk", "sales"]);
});

test("plainVaultBody stays plain text and caps length", () => {
  const inject = '<img src=x onerror="alert(1)"><script>alert(1)</script>';
  const out = plainVaultBody(inject);
  assert.equal(out, inject);
  assert.equal(out.includes("<script>"), true);
  const big = "a".repeat(BODY_CAP + 50);
  const capped = plainVaultBody(big);
  assert.equal(capped.length, BODY_CAP + 1);
  assert.ok(capped.endsWith("…"));
  assert.equal(plainVaultBody("a\u0000b"), "ab");
});

test("isStaleHealth uses stale flag or mismatch counts", () => {
  assert.equal(isStaleHealth(null), false);
  assert.equal(isStaleHealth({ stale: false }), false);
  assert.equal(isStaleHealth({ stale: true }), true);
  assert.equal(isStaleHealth({ stale: false, unindexed: 1 }), true);
  assert.equal(isStaleHealth({ missing_on_disk: 2 }), true);
});

test("boundNeighborhood caps nodes and keeps a usable list", () => {
  const selected = docs[0];
  const outbound = [{ from_id: "d1", to_id: "d2", raw: "Charter" }];
  const inbound = [{ from_id: "d3", to_id: "d1", raw: "Playbook" }];
  const g = boundNeighborhood(selected, inbound, outbound, docs, 2);
  assert.equal(g.nodes.length, 2);
  assert.equal(g.truncated, true);
  assert.equal(g.node_cap, 2);
  assert.equal(g.edges.length, 2);
  const full = boundNeighborhood(selected, inbound, outbound, docs);
  assert.ok(full.nodes.length <= GRAPH_NODE_CAP);
  assert.equal(full.truncated, false);
});

test("normalizeGraph and capRows enforce limits", () => {
  const g = normalizeGraph(
    {
      nodes: [
        { id: "a", title: "A", path: "a.md" },
        { id: "b", title: "B", path: "b.md" },
        { id: "a", title: "dup" },
      ],
      edges: [{ from_id: "a", to_id: "b", raw: "B" }],
      truncated: false,
      node_cap: 40,
    },
    1,
  );
  assert.equal(g.nodes.length, 1);
  assert.equal(g.truncated, true);
  assert.equal(g.edges[0].raw, "B");
  const many = Array.from({ length: LIST_CAP + 3 }, (_, i) => i);
  const capped = capRows(many);
  assert.equal(capped.rows.length, LIST_CAP);
  assert.equal(capped.truncated, true);
});

test("shortHash and formatMtime are operator-safe", () => {
  assert.equal(shortHash(""), "—");
  assert.equal(shortHash("abcdefghijklmnop"), "abcdefghijkl");
  assert.equal(formatMtime(""), "—");
  assert.equal(formatMtime("not-a-date"), "not-a-date");
  assert.equal(formatMtime("2026-08-30T12:00:00.000Z"), "2026-08-30 12:00:00 UTC");
});
