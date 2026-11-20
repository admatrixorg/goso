import assert from "node:assert/strict";
import test from "node:test";
import {
  asPublicFile,
  asPublicList,
  asPublicPlan,
  asPublicPreflight,
  asPublicS3,
  confirmMatches,
  filterByScope,
  publicHasSecrets,
} from "./backup-ops.ts";

test("asPublicFile drops secret-shaped snapshots", () => {
  assert.equal(asPublicFile({ file: "goso-a.db", bytes: 1, integrity: "ok", token: "sk-live-abcdefgh" }), undefined);
  const ok = asPublicFile({ file: "goso-a.db", bytes: 12, integrity: "ok", secret_policy: "excluded", scope: "system" });
  assert.equal(ok?.file, "goso-a.db");
  assert.equal(publicHasSecrets(ok), false);
});

test("asPublicList drops leaked rows", () => {
  const list = asPublicList({
    files: [
      { file: "ok.db", bytes: 1, integrity: "ok" },
      { file: "bad.db", bytes: 1, integrity: "ok", api_key: "sk-live-abcdefgh" },
    ],
  });
  assert.equal(list.files.length, 1);
  assert.equal(list.files[0].file, "ok.db");
});

test("asPublicS3 never keeps access_key/secret", () => {
  assert.equal(asPublicS3({ configured: true, access_key: "AKIAFAKE", secret: "wJalr" }), undefined);
  const row = asPublicS3({ configured: true, endpoint: "http://127.0.0.1:9000", bucket: "goso", access_key_set: true });
  assert.equal(row?.configured, true);
  assert.equal(row?.access_key_set, true);
  assert.equal(publicHasSecrets(row), false);
});

test("asPublicPreflight and plan helpers", () => {
  const pf = asPublicPreflight({ engine: "sqlite", can_backup: true, can_restore: false, checks: [{ id: "sqlite_file", ok: true }] });
  assert.equal(pf.can_backup, true);
  const plan = asPublicPlan({ valid: true, file: "goso-a.db", integrity: "ok", scope: "system", secret_policy: "excluded", credentials_excluded: true, errors: [], warnings: [], actions: [], confirm_required: true, confirm_target: "goso-a.db" });
  assert.equal(plan?.valid, true);
  assert.equal(asPublicPlan({ valid: true, token: "x" }), undefined);
  assert.equal(confirmMatches("goso-a.db", "goso-a.db"), true);
  assert.equal(confirmMatches("goso-a.db", "nope"), false);
  assert.equal(filterByScope([{ file: "a", bytes: 1, integrity: "ok", scope: "tenant" }], "tenant").length, 1);
});
