const MAX = 400;

const JSON_SECRET = /"(authorization|api[_-]?key|secret|hmac(?:_key)?|token|password|private_key|bot_token|access_token|arguments|tool_input|tool_result)"\s*:\s*"(?:\\.|[^"\\])*"/gi;
const KV_SECRET = /(authorization|api[_-]?key|secret|hmac(?:_key)?|token|password|private_key|bot_token|access_token)\s*[:=]\s*\S+/gi;
const TOOL_OBJECT = /"(arguments|tool_input|tool_result)"\s*:\s*\{[^{}]{0,4000}\}/gi;

/** Truncate and strip credential / tool-payload shapes. Never echo secrets. */
export function redactPublicText(raw: string): string {
  let s = raw;
  s = s
    .replace(JSON_SECRET, '"$1":"[redacted]"')
    .replace(TOOL_OBJECT, '"$1":{}')
    .replace(/Bearer\s+[^\s"'\\]+/gi, "Bearer [redacted]")
    .replace(/\bsk-[A-Za-z0-9_*-]+\b/g, "sk-[redacted]")
    .replace(/\bgsk_[A-Za-z0-9]+\b/g, "gsk_[redacted]")
    .replace(/\bxai-[A-Za-z0-9]+\b/g, "xai-[redacted]")
    .replace(/\bAIza[A-Za-z0-9_-]+\b/g, "AIza[redacted]")
    .replace(/\bwh_[A-Za-z0-9]+\b/g, "wh_[redacted]")
    .replace(KV_SECRET, "$1=[redacted]");
  if (s.length > MAX) s = `${s.slice(0, MAX)}…`;
  return s;
}

/** Surface gateway status text (502 / LLM 401). Truncate; never echo secrets. */
export function formatPublicError(e: unknown): string {
  const s = String(e);
  if (/<!doctype/i.test(s) || /Unexpected token\s+'<'/i.test(s)) return "non-JSON response";
  return redactPublicText(s);
}
