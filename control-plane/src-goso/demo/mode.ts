/** SPEC 017: DEMO screens only when VITE_DEMO_MODE is true/1 at build time. */

export function isDemoMode(): boolean {
  return import.meta.env.VITE_DEMO_MODE === "true" || import.meta.env.VITE_DEMO_MODE === "1";
}

export const DEMO_TABS = ["home", "tasks", "meetings", "friends", "calendar", "gallery"] as const;
