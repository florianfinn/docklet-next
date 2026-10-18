# Design decisions

These documents describe the agreed architecture and governance, not work status.
GitHub issues and milestones hold implementation and acceptance evidence.

| Document | Decision or procedure |
| --- | --- |
| [Arcane revision and module map](arcane-adoption.md) | Selected revision, local module identities and reproducible Go licence inventory |
| [SQLite licence assessment](sqlite-license-assessment.md) | Exact compiled header grants, selected Go/web evidence and packaging boundaries |
| [SQLite hub database](sqlite-database.md) | Driver, files, transactions, fresh schema, Francis, acceptance and the single implementation step |
| [Identity](identity.md) | Startup sign-in modes, direct-peer proxy trust, session boundaries and recovery |
| [Core adoption](core-adoption.md) | One-time Go adoption, scope boundaries and evaluation gates |
| [API contract and UI mapping](api-contract.md) | Reproducible OpenAPI export, generated types and owned DTO/UI gaps |
| [HTTP adapter, live channel and capabilities](adapter-live-capabilities.md) | M3 adapter outcomes, server capabilities, run/event refresh and contract matrix |
| [Run gate and single-hub operation](run-gate.md) | Activity states, gated admission, cancellation, restart recovery and one hub per database |
| [Runtime control and log lifecycle](runtime-control.md) | Bounded runtime retries, settled interruption and unknown recovery, logs/session closure and the v4 foundation |
| [Database baseline](database-baseline.md) | SQLite baseline contract, forward-only policy and historical integrity reference |
| [Agent surface](agent-surface.md) | Agent and Edge agent module allowlist and host exposure |
| [Edge threat review](edge-threat-review.md) | Assets, attacker positions and verdicts for the adopted Edge tunnel and agent mode |
| [Host onboarding](host-onboarding.md) | Direct and Edge registration, token lifecycle, installer directories and permissions, hidden agent and stale hosts |
| [Functional cut](functional-cut.md) | M1 module decisions and the R5 evaluation |
| [Frontend consolidation](frontend-consolidation.md) | Frozen React reference and product/demo boundary |
| [Language](language.md) | English repository and UI base, complete German |
| [Free shadcn](free-shadcn.md) | Free component source only |
| [Review workflow](review-workflow.md) | Independent review, findings and merge gates |
| [Publication policy](publication-policy.md) | Public data checks and local denylist |
| [Provenance](provenance.md) | Source headers, inventories, size ratchet and assets |
| [Licensing](licensing.md) | Six adoption and distribution duties |
| [Hub database backup](hub-backup.md) | Coordinated SQLite archives, paired restore and interruption recovery |
| [Practical acceptance](practical-acceptance.md) | Approved sandbox procedure |

Historical plans belong in the [history index](../history/README.md).
