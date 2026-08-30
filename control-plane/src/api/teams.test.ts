import assert from "node:assert/strict";
import test from "node:test";
import { classifyPageState, inventoryBlocksMutation } from "./page-state.ts";
import {
  agentLabel,
  EVOLUTION_TEXT_CAP,
  filterTeams,
  isTeamLead,
  linkArrow,
  linkDirection,
  lockedFields,
  namedConfirmTarget,
  mergeAgentLinks,
  resolveAgentLinkLoad,
  filterLinks,
  safeEvolutionText,
  teamDisplayName,
  validateTeamDraft,
} from "./teams-ops.ts";

const teams = [
  { id: "t1", name: "Ops", lead_agent_id: "a-lead" },
  { id: "t2", name: "Support desk", lead_agent_id: "a-sup" },
  { id: "abc-xyz", name: "", lead_agent_id: "" },
];

const agents = [
  { id: "a-lead", agent_key: "lead", display_name: "Lead bot" },
  { id: "a-sup", agent_key: "support", display_name: "  " },
];

test("filterTeams matches name, id, lead", () => {
  assert.equal(filterTeams(teams, "ops").map((t) => t.id).join(), "t1");
  assert.equal(filterTeams(teams, "ABC").map((t) => t.id).join(), "abc-xyz");
  assert.equal(filterTeams(teams, "a-sup").map((t) => t.id).join(), "t2");
  assert.equal(filterTeams(teams, "  ").length, 3);
  assert.equal(filterTeams(teams).length, 3);
});

test("teamDisplayName and agentLabel", () => {
  assert.equal(teamDisplayName({ id: "t1", name: " Ops " }), "Ops");
  assert.equal(teamDisplayName({ id: "missing", name: "  " }), "missing");
  assert.equal(agentLabel(agents, "a-lead"), "Lead bot");
  assert.equal(agentLabel(agents, "a-sup"), "support");
  assert.equal(agentLabel(agents, "gone"), "gone");
});

test("validateTeamDraft and lead helper", () => {
  assert.equal(validateTeamDraft("  ", "a1"), "teams.needName");
  assert.equal(validateTeamDraft("Ops", "  "), "teams.needLead");
  assert.equal(validateTeamDraft("Ops", "a1"), null);
  assert.equal(isTeamLead({ lead_agent_id: "a-lead" }, "a-lead"), true);
  assert.equal(isTeamLead({ lead_agent_id: "a-lead" }, "a-sup"), false);
  assert.equal(isTeamLead(undefined, "a-lead"), false);
});

test("link direction is visible as directed or bidirectional", () => {
  assert.equal(linkDirection({}), "directed");
  assert.equal(linkDirection({ bidirectional: false }), "directed");
  assert.equal(linkDirection({ bidirectional: true }), "bidirectional");
  assert.equal(linkArrow("directed"), "→");
  assert.equal(linkArrow("bidirectional"), "↔");
});

test("namedConfirmTarget requires the exact name", () => {
  assert.equal(namedConfirmTarget("Ops", "Ops"), true);
  assert.equal(namedConfirmTarget("Ops", " Ops "), true);
  assert.equal(namedConfirmTarget("Ops", "ops"), false);
  assert.equal(namedConfirmTarget("Ops", "Support"), false);
  assert.equal(namedConfirmTarget("  ", "  "), false);
});

test("mergeAgentLinks dedupes per-agent lists and drops blanks", () => {
  const merged = mergeAgentLinks([
    [
      { from_agent_id: "a", to_agent_id: "b", bidirectional: true },
      { from_agent_id: "a", to_agent_id: "c" },
    ],
    [
      { from_agent_id: "a", to_agent_id: "b", bidirectional: true },
      { from_agent_id: " ", to_agent_id: "x" },
    ],
  ]);
  assert.equal(merged.length, 2);
  assert.equal(merged[0]?.to_agent_id, "b");
  assert.equal(merged[1]?.to_agent_id, "c");
});

test("failed agent inventory does not become a successful empty link list", () => {
  const inventoryErr = new Error("non-JSON response");
  const resolved = resolveAgentLinkLoad({
    agentInventoryError: inventoryErr,
    agentInventoryLoaded: false,
    groups: [],
  });
  assert.equal(resolved.loaded, false);
  assert.equal(resolved.error, inventoryErr);
  assert.equal(resolved.links.length, 0);
  const page = classifyPageState({
    loading: false,
    loaded: resolved.loaded,
    error: resolved.error,
    itemCount: resolved.links.length,
  });
  assert.equal(page.kind, "error");
  assert.equal(page.showEmpty, false);
  assert.equal(inventoryBlocksMutation(page.kind), true);
});

test("permission on agent inventory blocks link create and is not empty", () => {
  const resolved = resolveAgentLinkLoad({
    agentInventoryError: new Error('401 {"error":"unauthorized"}'),
    agentInventoryLoaded: false,
    groups: [],
  });
  const page = classifyPageState({
    loading: false,
    loaded: resolved.loaded,
    error: resolved.error,
    itemCount: resolved.links.length,
  });
  assert.equal(page.kind, "permission");
  assert.equal(page.showEmpty, false);
  assert.equal(inventoryBlocksMutation(page.kind), true);
});

test("successful empty agent inventory is true-empty links", () => {
  const resolved = resolveAgentLinkLoad({
    agentInventoryError: null,
    agentInventoryLoaded: true,
    groups: [],
  });
  assert.equal(resolved.loaded, true);
  assert.equal(resolved.error, null);
  const page = classifyPageState({
    loading: false,
    loaded: resolved.loaded,
    error: resolved.error,
    itemCount: resolved.links.length,
  });
  assert.equal(page.kind, "empty");
  assert.equal(page.showEmpty, true);
  assert.equal(inventoryBlocksMutation(page.kind), false);
});

test("partial link fetch failure keeps the upstream error", () => {
  const boom = new Error("502 upstream");
  const resolved = resolveAgentLinkLoad({
    agentInventoryError: null,
    agentInventoryLoaded: true,
    groups: [
      { status: "fulfilled", value: [{ from_agent_id: "a", to_agent_id: "b" }] },
      { status: "rejected", reason: boom },
    ],
  });
  assert.equal(resolved.loaded, true);
  assert.equal(resolved.error, boom);
  assert.equal(resolved.links.length, 1);
});

test("filterLinks matches source/target labels", () => {
  const links = [
    { from_agent_id: "a1", to_agent_id: "a2", bidirectional: false },
    { from_agent_id: "a3", to_agent_id: "a4", bidirectional: true },
  ];
  const label = (id: string) => (id === "a2" ? "Sales" : id);
  assert.equal(filterLinks(links, "sales", label).length, 1);
  assert.equal(filterLinks(links, "a3", label).length, 1);
  assert.equal(filterLinks(links, "  ", label).length, 2);
});

test("safeEvolutionText never dumps a full system prompt", () => {
  assert.equal(safeEvolutionText("  Tool failure rate is high.  "), "Tool failure rate is high.");
  const dump = "You are a helpful assistant. ".repeat(40) + "instructions: " + "secret policy ".repeat(20);
  const out = safeEvolutionText(dump);
  assert.ok(out.endsWith("…"));
  assert.ok(out.length <= EVOLUTION_TEXT_CAP + 1);
  assert.equal(out.includes("secret policy"), false);
  assert.deepEqual(lockedFields({ locked: [" display_name ", "agent_key", ""] }), ["display_name", "agent_key"]);
  assert.deepEqual(lockedFields(undefined), []);
});
