# Host maintenance and installation files

Hosts can be edited, checked, reconnected and removed from the web UI, remote
agents can be restarted, and installation files are available for every host in
production. Shared contract and migration items are in
[container and host operations](container-host-operations.md); onboarding of new
hosts stays in [host onboarding](host-onboarding.md).

## Decisions

- Edit host, check and reconnect, restart agent, agent components, Settings ›
  Hosts and installation files per host card are live in production.
- A host with stacks or containers can be removed after a warning; workloads
  and the agent keep running.
- Restart agent is a Docker restart of the agent container. The hub host has no
  restart action. The host is locked for runtime work during a restart, and a
  credential change supersedes the restart.
- Without a chosen colour, a host's colour comes from a palette by host ID.
- Installation files of an existing host open read-only. A new token is issued
  only through an explicit, warned action.

## Baseline

- The adapter operations `updateHost`, `restartAgent`, `connectHost`,
  `reconnectHost` and the aggregate `getSettings` have no HTTP mapping.
  `listSystemComponents` is only a capability label for image hints.
  `getConnectedSettings` already reads host defaults; saving them is refused.
- There is no colour column; the web uses a fixed grey.
- `testConnection` checks health and compatibility within 10 s; an Edge standby
  host can open a tunnel.
- There is no agent restart route. The updater identifies its own container via
  cgroup, mountinfo and hostname.
- Endpoint or credential edits revoke agent sessions.
- The installation sheet is development-only, and opening it requests a new
  token, which rotates the host credential and deletes the old key. Its address
  splitting is unsuitable for URLs and IPv6.
- Removal does not stop workloads; host counts may be unknown for unavailable
  hosts.

## Edit, check, components and settings

1. `environments.color` stores a lowercase `#rrggbb` or null (palette by host
   ID); `updateEnvironment` validates it. Names are trimmed, 1–40 characters and
   unique case-insensitively, enforced on the server (`host.name-invalid`,
   `host.name-taken`), at registration too.
2. Check and reconnect call `testConnection` and appear as an activity run with
   typed results: online, standby opened, offline `host.unreachable`, or a typed
   refusal from [host availability](host-availability.md).
3. `listSystemComponents` is defined from existing reads (agent version,
   contract version, tools image, update-required) per host, with per-host
   errors.
4. Settings › Hosts maps the host-defaults section to the existing hub keys
   through `getConnectedSettings` and a narrow save for those keys.

## Restart agent

5. Hub table `agent_restart_requests`: `run_id`, `environment_id`,
   `registration_id`, `credential_revision`, `old_instance`, `state` ∈
   {reserved, sent, awaiting, connected, failed, unknown}, timestamps, nullable
   `acknowledged_at` and terminal `code`. Failed and unknown rows require a code;
   connected rows have none. Acknowledgement requires a committed `sent_at`.
6. **Reservation**: in one hub transaction the restart intake checks that the
   environment has no unsettled runtime receipt and no active restart, then
   inserts the `reserved` row. Every runtime intake (HTTP, policy jobs,
   scheduled children) checks for an active restart in its admission
   transaction and refuses with 409 `host.restarting`. A restart on a busy host
   is 409 `host.busy`.
7. **Agent route** `POST /api/system/agent/restart` (agent mode, Edge allowlist
   `system.agent.restart`) resolves and inspects its own container first;
   unsupported setups get 409 `agent.restart-unsupported` and nothing restarts.
   Otherwise it replies 202 `{instanceId}` and restarts its own container after
   1 s.
8. **Credential revision**: `environments.credential_revision` increases in the
   same transaction as any change of access token, API key or endpoint,
   including a manual token replacement that keeps the key ID. Before sending,
   the hub reads a stable `instanceId` around a fresh compatibility check and
   authenticated system-health HEAD (Direct), or uses the current authenticated
   tunnel session (Edge), and commits it as
   `old_instance` with the current revision; without that evidence the restart
   is refused (`host.unreachable`). The hub sends at most once: `sent` is
   committed before sending, and nothing is replayed after errors or a hub
   restart. A received 202 commits `awaiting` with `acknowledged_at`; a send
   error without a response commits `awaiting` without acknowledgement. The
   response instance is evidence only. A definitive 409
   `agent.restart-unsupported` settles failed with that code.
9. **Completion proof**, fenced by an unchanged `registration_id` and
   `credential_revision` on every observation and again in the settlement
   transaction: on Edge a new tunnel session whose agent instance differs from
   `old_instance` under the same key; on Direct a `/health` `instanceId` that
   differs; then an authenticated compatibility check passes → `connected`. A
   reconnect of the same process never completes the run. Any change of
   registration or credential revision during the run settles it failed with
   `agent.restart-superseded`. Late responses of the old instance are ignored.
10. **Deadline** 3 min: failed `agent.restart-timeout` if the 202 arrived,
    otherwise unknown `agent.restart-unconfirmed`. After a hub restart, startup
    recovery settles an unsent `reserved` row as failed
    `agent.restart-interrupted`; `sent` and `awaiting` resume observation until
    the stored deadline and never resend. The reservation ends with the terminal
    commit. Terminal states and codes are immutable.
