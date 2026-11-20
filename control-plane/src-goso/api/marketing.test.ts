import assert from "node:assert/strict";
import test from "node:test";
import { classifyPageState, inventoryBlocksMutation, listMetaCount } from "./page-state.ts";
import { audienceSourceExecutes, audienceSourceNote, campaignStatusExecutes } from "./marketing-ops.ts";

test("audience sources and campaign statuses do not execute vendors", () => {
  assert.equal(audienceSourceExecutes("paste"), false);
  assert.equal(audienceSourceExecutes("file"), false);
  assert.equal(audienceSourceExecutes("leadads"), false);
  assert.equal(campaignStatusExecutes("draft"), false);
  assert.equal(campaignStatusExecutes("scheduled"), false);
  assert.equal(campaignStatusExecutes("done"), false);
  assert.equal(audienceSourceNote("file"), "file");
  assert.equal(audienceSourceNote("leadads"), "leadads");
  assert.equal(audienceSourceNote("paste"), "paste");
});

test("campaign-kind empty is not a CRM permission failure", () => {
  const ready = classifyPageState({
    loading: false,
    loaded: true,
    error: null,
    itemCount: 2,
  });
  assert.equal(ready.kind, "ready");
  assert.equal(inventoryBlocksMutation(ready.kind), false);
});

test("marketing inventory error never claims empty audiences", () => {
  const perm = classifyPageState({
    loading: false,
    loaded: false,
    error: new Error("401 unauthorized"),
    itemCount: 0,
  });
  assert.equal(perm.kind, "permission");
  assert.equal(perm.showEmpty, false);
  assert.equal(inventoryBlocksMutation(perm.kind), true);
  assert.equal(listMetaCount(perm.kind, 0), null);

  const boom = classifyPageState({
    loading: false,
    loaded: false,
    error: new Error("500"),
    itemCount: 0,
  });
  assert.equal(boom.kind, "error");
  assert.equal(boom.showEmpty, false);
  assert.equal(inventoryBlocksMutation(boom.kind), true);
});
