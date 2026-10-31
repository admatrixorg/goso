/** Session agent / model pickers and one-message @ mentions (SPEC 133). */

export type TurnAgent = {
  id: string;
  display_name?: string;
  agent_key?: string;
  model?: string;
  llm_provider?: string;
  enabled?: boolean;
};

export type ModelChoice = { provider: string; model: string };

const SEP = "\u0001";

export function modelChoiceValue(provider: string, model: string): string {
  const p = provider.trim();
  const m = model.trim();
  if (!p || !m) return "";
  return `${p}${SEP}${m}`;
}

export function selectValueForChoice(choice: ModelChoice): string {
  const model = choice.model.trim();
  const provider = choice.provider.trim();
  if (!model) return "";
  if (!provider) return `\u0002${model}`;
  return modelChoiceValue(provider, model);
}

export function choiceFromSelect(value: string): ModelChoice | null {
  if (!value) return null;
  if (value.startsWith("\u0002")) {
    const model = value.slice(1).trim();
    return model ? { provider: "", model } : null;
  }
  return parseModelChoice(value);
}

export function parseModelChoice(value: string): ModelChoice | null {
  const i = value.indexOf(SEP);
  if (i <= 0 || i >= value.length - 1) return null;
  return { provider: value.slice(0, i), model: value.slice(i + 1) };
}

export function modelOptions(agents: TurnAgent[]): ModelChoice[] {
  const seen = new Set<string>();
  const out: ModelChoice[] = [];
  for (const a of agents) {
    if (a.enabled === false) continue;
    const model = (a.model || "").trim();
    const provider = (a.llm_provider || "").trim();
    if (!model || !provider) continue;
    const key = modelChoiceValue(provider, model);
    if (!key || seen.has(key)) continue;
    seen.add(key);
    out.push({ provider, model });
  }
  out.sort((a, b) => a.model.localeCompare(b.model) || a.provider.localeCompare(b.provider));
  return out;
}

export function ensureModelChoice(options: ModelChoice[], provider?: string, model?: string): ModelChoice[] {
  const p = (provider || "").trim();
  const m = (model || "").trim();
  if (!m) return options;
  if (options.some((o) => o.provider === p && o.model === m)) return options;
  return [...options, { provider: p, model: m }];
}

export function replyCaption(agentName: string, model: string): string {
  const a = agentName.trim();
  const m = model.trim();
  if (a && m) return `${a} · ${m}`;
  return a || m;
}

export function agentDisplayName(a: TurnAgent): string {
  return (a.display_name || a.agent_key || a.id).trim() || a.id;
}

/** Text after @ at the cursor, or null when the caret is not in an @ token. */
export function mentionQueryAt(text: string, cursor: number): string | null {
  const upto = text.slice(0, Math.max(0, Math.min(cursor, text.length)));
  const m = /(^|[\s])@([^\s@]*)$/.exec(upto);
  if (!m) return null;
  return m[2];
}

export function filterMentionAgents(agents: TurnAgent[], query: string): TurnAgent[] {
  const q = query.trim().toLowerCase();
  return agents.filter((a) => a.enabled !== false).filter((a) => {
    if (!q) return true;
    return `${a.display_name || ""} ${a.agent_key || ""}`.toLowerCase().includes(q);
  });
}

export function applyMention(text: string, cursor: number, name: string): { text: string; cursor: number } {
  const at = Math.max(0, Math.min(cursor, text.length));
  const upto = text.slice(0, at);
  const m = /(^|[\s])@([^\s@]*)$/.exec(upto);
  const insert = `@${name} `;
  if (!m) {
    const next = text.slice(0, at) + insert + text.slice(at);
    return { text: next, cursor: at + insert.length };
  }
  const start = upto.length - m[2].length - 1;
  const next = text.slice(0, start) + insert + text.slice(at);
  return { text: next, cursor: start + insert.length };
}

export function mentionStillPresent(text: string, name: string): boolean {
  const n = name.trim();
  if (!n) return false;
  return text.includes(`@${n}`);
}

export type TextSpan = { text: string; mention: boolean };

export function highlightMentions(content: string, names: string[]): TextSpan[] {
  const sorted = [...names]
    .map((n) => n.trim())
    .filter(Boolean)
    .sort((a, b) => b.length - a.length);
  if (!sorted.length || !content) return [{ text: content, mention: false }];
  const spans: TextSpan[] = [];
  let i = 0;
  while (i < content.length) {
    if (content[i] === "@") {
      const rest = content.slice(i + 1);
      const hit = sorted.find((n) => {
        if (!rest.startsWith(n)) return false;
        const next = rest[n.length];
        return next == null || /[\s.,!?;:)\]，。]/.test(next);
      });
      if (hit) {
        spans.push({ text: `@${hit}`, mention: true });
        i += 1 + hit.length;
        continue;
      }
    }
    let j = i + 1;
    while (j < content.length && content[j] !== "@") j++;
    spans.push({ text: content.slice(i, j), mention: false });
    i = j;
  }
  return spans;
}
