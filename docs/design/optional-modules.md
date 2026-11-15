# Optional modules

Design for the M27 foundation step (#472). The proxy module
([proxy module](proxy-module.md)) is the first consumer; later modules reuse the
same foundation. The decisions of the M27 kickoff (2026-11-15) are listed in the
last section.

## Model

Maintainer decisions from 2026-10-18:

- Optional modules ship inside the hub image. There is no store, runtime
  download, extra container, label convention or registration API.
- An administrator switches a module on or off hub-wide in Settings.
- A disabled module starts no module work: no reachable routes, menu entries,
  jobs, module-specific connections or chunk request. A chunk imported while the
  module was on may stay in the browser cache; its rendered surfaces are removed
  and late results are ignored. General host connectivity and the settlement of
  already admitted module operations continue.
- The UI is native Docklet UI with the standard catalogues and components.
- Deactivation keeps module data. A separate confirmed action deletes it.

## Building on the feature registry

`types/features` defines runtime features (`Definition{ID, SettingKey,
DefaultEnabled}`); vulnerability management is the only entry today. Its gate is
module-local (`internal/vulnerability/module.go`), the environment proxy checks
vulnerability paths by name, and the flag is not synchronised to agents. Modules
extend the registry with a generic gate instead of copying that pattern:

| Part | Contract |
| --- | --- |
| Definition | `features.Definition` gains `Kind` (`feature` or `module`), the route prefixes the module owns (`/api/modules/<id>/…` on the hub, `/api/environments/{id}/modules/<id>/…` for host operations) and the agent module name in `routeModules`. Modules default off. |
| Settings | One setting per module (`module<Id>Enabled`), public, internal to the hub: no environment override on hub or agent, not editable through per-host settings. The key joins `AgentSyncedSettingKeys`, so the existing push on change and on agent reconnect carries it; the agent ignores local environment values for it. |
| Access order | Authentication, then the existing access decision (module routes are administrator routes, so a simple user gets the existing 403), then the module gate: an authorised caller gets 404 while the module is off. This holds for hub-local routes, the hub-local environment shortcut and forwarded environment requests. |
| Hub routes | Always registered, so OpenAPI generation sees them. A Huma route middleware on every module operation and the environment middleware for forwarded module paths apply the gate. |
| Agent routes | Registered statically by the agent's mode, like every other agent module. Admission is dynamic: the agent refuses a module route with the typed `feature-disabled` problem while its synchronised setting is off. The Edge tunnel advertises the module commands statically; the hub's gate decides. #472's "registered only while enabled" is met as "admitted only while enabled" (kickoff question). |
| Admission fence | Module admission and the module setting share one hub-side gate: a module operation takes a reservation under the gate only while the module is on; switching off or deleting data takes the gate, is refused with `module-busy` naming the operation while a reservation exists, and closes admission atomically. Reservations are released only by a final operation state. |
| Agent authority | The hub gate is authoritative; agent routes are reachable only with the agent token. The agent's own refusal is defence in depth: a stale "on" after a missed switch-off can only be used by the hub, which refuses first. |
| Jobs | A module job implements `ConditionalJob` and is rescheduled when the module setting changes, as vulnerability jobs are today. |
| Setup | The first-run setup wizard has a Modules step that lists every module with its switch, all off by default; the review step shows the choice and saving applies it with the other setup settings. |
| UI | Settings → Modules is always available to administrators and lists every module with its toggle. A module's own settings, navigation entries and lazy route chunk exist only while it is on; a disabled module's route renders the standard not-found page without requesting its chunk. The web reads module state from the public settings it already loads and refetches after a toggle. |
| Data | Module tables belong to the module's migration. Deletion is allowed only while the module is off and no operation is unfinished; it removes hub rows and asks each reachable agent to remove its module journal; unreachable agents are listed as pending cleanup. Host files are never touched. |

## Host compatibility

The module routes raise the agent contract once, to version 19. Later M27 steps
update the v19 fixture without another increase, because no package with v19 is
released before the M27 merge. The minimum agent version stays 18: a host below
19, with a different contract fingerprint, or an Edge agent that does not
advertise the module commands shows the module as unavailable for that host
(`agent.update-required`) and receives no module call. The host stays usable for
everything else.

## Lifecycle rules

- Switching a module off while one of its operations is unfinished is refused
  with the operation named. The operator waits for its final state first.
- An agent restart or a reconnect during a module operation keeps the module's
  own recovery contract (for the proxy module, its operation journal).
- Switching a module on again uses the kept data unchanged.

## Break map

Each row names the test seam, the assertion and the red mutation; a red proof is
a failing assertion, never a compile error or timeout.

| # | Row | Test seam and assertion | Red mutation |
| --- | --- | --- | --- |
| M1 | Off, hub | Real router over every registered module operation, admin and simple user: admin 404, simple user 403, no handler or Docker call. | Remove the gate: an instrumented handler runs. |
| M2 | Off, forwarded | Real Direct proxy and Edge command path: hub 404 and zero forwarded requests; agent ingress with its setting off returns `feature-disabled`. | Remove the hub gate: the forwarded call reaches the agent (agent refusal observed instead of 404). Separately remove the agent gate: the agent handler runs. |
| M3 | Missed switch-off | Agent persisted "on", disconnected, hub switches off, reconnect: agent refuses after the resync. | Drop the key from `AgentSyncedSettingKeys`: the agent still accepts. |
| M4 | Toggle during work | Latched reservation versus `SettingsService` update: switch-off refused with `module-busy`; after release it succeeds; no reservation is taken after off. | Check the reservation outside the gate: the switch succeeds while work is reserved. |
| M5 | Old agent | Capability and dispatch for v18, wrong fingerprint and an Edge agent without the commands, Direct and Edge: unavailable, zero module sends, other host operations work. | Drop one veto: a send is attempted. |
| M6 | Data deletion | Store and API: refused while on or with unfinished work; off deletes module rows only, keeps host files and unrelated rows, lists unreachable agents. | Allow deletion while on: rows disappear under a running module. |
| M7 | New codes | `module-busy`, `operation-conflict`, the module 404 and the agent `feature-disabled` through the real decoder of both transports to the rendered reason in EN and DE. | Remove a code from the web decoder: the rendered reason assertion fails. |
| M8 | UI chunk | Router with module off: zero chunk imports on direct URL and navigation. | Import unconditionally: the counter is non-zero. |
| M9 | Environment override | Agent and hub with the module variable in the environment: the effective value follows the hub setting. | Allow the override: the agent value differs from the hub. |
| M10 | Late UI results | Toggle off or switch user during a held module read; release: no module surface revives and nothing is written to the new cache key. | Drop the identity check: the old result renders. |
| M11 | Fresh default | Fresh database bootstrap and an upgraded database without the key: both report the module off and register no module job. | Default the definition to on: both assertions fail. |
| M12 | Setup step | Real setup flow with the Modules step: switched on, the saved setup enables the module; left off or skipped, the module stays off; the draft keeps the choice across steps. | Drop the module field from the setup save: the module stays off after choosing on. |

## Kickoff decisions (2026-11-15)

| Topic | Decision |
| --- | --- |
| Refusal for a disabled module route | 404 for authorised callers after the normal access check; simple users get the existing 403; the typed `feature-disabled` only between hub and agent |
| Who may see and use a module | Administrators only |
| Default on a fresh installation | Off; the first-run setup offers a Modules step to switch modules on |
| Agent registration | Admitted only while on (static routes, dynamic refusal); #472 is amended |
| Deleting data with unreachable agents | Delete hub rows and list the agents as pending cleanup |
