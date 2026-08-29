# QA — SPEC 108 Storage

Date: 2026-08-30. Clean-room React/Go. No ZaloCRM / goclaw-source copy. No banned author ids. Demos `:8082` `:8091` `:18080` `:18791` — not bound or killed. `:3000` belongs to the coordinator. Do not paste goclaw Go. Do not merge. No live vendor tokens. No secrets in git/QA.

## GoClaw behavior (READ-ONLY cite — paths only)

Source tree: `/Users/mqglobal/Documents/goclaw/goclaw-source` (CC-BY-NC). Behavior was read from the 090 audit notes, never from `.go` sources, never vendored. Do not paste goclaw Go.

| Behavior | Cite |
|----------|------|
| Storage: file-browser rooted at the workspace with size summary and upload; upload/refresh/navigate/open/download/delete; error/empty states | `docs/qa/090-goclaw-sidebar-ux.md` Storage |

goso mapping (self-written): live tab `storage` in [App.tsx](../../control-plane/src/App.tsx) renders [StoragePage](../../control-plane/src/pages/StoragePage.tsx). Listing is `GET /api/storage?path=` (`/v1/storage` alias) with breadcrumbs plus size/type/mtime metadata only. Preview is `GET /api/storage/preview?path=` (64KiB cap). Download is `GET /api/storage/download?path=`. Upload is `POST /api/storage/upload` (multipart `file` + dest `path`). Delete is `POST /api/storage/delete` with `confirm` matching basename or relative path. Fail-closed when `GOSO_WORKSPACE` is empty (`configured: false`). Path jail reuses `security.Confine` / `GOSO_WORKSPACE`. Hidden/secret/runtime names (`.env`, `secrets`, `id_rsa`, `*.pem`, `runtime`, …) are omitted by default and refused on GET. Quota is `used_bytes` / `max_bytes` (`GOSO_STORAGE_MAX_BYTES`, default 64MiB). File cap 1MiB + extension allowlist. GET never returns credential values.

Out of scope: Realtime Events (109). Activity/Logs (110/111). Packages (114). Copying GoClaw chrome. Live vendor tokens. SPECs 109–118.

## What changed

- Live nav tab + page binding in `App.tsx` (work group, after Knowledge Graph). Breadcrumbs, size/type/mtime, loading / empty / not-configured / error.
- Upload/download/preview. Guarded delete with named confirm. Server-side path jail. Listing is metadata; preview is bounded. Internal runtime/secret/credential paths are not listed by default. GET never returns credential values.
- i18n vi+en. CP typecheck. Tests. agpl 0. `docs/qa/108-storage.md`.

## Commands

```
cd control-plane && npm run typecheck && npm test
go test ./gateway/internal/storage ./gateway/internal/httpapi ./gateway/internal/auth ./gateway/internal/serve -count=1
GOSO_ROOT=$PWD /Users/mqglobal/Documents/goclaw-binary/goso-crm/scripts/agpl-check.sh
./scripts/agpl-check-docs.sh
```

Do not bind or kill demo ports `:8082` `:8091` `:18080` `:18791`. Do not merge. Coordinator owns `:3000`.

## Proof

- `npm run typecheck` exit 0. `npm test` including `asPublicListing` dropping `.env` / `id_rsa` / `api_key` rows, `publicHasSecrets` on token-shaped values, `asPublicPreview` dropping Bearer text, `storageConfirmMatch` for name/path, quota/formatBytes.
- `go test` storage: empty env `not_configured`; listing hides `.env`/`secrets`/`id_rsa`; `..` and symlink escape denied; preview cap + secret-shaped text denied; upload type/size/quota; delete confirm; root delete refused. httpapi: empty `configured:false` + `/v1` alias; GET omits secret values; preview `.env` 403; download of `sk-live-` body 403; delete requires confirm. auth/serve: view-token GET list/preview 200, POST upload/delete 403.
- Page copy: “GOSO_WORKSPACE is not set” / “GOSO_WORKSPACE chưa đặt”. Confirm gõ tên. Preview is `<pre>` / image blob, no `dangerouslySetInnerHTML`.
- `agpl-check` and `agpl-check-docs` exit 0.

## Non-goals

Realtime Events (109). Activity/Logs (110/111). Packages (114). Copying GoClaw chrome. Live vendor tokens. Binding/killing demo ports. Merge. SPECs 109-118.