11. Environment 0 has no restart action.

## Persistence and check transport constraints

POST `/environments/{id}/agent/restart` returns a durable restart run with 202;
GET `/environments/{id}/agent/restarts/{runId}` exposes its state, deadline and
terminal code. Both routes stay on the hub. Environment 0 refuses restart, and
an incompatible agent is refused before reservation or a restart POST.

The row durably distinguishes received acknowledgement from an unconfirmed
send, independently of activity-history retention. Write failures retain joined
preparation, acknowledgement and observations for retry; the settling transaction
rechecks registration and credential revision. Proof observed before the deadline
survives a later persistence retry, while a later credential edit still supersedes
it. An ambiguous committed `sent` write is observed without sending again. An
unclaimed reservation is recovered as interrupted; intake and recovery share the
worker handoff lock. Controller work belongs to application lifetime and is joined
before the database closes.

Credential revisions also advance when a credential field is resubmitted with
an unchanged value. This conservatively supersedes any observation begun before
the write, without exposing credential contents.

The deliberate check endpoint uses the existing `resource_action` activity type.
Its successful status is `online` or `standby-opened`, with a typed host problem
or agent-access envelope on failure. This uses the existing string status field;
connect/reconnect consumers share the same endpoint.

## Installation files

12. Opening the sheet for an existing host is read-only: it shows Compose, env
    and installer files built from validated host fields (URL parsed with the
    URL API including IPv6, for example `http://[2001:db8::10]:3553`; Edge versus
    Direct) and the hub's host defaults for missing values (install directory,
    UID and GID, editable in the sheet). The token is a placeholder.
13. An explicit "Generate new token" action warns that the running agent
    disconnects until it is reinstalled with the new files, rotates the
    credential once through the existing route and inserts the token into the
    files. Closing the sheet or switching hosts clears it from memory; it never
    enters URLs, logs or persistent browser storage.
14. New hosts keep the existing onboarding flow.

## Removal

15. The removal dialog shows stack and container counts when known and
    "unknown" when the host is unavailable, and states that workloads and the
    agent keep running. Overrides and colour go with the row.

## Related findings

- Version 10 agents (the M13 package) become update-required with contract v11;
  acceptance verifies this on a real version 10 agent.

Codes: `agent.restart-unsupported`, `agent.restart-superseded`,
`agent.restart-timeout`, `agent.restart-unconfirmed`,
`agent.restart-interrupted`, `host.busy`,
`host.restarting`, `host.name-invalid`, `host.name-taken`, `host.unreachable`
and the `/health` `instanceId`.

## Break map

| # | Row | Decision | Entry boundary | Test (red mutation) |
| --- | --- | --- | --- | --- |
| HM1 | colour | CHECK and handler validation | migration and update handler | fresh, upgraded and reopened database; invalid value not persisted (mutation: accept) |
| HM2 | completion proof | new instance plus compatibility, fenced tuple | restart run ← Direct health or Edge session | real Direct receiver and Edge disconnect/reconnect; same instance never completes (mutation: UUID only, or first 200) |
| HM3 | Edge tunnel drop | 202 before restart; completion through a new session | Edge allowlist and run | real Edge path test |
| HM4 | unsupported | resolve self before 202 | agent route | no self ID or Docker unavailable → 409 and zero restart calls (mutation: arbitrary ID) |
| HM5 | busy | 409 `host.busy` | restart intake transaction | held run (mutation: skip the check) |
| HM6 | v10 agent | disabled, zero sends | version gate | FND2 counter |
| HM7 | name rules | server-side, at registration, case-insensitive | update and register | duplicate and concurrent rename (mutation: UI-only) |
| HM8 | components | method defined, per-host errors | adapter and hook | consumer resolves (mutation: remove the method) |
| HM9 | production installation sheet | bundle guard, no demo wording | production build | bundle guard test |
| HM10 | codes | listed above | types, OpenAPI, v11 fixture, EN/DE, web mapper | catalogue and mapper tests |
| HM11 | removal | counts or unknown; overrides removed | dialog and delete | test (mutation: resurrect an override) |
| HM12 | reservation versus runtime intakes | atomic, all intakes including policy jobs | admission transaction | race restart against ordinary and policy runs; one restart call, scope held (mutation: check only) |
| HM13 | fencing | credential revision and registration, checked per observation and at settlement | run observer → settlement transaction | endpoint edit, key rotation, manual token replacement with unchanged key ID and endpoint, delete and re-register, delayed old 200 (mutation: compare registration, key and endpoint only) |
| HM14 | installation sheet credentials | opening mutates nothing; explicit rotation once | sheet lifecycle → token route → file generator | open, close and switch → no rotation; rotate once; URL, IPv6 and Edge files valid (mutation: rotate on mount, or naive split) |
| HM15 | no replay | hub restart, lost 202 | startup recovery | restart call count stays 1 (mutation: resend on resume) |
