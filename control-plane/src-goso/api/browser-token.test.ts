import assert from "node:assert/strict";
import test from "node:test";
import { classifyPageState, inventoryBlocksMutation } from "./page-state.ts";
import {
  BROWSER_TOKEN_STORAGE_KEY,
  browserTokenClearable,
  browserTokenControlVisible,
  browserTokenKind,
  browserTokenProbeFromInventory,
  browserTokenSaveBlockedByInventory,
  browserTokenWritable,
  classifyTokenProbeStatus,
  clearBrowserToken,
  consumeBrowserTokenProbe,
  emptyBrowserTokenInput,
  hydrateBrowserTokenInput,
  probeBrowserToken,
  saveBrowserToken,
  writeBrowserTokenProbe,
  type BrowserTokenStore,
} from "./browser-token.ts";

function memoryStore(init?: Record<string, string>): BrowserTokenStore {
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

test("empty-on-load never hydrates from storage or env", () => {
  const store = memoryStore({ [BROWSER_TOKEN_STORAGE_KEY]: "stored-sample" });
  assert.equal(emptyBrowserTokenInput(), "");
  assert.equal(hydrateBrowserTokenInput(store, { viteAdminToken: "env-sample" }), "");
  assert.equal(store.getItem(BROWSER_TOKEN_STORAGE_KEY), "stored-sample");
});

test("env-owned disables write and does not touch localStorage", () => {
  const store = memoryStore({ [BROWSER_TOKEN_STORAGE_KEY]: "stored-sample" });
  const env = { viteAdminToken: "env-sample" };
  const kind = browserTokenKind(env, store);
  assert.equal(kind, "env-owned");
  assert.equal(browserTokenWritable(kind), false);
  assert.equal(browserTokenClearable(kind), false);
  const saved = saveBrowserToken("typed-sample", env, store);
  assert.equal(saved.ok, false);
  if (!saved.ok) assert.equal(saved.reason, "env-owned");
  assert.equal(store.getItem(BROWSER_TOKEN_STORAGE_KEY), "stored-sample");
  const cleared = clearBrowserToken(env, store, "typed-sample");
  assert.equal(cleared.ok, false);
  if (!cleared.ok) assert.equal(cleared.reason, "env-owned");
  assert.equal(store.getItem(BROWSER_TOKEN_STORAGE_KEY), "stored-sample");
});

test("save writes localStorage and clears the typed value from state", () => {
  const store = memoryStore();
  const env = { viteAdminToken: "" };
  assert.equal(browserTokenKind(env, store), "unset");
  const empty = saveBrowserToken("   ", env, store);
  assert.equal(empty.ok, false);
  if (!empty.ok) assert.equal(empty.reason, "empty");
  assert.equal(store.getItem(BROWSER_TOKEN_STORAGE_KEY), null);
  const typed = "typed-sample";
  const saved = saveBrowserToken(typed, env, store);
  assert.equal(saved.ok, true);
  assert.equal(saved.input, "");
  assert.equal(saved.reload, true);
  assert.equal(store.getItem(BROWSER_TOKEN_STORAGE_KEY), "typed-sample");
  assert.notEqual(saved.input, typed);
  assert.equal(browserTokenKind(env, store), "set");
  assert.equal(browserTokenWritable("set"), true);
  assert.equal(browserTokenClearable("set"), true);
});

test("clear removes the browser token key", () => {
  const store = memoryStore({ [BROWSER_TOKEN_STORAGE_KEY]: "typed-sample" });
  const env = { viteAdminToken: "" };
  const skipped = clearBrowserToken(env, memoryStore(), "");
  assert.equal(skipped.ok, false);
  if (!skipped.ok) assert.equal(skipped.reason, "unset");
  const cleared = clearBrowserToken(env, store, "leftover");
  assert.equal(cleared.ok, true);
  assert.equal(cleared.input, "");
  assert.equal(store.getItem(BROWSER_TOKEN_STORAGE_KEY), null);
  assert.equal(browserTokenKind(env, store), "unset");
});

test("401 inventory still allows the browser token control", () => {
  const gw = classifyPageState({
    loading: false,
    loaded: false,
    error: new Error('401 {"error":"unauthorized"}'),
    itemCount: 0,
  });
  assert.equal(gw.kind, "permission");
  assert.equal(gw.showItems, false);
  assert.equal(inventoryBlocksMutation(gw.kind), true);
  assert.equal(browserTokenControlVisible(gw.kind), true);
  assert.equal(browserTokenControlVisible("error"), true);
  assert.equal(browserTokenControlVisible("loading"), true);
  assert.equal(browserTokenSaveBlockedByInventory(gw.kind), false);
  assert.equal(inventoryBlocksMutation(gw.kind), true);
  assert.equal(browserTokenProbeFromInventory(gw.kind, "unset"), "");
  assert.equal(browserTokenProbeFromInventory(gw.kind, "set"), "unauthorized");
  assert.equal(browserTokenProbeFromInventory("ready", "set"), "accepted");
  const store = memoryStore();
  const saved = saveBrowserToken("typed-sample", { viteAdminToken: "" }, store);
  assert.equal(saved.ok, true);
  assert.equal(store.getItem(BROWSER_TOKEN_STORAGE_KEY), "typed-sample");
});

test("probe classifies status only and never returns a body", async () => {
  assert.equal(classifyTokenProbeStatus(200), "accepted");
  assert.equal(classifyTokenProbeStatus(401), "unauthorized");
  assert.equal(classifyTokenProbeStatus(403), "unauthorized");
  assert.equal(classifyTokenProbeStatus(0), "unreachable");
  const fetchImpl = async () => ({
    status: 200,
    text: async () => '{"secret":"should-not-leak"}',
  });
  const result = await probeBrowserToken(fetchImpl, "", "typed-sample");
  assert.equal(result, "accepted");
  assert.equal(typeof result === "string" && !result.includes("should-not-leak"), true);
  const unauthorized = await probeBrowserToken(async () => ({ status: 401, text: async () => '{"error":"unauthorized"}' }), "", "");
  assert.equal(unauthorized, "unauthorized");
  const unreachable = await probeBrowserToken(async () => {
    throw new Error("network");
  }, "", "typed-sample");
  assert.equal(unreachable, "unreachable");
  const store = memoryStore();
  writeBrowserTokenProbe(store, result);
  assert.equal(consumeBrowserTokenProbe(store), "accepted");
  assert.equal(consumeBrowserTokenProbe(store), "");
});
