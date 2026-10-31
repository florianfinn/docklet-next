# Stack stop mode, stopped stacks and single service restart

Stop follows a hub setting, a stack that is down stays visible with its
services, and a single Compose service can be restarted. Shared contract and
migration items are in [container and host operations](container-host-operations.md).

## Decisions

- Stop uses `compose down` by default; a hub-wide setting switches it to
  `compose stop`. The stack menu offers the other action explicitly.
- A stack that is down stays listed as stopped, with its services.
- A single service can be restarted from the services card and the attention
  widget.
- Untracked Compose projects (no project folder) disappear after `down`. Docklet
  stacks always live in the project directory, so no warning is built; this is
  a documented limit.

## Baseline

- The web Stop runs runtime `down-project` → `DownProject` →
  `ComposeDown(RemoveOrphans)`; `down-project` accepts no options.
  `ComposeStop(proj, nil)` is a no-op for an empty service list.
- Tracked stacks persist, but their services come only from live containers.
  The resolved-model cache (`backend/pkg/projects/cache.go`) already tracks
  includes, overrides, `.env` and env files.
- Settings are per environment and proxied to agents. `restart-project` accepts
  `restartServices`, but the web never sends it.

## Design

1. New runtime operation `stop-project` without options. The executor resolves
   the project's service names from the resolved model and calls
   `ComposeStop(proj, services)` with the explicit full list. Status `stopped`
   is written only after success; a failure keeps the previously observed
   state. `down-project` is unchanged.
2. Hub setting `stackStopMode` ∈ {down, stop}, default `down`, stored in the
   hub settings (environment 0) and typed in OpenAPI. The web reads it from the
   hub regardless of the host filter. Stop dispatches `down-project` or
   `stop-project`; the menu offers "Shut down (remove containers)" and "Stop
   containers". No setting travels to agents.
3. Start stays `deploy-project` (`up`).
4. When a tracked stack has no containers, the listing fills `services` from the
   resolved model through the dependency-aware cache, with at most four
   concurrent resolutions per listing page. Such services have state
   `not-created`. A resolution failure yields the stack row with hint
   `definition-invalid` and no services; the list request still succeeds. Rows
   with containers keep the live services projection; definition services
   never replace it. Their existing Compose-backed metadata and update
   enrichment is unchanged. The web
   shows `not-created` as stopped ("containers removed") and offers no
   container links or logs for those services.
5. A single service restart maps `restartService(stackId, svc)` to
   `restart-project` with `restartServices: [svc]`. The run gate and pending
   keys include the service; admission stays stack-scoped.
6. Version 10 agents are update-required; actions are disabled with the typed
   reason.

## Web

The general settings page stores the hub-wide stop mode. Both a filtered view
and the Stop command read environment 0. Missing values from older settings
responses preserve the down default; unknown values fail the read. The stack
page and row menu label the alternate action explicitly. Version-incompatible
hosts disable actions with the update-required explanation.

Service restart controls use a stack-scoped capability from the services card,
attention queue and Compose-child hint. The public restart route takes the
selected service through its existing `services` query parameter, which the
hub maps to `restartServices: [service]`; this route accepts no request body.
Both pending and run-gate keys include the selected service, while admission
continues to exclude parallel stack actions.

Definition services with `not-created` state render as stopped with the
containers-removed explanation and no container links or log controls. An
invalid-definition hint leaves the stack visible in the list and explains the
repair on its detail page.

Codes and states: `stop-project`, `not-created`, `definition-invalid`.

## Break map

| # | Row | Decision | Entry boundary | Test (red mutation) |
| --- | --- | --- | --- | --- |
| ST1 | `stop-project` | no options, own digest, admitted by the receipt CHECK | validation and receipt store | options refused; digest differs from down; receipt insert after 014 (mutation: alias to down, or the old CHECK) |
| ST2 | explicit service list | full list from the resolved model | executor → Compose | fake Compose records every service (mutation: pass nil) |
| ST3 | status write order | after success; failure keeps the observation | stop and down mutation | blocked or failing Compose effect (mutation: write before) |
| ST4 | Direct/Edge parity | remote runtime path | real sender and receiver | Direct and Edge stop-project; down unchanged (mutation: route stop to down) |
| ST5 | v10 agent | zero sends | version gate | FND2 counter |
| ST6 | definition services | dependency-aware cache, `not-created` | listing on hub-local and agents | services listed after down on all transports (mutation: live-only) |
| ST7 | setting | enum, hub scope | settings validation and web read | invalid value refused; an Edge-filtered view still reads environment 0 (mutation: read the selected host) |
| ST8 | service restart | `[svc]`, key includes the service | adapter and admission | other services untouched; a second service restart is blocked (mutation: omit services) |
| ST9 | codes and states | listed above | types, OpenAPI, v11 fixture, EN/DE | catalogue tests; the web maps `not-created` to stopped |
| ST10 | dependency changes | include, env and override edits and repaired YAML are reflected | resolved-model cache | change only an included file, an env input, a new override; invalid → repaired (mutation: main-file-only cache) |
