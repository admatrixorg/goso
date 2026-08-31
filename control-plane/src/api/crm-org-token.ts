/** Write-only CRM org token (X-Org-Token). Never hydrate. Distinct from goso_token. */

export const CRM_ORG_TOKEN_STORAGE_KEY = "goso_crm_org_token";

export type CrmOrgTokenKind = "env-owned" | "set" | "unset";

export type CrmOrgTokenStore = {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
  removeItem(key: string): void;
};

export type CrmOrgTokenEnv = {
  viteOrgToken?: string;
};

export type SaveCrmOrgTokenResult =
  | { ok: true; input: ""; reload: true }
  | { ok: false; reason: "empty" | "env-owned"; input: string; reload: false };

export type ClearCrmOrgTokenResult =
  | { ok: true; input: ""; reload: true }
  | { ok: false; reason: "env-owned" | "unset"; input: string; reload: false };

/** Password field starts empty. Never read localStorage, env, or GET. */
export function emptyCrmOrgTokenInput(): string {
  return "";
}

export function hydrateCrmOrgTokenInput(_store?: CrmOrgTokenStore, _env?: CrmOrgTokenEnv): string {
  return emptyCrmOrgTokenInput();
}

function envToken(env: CrmOrgTokenEnv): string {
  return (env.viteOrgToken || "").trim();
}

function storedToken(store: CrmOrgTokenStore): string {
  try {
    return (store.getItem(CRM_ORG_TOKEN_STORAGE_KEY) || "").trim();
  } catch {
    return "";
  }
}

/** Env token wins. Never log the value. */
export function crmOrgTokenValue(env: CrmOrgTokenEnv, store: CrmOrgTokenStore): string {
  return envToken(env) || storedToken(store);
}

export function crmOrgTokenKind(env: CrmOrgTokenEnv, store: CrmOrgTokenStore): CrmOrgTokenKind {
  if (envToken(env)) return "env-owned";
  return storedToken(store) ? "set" : "unset";
}

export function crmOrgTokenWritable(kind: CrmOrgTokenKind): boolean {
  return kind !== "env-owned";
}

export function crmOrgTokenClearable(kind: CrmOrgTokenKind): boolean {
  return kind === "set";
}

/** Always shown on Config → Account, including 401/blocked CRM inventory. */
export function crmOrgTokenControlVisible(_inventoryKind?: string): boolean {
  return true;
}

/** Save/Clear must not consult CRM inventory (401/error). */
export function crmOrgTokenSaveBlockedByInventory(_inventoryKind?: string): boolean {
  return false;
}

export function saveCrmOrgToken(input: string, env: CrmOrgTokenEnv, store: CrmOrgTokenStore): SaveCrmOrgTokenResult {
  if (crmOrgTokenKind(env, store) === "env-owned") {
    return { ok: false, reason: "env-owned", input, reload: false };
  }
  const value = input.trim();
  if (!value) {
    return { ok: false, reason: "empty", input, reload: false };
  }
  store.setItem(CRM_ORG_TOKEN_STORAGE_KEY, value);
  return { ok: true, input: "", reload: true };
}

export function clearCrmOrgToken(env: CrmOrgTokenEnv, store: CrmOrgTokenStore, input = ""): ClearCrmOrgTokenResult {
  const kind = crmOrgTokenKind(env, store);
  if (kind === "env-owned") {
    return { ok: false, reason: "env-owned", input, reload: false };
  }
  if (kind !== "set") {
    return { ok: false, reason: "unset", input, reload: false };
  }
  store.removeItem(CRM_ORG_TOKEN_STORAGE_KEY);
  return { ok: true, input: "", reload: true };
}
