import assert from "node:assert/strict";
import test from "node:test";
import { liveTabIds } from "./hash-route.ts";
import {
  FOLDED_HASHES,
  HASH_ALIASES,
  SIDEBAR_TABS,
  ignoredConsole,
  ignoredFailure,
  unfinishedMarker,
} from "./qa-tabs.ts";

test("qa tab list is every live sidebar tab once", () => {
  const live = liveTabIds().filter((id) => id !== "functions" && id !== "webhooks");
  assert.deepEqual(
    SIDEBAR_TABS.map((row) => row.tab).sort(),
    [...live].sort(),
  );
  assert.equal(new Set(SIDEBAR_TABS.map((row) => row.hash)).size, SIDEBAR_TABS.length);
  assert.equal(new Set(SIDEBAR_TABS.map((row) => row.proto)).size, SIDEBAR_TABS.length);
});

test("aliases and folded routes are part of the check", () => {
  assert.equal(HASH_ALIASES.find((row) => row.from === "#projects")?.to, "#/chat");
  assert.equal(HASH_ALIASES.find((row) => row.from === "#agent-team")?.to, "#/agents");
  assert.ok(FOLDED_HASHES.includes("#/heatmap"));
});

test("unfinished marker is chưa dựng, not an honest empty state", () => {
  assert.equal(unfinishedMarker("Trang này chưa dựng xong"), "chưa dựng");
  assert.equal(unfinishedMarker("Màn hình này không có trên bản non-demo. Bật demo."), "màn hình này không có trên bản non-demo");
  assert.equal(unfinishedMarker("Chưa có cuộc họp. Không hiện số liệu mẫu."), null);
  assert.equal(unfinishedMarker("Chưa có dữ liệu. Không hiện số liệu mẫu."), null);
  assert.equal(unfinishedMarker("Chưa có hook. Tạo hook chưa lưu được"), null);
});

test("crm proxy and favicon are outside the Goso gateway check", () => {
  assert.equal(ignoredFailure("http://127.0.0.1:3000/crm-api/healthz"), true);
  assert.equal(ignoredFailure("http://127.0.0.1:3000/favicon.ico"), true);
  assert.equal(ignoredFailure("http://127.0.0.1:3000/api/agents"), false);
  assert.equal(ignoredFailure("http://127.0.0.1:3000/healthz"), false);
  assert.equal(ignoredConsole("Failed to load resource: the server responded with a status of 500 ()"), true);
  assert.equal(ignoredConsole("TypeError: cannot read agents"), false);
});
