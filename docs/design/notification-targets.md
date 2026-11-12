<!--
Source: docklet-next/issues/351
Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
Retrieved: 2026-11-06
License: Apache-2.0
Deviations: Original named notification targets, readable notices and link base rationale.
-->

# Named notification targets, readable notices and link base

Decision for #351, #354, #308, #269, #270 and #301 (M19, maintainer kickoff 2026-11-06). Docklet is a
homelab tool with normally one administrator. Extends [notification delivery](notifications.md).

## Accepted decisions (do not reopen)

- Several named targets per channel, replacing one configuration per channel.
- Notification links follow the link base setting (`auto`, `internal`, `domain`).
- Notices are readable text, not event keys and raw codes.
- Credentials, redaction and event subscription stay per target row.

## Starting point (main b5068aff)

- `notification_settings(id, provider, enabled, config JSON, created_at, updated_at)`
  (`001_baseline.sql:20`). The provider index is not unique, but every read, write, delete, route
  (`/environments/{id}/notifications/settings/{provider}`, `/notifications/test/{provider}`), the test
  lock (`handler.go:217`) and the web "used channel" filter (`NotificationsSection.tsx:72-73`) are
  keyed by provider. There is no name. Routes are hub-only (`rejectHubRoute`, environment `0`); agents
  only send `POST /notifications/dispatch` to the hub. None of this is in the agent contract
  fingerprint.
- Event subscription is per row (`config.events`, a missing key means on, `sustained_load` opt-in).
  `notifyEnabledProvidersInternal` (`service.go:416-473`) already fans out over all enabled rows
  (concurrency 4), logs one `notification.send` or `notification.delivery-failed` event per row, and
  the outbox settles `sent` if any target succeeded.
- Outbox notices use the event key as title and subject and `<event> on <host>` plus `field: value`
  lines with raw codes (`outbox.go:76-125`); HTML is one `<p>` with collapsed newlines.
- `linkBase`, `domainUrl` and `hubAddress` exist as settings, but no backend code reads them; links
  use `cfg.GetAppURL()` (`outbox.go:86-90`, email templates `service.go:116`). The web computes the
  same preview (`domain/settings.ts:144-148`).

## Design

### Named targets (#351)

- Migration 019:
  - `ALTER TABLE notification_settings ADD COLUMN name TEXT NOT NULL DEFAULT ''`;
  - backfill `name` with the channel's English label ("Email", "Discord", "Telegram", "Signal",
    "Slack", "ntfy", "Pushover", "Gotify", "Matrix", "Google Chat", "Generic webhook"); further rows
    of the same provider get " 2", " 3" by `ROW_NUMBER()` over `created_at, id`;
  - unique index `notification_settings_name_unique ON notification_settings(name COLLATE NOCASE)`
    (SQLite `NOCASE` is ASCII case-insensitive).
- Name: required, trimmed, 1–64 characters, unique case-insensitively across all targets. Names are
  user data and never translated.
- Routes (hub-only, Admin, same `rejectHubRoute`):
  - `GET /environments/{id}/notifications/settings` (list, sorted by name);
  - `POST .../notifications/settings` (body `{name, provider, enabled, config}`);
  - `PUT .../notifications/settings/{targetId}` (provider immutable after creation);
  - `DELETE .../notifications/settings/{targetId}`;
  - `POST .../notifications/test/{targetId}`.
  The `{provider}` routes are removed; only the web used them and hub and web share a version.
- The test lock is keyed by target ID: at most one test per target, two targets of one channel can
  test in parallel. The service lookup of the test path (`service_delivery.go:275`) goes by target ID.
- Credentials, redaction, `credentialsSet` and `ValidateCredentialTargetChange` stay per row. Editing,
  testing or deleting one target leaves another target's row, secrets, events and delivery metadata
  unchanged.
- Delivery events (`notification.send`, `notification.delivery-failed`) gain metadata `targetId` and
  `targetName` on the per-row clone (`service.go:371`), never on the shared fan-out map. Titles read
  "Notification sent via {channel} ({name})", the failure title names the target the same way, and
  older rows without `targetName` keep today's title.
- Web: a list of targets (name, channel badge, destination summary, event chips, enabled switch,
  test, edit, delete). The add dialog offers every channel and defaults the name to the channel label,
  appending a number when taken.

### Link base (#354)

- One resolver `notification.LinkBase(ctx) string`, the rule of the web preview: `domainUrl` when set
  and `linkBase != internal`; else the internal hub address (`hubAddress` as URL, `http://` added to a
  bare address); else `APP_URL`. Trailing slashes are removed.
- Used by the outbox text, the email templates (AppURL and LogoURL) and every notice that embeds a hub
  link. Agents never build links; their notices go through the hub dispatch.
- Notifications settings show "Links in messages point to {url}" with a link to Settings › General.
  The web preview shows the same `APP_URL` fallback text when no internal address is set.
- A table test shares its cases with the web `notificationBaseUrl` test (including empty address,
  trailing slashes, bare address).

### Readable notices (#308)

One English text per outbox event; title, email subject and first line are the same:

- `deploy_failed`: "Deployment failed: stack {stack} on {host}" + reason phrase; a failed
  standalone-container image update or restore reads "Update failed: container {container}
  on {host}" (restore: "Restore failed: …") and links to Containers;
