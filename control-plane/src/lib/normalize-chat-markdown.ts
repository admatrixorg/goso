/**
 * Normalize LLM markdown so chat renders as readable prose, not a dump of hashes.
 * Leaves fenced code blocks untouched.
 */
export function normalizeChatMarkdown(src: string): string {
  if (!src) return src;
  const parts = src.replace(/\r\n/g, "\n").split(/(```[\s\S]*?```|~~~[\s\S]*?~~~)/);
  return parts
    .map((part, i) => (i % 2 === 1 ? part : normalizeProse(part)))
    .join("")
    .replace(/\n{3,}/g, "\n\n");
}

function normalizeProse(s: string): string {
  s = s.replace(/\uFF03/g, "#");
  // ##Heading → ## Heading (CommonMark needs a space after ATX hashes)
  s = s.replace(/^(#{1,6})([^\s#])/gm, "$1 $2");
  // "sentence. ## Section" mid-line → real heading
  s = s.replace(/([^\n])[ \t]+(#{1,6}[ \t]+)/g, "$1\n\n$2");
  // *item / -item at line start → list markers
  s = s.replace(/^[ \t]*\*(?![*\s])/gm, "* ");
  s = s.replace(/^[ \t]*-(?=[^\s-])/gm, "- ");
  return s;
}
