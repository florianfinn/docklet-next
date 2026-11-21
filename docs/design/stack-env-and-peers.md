# Independent stacks and recorded env files

Decisions for #547 (unreadable stacks no longer interrupt other stacks) and #548
(load a stack with the env file Compose recorded). Both change how a project's
Compose model is loaded and how runtime admission treats other projects. This
note replaces the peer-confirmation rule in
[runtime control](runtime-control.md#admission-identities-and-scopes).

## Decision

A stack's readability is that stack's own concern. Docker Compose does not
refuse duplicate project names and the Arcane baseline has no peer check, so
Docklet keeps only the checks that protect the action's own target.

- **Unreadable peers.** A project whose Compose model cannot be loaded no
  longer adds uncertainty to an action on any other project. Its only
  contribution is name evidence for the duplicate warning: a literal top-level
  `name:` read from the raw YAML (`ComposeContentProjectName`), never a stored
  or observed name.
- **Proven duplicates (maintainer decision D-1).** Two projects whose effective
  Compose names are equal produce a warning on both stacks. An action addressed
  by project ID runs without refusal or confirmation.
- **Name-addressed lookups.** A lookup that has only a Compose name, such as the
  parent of a container taken from its project label, cannot pick an owner when
  two projects share that name. It keeps the existing `compose.duplicate`
  refusal. This affects only containers that carry the duplicated name (AUTO-1:
  narrower than a refusal for the whole host; reversible by choosing the
  oldest row instead).
- **The target itself.** A target whose own model cannot be read stays
  non-overridable. Deploy, redeploy and update need the model. Container
  actions on its containers keep `runtime.parent-unresolved` (AUTO-2: no new
  name-only parent rule; #548 makes the common case readable).
- **Recorded env files (D-2).** When a project's containers carry
  `com.docker.compose.project.environment_file`, Docklet loads the project with
  those files instead of the default project `.env`. This matches the
  replacement rule of Compose `--env-file`. The paths must stay inside the
  projects directory, which hub and agent mount at the same path.
- **Deploy keeps the label.** A Docklet deploy of a project loaded with recorded
  env files writes the same `environment_file` label. A later read therefore
  finds the same inputs. Without this, a single Docklet redeploy would drop the
  label and make the stack unreadable again.
- **Named problems.** Every stack whose model fails shows its reason in the
  stack list and detail page, running stacks included. Possible reasons are a
  missing required variable (with the variable name), a missing env file (with
  a project-relative or projects-directory-relative path), an invalid recorded
  env file, conflicting recorded env files and an unsupported selection.

Rejected alternatives:
- Keeping the confirmation and remembering it: the maintainer rejected any
  interruption caused by another stack.
- A per-stack env-file setting: rejected by D-2; the label covers stacks that
  were started outside Docklet.
- Reading any absolute path from the label: the label is untrusted container
  metadata, so reads stay inside the projects directory.

## Contract

- **Peer wire unchanged (shape A, AUTO-3).** For the peer refusal, agent
  contract 20 and its fingerprint stay as released in 1.2.0 (see the
  fingerprint check below for the new display fields). New producers never emit the confirmable
  `compose.name-undeterminable` blocker or an uncertainty digest. The schema
  members stay until the next planned contract bump, when they are removed with
  a v20 compatibility fingerprint (v19 precedent). The minimum agent version
  stays 18.
- **Web.** The confirmation dialog stays for agents up to 1.2.0, which still
  produce the confirmable blocker on their own hosts. A new hub against an old
  agent therefore behaves as in 1.2.0. Nothing new is shown.
- **Problem reasons.** `Details.definitionReason` gains
  `compose.variable-missing`, `compose.env-label-invalid` and
  `compose.env-label-conflict`. `definitionVariable` (the variable name) is a
  new optional display field next to the existing `definitionFile`. The hint is
  filled for every stack whose model fails, not only for stacks without runtime
  services. The list producer uses the cached load to bound the cost.
- **Duplicate warning.** `Details` gains optional `composeNamePeers`: up to 16
  display names of other projects with the same effective Compose name. There
  is no such field today.
- These changes go into `types/project`, OpenAPI, the generated DTOs, the
  wire-code catalogue and both locales. Clients render an unknown reason as the
  generic hint.
- **Fingerprint check.** If the project `Details` schema is part of the agent
  fingerprint, the new enum values and fields need the contract bump to 21.
  That bump keeps a v20 compatibility fingerprint, following the v19 precedent,
  so 1.2.0 agents stay compatible. The design review verifies which applies.
- **No migration.** Receipts from 1.2.0 with a stored digest or override
  snapshot stay readable; the columns of migration 006 are not touched.

## State and lifecycle

- The recorded env files are read on every identity and lifecycle load,
  following the existing fresh-read rule for environment bytes. Inventory caches
  key on the resolved file list and content hash.
- Source of the label: the project's own containers, using the existing
  ownership mapping (`projectContainers`). If two containers disagree on the
  list, the result is the named problem "conflicting recorded env files" and
  nothing falls back silently.
- Path rule: each comma-separated entry must be absolute and clean, and its real
  path (after resolving symlinks) must lie inside the projects directory. It
  must be a regular file of at most 1 MiB. Any violation gives the named
  problem "invalid recorded env file", and nothing outside is read.
- After `down` the containers and their labels are gone. The stack then falls
  back to its own files and may show "required variable missing" again. This is
  a documented limit; remembering the last observed path is a possible
  follow-up (AUTO-4).
- Admission and the pre-effect recheck use the same resolver, so both apply the
  same rule. A duplicate that appears between admission and effect no longer
  settles a project-ID action with zero effects. A changed target still does.
- Values from env files never appear in API responses or logs. Load errors are
  classified into reason, variable name and safe file before they are logged.
  The raw interpolation error, including the `${VAR:?message}` text, is not
  logged or returned.

## Break map

| # | Row kind | Case | Decision | Entry boundary | Test | Red mutation |
|---|---|---|---|---|---|---|
| 1 | persisted state | 1.2.0 receipt with digest and override snapshot reopened | readable, retained retrieval and projection unchanged | `runtime_peer_store.go`, migration 006 | extend `runtime_peer_migration_test.go` | drop digest tolerance in the reader |
| 2 | behaviour | healthy A, peer B with `${VAR:?}` and literal `name: edge` | start/stop/restart/deploy/redeploy/update of A run without acknowledgement | `runtime_identity.go` resolver | resolver + admission test | restore the uncertainty branch |
| 3 | behaviour | A and B with equal readable or literal names | project-ID action runs; both stacks carry the duplicate warning | resolver + project list producer | resolver + list DTO test | re-add the duplicate refusal |
| 4 | behaviour | container whose label names a duplicated project | `compose.duplicate` refusal, others unaffected | parent lookup in resolver | resolver test | resolve first owner |
| 5 | identity change | peer turns duplicate between admission and effect | ID action proceeds; changed target still zero-effect | `runtime_executor.go` recheck | rewrite `TestS4AcknowledgedPreEffectRechecks…` | refuse on new duplicate |
| 6 | external data | label paths: relative, `..`, symlink out of root, directory, FIFO, > 1 MiB, empty entry, non-UTF-8 | named problem, file not read | env-file resolver | table test with temp dirs | remove the root check |
| 7 | external data | containers disagree on the label | "conflicting recorded env files", no fallback | env-file resolver | unit test | pick the first container |
| 8 | external data | raw `name:` with `${}`, anchors, multi-document, invalid YAML | no name evidence, no panic | `ComposeContentProjectName` | unit test | accept interpolated names |
| 9 | new codes | new definition reasons in types, OpenAPI, DTO, catalogue, en/de | rendered; unknown reason gives generic hint | `types/project`, web stack hint | wire catalogue, OpenAPI drift, web test | omit a catalogue key |
| 10 | transport parity | unreadable peer and recorded env file on Direct and Edge agents | same result as hub-local | sender/receiver | parity test over both transports | agent path keeps old resolver |
| 11 | mixed versions | new hub, agent 1.2.0 with unreadable peer | confirmable refusal and dialog as in 1.2.0 | `runtime-commands.ts` | existing dialog test stays green | delete the dialog branch |
| 12 | leak | sentinel secret in env file and in `${VAR:?hint}` | never in logs or API bodies | discovery/service/workspace load errors | log capture + body assertions | log the raw error |
| 13 | deploy | Docklet redeploy of a stack loaded with recorded env files | containers keep the `environment_file` label; next read is readable | `applyCustomLabelsInternal` | loader test + sandbox cell | omit the label |
| 14 | migration | none | migration manifest unchanged | — | existing manifest guard | — |

## Acceptance cells

Hub-local (H) plus Direct (D) and Edge (E) where the behaviour crosses the
agent path:

- **P1 (H, D, E).** A stack with a missing required variable exists. Start,
  stop, restart and redeploy of another stack run without a dialog.
- **P2 (H).** Two stacks share a literal `name:`. Both show the warning, and a
  redeploy of one runs.
- **P3 (H).** The unreadable stack shows the reason with the variable name. A
  sentinel value appears neither in the API nor in the hub log.
- **P4 (H, D).** A stack started with `docker compose --env-file` (file inside
  the projects directory) is readable, its update summary works, and after a
  Docklet redeploy it stays readable with the label present.
- **P5 (H).** A label pointing outside the projects directory gives the named
  problem, and the file is not read.
