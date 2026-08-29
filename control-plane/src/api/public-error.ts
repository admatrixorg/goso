const MAX = 400;

/** Surface gateway status text (502 / LLM 401). Truncate; never echo secrets. */
export function formatPublicError(e: unknown): string {
  let s = String(e);
  if (/<!doctype/i.test(s) || /Unexpected token\s+'<'/i.test(s)) return "non-JSON response";
  s = s
    .replace(/Bearer\s+\S+/gi, "Bearer [redacted]")
    .replace(/\bsk-[A-Za-z0-9_*-]+\b/g, "sk-[redacted]")
    .replace(/\bgsk_[A-Za-z0-9]+\b/g, "gsk_[redacted]")
    .replace(/\bxai-[A-Za-z0-9]+\b/g, "xai-[redacted]")
    .replace(/\bAIza[A-Za-z0-9_-]+\b/g, "AIza[redacted]")
    .replace(/\bwh_[A-Za-z0-9]+\b/g, "wh_[redacted]")
    .replace(/"(authorization|api[_-]?key|secret|hmac(?:_key)?|token)"\s*:\s*"[^"]*"/gi, '"$1":"[redacted]"')
    .replace(/(authorization|api[_-]?key|secret|hmac(?:_key)?|token)\s*[:=]\s*\S+/gi, "$1=[redacted]");
  if (s.length > MAX) s = `${s.slice(0, MAX)}…`;
  return s;
}
