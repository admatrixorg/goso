import assert from "node:assert/strict";
import test from "node:test";
import { classifyPageState, inventoryBlocksMutation, listMetaCount } from "./page-state.ts";
import {
  asPublicArchive,
  asPublicCatalog,
  asPublicJob,
  catalogHasSecrets,
  publicHasSecrets,
  selectionCount,
  toggleId,
  type Catalog,
} from "./impexp-ops.ts";

test("asPublicCatalog drops secret-shaped rows", () => {
  const cat = asPublicCatalog({
    teams: [{ id: "t1", name: "Ops", members: 1 }],
    agents: [{ id: "a1", agent_key: "bot", display_name: "Bot", enabled: true, token: "sk-live-abcdefgh" } as never],
    skills: [{ name: "demo" }],
    mcp: [{ name: "crm", transport: "http", enabled: true, token_set: true, env_owned: false }],
    skills_configured: true,
  });
  assert.equal(cat.teams.length, 1);
  assert.equal(cat.agents.length, 0);
  assert.equal(cat.mcp[0].token_set, true);
  assert.equal(publicHasSecrets(cat.mcp[0]), false);
  assert.equal(catalogHasSecrets(cat), false);
});

test("asPublicArchive refuses include_secrets and token fields", () => {
  assert.equal(asPublicArchive({ schema: "goso.portable/v1", schema_version: 1, include_secrets: true, manifest: { schema_version: 1, secret_policy: "excluded", teams: [], agents: [], skills: [], mcp: [] }, teams: [], agents: [], skills: [], mcp: [] }), undefined);
  assert.equal(
    asPublicArchive({
      schema: "goso.portable/v1",
      schema_version: 1,
      include_secrets: false,
      token: "sk-live-abcdefgh",
      manifest: { schema_version: 1, secret_policy: "excluded", teams: [], agents: [], skills: [], mcp: [] },
      teams: [],
      agents: [],
      skills: [],
      mcp: [],
    } as never),
    undefined,
  );
});

test("asPublicJob drops leaked jobs", () => {
  const ok = asPublicJob({
    id: "pe_1",
    kind: "export",
    status: "done",
    progress: 100,
    dry_run: false,
    steps: [],
    report: { created: [], skipped: [], overwritten: [], renamed: [], failed: [], credentials_needed: [] },
  });
  assert.equal(ok?.id, "pe_1");
  assert.equal(asPublicJob({ id: "pe_2", token: "abc" } as never), undefined);
});

test("selection helpers", () => {
  assert.equal(selectionCount({ team_ids: ["a"], agent_ids: ["b", "c"], skill_names: [], mcp_names: [] }), 3);
  assert.deepEqual(toggleId(["a"], "b"), ["a", "b"]);
  assert.deepEqual(toggleId(["a", "b"], "a"), ["b"]);
});

test("catalogHasSecrets flags nested tokens", () => {
  const leak: Partial<Catalog> = { teams: [{ id: "t", name: "Ops", members: 0, token: "x" } as never] };
  assert.equal(catalogHasSecrets(leak), true);
});

test("catalog 401 is exclusive chrome and blocks export/import", () => {
  const s = classifyPageState({
    loading: false,
    loaded: false,
    error: new Error("401 unauthorized"),
    itemCount: 0,
  });
  assert.equal(s.kind, "permission");
  assert.equal(s.showEmpty, false);
  assert.equal(inventoryBlocksMutation(s.kind), true);
  assert.equal(listMetaCount(s.kind, 0), null);
});

test("public catalog and jobs never carry archive secrets", () => {
  const cat = asPublicCatalog({
    teams: [{ id: "t1", name: "Ops", members: 1 }],
    agents: [],
    skills: [],
    mcp: [{ name: "crm", transport: "http", enabled: true, token_set: true, env_owned: false }],
    skills_configured: false,
  });
  assert.equal(cat.skills_configured, false);
  assert.equal(cat.mcp[0].token_set, true);
  assert.equal(JSON.stringify(cat).includes("sk-"), false);
  const job = asPublicJob({
    id: "pe_1",
    kind: "import",
    status: "done",
    progress: 100,
    dry_run: true,
    steps: [],
    report: { created: [], skipped: [], overwritten: [], renamed: [], failed: [], credentials_needed: [{ kind: "mcp", name: "crm" }] },
  });
  assert.equal(job?.dry_run, true);
});
