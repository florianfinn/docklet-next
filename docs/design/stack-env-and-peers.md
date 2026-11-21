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
  A `.env` that sets `COMPOSE_ENV_FILES`, or a recorded env selection, may pick
  another name, so such a stack gives no evidence.
- **Duplicates (maintainer decision D-1).**
  - When effective names are equal (readable model, or peer name evidence), both
    stacks show a warning.
  - An action addressed by project ID runs with neither a refusal nor a
    confirmation.
  - Peer names are other stacks' display names, so a user restricted to
    assigned stacks never receives them. The hub strips the field from that
    user's lists. For detail reads it marks the request: locally through the
    request context, and for agents through `X-Docklet-Peer-Names-Hidden`. The
    hub always clears a client-supplied value first, and agents honour it only
    on the agent-token path. The hub's snapshot fallback keeps such project
    reads under their own key, so administrator bytes never answer them.
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
  - **Bootstrap.** Candidates are containers whose
    `com.docker.compose.project.working_dir` equals the row's directory (both
    canonical real paths) and whose `com.docker.compose.oneoff` is not `True`.
    Every entry of their comma-separated `config_files` list must lie inside
    that directory. The entries are decoded in order. The recorded env files
    are read from the candidates' `environment_file` list, and only then is
    the file selection made from those bytes, so a recorded `COMPOSE_FILE`
    works. No conventional `.env` is read before this.
  - **Confirmation.** After loading, the model's `WorkingDir` must equal the
    row's directory. Its ordered `ComposeFiles` must equal the candidates'
    `config_files` list, compared as absolute canonical paths. These are the
    fields that Docklet (`applyCustomLabelsInternal`) and the Compose CLI both
    write. A mismatch gives the named problem "conflicting recorded env
    files", and the recorded selection is dropped for that target.
  - The Compose project name is not used for attribution, because it may itself
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
  - The entry must be an absolute, clean path below the root. The root may
    be named as configured or as its resolved real path, so a symlinked
    projects directory works; the `os.Root` confines the open either way.
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
  - Editor validation keeps detailed errors for a stack's own files. Once a
    recorded selection takes part, it returns only the classified problem.
  - The Traefik certificate resolver refuses a stack with a recorded,
    conflicting or invalid selection (`env-conflict`, before any write). Its
    `.env` edit would be inert, and its scoped recreate would drop the label.
- **Deploy keeps the label.** A Docklet deploy of a project with a recorded
  selection writes `environment_file` with the same ordered path list. This
  applies to every created or recreated service, through
  `applyCustomLabelsInternal`. Without the label, a single redeploy would make
  the stack unreadable again. Values never enter labels.
- **Lifecycle.** Recorded files are immutable external dependencies. A snapshot
  records each path with its SHA-256 in a new `External` list (no content). They
  are never part of the writable file set, so update, restore, recovery and
  rollback never write them.
  - **Hash compatibility.** `External` is omitted from the JSON when it is
    empty, so `Hash(Open(old payload))` and stored point revisions stay
    byte-identical for every existing snapshot.
  - **Revision binding.** With a non-empty `External`, `SavedRevision` becomes
    `Hash([]any{files, model, external})`, and without one the formula stays
    unchanged. A change to any byte of a recorded file therefore changes the
    revision and the baseline configuration, even for an unused key.
  - **Fresh checks.** Update, restore, recovery and the restart barrier
    recompute the external hashes before any image, tag, file or deploy effect
    and compare them with the snapshot. A mismatch settles with zero effects as
    `update.target-changed`. Recovery never adopts a fresh model whose
    external hashes differ.
  - A baseline counts as available only while the current external hashes
    equal the recorded ones. Otherwise it reports `update.config-drift`, and
    deploying once captures a new baseline.
  - Restoring a point whose external hashes differ from the current files is
    refused with `update.target-changed`. Docklet cannot
    reconstruct those files.
  - The project-root rule for writable files stays as it is.
  - The compensating re-tag after a failed rebuild reads no env input, so the
    external check never gates it; it restores the previous image tag.
  - Old snapshots without `External` decode as an empty list and so differ from
    a recorded selection. Deploying once captures the new baseline. No
    migration is needed because the payload is versionless encrypted JSON.
- **After `down`.** The containers and their labels are gone. The stack falls
  back to its own files and may show "required variable missing" again. This is
  a documented limit (AUTO-4).
- **Changed file selection.** Adding an override file or changing
  `COMPOSE_FILE` after the containers were created breaks the confirmation and
  shows "conflicting recorded env files". The recovery is to recreate the
  containers once with the Compose CLI and the same `--env-file`, or to run
  `down` and deploy from Docklet with the values in the stack's own `.env`.