- `backup_failed`: "Backup failed: volume {volume} on {host}" (restore: "Restore failed: …");
- `host_offline`: "Host {host} is offline" / "Host {host} is back online";
- `auto_heal`: "Auto Heal restarted {container} on {host}" / "Auto Heal could not restart …" /
  "Auto Heal restart limit reached for {container} on {host}";
- `sustained_load`: "High {metric} load on {host}: {percent}% above {threshold}% for {minutes} min";
- `container_unhealthy`, see [unhealthy notice](unhealthy-cooldown.md): "Container {container} on
  {host} is unhealthy (healthcheck failing for {minutes} min)" / "Container {container} on {host}
  keeps restarting ({restarts} restarts in 10 min)".

Reason codes map to short phrases (`reasonPhrase`, one Go map with a test over every typed reason of
these events); an unknown code renders "an unexpected error occurred". The code stays in the generic
webhook variables (`reason`, `event`) for receivers that parse it, never in title or text. HTML
renders lines as `<br>`, Markdown and Slack keep newlines; the last line is "Open Docklet: {link
base}/<deep path>" when a deep path exists (stack, volume or host page). The same newline fix applies
to the `vulnerability_scan` HTML format.

### Small items

- #301: remove `FormatDeliveryError`. The cited sites classify delivery failures into persisted event
  and API data (`service.go:480`, `handler.go:224`); tests assert that the
  `notification.delivery-failed` event metadata and the test-endpoint response carry only the
  classified reason, never provider error text, plus any real `slog` call on those paths through a
  capturing handler.
- #270: a stale-settler test for the outbox fence (`vulnerability/notifications.go:103`); assert that
  the hub-only keys `vulnerabilityScanSchedule` and `vulnerabilityNotify*` never enter the agent
  settings sync key set (`jobs_bootstrap.go:559-640`); route the two hub-originated vulnerability
  routes through the real Direct proxy and Edge command path in a test; merge the two import blocks;
  delete the unreferenced `vulnerability-summary_{html,text}.tmpl`.
- #269: the dispatch path persists the target progress with the accepted activity ID before inserting
  the receipt. A failed receipt insert can settle the dispatch run as Partial, after which
  `Reconcile` never runs again, so the repair lives in the periodic hub scan-notify pass
  (`HubScans.Notify`): before delivering, it reads persisted dispatch-run targets of origin
  `scheduled` from the scheduler run progress store that are younger than 24 h, have an activity ID
  and no receipt row, and inserts the receipt (`ON CONFLICT DO NOTHING`, original registration ID,
  activity ID, the target's acceptance time as `created_at` so the 24 h expiry keeps its meaning).
  Manual-origin targets are never repaired. Remaining limit: a hub crash between the agent's 202 and
  the progress write (milliseconds) still loses the notice; [vulnerability scanning](vulnerability-scanning.md)
  states it.

## Break map

| Row | Decision | Test |
| --- | --- | --- |
| Migration 019 on real old data (one row per provider, two rows of one provider, empty table) | backfill + unique index | migration test with M18-schema fixtures; red proof: no numbering → unique index fails |
| Upgrade keeps delivering | existing target still receives outbox events after 019 | integration test: seeded row, outbox delivery after migration |
| Name collision, blank, > 64 | typed 400/409 | handler tests; red proof: skip uniqueness → red |
| Target isolation | per-row secrets, events, metadata | two same-provider targets with different credentials and subscriptions; red proof: update by provider → red |
| Test lock per target | two targets of one channel test concurrently; the same target twice → busy | concurrent handler test |
| Hub-only | routes refuse non-zero environment and agent mode | existing `rejectHubRoute` tests extended to the new routes |
| Link base rule | shared case table Go/web | table tests both sides; red proof: ignore `linkBase=internal` → red |
| No raw key in notice text | every event kind and an unknown reason | template test over the known internal event keys and reason codes (not a ban on dots or underscores, names and URLs may contain them); email subject and HTML asserted from the rendered email, generic payload separately |
| #301 consumers | only the classified code is stored or returned | event and response assertions with a provider error containing a marker string; red proof: store the raw error → red |
| #269 repair after terminal Partial | notify pass repairs from persisted scheduled targets | through the real scheduler progress store: receipt insert fails, run settles Partial, restart, notify pass → exactly one receipt and one delivery; repeat → idempotent; manual target never repaired; red proof: repair only in `Reconcile` → red |
| #270 real transport | vulnerability routes through real proxy and Edge path | httptest agent and Edge event path test |
| New codes | types, OpenAPI, EN/DE through the wire-code guard | guard |

## Contract

- Routes as above; the `{provider}` routes are removed from OpenAPI.
- Request and response: `name` (required) and immutable `provider` on create and in lists.
- Codes: `notification.name-taken` (409), `notification.name-invalid` (400),
  `notification.target-not-found`; EN/DE text.
- Migration 019 (shared with [the unhealthy notice](unhealthy-cooldown.md)) adds the `name` column and
  index and rebuilds the outbox CHECK list for `container_unhealthy`; the migration evidence covers
  `internal/schema/models.go`, the integrity fixtures, `schema_integration_test.go` and
  `scripts/migration-hashes.json`.
- Agent contract unchanged by this note: notification routes are hub-only.
