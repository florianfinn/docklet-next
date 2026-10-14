# Design decisions

These documents describe the agreed architecture and governance, not work status.
GitHub issues and milestones hold implementation and acceptance evidence.

| Document | Decision or procedure |
| --- | --- |
| [Arcane revision and module map](arcane-adoption.md) | Selected revision, local module identities and reproducible Go licence inventory |
| [SQLite removal map](arcane-sqlite-removal.md) | Exact source, test, migration and build paths for the PostgreSQL-only cut |
| [Core adoption](core-adoption.md) | One-time Go adoption, scope boundaries and evaluation gates |
| [API contract and UI mapping](api-contract.md) | Reproducible OpenAPI export, generated types and owned DTO/UI gaps |
| [Run gate and single-hub operation](run-gate.md) | Activity states, gated admission, cancellation, restart recovery and one hub per database |
| [Database baseline](database-baseline.md) | PostgreSQL baseline generation, forward-only migration policy and schema proof |
| [Agent surface](agent-surface.md) | Agent and Edge agent module allowlist and host exposure |
| [Functional cut](functional-cut.md) | M1 module decisions and the R5 evaluation |
| [Frontend consolidation](frontend-consolidation.md) | Frozen React reference and product/demo boundary |
| [Language](language.md) | English repository and UI base, complete German |
| [Free shadcn](free-shadcn.md) | Free component source only |
| [Review workflow](review-workflow.md) | Independent review, findings and merge gates |
| [Publication policy](publication-policy.md) | Public data checks and local denylist |
| [Provenance](provenance.md) | Source headers, inventories, size ratchet and assets |
| [Licensing](licensing.md) | Six adoption and distribution duties |
| [Practical acceptance](practical-acceptance.md) | Approved sandbox procedure |

Historical plans belong in the [history index](../history/README.md).
