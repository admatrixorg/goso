import assert from "node:assert/strict";
import test from "node:test";
import {
  asPublicListing,
  asPublicPreview,
  formatBytes,
  formatWhen,
  isImageType,
  publicHasSecrets,
  quotaOver,
  secretPath,
  storageConfirmMatch,
} from "./storage-ops.ts";
import type { StorageListing } from "./storage.ts";

function listing(over: Partial<StorageListing> = {}): StorageListing {
  return {
    configured: true,
    path: "",
    breadcrumbs: [{ name: "workspace", path: "" }],
    entries: [{ name: "readme.txt", path: "readme.txt", dir: false, size: 12, type: "text/plain", mtime: "2026-08-30T12:00:00Z" }],
    used_bytes: 12,
    max_bytes: 1024,
    hidden_skipped: 0,
    ...over,
  };
}

test("asPublicListing drops secret-named rows and key-shaped fields", () => {
  const rows = asPublicListing(
    listing({
      entries: [
        { name: "readme.txt", path: "readme.txt", dir: false, size: 1, type: "text/plain" },
        { name: ".env", path: ".env", dir: false, size: 8, type: "text/plain" },
        { name: "id_rsa", path: "id_rsa", dir: false, size: 8, type: "application/octet-stream" },
        { name: "notes", path: "notes", dir: true, size: 0, type: "directory" },
        { name: "leaky", path: "leaky.txt", dir: false, size: 4, type: "text/plain", api_key: "sk-live-abcdefgh" } as never,
      ],
    }),
  );
  assert.equal(rows.entries.length, 2);
  assert.equal(rows.entries[0].name, "readme.txt");
  assert.equal(rows.entries[1].name, "notes");
  assert.equal(publicHasSecrets(rows.entries[0]), false);
});

test("publicHasSecrets flags credential values, not listing metadata", () => {
  assert.equal(publicHasSecrets({ name: "readme.txt", path: "readme.txt", size: 1 }), false);
  assert.equal(publicHasSecrets({ name: "x", token: "abc" }), true);
  assert.equal(publicHasSecrets({ name: "x", text: "sk-live-abcdefghijk" }), true);
  assert.equal(secretPath(".env"), true);
  assert.equal(secretPath("notes/key.pem"), true);
  assert.equal(secretPath("readme.txt"), false);
});

test("asPublicPreview drops secret-shaped text and caps length", () => {
  const ok = asPublicPreview({ path: "a.txt", type: "text/plain", size: 4, kind: "text", text: "hi", bytes: 2 });
  assert.equal(ok?.kind, "text");
  assert.equal(ok?.text, "hi");
  const leak = asPublicPreview({ path: "a.txt", type: "text/plain", size: 20, kind: "text", text: "Bearer abcdefghijk", bytes: 20 });
  assert.equal(leak, null);
  const denied = asPublicPreview({ path: "a.txt", type: "text/plain", size: 1, kind: "denied", bytes: 0 });
  assert.equal(denied?.kind, "denied");
});

test("storageConfirmMatch, formatBytes, quotaOver", () => {
  const row = { name: "readme.txt", path: "docs/readme.txt" };
  assert.equal(storageConfirmMatch("readme.txt", row), true);
  assert.equal(storageConfirmMatch("docs/readme.txt", row), true);
  assert.equal(storageConfirmMatch("nope", row), false);
  assert.equal(storageConfirmMatch("", row), false);
  assert.equal(formatBytes(0), "0 B");
  assert.equal(formatBytes(512), "512 B");
  assert.match(formatBytes(2048), /KiB/);
  assert.equal(quotaOver(10, 10), true);
  assert.equal(quotaOver(9, 10), false);
  assert.equal(isImageType("image/png"), true);
  assert.equal(isImageType("text/plain"), false);
  const shown = formatWhen("2026-08-30T12:00:00Z", "n/a");
  assert.ok(shown.includes("2026") || shown.includes("30"));
  assert.equal(formatWhen("", "n/a"), "n/a");
});
