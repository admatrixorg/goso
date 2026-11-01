import assert from "node:assert/strict";
import test from "node:test";
import { liveTabIds } from "./hash-route.ts";
import { NAV_ICONS } from "../ui/nav-icons.ts";

test("every sidebar tab has a distinct icon name", () => {
  const icons = liveTabIds().map((tab) => {
    const name = NAV_ICONS[tab];
    assert.ok(name, tab);
    return name;
  });
  assert.equal(new Set(icons).size, icons.length);
});
