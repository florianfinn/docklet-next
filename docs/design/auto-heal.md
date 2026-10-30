# Auto Heal

Auto Heal belongs to each executing Docker host. A global administrator reads or
updates typed settings at `/environments/{id}/auto-heal`. The hub serves local
settings and proxies remote settings through Direct HTTP or the Edge command
allowlist. Agent contract 10 is the minimum: older agents refuse through the
shared agent-access envelope.

Settings expose enabled, maximum restarts, window minutes and exact stack and
container-name exclusions. The interval remains on the schedule surface.
Generic settings writes reject all Auto Heal fields, including the interval,
atomically. Typed writes validate ranges, name patterns and list limits and
publish changed settings through the existing scheduler subscription.

Environment precedence stays role-dependent. Normal hub settings are database
managed. Agent, Edge agent and UI-configuration-disabled hosts use the effective
loader's presence rules, including explicit false, empty strings and readable
`_FILE` or `__FILE` values. An effective Auto Heal environment override makes
typed writes read-only on that executing host. Unreadable files use the loader's
normal fallback semantics.
Effective values outside the typed contract fail closed: the host is skipped and
each run logs the invalid fields.

## Eligibility and ownership

The daemon list is only a candidate set. Immediately before reserving an
attempt, inspect verifies exact identity, running state, an active healthcheck
and unhealthy status. Paused, restarting, internal and self containers are
excluded. Current settings and exact Compose project labels are read again, so
an exclusion or disable applied during a delayed inspect takes effect.

Auto Heal skips ownership held by runtime receipts, the resolved parent stack,
queued/running gates, volume backup admission, workspace locks, persisted backup
or restore work, restore-as-new destinations and writer fences. Missing or
unreadable ownership evidence is busy. Narrow read accessors inspect existing
ownership without acquiring runtime or backup leases. The workspace lock probe
holds a volume lock for an instant, so a concurrent volume write can meet a short
busy refusal.

## Durable attempts and recovery

Before Docker restart, a transaction records an `attempting` row with the
scheduler run and container identities, baseline start time and restart count.
The unique run/container pair closes a crash between reservation and scheduler
progress persistence. Failed and interrupted attempts still count toward the
configured per-container window. A single `limit-reached` marker is emitted per
window. Settled history outside the maximum window is pruned. Row IDs never repeat after
pruning, because projection and dispatch identities depend on them.

After a crash, a changed start time or restart count confirms restart only when
the container is no longer restarting. Otherwise the attempt becomes
interrupted. Recovery never repeats a reserved Docker restart. Unreserved
candidates resume through the same current eligibility path.

Each settled row projects once into a container Auto Heal activity and a coded
event with environment ID `0`. Live hub projection inserts its notification in
the same transaction; live agent dispatch follows commit and is best effort.
Interrupted and replayed projections produce no notification. Historical agent
rows therefore cannot send under a replacement pairing. A failed projection
leaves its marker unset for retry. Scheduled and recovery scheduler runs produce
no empty batch activity; manual runs retain ordinary job visibility.
