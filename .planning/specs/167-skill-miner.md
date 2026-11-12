# SPEC 167 — Skill miner

Status: proposed on 2026-11-12 (MQG-229). This file is the spec. A later PR implements it. Do not merge an implementation from this file.

Sample data is Minh An only. The sample proposal is a skill named `bao-gia` for project `mien-tay`, with a step that checks VAT, after repeated quote sessions and one failed note `thiếu VAT` (SPEC 163). No MQ Capital. No customer data. No real names. No phone numbers.

No Dewee source and no GoClaw source was read or used. GoClaw is CC BY-NC. Only public articles were consulted. This spec does not copy either codebase, and it does not copy a skill file out of either tree.

## Number

Numbering is the table in SPEC 153. This file is 167. Proposals are not `memory_facts` rows. If a proposal later becomes a fact, that write goes through SPEC 161 and therefore already has `scope` and `owner_id`. This spec does not add those columns.

Checked on `main` at `9545d08`: 153–169 were unused. PR #24 holds 140. PR #10 holds 136. Old 134 and 137 are SPEC 160, 161, and 164. The miner’s only exit is SPEC 160.

## Why

Repeated successful sessions are still episodic notes. A weekly job can propose a skill. It must not write the skill store itself. SPEC 160 is the only way a proposal becomes a skill.

## Behavior

1. Flag `GOSO_MEMORY_SKILL_MINER`. Default off. Off: the cron does not run.
2. On, and only when `GOSO_MEMORY_APPROVAL` is on: a weekly cron groups episodic notes with `outcome=success` from the last 30 days, by agent and by the ordered tool-call signature.
3. A group of at least 3 sessions, with no skill that already matches the signature, becomes one proposal: a short name, when to use it, the steps, and the tools. The proposal is inserted into the SPEC 160 queue. The skill directory is not written.
4. A group that matches an existing skill and also has a `failed` note becomes an edit proposal with a diff, also only through SPEC 160.
5. Fewer than 3 sessions produces no proposal. Two Minh An quote sessions do not propose `bao-gia`. Four successful quote sessions plus the `thiếu VAT` failure may propose it, and the proposal waits in SPEC 160.
6. The cron does not put proposal text into the prompt, so it does not spend the SPEC 165 budget.
7. Production stays off until Dat approves this flag.

## Gate

- **Invariant.** The miner does not write the skill store.
- **Acceptance.** A proposal lands in the SPEC 160 queue.
- **Negative test.** Fewer than 3 sessions produces no proposal.
- **Rollback.** Turn the cron off (`GOSO_MEMORY_SKILL_MINER` off).

## Non-goals

Auto-merge of a skill. A proposal path that bypasses SPEC 160. Designing SPEC 168 or SPEC 169.
