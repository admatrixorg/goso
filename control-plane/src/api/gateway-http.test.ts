import assert from "node:assert/strict";
import test from "node:test";
import {
  NON_JSON_RESPONSE,
  gatewayFetch,
  gatewayFetchInit,
  isHtmlGatewayBody,
  parseGatewayJson,
  readGatewayJson,
} from "./gateway-http.ts";

const VITE_INDEX = "<!doctype html>\n<html><head></head><body>vite</body></html>";

test("gatewayFetchInit defaults cache to no-store", () => {
  assert.equal(gatewayFetchInit().cache, "no-store");
  assert.equal(gatewayFetchInit({ method: "GET" }).cache, "no-store");
});

test("gatewayFetchInit keeps a caller-supplied cache", () => {
  assert.equal(gatewayFetchInit({ cache: "reload" }).cache, "reload");
  assert.equal(gatewayFetchInit({ cache: "force-cache" }).cache, "force-cache");
});

test("mock fetch captures init.cache === no-store", async () => {
  const captured: RequestInit[] = [];
  const fetchImpl: typeof fetch = async (_input, init) => {
    captured.push(init ?? {});
    return new Response(JSON.stringify({ agents: [{ id: "a1" }] }), {
      status: 200,
      headers: { "content-type": "application/json" },
    });
  };
  const res = await gatewayFetch("/api/agents", undefined, fetchImpl);
  assert.equal(captured.length, 1);
  assert.equal(captured[0]?.cache, "no-store");
  const body = await readGatewayJson<{ agents: unknown[] }>(res);
  assert.equal(body.agents.length, 1);
});

test("HTML 200 does not parse as empty agents inventory", async () => {
  const fetchImpl: typeof fetch = async (_input, init) => {
    assert.equal(init?.cache, "no-store");
    return new Response(VITE_INDEX, { status: 200, headers: { "content-type": "text/html" } });
  };
  const res = await gatewayFetch("/api/agents", undefined, fetchImpl);
  await assert.rejects(() => readGatewayJson<{ agents: unknown[] }>(res), (e: unknown) => {
    assert.equal((e as Error).message, NON_JSON_RESPONSE);
    return true;
  });
  assert.throws(() => parseGatewayJson<{ agents: unknown[] }>("text/html", VITE_INDEX), (e: unknown) => {
    assert.equal((e as Error).message, NON_JSON_RESPONSE);
    return true;
  });
});

test("doctype body is rejected even without a html content-type", () => {
  assert.equal(isHtmlGatewayBody("application/json", VITE_INDEX), true);
  assert.throws(() => parseGatewayJson<{ agents: unknown[] }>("application/json", VITE_INDEX), (e: unknown) => {
    assert.equal((e as Error).message, NON_JSON_RESPONSE);
    return true;
  });
});

test("legitimate empty agents JSON still parses", () => {
  const body = parseGatewayJson<{ agents: unknown[] }>("application/json", '{"agents":[]}');
  assert.deepEqual(body, { agents: [] });
});
