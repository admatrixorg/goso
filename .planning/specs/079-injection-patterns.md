# SPEC 079 — Injection pattern completeness

> After 078. Production default block already 066.

Closes **S2, S7** (four of six patterns).

## GoClaw cite

`docs/09-security.md` injection scanning (behavior: more prompt-injection phrases, fail-closed in production).

## goso plan

1. Extend `security.ScanInjection` to **six** documented substrings (keep the four; add two from the 041 QA gap — document the exact strings in QA, no copy from goclaw source).
2. Tests: each pattern matches; production default block; demo log.
3. Do not weaken 066 gates.

Commit `admatrixmdp/spec079-injection-patterns`.
