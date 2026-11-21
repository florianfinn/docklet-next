# Independent stacks and recorded env files

Decisions for #547 (unreadable stacks no longer interrupt other stacks) and
#548 (load a stack with the env file Compose recorded). Both change how a
project's Compose model is loaded and how runtime admission treats other
projects. This note replaces the peer-confirmation rule in
[runtime control](runtime-control.md#admission-identities-and-scopes).

## Decision

A stack's readability is that stack's own concern. Docker Compose does not
refuse duplicate project names and the Arcane baseline has no peer check, so
Docklet keeps only the checks that protect the action's own target.

### Peers (#547)

- **Unreadable peers.** A project whose Compose model cannot be loaded no longer
  adds uncertainty to an action on any other project. It contributes only to the
  duplicate warning, and only with *peer name evidence*. Such evidence exists
  only when the selection consists of exactly one Compose file, the file has a
  single YAML document, and its top-level `name:` is a plain scalar without `$`.
  A dedicated reader provides it. `ComposeContentProjectName`, which create and
  rename use, is not changed. Stored or observed names are never evidence.
- **Duplicates (maintainer decision D-1).**
  - When effective names are equal (readable model, or peer name evidence), both
    stacks show a warning.
  - An action addressed by project ID runs with neither a refusal nor a
    confirmation.
  - Evidence used for the warning never selects or authorizes a target. The
    current guards in `runtime_identity.go` stay in place: a recovered name
    never authorizes a row as target, and the target must match exactly.
- **Name-addressed lookups (AUTO-1).** A lookup that has only a Compose name
  keeps `compose.duplicate` when two projects own that name. The typical case is
  the parent of a container taken from its project label. Scope locking needs an
  unambiguous parent. The refusal only affects containers carrying that name.
- **The target itself.**
  - A target whose own model cannot be read stays non-overridable.
  - Container actions on its containers keep `runtime.parent-unresolved`
    (AUTO-2).
- **Pre-effect recheck.** `ExecuteRuntime` (`activity/runtime_executor.go`) calls
  the same resolver.
  - A newly appearing peer duplicate does not stop a project-ID action.
  - A changed target, parent or name still settles with zero effects.
  - Image operations keep their own effect-time identity path.

### Recorded env files (#548, D-2)

- **Attribution.** The recorded selection is taken from the label
  `com.docker.compose.project.environment_file`. Only the target row's own
  containers count, and they are proven by fresh container labels that are
  independent of display ownership:
  - `com.docker.compose.project.working_dir` equals the row's directory
    (cleaned, real path);
  - `com.docker.compose.project.config_files` equals the row's selected Compose
    file paths;
  - `com.docker.compose.oneoff` is not `True`.

  The Compose project name is not used for attribution, because it may itself
  come from the recorded file.

  Treatment of edge cases:
  - Containers that miss these labels, or match another directory, are ignored.
    The display tie-break of `projectContainers` is never used for inputs.
  - Attributed containers with different lists give the named problem
    "conflicting recorded env files" for that target only.
  - No attributed container, a missing label or an empty label means no recorded
    selection, which is the current behaviour.
  - A label that lists exactly the row's own `.env` means the same as no label.
  - An unattributable label never makes another target fail.
  - One bounded container listing per request serves all attribution in that
    request.
- **Safe read.** Every recorded file is read through an `os.Root` opened on the
  projects directory, after resolving the root itself once:
  - The entry must be an absolute, clean path below the root.
  - The file is opened relative to the root with `O_NONBLOCK`. Symlinks resolve
    only inside the root.
  - On the opened descriptor the file must be regular.
  - At most 1 MiB is read, and one extra byte detects overflow.
  - Parsing and hashing use exactly those bytes, and nothing is reopened by path.

  Any violation gives the named problem "invalid recorded env file". This
  policy applies only to recorded selections. The existing conventional `.env`
  and `COMPOSE_ENV_FILES` symlink contract stays unchanged when there is no
  recorded selection.
- **Replacement semantics.** The recorded selection is resolved *before* file
  selection, into one resolved-input value: ordered paths, the bytes read, the
  merged map and a dependency hash.
  - With a recorded selection, the files replace the project's `.env` and its
    `COMPOSE_ENV_FILES` overlays, as Compose `--env-file` does.
  - Later files win.
  - `COMPOSE_FILE`, `COMPOSE_PROFILES` and `COMPOSE_PROJECT_NAME` inside them
    act as they act in `.env` today.
  - `COMPOSE_ENV_FILES` inside a recorded file is ignored, so it never causes a
    second read.
  - AutoInject uses the same merged map in place of `.env`.
- **One path for all loads.** Executable loads, metadata discovery, strict
  identity, cached inventory, include discovery, validation and preview take
  the resolved-input value from the same function. No path reads `.env` first.
  - The inventory cache key contains the selection state (none, recorded,
    conflict, invalid), the ordered paths and the content hash of the bytes
    read.
  - A cache hit never skips resolving the current label and bytes.
- **Deploy keeps the label.** A Docklet deploy of a project with a recorded
  selection writes `environment_file` with the same ordered path list. This
  applies to every created or recreated service, through
  `applyCustomLabelsInternal`. Without the label, a single redeploy would make
  the stack unreadable again. Values never enter labels.
- **Lifecycle.** Recorded files are immutable external dependencies. A snapshot
  records each path with its SHA-256 in a new optional `External` list (no
  content). They are never part of the writable file set, so update, restore,
  recovery and rollback never write them.
  - A baseline counts as available only while the current external hashes
    equal the recorded ones. Otherwise it reports `update.config-drift`, and
    deploying once captures a new baseline.
  - Restoring a point whose external hashes differ from the current files is
    refused with the new code `update.external-input-changed`. Docklet cannot
    reconstruct those files.
  - The project-root rule for writable files stays as it is.
  - Old snapshots without `External` decode as an empty list and so differ from
    a recorded selection. Deploying once captures the new baseline. No
    migration is needed because the payload is versionless encrypted JSON.
- **After `down`.** The containers and their labels are gone. The stack falls
  back to its own files and may show "required variable missing" again. This is
  a documented limit (AUTO-4).

### Problems and values

- **Named problems.** Every stack whose model fails shows its reason in the
  stack list and on its detail page, running stacks included. The possible
  reasons are:
  - a missing required variable, with the variable name;
  - a missing env file, with a project-relative or projects-root-relative path;
  - an invalid recorded env file;
  - conflicting recorded env files;
  - an unsupported selection.
- **Typed load problem.** Loaders return a typed problem: reason, variable name
  and safe file. It wraps the raw error only for `errors.Is`/`As` and has no
  printable raw text. Every log or response sink named below records only the
  typed problem. Raw interpolation and dotenv messages, including the
  `${VAR:?message}` text, never appear in logs, responses, activity output or
  agent-proxied bodies:
  - `service_discovery.go:147,466,669,688`
  - `service.go:527,530`
  - `service_mutations.go:259`
  - `service_workspace.go:568`
  - `pkg/projects/load.go:331` and the panic recovery at `:590`
  - `env.go:196,497,503,508`
- **Masked environment (AUTO-13).** For a project with a recorded selection,
  the project-level service projections keep the environment keys but return
  no values: `services[].environment` and
  `runtimeServices[].serviceConfig.environment`. The executable model is not
  changed. The Docker container inspect view is the daemon's own data and is
  outside this rule.

Rejected alternatives:
- Keeping the confirmation and remembering it: the maintainer rejected any
  interruption caused by another stack.
- A per-stack env-file setting: rejected by D-2.
- Attribution by Compose name: two same-name stacks could exchange env files.
- Path checks followed by a plain read: open to swaps between check and read.

## Contract

- **Peer wire unchanged (shape A, AUTO-3).** Agent contract 20, minimum agent 18
  and the released fingerprint all stay. New producers never emit the
  confirmable blocker or an uncertainty digest. The schemas, the
  `invalid-acknowledgement` validation, canonical digest encoding and receipt
  reconstruction stay intact.
  - A retry with the same key and the original digest returns the same run.
  - A retry without the digest remains a mismatch.
  - The web keeps the confirmation dialog for agents with contract 20 (1.2.0).
    Contract 18 and 19 agents keep the legacy update notice.
  - Agents older than M34 still produce the refusal on their own hosts and get
    none of the new problem texts or warnings. The update notice covers this
    limit.
- **Details fields (no bump).** `scripts/agent-contract.mjs` fingerprints only
  `ProjectDetails.properties.definitionHint`. The design review checked that
  mutating `definitionReason` or adding `definitionVariable` or
  `composeNamePeers` leaves the agent fixture byte-identical. This adds:
  - `Details.definitionReason`: `compose.variable-missing`,
    `compose.env-label-invalid`, `compose.env-label-conflict`;
  - `definitionVariable`: a variable name, at most 128 characters;
  - `composeNamePeers`: up to 16 peer display names.

  The new values go into `types/project`, OpenAPI, the generated DTOs, the
  wire-code catalogue and both locales. A focused drift test keeps them outside
  the fingerprint. Clients render an unknown reason as the generic hint.
- **New code.** `update.external-input-changed` goes into `types/updater`, the
  catalogue and both locales.
- **No migration.** Migration 006 is untouched. Snapshot payloads gain an
  optional field (see Lifecycle).

## Break map

| # | Row kind | Case | Decision | Entry boundary | Test | Red mutation |
|---|---|---|---|---|---|---|
| 1 | persisted state | 1.2.0 receipt with acknowledged canonical command, digest, snapshot | same-key replay with digest returns the same run, no extra dispatch after restart; retry without digest is a mismatch | `runtime_ownership.go:88`, `runtime_command.go:344`, `runtime_remote.go:127` | extend `runtime_peer_migration_test.go` with an acknowledged command | drop the digest from the retry comparison |
| 2 | behaviour | healthy A, peer B with `${VAR:?}` and literal `name: edge` | all stack operations and Compose-container operations on A run, no acknowledgement | `runtime_identity.go:31` via admission, agent receiver, executor | resolver + admission + executor tests | restore the uncertainty branch |
| 3 | behaviour | A and B with equal effective or evidenced names | project-ID action runs; `composeNamePeers` on both, archived and off-page peers included | resolver; list/detail producer | resolver test; DTO test; web rendering test | re-add the duplicate refusal / drop the field |
| 4 | behaviour | container labelled with a duplicated name | `compose.duplicate`; containers of other names unaffected | resolver parent lookup | resolver test | resolve first owner |
| 5 | identity change | peer turns duplicate between admission and effect; target or parent changes | duplicate proceeds; target/parent change zero-effect | `runtime_executor.go:157` | two tests, peer and target mutated separately | refuse on new duplicate / ignore target change |
| 6 | external data | recorded path relative, `..`, prefix sibling of root, symlink out of root, parent symlink swapped after check, directory, FIFO, growth past 1 MiB, missing, permission denied, symlink loop | named problem; zero bytes read outside root | recorded-input reader | table test with a counting reader seam; swap test | one mutation per guard: drop root, drop regular check, drop size cap, reopen by path |
| 7 | external data | attribution: same-name A/B in different directories with distinct sentinel files; stale observation; one-off container; container recreated during resolution | A never consumes B's label or bytes; unrelated C actionable | attribution function | test with fake container listing | attribute by Compose name |
| 8 | external data | conflicting lists; missing vs empty label; label equal to own `.env` | conflict → named problem; missing/empty/own → current behaviour | attribution function | unit test | pick the first container |
| 9 | external data | peer evidence: `${}`, `$VAR`, `$$`, aliases, override files, multi-document, invalid YAML | no evidence; no panic; create/rename unchanged | peer evidence reader | unit test | reuse `ComposeContentProjectName` |
| 10 | replacement | malformed `.env` with recorded selection; stale keys in `.env`; conflicting `COMPOSE_FILE`/`COMPOSE_PROFILES`; ordered files; AutoInject on/off | recorded bytes replace `.env` before selection | resolved-input function | loader tests | merge `.env` first |
| 11 | cache | label added/removed/conflicting; same-mtime byte change; symlink retarget inside root; repaired file | cache key changes; current label always resolved | `cache.go` lookup | cache tests | resolve label after cache lookup / omit content hash |
| 12 | deploy | Docklet redeploy of recorded-selection project | every created/recreated service carries the same `environment_file`; fresh read stays readable | `applyCustomLabelsInternal` | model label test + sandbox P4 | omit the label |
| 13 | lifecycle | external file inside root but outside the stack; baseline query, update preflight, effect; external change between preflight and effect; restore and interrupted recovery | hash-checked, never written; drift → `config-drift`; restore with drift → `update.external-input-changed` | `lifecycle_inputs.go:84`, `lifecycle_baseline.go:33`, `lifecycle_preflight.go:51`, `lifecycle_recovery.go:203` | lifecycle tests with write counter on external path | include external file in the writable set / omit its hash |
| 14 | persisted state | snapshot payload without `External` | decodes; baseline unavailable until deploy | `lifecycle/store.go` Open | decode test with a stored old payload | make `External` required |
| 15 | leak | sentinel in a recorded file used by a service; `${VAR:?hint}` with expanded sentinel; malformed dotenv; panic path | absent from hub and executor logs, all project APIs, activity output; env keys kept, values masked | every listed sink; service projections | log capture + body assertions per sink class | restore a raw-error sink / unmask projection |
| 16 | new codes | definition reasons, `update.external-input-changed` | rendered in en/de; unknown reason → generic hint; older producers omit optional fields | types, OpenAPI, catalogue, web | wire catalogue, OpenAPI drift, mapper and rendering tests | omit a key / map unknown to blank |
| 17 | transport parity | unreadable peer, recorded selection and label preservation on Direct and Edge | same results as hub-local | real sender and receiver adapters | parity test over both transports | agent path keeps old resolver |
| 18 | mixed versions | new hub + 1.2.0 agent; 1.2.0 hub + new agent with stray or retained digest; v18/v19 notice | as in Contract | `runtime-commands.ts`, `runtime_sender.go:236` | dialog test, digest acceptance test, notice test | delete dialog branch / reject valid digest |
| 19 | migration | none for receipts; snapshot field optional | manifest unchanged | migration guard | existing guard | — |

## Acceptance cells

See the milestone matrix: P1 to P5, with H, D and E where the agent path
matters. P5 uses a positive access witness (see the matrix).
