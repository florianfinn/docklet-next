<!--
Source: docklet-next/issues/440
Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
Retrieved: 2026-11-12
License: Apache-2.0
Deviations: Original observed-update recovery decisions and rationale.
-->

# Observed updates and recovery

Scheduled workload updates and the detail action **Update with check** observe the
new version and attempt recovery if it fails. The ordinary manual Update action
keeps its existing behaviour. The split button makes observation an explicit
choice for a single container or stack; list and bulk updates keep their existing
behaviour. Docklet and agent self-updates follow their separate
[in-app update contract](app-updates.md).

## Observation and pass rule

The executor starts a full two-minute observation after deployment/recreation and
image identity verification finish. Slow deployment does not consume that window.
Every required running instance must stay running with the same identity and no
restart throughout the window. Instances with a healthcheck must be healthy at
the deadline; an unhealthy result fails early. A healthcheck still starting at
the deadline fails. The required service set and replica counts come from the
frozen deployment model, rather than whichever instances happen to be found.
Declared successful-completion dependencies must finish with exit code zero;
proven Compose one-offs are excluded. A successful update records its new baseline
or previous-image owner only after verification and observation finish.

## Container and stack recovery

For an observed standalone container update, the original container is retained
until the candidate passes. If the candidate fails, compensation removes it and
resumes the original container with its original Docker ID, configuration, name
and network membership. The resumed original receives its own two-minute
observation. Restart detection uses the restarted original's start identity and
post-start restart baseline, because Docker may reset its restart counter on start.

A stack update captures the previous configuration and image bindings before its
workload effects. Recovery restores the whole captured point, including the
previous service set, even when only one service was selected for the update.
Recovery uses the existing restore path and receives a fresh observation window.
It never recursively attempts another restore.

## Outcomes and notices

Recovery evidence is separate from the update's result. A failed candidate remains
a failed update even when the previous version is restored successfully.

| Recovery outcome | Meaning |
|---|---|
| Restored | Previous workload restored and checked; update failed |
| Restore failed | Recovery also failed; operator action is needed |
| Unavailable | Recovery could not safely run; the reason is shown |
| Not needed | The update stopped before changing the workload, or it succeeded |
| Interrupted | Execution continuity was lost; inspect the target |

Activity details show the recovery outcome and reason. Scheduled job targets use
`update.auto-restored`, `update.auto-restore-failed`,
`update.auto-restore-unavailable` and `update.interrupted`. A suppressed failed
candidate is **Skipped**, with `update.candidate-failed`.

The hub emits one scheduled failure notice at authoritative settlement. A transport
hold alone sends no notice and never proves execution failure. Interrupted runs
settle with an interrupted notice once executor evidence is available: while its
run is held because the agent is unreachable, a scheduled window keeps querying the
executor every 30 seconds for up to 10 minutes, without resending the update, so a
restarted agent's interrupted result reaches the job target and the notice. After
that limit the target stays `runtime.outcome-unknown`. Refreshing the child run
brings the executor's evidence to the hub; a Retry of the window job then
reconciles its target from that receipt. A finished job run is never rewritten. Successful
updates and skipped candidates send no failure notice. Manual **Update with check**
keeps the existing manual notification rules; its activity carries recovery evidence.
When the run changed the workload (restored, restore failed, unavailable or
interrupted), notices and details add: “Volume data was not restored; the app may
already have migrated it.” A failure before any workload effect omits the sentence,
because nothing was changed that could have migrated data. Notice subjects stay
short (“Scheduled update failed: …”); details follow in the body.

## Limits and suppression

Recovery restores workload configuration and image identity, never volume data.
An application may already have migrated its data; restoring its old image can
require operator intervention. Running AutoRemove (`--rm`) containers cannot retain
the original. They use the ordinary update path without observation; failure reports
recovery unavailable with `container.auto-remove`.

Changed captured files or other external edits can make recovery unavailable.
Docklet refuses to overwrite external changes. Process loss uses existing journal
safety recovery and reports **Interrupted**, with verification false; startup does
not launch another observed recovery. Cancellation after the first container effect
does not interrupt its detached safety sequence. Stack cancellation stops observation
and does not start further recovery effects.

After a scheduled candidate fails and recovery succeeds, later windows skip the same
policy/target, repository, platform and candidate manifest tuple. The manifest is
pullable identity, not the local Docker image ID, so changing only the tag does not
bypass suppression. Another manifest can proceed. Successful manual update clears
the target's failed-candidate history; manual checked updates do not add suppression
entries. The newest 64 tuples are kept per policy and target; older candidates may
be tried again. Manual restore does not clear suppression.
