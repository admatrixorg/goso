import assert from "node:assert/strict";
import test from "node:test";
import {
  DEMO_HASH_TABS,
  SETTINGS_PAGES,
  hashForTab,
  liveTabIds,
  parseHash,
  serializeHash,
  type Tab,
} from "./hash-route.ts";

test("every live tab round-trips through serialize/parse", () => {
  for (const tab of liveTabIds()) {
    const hash = serializeHash({ tab });
    const parsed = parseHash(hash);
    assert.equal(parsed.tab, tab, tab);
    assert.equal(parsed.unknown, false, tab);
    assert.equal(parsed.canonical, hash, tab);
    assert.equal(hashForTab(tab), hash, tab);
  }
});

test("overview hashes map to crm without crashing", () => {
  for (const h of ["", "#", "#/", "#/overview"]) {
    const p = parseHash(h);
    assert.equal(p.tab, "crm", h);
    assert.equal(p.unknown, false, h);
    assert.equal(p.rewrite, false, h);
  }
  assert.equal(serializeHash({ tab: "crm" }), "#/overview");
});

test("unknown hash rewrites to overview", () => {
  const p = parseHash("#/not-a-menu");
  assert.equal(p.tab, "crm");
  assert.equal(p.unknown, true);
  assert.equal(p.rewrite, true);
  assert.equal(p.canonical, "#/overview");
  const extra = parseHash("#/agents/nope");
  assert.equal(extra.tab, "agents");
  assert.equal(extra.rewrite, true);
  assert.equal(extra.canonical, "#/agents");
});

test("old #traces aliases rewrite to #/traces", () => {
  const list = parseHash("#traces");
  assert.equal(list.tab, "traces");
  assert.equal(list.traceId, undefined);
  assert.equal(list.rewrite, true);
  assert.equal(list.canonical, "#/traces");
  const one = parseHash("#traces/abc_1");
  assert.equal(one.tab, "traces");
  assert.equal(one.traceId, "abc_1");
  assert.equal(one.rewrite, true);
  assert.equal(one.canonical, "#/traces/abc_1");
  assert.equal(parseHash("#/traces/abc_1").traceId, "abc_1");
  assert.equal(parseHash("#/traces/abc_1").rewrite, false);
  assert.equal(serializeHash({ tab: "traces", traceId: "abc_1" }), "#/traces/abc_1");
  assert.equal(serializeHash({ tab: "traces" }), "#/traces");
});

test("settings pages serialize under #/config", () => {
  assert.equal(serializeHash({ tab: "settings" }), "#/config");
  assert.equal(parseHash("#/config").tab, "settings");
  assert.equal(parseHash("#/config").settingsPage, "gateway");
  for (const page of SETTINGS_PAGES) {
    const hash = serializeHash({ tab: "settings", settingsPage: page });
    const parsed = parseHash(hash);
    assert.equal(parsed.tab, "settings", page);
    assert.equal(parsed.settingsPage, page, page);
    if (page === "gateway") assert.equal(hash, "#/config");
    else assert.equal(hash, `#/config/${page}`);
  }
  const account = parseHash("#/config/account");
  assert.equal(account.tab, "settings");
  assert.equal(account.settingsPage, "account");
  assert.equal(account.canonical, "#/config/account");
  assert.equal(account.rewrite, false);
  const gateway = parseHash("#/config/gateway");
  assert.equal(gateway.tab, "settings");
  assert.equal(gateway.settingsPage, "gateway");
  assert.equal(gateway.canonical, "#/config");
  assert.equal(gateway.rewrite, true);
  const bad = parseHash("#/config/not-a-page");
  assert.equal(bad.tab, "settings");
  assert.equal(bad.settingsPage, "gateway");
  assert.equal(bad.rewrite, true);
  assert.equal(bad.canonical, "#/config");
});

test("bare #/config and empty settings hash open gateway not account", () => {
  for (const h of ["#/config", "#/config/", "#/config/gateway"]) {
    const p = parseHash(h);
    assert.equal(p.tab, "settings", h);
    assert.equal(p.settingsPage, "gateway", h);
    assert.equal(p.canonical, "#/config", h);
  }
  assert.equal(serializeHash({ tab: "settings" }), "#/config");
  assert.equal(serializeHash({ tab: "settings", settingsPage: "gateway" }), "#/config");
  assert.equal(serializeHash({ tab: "settings", settingsPage: "account" }), "#/config/account");
  assert.equal(hashForTab("settings"), "#/config");
  assert.equal(hashForTab("settings", { settingsPage: "gateway" }), "#/config");
  assert.equal(hashForTab("settings", { settingsPage: "account" }), "#/config/account");
});

test("demo tabs parse only in demo mode", () => {
  for (const tab of DEMO_HASH_TABS) {
    const hash = serializeHash({ tab }, { demo: true });
    assert.equal(hash, `#/${tab}`);
    const live = parseHash(hash);
    assert.equal(live.tab, "crm", tab);
    assert.equal(live.unknown, true, tab);
    assert.equal(live.canonical, "#/overview", tab);
    const demo = parseHash(hash, { demo: true });
    assert.equal(demo.tab, tab, tab);
    assert.equal(demo.unknown, false, tab);
    assert.equal(serializeHash({ tab }, { demo: false }), "#/overview", tab);
  }
});

test("functions segment is a tools alias", () => {
  const p = parseHash("#/functions");
  assert.equal(p.tab, "tools");
  assert.equal(p.rewrite, true);
  assert.equal(p.canonical, "#/tools");
  assert.equal(serializeHash({ tab: "functions" }), "#/tools");
});

test("hashForTab preserves traces id and settings page", () => {
  assert.equal(hashForTab("traces", { traceId: "t-9" }), "#/traces/t-9");
  assert.equal(hashForTab("settings", { settingsPage: "gateway" }), "#/config");
  assert.equal(hashForTab("settings", { settingsPage: "account" }), "#/config/account");
  const live: Tab[] = ["chat", "mcp", "kg", "impexp"];
  for (const tab of live) assert.match(hashForTab(tab), new RegExp(`^#/${tab === "settings" ? "config" : tab}`));
});
