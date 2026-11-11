# Design decisions

These documents describe the agreed architecture and governance, not work status.
GitHub issues and milestones hold implementation and acceptance evidence.

| Document | Decision or procedure |
| --- | --- |
| [Arcane revision and module map](arcane-adoption.md) | Selected revision, local module identities and reproducible Go licence inventory |
| [SQLite licence assessment](sqlite-license-assessment.md) | Exact compiled header grants, selected Go/web evidence and packaging boundaries |
| [SQLite hub database](sqlite-database.md) | Driver, files, transactions, fresh schema, Francis, acceptance and the single implementation step |
| [Runtime history](runtime-history.md) | M17 decision: minute sampling, four conditions, stop intents, downsampled history, capacity trend and sustained-load alerts |
| [Service widgets](service-widgets.md) | M17 decision: hub-side Plex, Overseerr and Minecraft widgets, encrypted credentials, certificate pins, egress limits and app links |
| [Personal preferences](preferences.md) | M17 decision: preference sections on the user profile, web synchronisation and the one-time browser migration |
| [Bind mounts](bind-mounts.md) | M16 decision: bind keys, shared path rules, file browser and backups for directory and single-file binds, size scans |
| [Backup completion](backup-completion.md) | M16 decision: hub archive copies to targets, draft target test, per-host delivery state, registration revision, backups overview |
| [Volume backups and recovery](volume-backups.md) | Owning-host lifecycle, immutable targets, recovery keys, observed availability, archive validation and transport limits |
| [Identity](identity.md) | Startup sign-in modes, direct-peer proxy trust, session boundaries and recovery |
| [Core adoption](core-adoption.md) | One-time Go adoption, scope boundaries, evaluation gates and the dated decision ledger |
| [API contract and UI mapping](api-contract.md) | Reproducible OpenAPI export, generated types and owned DTO/UI gaps |
| [HTTP adapter, live channel and capabilities](adapter-live-capabilities.md) | M3 adapter outcomes, server capabilities, run/event refresh and contract matrix |
| [Activity discovery and bounded recovery](activity-discovery.md) | List projection, lazy detail demand, recovery state table and behavioural guards for #225 and #223 |
| [Run gate and single-hub operation](run-gate.md) | Activity states, gated admission, cancellation, restart recovery and one hub per database |
| [Runtime control and log lifecycle](runtime-control.md) | Opaque-key retries, bounded outcome evidence and unknown recovery, logs/session closure and staged v4 foundation activation |
| [In-app updates](app-updates.md) | Narrow Compose helper, correlated hub/agent operations, frozen recovery contract and manual failure recovery |
| [Images and updates](images-updates.md) | Host image guards, safe registry credentials, update policies, scheduled apply, restore points and lifecycle prerequisites |
| [Image identity and initial update reads](images-updates-identity.md) | Classic/containerd identity, initial policy/schedule/baseline DTOs, producer fixtures and candidate regression proofs |
| [Stack updates UI contract](updates-ui.md) | Production update/restore dialogs, policy and notification states, contract gaps and behavioural proofs |
| [Hub-owned stack update policies](update-policies.md) | Policy trigger state table, scheduler authority, frozen runtime admission and durable candidate notifications |
| [Update policy model](update-policy-model.md) | M18 decision: check-only default for stacks and containers, global interval, install windows, migration 018, pruning and agent v14 |
| [Schedule rule and hub time zone](schedule-rule.md) | M18 decision: one wall-clock rule with spring gap and autumn fold, selectable hub zone and manual scans |
| [Restore points before deploy](deploy-restore-points.md) | M18 decision: points before deploy and redeploy, durable point evidence, recovery offer and startup cleanup |
| [Container and single service updates](container-service-updates.md) | M18 decision: standalone container update by recreate, previous-image restore and single-service updates |
| [Wire codes in the web catalogue](wire-code-catalogue.md) | M19 decision: total compiler-checked maps from contract enums to EN/DE catalogue keys and the closed completeness criterion |
| [Stack import](stack-import.md) | M19 decision: one-time GitHub and Docker Hub import, content-only proposal pass, egress limits, mirror address and dialog fences |
| [Bulk actions](bulk-actions.md) | M19 decision: client-side fan-out over single commands, per-action capability, admission outcomes and bulk update eligibility |
| [Notification targets](notification-targets.md) | M19 decision: named targets per channel, migration 019, link base rule, readable notices and the vulnerability notice repair |
| [Unhealthy notice and Auto Heal cooldown](unhealthy-cooldown.md) | M19 decision: persisted condition state, 10-minute minimum duration and hourly quiet period, cooldown, validity hint and agent v15 |
| [Activity retention](activity-retention.md) | M19 decision: configurable hub-wide event and run limits, agent sync and event list order |
| [Global search](global-search.md) | M19 decision: Ctrl/⌘+K palette, rights-bound destinations and quick actions, removal of the Arcane settings search |
| [Network management](network-management.md) | Host-side validation, protected deletion, Direct/Edge writes and agent v6 |
| [Storage and file workspace](storage-workspace.md) | Stateless prune selection and re-checks, helper image policy, workspace mapping and agent compatibility |
| [Stack authoring and deployment](stack-authoring.md) | M7 create/save/label/deploy mapping, non-idempotent create, partial state and hub docker run conversion |
| [Database baseline](database-baseline.md) | SQLite baseline contract, forward-only policy and historical integrity reference |
| [Vulnerability scanning](vulnerability-scanning.md) | Local Docker scanner authority, host admission, retained findings, run evidence and agent v8 |
| [Notification delivery](notifications.md) | Hub-owned delivery, credential boundaries, host ingress and availability |
| [Auto Heal](auto-heal.md) | Per-host settings, ownership, durable restart attempts and silent recovery |
| [Container and host operations](container-host-operations.md) | M14 shared foundation: migration 014, agent contract v11 and ownership |
| [Simple users](simple-users.md) | M15 decision: administrators, users with two levels per assigned stack, login modes, personal keys, sign-out |
| [Resource rights](resource-rights.md) | M15 foundation: rule table, decision points, stack/container resolution, stream revocation |
| [Container shell](container-shell.md) | Pre-upgrade hub acceptance, shell choice, frame protocol, close reasons and session release |
| [Container origin, recreate and edit](container-origin-recreate.md) | Unraid and manual origin, overlay, guards, idempotent recreate and journal recovery |
| [Stack stop mode and service restart](stack-stop-service.md) | Hub stop-mode setting, stopped stacks from the resolved model and single service restart |
| [Host maintenance](host-maintenance.md) | Host edit, fenced agent restart, components, Settings › Hosts and read-only installation files |
| [Host availability](host-availability.md) | Heartbeat liveness, standby and typed agent refusals |
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
| [Tools helper image](tools-image.md) | Pinned two-platform helper, offline acfs, gated publication and corresponding BusyBox source |
| [Hub database backup](hub-backup.md) | Coordinated SQLite archives, paired restore and interruption recovery |
| [Request hygiene](request-hygiene.md) | M23 session hint, hub maintenance hold, maintenance outcome for every reader and unavailable-host skips |
| [Dashboard load](dashboard-load.md) | M23 bounded events window, active vulnerability counts, inventory scope retention, metrics lifetime and readiness markers |
| [Capacity, run gate and density](capacity-gate.md) | M23 single disk source and time axis, run gate confirmation read and supported containers per host |
| [Practical acceptance](practical-acceptance.md) | Approved sandbox procedure |

Historical plans belong in the [history index](../history/README.md).
