import assert from "node:assert/strict";
import test from "node:test";
import {
  applyMention,
  ensureModelChoice,
  filterMentionAgents,
  highlightMentions,
  mentionQueryAt,
  mentionStillPresent,
  choiceFromSelect,
  modelChoiceValue,
  modelOptions,
  parseModelChoice,
  selectValueForChoice,
  replyCaption,
} from "./chat-turn.ts";

const agents = [
  { id: "a", display_name: "Đầu tư", agent_key: "dau-tu", model: "grok-4.5", llm_provider: "xai", enabled: true },
  { id: "b", display_name: "Pháp chế", agent_key: "phap-che", model: "gemini-2.5-flash", llm_provider: "gemini", enabled: true },
  { id: "c", display_name: "Nháp", agent_key: "nhap", model: "echo", llm_provider: "", enabled: true },
  { id: "d", display_name: "Tắt", agent_key: "tat", model: "gpt-4o", llm_provider: "openai", enabled: false },
];

test("model options skip empty provider and paused agents", () => {
  const opts = modelOptions(agents);
  assert.deepEqual(opts, [
    { provider: "gemini", model: "gemini-2.5-flash" },
    { provider: "xai", model: "grok-4.5" },
  ]);
  const value = modelChoiceValue("xai", "grok-4.5");
  assert.deepEqual(parseModelChoice(value), { provider: "xai", model: "grok-4.5" });
  assert.equal(parseModelChoice(""), null);
  const withCurrent = ensureModelChoice(opts, "", "custom-model");
  assert.equal(withCurrent.at(-1)?.model, "custom-model");
  const bare = selectValueForChoice({ provider: "", model: "custom-model" });
  assert.deepEqual(choiceFromSelect(bare), { provider: "", model: "custom-model" });
  assert.equal(choiceFromSelect(""), null);
});

test("reply caption joins agent and model", () => {
  assert.equal(replyCaption("Đầu tư", "grok-4.5"), "Đầu tư · grok-4.5");
  assert.equal(replyCaption("Đầu tư", ""), "Đầu tư");
  assert.equal(replyCaption("", "echo"), "echo");
});

test("@ query, insert, and highlight", () => {
  assert.equal(mentionQueryAt("xem @pha", 8), "pha");
  assert.equal(mentionQueryAt("xem @pha", 3), null);
  assert.equal(mentionQueryAt("@", 1), "");
  const hits = filterMentionAgents(agents, "pháp");
  assert.deepEqual(hits.map((a) => a.id), ["b"]);
  assert.equal(filterMentionAgents(agents, "").some((a) => a.id === "d"), false);
  const applied = applyMention("xem @pha", 8, "Pháp chế");
  assert.equal(applied.text, "xem @Pháp chế ");
  assert.equal(mentionStillPresent(applied.text, "Pháp chế"), true);
  assert.equal(mentionStillPresent("không", "Pháp chế"), false);
  const spans = highlightMentions("hỏi @Pháp chế giúp", ["Pháp chế", "Đầu tư"]);
  assert.deepEqual(spans, [
    { text: "hỏi ", mention: false },
    { text: "@Pháp chế", mention: true },
    { text: " giúp", mention: false },
  ]);
});
