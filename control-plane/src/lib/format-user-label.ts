type ContactLike = { display_name?: string; username?: string } | null;
type Resolver = (id: string) => ContactLike;

/**
 * Format a user/sender ID into a human-readable label.
 * Display hierarchy: display_name > @username > formatted ID fallback.
 */
/** Best-effort email from a JWT payload. Display only — never used for auth. */
export function emailFromJwt(token: string): string {
  const parts = token.split(".");
  if (parts.length < 2) return "";
  try {
    const b64 = parts[1]!.replace(/-/g, "+").replace(/_/g, "/");
    const padded = b64 + "=".repeat((4 - (b64.length % 4)) % 4);
    const json = atob(padded);
    const payload = JSON.parse(json) as { email?: string };
    return typeof payload.email === "string" ? payload.email : "";
  } catch {
    return "";
  }
}

export function formatAuthUserLabel(opts: {
  userId: string;
  email?: string;
  displayName?: string;
  token?: string;
}): string {
  if (opts.displayName) return opts.displayName;
  if (opts.email) return opts.email;
  const fromJwt = opts.token ? emailFromJwt(opts.token) : "";
  if (fromJwt) return fromJwt;
  return formatUserLabel(opts.userId);
}

export function formatUserLabel(userId: string, resolve?: Resolver): string {
  if (!userId) return "";

  // Try contact resolver first
  if (resolve) {
    const contact = resolve(userId);
    if (contact?.display_name) return contact.display_name;
    if (contact?.username) return `@${contact.username}`;
  }

  // Special cases
  if (userId === "system") return "System";
  if (userId.startsWith("group:")) {
    const parts = userId.split(":");
    if (parts.length >= 3) {
      const channel = parts[1]!.charAt(0).toUpperCase() + parts[1]!.slice(1);
      return `${channel} ${parts.slice(2).join(":")}`;
    }
  }

  // Fallback: prefix numeric IDs with #
  if (/^-?\d+$/.test(userId)) return `#${userId}`;

  // UUID (incl. ULID-like 8-4-4-4-12) is not a display name.
  if (/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(userId)) {
    return userId.slice(0, 8);
  }

  // Truncate long opaque IDs (e.g. oc_295eb80d325c976cbeb4a779e2010518)
  if (userId.length > 20) return `${userId.slice(0, 10)}…${userId.slice(-4)}`;

  return userId;
}
