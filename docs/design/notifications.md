# Notification delivery

The hub owns one configuration per supported channel. All settings and test
routes require a global administrator and environment `0`. Agents send host
events to the hub over authenticated HTTP or the existing Edge event transport;
they expose no notification configuration routes or Edge notification commands.

Provider configurations are typed in the shared contract. Save validates the
effective configuration after preserving blank credentials. Scalar credentials
are omitted from responses and identified by `credentialsSet`. Changing a
credential-bearing destination requires replacement credentials. Generic webhook
URLs and custom headers remain visible to administrators because they are part
of the configured request. Validation follows sender inputs, including bare
hosts, optional ports and the ntfy default host.

The event map includes image updates, vulnerabilities, Auto Heal, backup and
restore failures, deployment failures, host availability and cleanup. New outcome
and refusal codes are shared by OpenAPI, generated DTOs and the English and German
catalogues. Test results classify failures without returning provider text or
credential-bearing URLs. One test per provider can be active at a time.

## Durable hub delivery

Deployment failures, backup/restore failures, host availability and Auto Heal
use `notification_outbox`. Each event has an opaque identity and a unique
idempotency key. Deployment inserts share the terminal receipt transaction;
backup notices follow a successful live terminal save. Safety captures,
retention-only failures, cancellation, shutdown or restart interruptions and
startup/history repair remain silent; an administrator disposition of an
interrupted deployment notifies.

The hub delivery job claims rows with a compare-and-swap predicate. A five-minute
lease allows recovery of abandoned sends. Every claim, including a reclaimed
lease, counts toward three attempts. Unsent rows expire after 24 hours and
settled rows are pruned after 30 days. A successful target settles the event even
when another target fails; each failed target creates a classified
`notification.delivery-failed` event. No eligible target settles as unconfigured.
Existing image-update, vulnerability and cleanup fan-out keeps its owning policy
and delivery semantics.

Wake signals submit coordinator-owned work. A wake can arrive before its row
commits; the one-minute delivery tick bounds that delay. Maintenance cancels and joins that
work before database replacement. Restore expires pending and sending rows and
clears availability tracking, so restored history cannot emit stale notices.
Native transports have 15-second timeouts. SDK calls also have a bounded wait;
an SDK that ignores cancellation can finish after that wait, which leaves an
accepted possibility of a duplicate external delivery after retry.

## Host ingress and availability

The transport captures the environment, API key, token hash and Edge session
before asynchronous dispatch. Validation rejects unsupported kinds, old Auto
Heal shapes, unknown members and oversized display facts. A transaction rechecks
the captured credential binding before inserting a new host event. Edge ingress
also checks the current session; replacement between that check and commit
remains a bounded process-local race. Legacy kinds require no new event identity.
Agent delivery is best effort, with no durable agent outbox or Edge acknowledgement.

Availability reads the host runtime projection rather than persisted Edge flags.
Online and poll-only standby are available; offline and error start a five-minute
timer. Pending, token-rejected and update-required are not outages. Disabled,
removed and never-seen hosts do not notify. Recovery follows an emitted outage.
A process restart resets an unnotified timer, preventing old downtime from
immediately producing a new alert.
