# QA — SPEC 114 Packages

Date: 2026-08-30. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge. No live vendor tokens. No secrets in git/QA.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Packages: runtime summary plus System/Python/Node/GitHub/CLI Credentials tabs; refresh, browse/install/uninstall; compatibility warnings | `docs/qa/090-goclaw-sidebar-ux.md` Packages |

goso mapping (self-written): live tab `packages` in [App.tsx](../../control-plane/src/App.tsx) renders [PackagesPage](../../control-plane/src/pages/PackagesPage.tsx). Listing is `GET /api/packages` (`/v1/packages` alias) returning `{runtimes,allowlist,packages,jobs,credentials}`. Runtime rows are live PATH probes (`python`/`node`/`git`/`go`) with `present`/`version`/`compatible`/`warning`. Packages are declared, pinned inventory — not an untrusted `pip`/`npm` shell-out. Allowlist pins are required (`latest` and ranges refused). Install/uninstall/recover require `confirm` matching name, id, or `ecosystem/name`. Partial jobs stay `status=partial` until recover. CLI credentials are a separate write-only surface (`POST /api/packages/cli` `{kind,token}`, `POST /api/packages/uncli` `{kind,confirm}`). GET returns `{kind,set,updated_at}` only — never `token`/`hash`. Install bodies that include secret fields are 400. View-token GET 200 / POST 403. Mutations are privileged (admin); `write` issued keys cannot install. Audit `entity=package` / `package_allow` / `package_cli` without tokens.

Out of scope: Approvals (115). Import/Export (116). Copying GoClaw chrome. Live vendor tokens. SPECs 115–118.

## What changed

- Live nav tab + page. System/Python/Node/GitHub inventory, runtime summary, loading / empty / error / progress.
- Install/uninstall with typed confirm. Allowlists/pinning. Failure logs. Partial install + recover. GET never returns credentials. Elevated permission.
- i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/114-packages.md`.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/pkgmgr ./gateway/internal/auth ./gateway/internal/httpapi ./gateway/internal/serve -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge. Coordinator owns `:3000`.

## Proof

- `npm run typecheck` exit 0. `npm test` including `asPublicSnapshot` dropping secret/`ghp_` rows, `pinValid` rejecting latest/ranges, confirm matchers, CLI metadata without token.
- `go test` pkgmgr: allowlist+pin required, confirm, partial→recover, runtime incompatibility fails the job, CLI hashed and omitted from GET, ProbeHost reports four tools. httpapi: GET snapshot omits tokens; install 201; secret field on install 400; `/v1/packages` alias; view-token GET 200 / POST 403; issued `read` GET 200 / POST 403, `write` cannot allow/install. auth/serve: view GET `/api/packages` 200, POST 403.
- Page copy: “No packages declared.” / “Chưa khai báo gói.” CLI tab is a separate write-only form (password inputs, never hydrated). Confirm types name, id, or ecosystem/name. Expand is text, no `dangerouslySetInnerHTML`.
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

Approvals (115). Import/Export (116). Copying GoClaw chrome. Live vendor tokens. Binding/killing demo ports. Merge. SPECs 115-118. Host-side `pip`/`npm`/`gh` execution (declared inventory + live runtime probe only).
