# SPEC 029 — Unsigned desktop zip (no Apple cert)

> LOCKED: 2026-08-26. goclaw has `make desktop-dmg` (needs signing). We **N/A** notarize. Rewrite: zip + README.

## Goal

`scripts/package-desktop.sh` produces `dist/GOSO-darwin-arm64-unsigned.zip` containing `GOSO.app` if present **or** a stub note if wails build skipped (`SKIP_WAILS=1` for tests). README-UNSIGNED: `xattr -cr GOSO.app`, Gatekeeper, no notarization.

## Own

`scripts/package-desktop.sh`, `desktop/README.md` section. Do not edit mcp/.

## AC

- [ ] AC-01 SKIP_WAILS=1 still writes zip with README-UNSIGNED.md inside.
- [ ] AC-02 README states unsigned / no auto-update.
- [ ] AC-03 no codesign invocation required.

## Non-goals

Developer ID, Sparkle updater, Windows exe.
