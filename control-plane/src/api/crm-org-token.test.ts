import assert from "node:assert/strict";
import test from "node:test";
import { classifyPageState, inventoryBlocksMutation } from "./page-state.ts";
import {
  CRM_ORG_TOKEN_STORAGE_KEY,
  clearCrmOrgToken,
  crmOrgTokenClearable,
  crmOrgTokenControlVisible,
  crmOrgTokenKind,
  crmOrgTokenSaveBlockedByInventory,
  crmOrgTokenValue,
  crmOrgTokenWritable,
  emptyCrmOrgTokenInput,
  hydrateCrmOrgTokenInput,
  saveCrmOrgToken,
  type CrmOrgTokenStore,
} from "./crm-org-token.ts";

function memoryStore(init?: Record<string, string>): CrmOrgTokenStore {
  const m = new Map(Object.entries(init ?? {}));
  return {
    getItem: (k) => (m.has(k) ? m.get(k)! : null),
    setItem: (k, v) => {
      m.set(k, v);
    },
    removeItem: (k) => {
      m.delete(k);
    },
  };
}

test("empty-on-load never hydrates CRM org token from storage or env", () => {
  const store = memoryStore({ [CRM_ORG_TOKEN_STORAGE_KEY]: "stored-org" });
  assert.equal(emptyCrmOrgTokenInput(), "");
  assert.equal(hydrateCrmOrgTokenInput(store, { viteOrgToken: "env-org" }), "");
  assert.equal(store.getItem(CRM_ORG_TOKEN_STORAGE_KEY), "stored-org");
});

test("env-owned CRM org token disables write and does not touch localStorage", () => {
  const store = memoryStore({ [CRM_ORG_TOKEN_STORAGE_KEY]: "stored-org" });
  const env = { viteOrgToken: "env-org" };
  const kind = crmOrgTokenKind(env, store);
  assert.equal(kind, "env-owned");
  assert.equal(crmOrgTokenWritable(kind), false);
  assert.equal(crmOrgTokenClearable(kind), false);
  assert.equal(crmOrgTokenValue(env, store), "env-org");
  const saved = saveCrmOrgToken("typed-org", env, store);
  assert.equal(saved.ok, false);
  if (!saved.ok) assert.equal(saved.reason, "env-owned");
  assert.equal(store.getItem(CRM_ORG_TOKEN_STORAGE_KEY), "stored-org");
  const cleared = clearCrmOrgToken(env, store, "typed-org");
  assert.equal(cleared.ok, false);
  if (!cleared.ok) assert.equal(cleared.reason, "env-owned");
  assert.equal(store.getItem(CRM_ORG_TOKEN_STORAGE_KEY), "stored-org");
});

test("save writes goso_crm_org_token and clears the typed value from state", () => {
  const store = memoryStore();
  const env = { viteOrgToken: "" };
  assert.equal(crmOrgTokenKind(env, store), "unset");
  const empty = saveCrmOrgToken("   ", env, store);
  assert.equal(empty.ok, false);
  if (!empty.ok) assert.equal(empty.reason, "empty");
  assert.equal(store.getItem(CRM_ORG_TOKEN_STORAGE_KEY), null);
  const typed = "typed-org";
  const saved = saveCrmOrgToken(typed, env, store);
  assert.equal(saved.ok, true);
  assert.equal(saved.input, "");
  assert.equal(saved.reload, true);
  assert.equal(store.getItem(CRM_ORG_TOKEN_STORAGE_KEY), "typed-org");
  assert.notEqual(saved.input, typed);
  assert.equal(crmOrgTokenKind(env, store), "set");
  assert.equal(crmOrgTokenWritable("set"), true);
  assert.equal(crmOrgTokenClearable("set"), true);
  assert.equal(crmOrgTokenValue(env, store), "typed-org");
});

test("clear removes the CRM org token key", () => {
  const store = memoryStore({ [CRM_ORG_TOKEN_STORAGE_KEY]: "typed-org" });
  const env = { viteOrgToken: "" };
  const skipped = clearCrmOrgToken(env, memoryStore(), "");
  assert.equal(skipped.ok, false);
  if (!skipped.ok) assert.equal(skipped.reason, "unset");
  const cleared = clearCrmOrgToken(env, store, "leftover");
  assert.equal(cleared.ok, true);
  assert.equal(cleared.input, "");
  assert.equal(store.getItem(CRM_ORG_TOKEN_STORAGE_KEY), null);
  assert.equal(crmOrgTokenKind(env, store), "unset");
});

test("401 CRM inventory still allows the org token control", () => {
  const crm = classifyPageState({
    loading: false,
    loaded: false,
    error: new Error('401 {"error":"unauthorized"}'),
    itemCount: 0,
  });
  assert.equal(crm.kind, "permission");
  assert.equal(inventoryBlocksMutation(crm.kind), true);
  assert.equal(crmOrgTokenControlVisible(crm.kind), true);
  assert.equal(crmOrgTokenControlVisible("error"), true);
  assert.equal(crmOrgTokenControlVisible("loading"), true);
  assert.equal(crmOrgTokenSaveBlockedByInventory(crm.kind), false);
  const store = memoryStore();
  const saved = saveCrmOrgToken("typed-org", { viteOrgToken: "" }, store);
  assert.equal(saved.ok, true);
  assert.equal(store.getItem(CRM_ORG_TOKEN_STORAGE_KEY), "typed-org");
});

test("CRM org token key is distinct from gateway goso_token", () => {
  assert.equal(CRM_ORG_TOKEN_STORAGE_KEY, "goso_crm_org_token");
  assert.notEqual(CRM_ORG_TOKEN_STORAGE_KEY, "goso_token");
});
