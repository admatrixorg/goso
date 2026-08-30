import assert from "node:assert/strict";
import test from "node:test";
import { crmAdvisorChrome } from "./crm.ts";

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
