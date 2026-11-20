import assert from "node:assert/strict";
import test from "node:test";
import { asCrmError, CRM_UPSTREAM_DEFAULT, crmAdvisorChrome, crmOrgHeaders } from "./crm.ts";
import { CRM_ORG_TOKEN_STORAGE_KEY, type CrmOrgTokenStore } from "./crm-org-token.ts";
import { crmPermissionBanner, formatCrmPublicError } from "./public-error.ts";
import { en } from "../i18n/en.ts";
import { vi } from "../i18n/vi.ts";

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

test("CRM upstream default is the live goso-crm port", () => {
  assert.equal(CRM_UPSTREAM_DEFAULT, "http://127.0.0.1:8082");
  assert.equal(CRM_UPSTREAM_DEFAULT.includes("8089"), false);
});

test("offline CRM advisor never claims 0 tips or empty advice", () => {
  const s = crmAdvisorChrome({ online: false, permission: false, advisorLoaded: false, adviceCount: 0 });
  assert.equal(s.metaDash, true);
  assert.equal(s.showEmpty, false);
});

test("permission CRM advisor uses dash meta and hides empty table", () => {
  const s = crmAdvisorChrome({ online: true, permission: true, advisorLoaded: false, adviceCount: 0 });
  assert.equal(s.metaDash, true);
  assert.equal(s.showEmpty, false);
});

test("advisor fetch error is dash, not 0 tips", () => {
  const s = crmAdvisorChrome({ online: true, permission: false, advisorLoaded: false, adviceCount: 0 });
  assert.equal(s.metaDash, true);
  assert.equal(s.showEmpty, false);
});

test("checking CRM advisor does not flash empty advice", () => {
  const s = crmAdvisorChrome({ online: null, permission: false, advisorLoaded: false, adviceCount: 0 });
  assert.equal(s.metaDash, true);
  assert.equal(s.showEmpty, false);
});

test("true-empty advisor after a successful load still shows empty", () => {
  const s = crmAdvisorChrome({ online: true, permission: false, advisorLoaded: true, adviceCount: 0 });
  assert.equal(s.metaDash, false);
  assert.equal(s.showEmpty, true);
});

test("loaded advisor with rows shows a count", () => {
  const s = crmAdvisorChrome({ online: true, permission: false, advisorLoaded: true, adviceCount: 3 });
  assert.equal(s.metaDash, false);
  assert.equal(s.showEmpty, false);
});

test("crm headers include localStorage token when env empty", () => {
  const store = memoryStore({ [CRM_ORG_TOKEN_STORAGE_KEY]: "stored-org" });
  const h = crmOrgHeaders("org-1", { viteOrgToken: "" }, store);
  assert.equal(h["X-Org-ID"], "org-1");
  assert.equal(h["X-Org-Token"], "stored-org");
  assert.equal(h.Accept, "application/json");
});

test("env-owned CRM org token wins over localStorage in headers", () => {
  const store = memoryStore({ [CRM_ORG_TOKEN_STORAGE_KEY]: "stored-org" });
  const h = crmOrgHeaders("org-1", { viteOrgToken: "env-org" }, store);
  assert.equal(h["X-Org-Token"], "env-org");
  const empty = crmOrgHeaders("org-1", { viteOrgToken: "" }, memoryStore());
  assert.equal("X-Org-Token" in empty, false);
});

test("CRM 401 copy has no raw JSON", () => {
  const err = new Error('401 {"error":"unauthorized"}');
  assert.equal(asCrmError(err).includes("{"), false);
  assert.equal(asCrmError(err).includes('"error"'), false);
  assert.equal(formatCrmPublicError(err), "");
  assert.equal(formatCrmPublicError(err).includes("{"), false);
  for (const copy of [en["crm.permission"], vi["crm.permission"]]) {
    const banner = crmPermissionBanner(copy, err);
    assert.equal(banner.includes("{"), false);
    assert.equal(banner.includes('"error"'), false);
    assert.equal(banner.includes("401"), false);
    assert.equal(banner, copy);
    assert.match(copy, /gateway/i);
  }
});
