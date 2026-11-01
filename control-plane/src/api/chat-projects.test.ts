import assert from "node:assert/strict";
import test from "node:test";
import { MINH_AN_PROJECTS, closedProjects, runningProjects } from "./chat-projects.ts";

test("sidebar projects are the Minh An sample, running above closed", () => {
  assert.deepEqual(runningProjects().map((row) => row.name), [
    "Mở kênh đại lý miền Tây",
    "Ra mắt nồi chiên dòng mới",
  ]);
  assert.deepEqual(closedProjects().map((row) => row.id), ["kiem-toan-2025"]);
  const blob = JSON.stringify(MINH_AN_PROJECTS);
  assert.doesNotMatch(blob, /MQ|Capital|Vietcombank|@[a-z]/i);
});
