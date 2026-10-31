# Truthful host availability

Host status reflects whether an agent actually answers: a frozen Edge agent
becomes offline, a standby Edge host stays usable, and a host that refuses a
request for a configuration reason is not reported as a bad credential. Shared
contract items are in [container and host operations](container-host-operations.md).

## Baseline

- The Edge projection reports online for a retained tunnel with a current key,
  standby for a recent poll and offline otherwise. Every heartbeat writes
  `LastHeartbeat` on both transports, but nothing checks its age. Heartbeats run
  every 30 s; the WebSocket read wait is 90 s; gRPC `Recv` has no deadline. The
  stale sweeper reaps after 2 min with a 1 min sweep, so a frozen agent stays
  online for 120–180 s. gRPC keepalive options are inert under the shared
  ServeHTTP listener.
- Host SSE reprojects runtime state every 5 s.
- The web maps standby to offline, and the Auto Heal row exists only for online
  hosts.
- A disabled UI configuration on an agent answers Auto Heal writes with a
  framework 403 text. Proxied 401 and 403 responses collapse into 502
  `agent.invalid-token` in the environment middleware, the Edge proxy and the
  agent verdict for hub-owned routes.

## Design

1. **Liveness on the application heartbeat**, without transport keepalive
   changes:
   - `live(session) = now - LastHeartbeat ≤ 90 s` (three intervals); a new
     session counts as live from registration;
   - the projection, runtime dispatch admission and the M13 availability
     observer use the predicate;
   - the stale sweeper uses the same window with a 15 s sweep (worst case
     105 s) and removes a tunnel only if it is still the current session for
     its environment, so a replacement is never reaped. The status becomes
     offline; the reason `edge.heartbeat-stale` is recorded on the closed
     session and in the log only, not in the host projection.
2. **Standby**: the web keeps `standby` (neutral colour, "Standby – connects on
   demand") and treats it as available for actions that can open a tunnel. The
   Auto Heal row and other per-host settings read and save for standby hosts;
   offline, disabled and refused hosts stay unavailable. All online-only web
   consumers are audited.
3. **Typed refusals**:
   - the agent's UI-configuration guard (Auto Heal writes and generic settings
     writes) returns problem `settings.ui-configuration-disabled` (403);
   - one bounded, body-aware classifier reads at most 16 KiB and is used by the
     environment middleware, the Edge proxy and the agent verdict;
   - the public envelope keeps the existing host-access shape with status 502,
     so the browser never treats a host refusal as a hub sign-out:

     | Agent status and code | Public code (502) |
     | --- | --- |
     | 403 `settings.ui-configuration-disabled` | `agent.ui-configuration-disabled` |
     | 401 or 403 `agent.update-required` | `agent.update-required` |
     | 401 or 403 with any other, missing or malformed code | `agent.invalid-token` |

   - the web maps `agent.ui-configuration-disabled` to "Settings on this host
     are managed through its environment".

4. **Bounded inventory reads**: an ordinary inventory read (lists and point
   reads that keep snapshots) gives up after 20 s, including agent access, and
   answers from its snapshot or with `agent.unavailable`. A frozen Direct agent
   accepts connections but never answers. Without this bound, its reads held all
   six HTTP/1.1 browser connections, and hub-local reads such as host
   descriptors waited until the web's 40 s deadline, so Check stayed disabled.
   Other proxied operations, such as pulls, uploads, streams and lifecycle
   commands, keep their own budgets.

The 90 s window follows the existing WebSocket read wait.

## Break map

| # | Row | Decision | Entry boundary | Test (red mutation) |
| --- | --- | --- | --- | --- |
| AV1 | stale heartbeat | offline after 90 s, dispatch refused | projection and admission | fake clock (mutation: ignore the heartbeat) |
| AV2 | frozen peer on the shipped listener | the sweeper reaps the current stale session | real ServeHTTP listener, WebSocket and gRPC | peer stops heartbeating while the transport stays open → offline within 105 s (mutation: gRPC-only server setup, or no sweep) |
| AV3 | standby | own state, actionable | web mapper and consumers | Auto Heal row for standby; offline stays unavailable (mutation: map to offline) |
| AV4 | typed refusal on Direct | classifier in the environment middleware | real agent producer → hub proxy → web mapper | UI-disabled Auto Heal write and generic settings save (mutation: collapse) |
| AV5 | typed refusal on Edge | classifier in the Edge proxy | real tunnel | same two producers (mutation: collapse) |
| AV6 | unknown or malformed | `agent.invalid-token`, browser identity kept | classifier | arbitrary code, bad JSON and a real bad token → invalid token; the browser session is retained (mutation: pass through) |
| AV7 | observer | a stale tunnel counts as down | observer → outbox | injected clock: one offline notification after 5 min (mutation: stale online cache) |
| AV8 | codes | `edge.heartbeat-stale`, `agent.ui-configuration-disabled`, `settings.ui-configuration-disabled`, standby label | types, OpenAPI, v11 fixture, EN/DE | catalogue and mapper tests |
| AV9 | healthy and replaced sessions | not reaped | sweeper | a healthy peer beyond 90 s stays; a replacement during the scan survives (mutation: close by environment ID) |
| AV11 | frozen Direct agent | inventory reads end within 20 s | environment middleware inventory forward | a never-answering agent → `agent.unavailable` or the snapshot within the bound; another proxied read keeps its budget (mutation: no bound) |
| AV10 | producers emit codes | exact problem at the agent | agent guard | a producer with framework text only fails (mutation) |

## Web availability consumers

The shared availability predicate admits online and standby hosts for deliberate
container, stack, image, volume, network, backup and per-host settings operations.
Producer capability refusals still disable their controls. Standby keeps its own
neutral status in host cards, the host filter and topology labels; it is excluded
from disconnected-host attention counts.

Metrics subscriptions and load sorting remain online-only because they require a
live sample stream. Connection completion still requires online; standby is not
proof of an authenticated connection. A disabled host projects offline. Typed
agent configuration refusals are translated without changing browser identity;
`edge.heartbeat-stale` remains internal and has no web reason mapping.
