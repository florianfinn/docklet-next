# M6 runtime acceptance — 2026-10-21

[M6 closing PR #134](https://github.com/florianfinn/docklet-next/pull/134)
merged at `6e853b136e0ef48d77a2edea9f37c86df691a2e5`. Its source tree is
`9f383078410272645e45325ba7ec681e53e3fbc3`, identical to the independently
reviewed closing head. [Independent review](https://github.com/florianfinn/docklet-next/pull/134#issuecomment-6303631953),
[main CI](https://github.com/florianfinn/docklet-next/actions/runs/39511671926)
and [exact-package practical acceptance](https://github.com/florianfinn/docklet-next/issues/104#issuecomment-6304179149)
passed. The [closing issue #104](https://github.com/florianfinn/docklet-next/issues/104)
and M6 milestone were closed. This records sandbox acceptance, not a product
release or production deployment.

## Accepted scope and limits

The sandbox exercised all seven lifecycle actions through the real API on Direct
and Edge agents, with independent Docker-state checks. It also covered retry-key
joins and conflicts, cancellation, hub/agent restarts, unknown outcomes and
reasoned administrator acknowledgement, live logs and logout across tabs.
SQLite migration, consistent backups and integrity were checked. English and
German views were inspected at desktop and mobile widths in Chromium.

A stack restart was additionally triggered through the UI; the remaining UI
command mappings have automated consumer coverage. This does not establish every
UI gesture, browser, interleaving or compatibility with an old released agent.
Live logs do not promise durable replay or an exhaustive memory-bound proof.
Private operational evidence and screenshots remain outside the repository.

## Coordination lessons

Use coherent deliveries with one responsible owner and local checkpoints;
file-count heuristics do not define PR boundaries. Keep a current context pack
with complete accessible reports rather than recursively copied history. Map a
correctness fix across producer, transport and actual consumer from the first
attempt, including initial failure, partial authority, latency and identity
changes where relevant. Verify asynchronous assertions after the relevant effect.

Independent review found real errors despite green preliminary checks. The
benefit is demonstrated by those findings and discriminating regression probes;
there is no complete time, token or cost ledger from which to claim savings.
Project rules now live in [AGENTS.md](../../AGENTS.md#agent-coordination), while
reusable procedures live in the separate orchestration skill repository.

## Follow-up boundary at handoff

As of this record, optional M6 follow-up packages #115, #118, #128, #131, #133 and
#135 remain open. Earlier #24, #27 and #100 also remain open. Their live scope and
disposition belong in GitHub; this list does not declare them complete or make all
of them prerequisites for subsequent work.

The API mapping identifies **M7 Stack authoring and deployment** as the next
planned feature area, followed by M8 Storage and file workspace. At handoff no M7
milestone or accepted ready gate has been created. Before implementation, agree
M7 scope and acceptance with the maintainer, inspect existing authoring/import
paths and determine which open findings are actual prerequisites. Runtime
execution and retry semantics should reuse the accepted M6 contracts.
