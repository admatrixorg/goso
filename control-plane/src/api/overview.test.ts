import assert from "node:assert/strict";
import test from "node:test";
import { formatPublicError } from "./public-error.ts";
import { parseStatsBody } from "./stats.ts";
import {
  canKeepOverviewStale,
  countChannelHealth,
  deriveOverviewKind,
  errorStatus,
  formatUptime,
  isUnauthorizedStatus,
  markOverviewStale,
  type OverviewSnapshot,
} from "./overview.ts";

test("parseStatsBody reads gateway counters and ignores extra keys", () => {
  const s = parseStatsBody(
    {
      uptime_seconds: 125,
      request_count: "40",
      llm_call_count: 3,
      ws_up: true,
      last_heartbeat: " 2026-08-29T01:02:03Z ",
      bot_token: "should-never-surface",
      token: "nope",
    },
    200,
  );
  assert.equal(s.status, 200);
  assert.equal(s.uptimeSeconds, 125);
  assert.equal(s.requestCount, 40);
  assert.equal(s.llmCallCount, 3);
  assert.equal(s.wsUp, true);
  assert.equal(s.lastHeartbeat, "2026-08-29T01:02:03Z");
  assert.equal("bot_token" in s, false);
  assert.equal("token" in s, false);
});

test("parseStatsBody rejects non-objects and negative counts", () => {
  assert.deepEqual(parseStatsBody(null, 502), {
    status: 502,
    uptimeSeconds: 0,
    requestCount: 0,
    llmCallCount: 0,
    wsUp: false,
    lastHeartbeat: "",
  });
  const s = parseStatsBody({ uptime_seconds: -9, ws_up: "yes", last_heartbeat: 1 }, 200);
  assert.equal(s.uptimeSeconds, 0);
  assert.equal(s.wsUp, false);
  assert.equal(s.lastHeartbeat, "");
});

test("countChannelHealth buckets running/missing/failed/parked and ignores secrets", () => {
  const counts = countChannelHealth([
    { health: "running", missing: false, env: "sk-leaked", env_names: ["GOSO_TELEGRAM_BOT_TOKEN"] } as {
      health?: string;
      missing?: boolean;
      env?: string;
      env_names?: string[];
    },
    { health: "missing" },
    { health: "failed" },
    { health: "parked" },
    { health: "parked" },
    { health: "stopped" },
    { missing: true },
  ]);
  assert.deepEqual(counts, { running: 1, missing: 2, failed: 1, parked: 2, stopped: 1 });
});

test("formatUptime uses compact units", () => {
  assert.equal(formatUptime(0), "0s");
  assert.equal(formatUptime(45), "45s");
  assert.equal(formatUptime(90), "1m 30s");
  assert.equal(formatUptime(3661), "1h 1m");
  assert.equal(formatUptime(90000), "1d 1h");
  assert.equal(formatUptime(-3), "0s");
});

test("errorStatus reads jsonFetch-style messages", () => {
  assert.equal(errorStatus(new Error("401 {\"error\":\"unauthorized\"}")), 401);
  assert.equal(errorStatus(new Error("403 forbidden")), 403);
  assert.equal(errorStatus(new Error("boom")), 0);
  assert.equal(isUnauthorizedStatus(401), true);
  assert.equal(isUnauthorizedStatus(403), true);
  assert.equal(isUnauthorizedStatus(500), false);
});

test("deriveOverviewKind maps gateway and list failures", () => {
  const base = { statsStatus: 200, agents: 1, sessions: 2, channels: countChannelHealth([]), errors: [] as string[] };
  assert.equal(deriveOverviewKind({ ...base, health: "connected" }), "connected");
  assert.equal(deriveOverviewKind({ ...base, health: "degraded" }), "degraded");
  assert.equal(deriveOverviewKind({ ...base, health: "offline" }), "offline");
  assert.equal(deriveOverviewKind({ ...base, health: "unauthorized" }), "unauthorized");
  assert.equal(
    deriveOverviewKind({ health: "connected", statsStatus: 401, agents: null, sessions: null, channels: null, errors: [] }),
    "unauthorized",
  );
  assert.equal(
    deriveOverviewKind({ health: "connected", statsStatus: 401, agents: 1, sessions: 0, channels: null, errors: [] }),
    "degraded",
  );
  assert.equal(
    deriveOverviewKind({
      health: "connected",
      statsStatus: 200,
      agents: 1,
      sessions: 1,
      channels: countChannelHealth([]),
      errors: ["agents: 502"],
    }),
    "degraded",
  );
  assert.equal(
    deriveOverviewKind({ health: "connected", statsStatus: 0, agents: 1, sessions: 2, channels: countChannelHealth([]), errors: [] }),
    "degraded",
  );
  assert.equal(
    deriveOverviewKind({ health: "connected", statsStatus: 500, agents: 1, sessions: 2, channels: countChannelHealth([]), errors: [] }),
    "degraded",
  );
});

test("formatPublicError redacts tokens and HTML bodies", () => {
  assert.equal(formatPublicError('401 {"token":"sk-live-abc"}').includes("sk-live-abc"), false);
  assert.match(formatPublicError("Bearer secret-value boom"), /Bearer \[redacted\]/);
  assert.equal(formatPublicError("500 <!doctype html>"), "non-JSON response");
});

test("non-JSON list failure is degraded not empty inventory", () => {
  assert.equal(
    deriveOverviewKind({
      health: "connected",
      statsStatus: 200,
      agents: null,
      sessions: null,
      channels: null,
      errors: ["agents: non-JSON response", "sessions: non-JSON response", "channels: non-JSON response"],
    }),
    "degraded",
  );
});

test("stale overview keeps last-known snapshot and never invents zeros", () => {
  const prev: OverviewSnapshot = {
    health: "connected",
    healthStatus: 200,
    stats: { status: 200, uptimeSeconds: 12, requestCount: 4, llmCallCount: 1, wsUp: true, lastHeartbeat: "t" },
    agents: 2,
    sessions: 3,
    channels: countChannelHealth([{ health: "running" }]),
    cronJobs: 0,
    errors: [],
    kind: "connected",
    loadedAt: "2026-08-30T01:00:00Z",
    stale: false,
  };
  const stale = markOverviewStale(prev, "2026-08-30T02:00:00Z");
  assert.equal(stale.stale, true);
  assert.equal(stale.kind, "degraded");
  assert.equal(stale.agents, 2);
  assert.equal(stale.sessions, 3);
  assert.equal(canKeepOverviewStale(prev), true);
  assert.equal(canKeepOverviewStale(null), false);
  assert.equal(
    canKeepOverviewStale({
      ...prev,
      loadedAt: null,
      agents: null,
      sessions: null,
      channels: null,
      stats: { ...prev.stats, status: 0 },
      stale: true,
    }),
    false,
  );
});
