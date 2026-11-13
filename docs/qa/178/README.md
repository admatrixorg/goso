# SPEC 178 qa:tabs

Latest code under test: `74b3effbe35d996e275dc9f6daa3695d9e085cbf` (channel inbound stores the customer message before any model call).

Earlier logs for `3a238bc5a152169338827e93fdadd30452c82149`, `ec929e8d7008783158f9474d3ad6a15dc3c1404d`, and `629dd8ce0c82d56d029e1f7de4b459035aa7c2b2` are kept.

Gateway binary built from that commit. Fresh `GOSO_DB_PATH` and `GOSO_VAULT_DIR`. `GOSO_DEV_MODE=1`, `GOSO_RATE_LIMIT=0`, `GOSO_ENV=development`. Unset: `GOSO_DATABASE_URL`, `GOSO_ADMIN_TOKEN`, `GOSO_VIEW_TOKEN`, `GOSO_VAULT_SCOPED_SYNC`, `GOSO_LITE`, `GOSO_CONTACTS_DURABLE`.

Seed: `POST /api/agents` with `agent_key` `minh-an-sales`, `display_name` `Cố vấn Minh An`, `model` `echo`.

```bash
GOSO_UI=http://127.0.0.1:3000 GOSO_GW=http://127.0.0.1:8080 \
  CHROME_PATH=/usr/local/bin/google-chrome npm run qa:tabs
```

Full log: [qa-tabs-74b3effbe35d996e275dc9f6daa3695d9e085cbf.log](qa-tabs-74b3effbe35d996e275dc9f6daa3695d9e085cbf.log). Exit 0. Last lines: `flow chat` then `qa:tabs ok`.
