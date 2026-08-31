import assert from "node:assert/strict";
import test from "node:test";
import { crmPermissionBanner, formatCrmPublicError, formatPublicError, redactPublicText } from "./public-error.ts";

test("formatPublicError redacts bearer and vendor keys", () => {
  const s = formatPublicError(new Error("401 Bearer abc.def sk-live-ABCDEFG xai-ZZZZYYYY"));
  assert.equal(s.includes("abc.def"), false);
  assert.equal(s.includes("sk-live-ABCDEFG"), false);
  assert.equal(s.includes("xai-ZZZZYYYY"), false);
  assert.match(s, /Bearer \[redacted\]/);
  assert.match(s, /sk-\[redacted\]/);
  assert.match(s, /xai-\[redacted\]/);
});

test("formatPublicError redacts JSON credential keys", () => {
  const s = formatPublicError('502 {"api_key":"abc123","token":"zzz","secret":"shh"}');
  assert.equal(s.includes("abc123"), false);
  assert.equal(s.includes("zzz"), false);
  assert.equal(s.includes("shh"), false);
  assert.match(s, /"api_key":"\[redacted\]"/);
});

test("redactPublicText strips tool payload arguments and tool_input", () => {
  const raw =
    'tool error {"name":"http","arguments":"{\\"Authorization\\":\\"Bearer leak\\",\\"url\\":\\"https://x\\"}","tool_input":{"api_key":"k-live"}}';
  const s = redactPublicText(raw);
  assert.equal(s.includes("Bearer leak"), false);
  assert.equal(s.includes("k-live"), false);
  assert.match(s, /"arguments":"\[redacted\]"/);
  assert.match(s, /"tool_input":\{\}/);
});

test("redactPublicText strips tool_result blobs and kv bot_token", () => {
  const s = redactPublicText('tool_result: {"ok":true} bot_token=12345:AA secret=plain');
  assert.equal(s.includes("12345:AA"), false);
  assert.equal(s.includes("plain"), false);
  assert.match(s, /bot_token=\[redacted\]/);
});

test("formatPublicError truncates long diagnostic text", () => {
  const s = formatPublicError("e".repeat(500));
  assert.equal(s.length, 401);
  assert.equal(s.endsWith("…"), true);
});

test("formatPublicError maps HTML doctype to non-JSON response", () => {
  assert.equal(formatPublicError(new Error("<!doctype html><html></html>")), "non-JSON response");
  assert.equal(formatPublicError(new Error("non-JSON response")), "non-JSON response");
  assert.equal(formatPublicError("non-JSON response"), "non-JSON response");
});

test("formatCrmPublicError does not dump 401 JSON body", () => {
  const err = new Error('401 {"error":"unauthorized"}');
  assert.equal(formatCrmPublicError(err), "");
  assert.equal(formatCrmPublicError(err).includes("{"), false);
  assert.equal(formatCrmPublicError(err).includes("401"), false);
  const banner = crmPermissionBanner("CRM refused authorization — separate from the gateway.", err);
  assert.equal(banner.includes("{"), false);
  assert.equal(banner.includes('"error"'), false);
  assert.equal(banner.includes("401"), false);
  assert.equal(banner.includes("Error:"), false);
});
