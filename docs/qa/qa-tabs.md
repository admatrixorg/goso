# All-tabs check (v2 §13 rule 2)

`npm run verify` typechecks and builds the control plane. It does not start a gateway, so the browser check is a separate command.

```bash
# gateway on :8080 (GOSO_DEV_MODE=1 is enough for local QA)
# control plane: cd control-plane && npm run dev
cd control-plane
npm run qa:tabs
```

Environment:

| Variable | Default | Meaning |
|---|---|---|
| `GOSO_UI` | `http://127.0.0.1:3000` | Control plane |
| `GOSO_GW` | `http://127.0.0.1:8080` | Gateway `/healthz` must be 200 before the browser starts |
| `CHROME_PATH` | `/usr/local/bin/google-chrome`, else `google-chrome` | Browser binary. The script uses `puppeteer-core` and does not download Chrome |
| `QA_SHOTS=1` | off | Also write a screenshot per sidebar tab |
| `PROTO_HTML` | the uploaded prototype path if present | Prototype half of each shot |
| `QA_SHOTS_DIR` | `/tmp/goso-qa-tabs` | Screenshot directory |

The check opens every sidebar hash, every v2 alias, and the folded routes `#/teams`, `#/heatmap`, `#/marketing`. It then runs chat send, the agent and model menus, `@` mention, conversation search, and rename / archive / restore.

It fails on:

- an uncaught page error
- a console error that is not Chrome’s generic “Failed to load resource” line (those HTTP failures are judged from the response)
- a response `>= 400` from `/api/…` or `/healthz`
- the copy “chưa dựng”, or the Coming Soon page (“Màn hình này không có trên bản non-demo”, “Trạng thái đang phát triển — không có API”)

It does **not** fail on “Chưa có dữ liệu”, “Chưa có cuộc họp”, or “Chưa có hook”. Those are honest empty states for Decision, Cuộc họp, and lifecycle hooks. The script prints them as `empty` so a reviewer can see them. They are not a “chưa dựng” screen.

`/crm-api` is ignored. That prefix is the Vite proxy to goso-crm on port 8082, a separate product. Overview and the folded heatmap call it when a CRM is configured. A down CRM is not a Goso gateway failure. Favicon 404s are ignored for the same reason: they are not a page dependency.
