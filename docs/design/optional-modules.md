# Optional modules

Proposal for the M27 foundation step (#472), confirmed at the M27 kickoff. The
proxy module ([proxy module](proxy-module.md)) is the first consumer; later
modules reuse the same foundation. Open questions are listed in the last section.

## Model

Maintainer decisions from 2026-10-18:

- Optional modules ship inside the hub image. There is no store, runtime
  download, extra container, label convention or registration API.
- An administrator switches a module on or off hub-wide in Settings.
- A disabled module has no reachable routes, menu entries, jobs, agent
  connections or loaded UI chunk.
- The UI is native Docklet UI with the standard catalogues and components.
- Deactivation keeps module data. A separate confirmed action deletes it.

## Building on the feature registry

`types/features` already defines runtime features with a persisted setting and a
default, and `RequireFeature` gates vulnerability routes on the hub and on agents
with a typed `feature-disabled` refusal. Modules extend this registry instead of
adding a second mechanism:

| Part | Contract |
| --- | --- |
| Definition | A feature ID with `DefaultEnabled: false`, a catalogue label and description, its agent module name and its web route. |
| Hub routes | Registered for OpenAPI generation in every build. A middleware refuses every module route while the module is off. |
| Agent routes | The agent registers the reviewed module routes. The hub vetoes before forwarding; the agent refuses independently when its synchronised setting is off. |
| Jobs | Module jobs start only while the module is on and stop when it is switched off. |
| UI | Server capabilities list enabled modules. Navigation, settings entries and the lazy route chunk load only for enabled modules. |
| Data | Module tables belong to the module's migration. Deletion removes only module rows and never touches files on hosts. |

## Host compatibility

A module needs a minimum agent contract version. A host below that version shows
the module as unavailable for that host instead of failing the whole module. The
agent contract version rises once for the module routes; its number follows the
version on main after M26.

## Lifecycle rules

- Switching a module off while one of its operations is running is refused with
  the running operation named. The operator waits or cancels first.
- An agent restart or a reconnect during a module operation keeps the module's
  own recovery contract (for the proxy module, its write journal).
- Switching a module on again uses the kept data unchanged.

## Break map

| Row | Required test |
| --- | --- |
| Toggle off | Hub and agent routes refuse, navigation and the chunk disappear, jobs stop. Red proof: remove the hub veto; the forwarded call must be refused by the agent and the test must notice the missing hub refusal. |
| Toggle during an operation | Switching off with a running module operation is refused. Red proof: drop the check; the test observes a write after the module reports off. |
| Settings synchronisation | An agent that missed the off switch still refuses after reconnect. Red proof: skip the resync; the agent accepts a call. |
| Old agent | A host below the minimum version shows the module as unavailable and receives no module call, Direct and Edge. |
| Data deletion | Deletion removes module rows only. Red proof: delete with the module on; the action must be refused. |

## Kickoff questions

| Question | Options | Recommendation |
| --- | --- | --- |
| Refusal for a disabled module route | 404 (route appears absent, as decided in 2026-10) or the existing typed 403 `feature-disabled` | 404 for browser and API callers, typed refusal only between hub and agent |
| Who may see and use a module | All signed-in users see, a per-module right acts (decision of 2026-10) or administrators only | Administrators only in the first version, matching the [simple users](simple-users.md) allowlist; a module right can be added later |
| Default on a fresh installation | Off or on | Off |
