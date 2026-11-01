## Spec item

One spec item per PR. This PR is SPEC ___ only.

## Merge checklist (v2 §13, 2026-11-01)

All three are required. Missing one means do not merge.

- [ ] **(1) Screenshots.** Side-by-side HTML prototype vs real Goso for every tab this PR changes.
- [ ] **(2) All-tabs check.** `cd control-plane && npm run qa:tabs` is green against a running gateway and control plane. No “chưa dựng” screen, no console error, no failed request the page depends on. Chat send, agent/model, @mention, search, and rename / archive / restore pass. See `docs/qa/qa-tabs.md`. `/crm-api` (goso-crm on :8082) is ignored: it is a separate product, not the Goso gateway.
- [ ] **(3) Sample data.** No MQ Capital or customer names, phones, or emails in code, examples, or screenshots. Minh An (demo) only.

## What changed

## How to verify