- **Listing failure (AUTO-19).** Without a container listing, a request uses
  the stack's own inputs. A deploy in that state would recreate services
  without the label; this is a documented limit (follow-up).

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
- **Uninterpolated projection (AUTO-13, revised).** For a project with a
  recorded selection, the project-level service projections
  (`services[]` and `runtimeServices[].serviceConfig`) come from a copy of the
  definition loaded without interpolation. They show `${VAR}` placeholders in
  every field: environment, command, entrypoint, labels, healthcheck, volumes
  and the rest. `image` is the only field taken from the interpolated model,
  because the daemon reports image references anyway. The executable and cached
  models are not changed. No production web consumer reads these environment
  maps (design review R2). Daemon data, meaning container inspect and the
  runtime `ContainerLabels`, is outside this rule.
  - When a raw load cannot type a `${VAR}` field (ports, cpus, durations,
    replicas), the projection falls back to a value-free list: service name,
    image, build presence and dependency keys. A projection never makes a
    loadable stack unreadable.
  - Project and service URLs come from interpolated labels on purpose: they
    are operator-configured links, and placeholders would break them.

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
- **No new code (AUTO-18).** The updater enums are part of the agent
  fingerprint, so an external input change reuses existing codes:
  `update.config-drift` for the baseline, and `update.target-changed` at the
  effect, at the restart barrier and for a restore.
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
| 7 | external data | attribution: same-name A/B in different directories with distinct sentinel files; stale observation; one-off container; container recreated during resolution; own recorded file selects a custom file and an ordered override list; Docklet- and CLI-produced labels | A never consumes B's label or bytes; custom selection loads without a conventional `.env` read; unrelated C actionable | attribution bootstrap + confirmation | test with fake container listing and read counter | attribute by Compose name / compare against the pre-recorded selection |
| 8 | external data | conflicting lists; missing vs empty label; label equal to own `.env` | conflict → named problem; missing/empty/own → current behaviour | attribution function | unit test | pick the first container |
| 9 | external data | peer evidence: `${}`, `$VAR`, `$$`, aliases, override files, multi-document, invalid YAML | no evidence; no panic; create/rename unchanged | peer evidence reader | unit test | reuse `ComposeContentProjectName` |
| 10 | replacement | malformed `.env` with recorded selection; stale keys in `.env`; conflicting `COMPOSE_FILE`/`COMPOSE_PROFILES`; ordered files; AutoInject on/off | recorded bytes replace `.env` before selection | resolved-input function | loader tests | merge `.env` first |
| 11 | cache | label added/removed/conflicting; same-mtime byte change; symlink retarget inside root; repaired file | cache key changes; current label always resolved | `cache.go` lookup | cache tests | resolve label after cache lookup / omit content hash |
| 12 | deploy | Docklet redeploy of recorded-selection project | every created/recreated service carries the same `environment_file`; fresh read stays readable | `applyCustomLabelsInternal` | model label test + sandbox P4 | omit the label |
| 13 | lifecycle | external file inside root but outside the stack; baseline query, update preflight, effect; unused key changed between admission and effect; restore and interrupted recovery | hash-checked before effects, never written; drift → `config-drift` / zero-effect `update.target-changed`; recovery does not adopt a changed external | `lifecycle_inputs.go:84,146`, `lifecycle_baseline.go:33`, `lifecycle_preflight.go:43`, `lifecycle_recovery.go:52,70,129,203` | lifecycle tests with write counter on external path | include external file in the writable set / omit `External` from `SavedRevision` / skip the recovery comparison |
| 14 | persisted state | sealed point and baseline payloads from 1.2.0, with stored revisions | `Hash(Open(payload))` unchanged; reopened recovery accepts them | `lifecycle/store.go` Hash/Open, `snapshot.go` alias | test with real old sealed payloads | encode empty `External` |
| 15 | leak | sentinel in a recorded file used in environment, `command` and a label; `${VAR:?hint}` with expanded sentinel; malformed dotenv; panic path | absent from hub and executor logs, all project APIs, activity output; projections show placeholders | every listed sink; service projections | log capture + body assertions per sink class and per field | restore a raw-error sink / project the interpolated model |
| 16 | new codes | definition reasons (updater codes reused, AUTO-18) | rendered in en/de; unknown reason → generic hint; older producers omit optional fields | types, OpenAPI, catalogue, web | wire catalogue, OpenAPI drift, mapper and rendering tests | omit a key / map unknown to blank |
| 17 | transport parity | unreadable peer, recorded selection and label preservation on Direct and Edge | same results as hub-local | real sender and receiver adapters | parity test over both transports | agent path keeps old resolver |
| 18 | mixed versions | new hub + 1.2.0 agent; 1.2.0 hub + new agent with stray or retained digest; v18/v19 notice | as in Contract | `runtime-commands.ts`, `runtime_sender.go:236` | dialog test, digest acceptance test, notice test | delete dialog branch / reject valid digest |
| 19 | migration | none for receipts; snapshot field optional | manifest unchanged | migration guard | existing guard | — |

## Acceptance cells

See the milestone matrix: P1 to P5, with H, D and E where the agent path
matters. P5 uses a positive access witness (see the matrix).
