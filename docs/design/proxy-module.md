# Proxy module

Proposal for M27 (#473 Traefik, #474 Caddy), confirmed at the M27 kickoff. The
module manages routes of an existing Traefik or Caddy container through the host
agent. It is the first [optional module](optional-modules.md). The UI follows the
[proxy UI concept](proxy-ui-concept.md); constraints from a real installation are
in the [proxy import analysis](proxy-import-analysis.md). Open questions are
listed in the last section.

## Responsibilities

- The proxy configuration on the host is the source of truth. The hub stores the
  instance registration, the last observation and the operation records, never a
  second copy of the routes.
- The hub validates rights and forwards. It has no direct proxy client.
- The agent discovers proxy containers, reads the installation, applies one change
  set with the expected source hashes, verifies the result and journals each step.
  Direct and Edge agents expose the same module routes.
- An instance records host, container identity, proxy kind, configuration
  location and the write authority (for Caddy: Caddyfile or admin API).

## Agent operations

| Operation | Contract |
| --- | --- |
| Discover | List Traefik and Caddy containers per host with image, version and mounts. |
| Inventory | Read the static configuration: entry points, certificate resolvers, file provider location and watch settings, plugins; for Caddy the root Caddyfile, imports, admin address and persistence mode. Unreadable required input yields an incomplete inventory. |
| Read | Return the dynamic files or Caddyfile graph with source hashes. Secret files (`*.secret*`) are never read; definitions in them count as external. |
| Apply | Accept a change set for named routes with the expected hashes of every file it reads or writes. Run the write protocol below. |
| Verify | Probe the requested Host, path, scheme and SNI from the proxy's network namespace and read correlated logs. |

## Write protocol

Each apply runs these steps and journals them on the agent before the next step:

1. **Snapshot.** Read every file the proxy consumes and compare the expected
   hashes. A changed file refuses the apply with the changed path.
2. **Pre-check.** Validate the complete candidate: every consumed file, not only
   the edited one. An already invalid unrelated file blocks the apply with its
   path; the module never repairs or deletes files it does not own.
3. **Patch.** Change only the edited value spans of the original bytes. New routes
   get their own block-style file. Unsupported constructs are refused, never
   reserialized.
4. **Publish.** Write to a temporary name the proxy does not consume, in the same
   directory, then rename atomically. Keep the previous bytes as the rollback source.
5. **Apply.** Traefik applies through its file watch. Caddy runs `caddy reload`
   inside the container after `caddy validate` of the complete graph.
6. **Verify.** Poll for the intended, discriminating effect until a deadline
   derived from the discovered watch throttle, and read logs bound to the
   container identity and start time.
7. **Settle.** The outcome is `verified`, `applied-unverified`, `rejected` (nothing
   published) or `restored`. When the proxy rejects a published file, the agent
   restores the previous bytes, because a rejected Traefik file blocks every
   later change.

After an agent restart, the journal reconciles disk, runtime and the pending step:
temporary files are removed, an unfinished publication is completed or restored,
and the operation keeps its identity.

## Traefik

- Edits use `go.yaml.in/yaml/v4` (already a dependency) for structure and splice
  validated byte spans of the original text. Anchors, aliases, tags, duplicate
  keys and removals from compact flow maps are refused.
- Validation covers a stated subset: full YAML syntax plus references to routers,
  services, middlewares, entry points, certificate resolvers and TLS options. A
  file outside that subset stays readable but is not written.
- Traefik v3.7.16 has no `check-config` command. Template-bearing files need
  version-matched rendering or are reported as not validatable.
- Routes on shared services stay text-only in the first version; resolvers are
  selected, never created. Docker label routes are shown read-only and link to the
  stack editor.

## Caddy

- The Caddyfile is the authority. Owned routes live in a route directory imported
  by the root Caddyfile; an existing single-file layout stays readable until
  editing it is accepted.
- Validation uses `caddy validate` (or `adapt --validate`) on the complete graph
  inside the running container. `adapt` alone accepts configurations that fail
  at provisioning.
- An installation started with `--resume` uses saved API JSON and ignores later
  Caddyfile edits until an explicit file reload. The module detects that mode and
  applies through an explicit file reload, or reports it as unsupported.
- With `admin off` the module is read-only unless an operator accepts a restart.
  Caddy 2.10.2 does not reload on SIGUSR1.

## Measured evidence (2026-11-12, sandbox)

- Traefik v3.7.16 file watch: the new Host answered after 1.0–2.2 s across five
  successive writes (median 1.9 s), including the default provider throttle.
- Caddy 2.10.2: reload 131–166 ms, write to changed response 232–270 ms.
- Traefik faults (invalid YAML, missing middleware, resolver, entry point, TLS
  options, malformed rule) were recognisable in common and JSON logs at INFO and
  ERROR level: 24 of 24 cases. JSON errors may omit `message`; common logs may
  contain ANSI escapes.
- An invalid file keeps old routes serving but blocks every new change; after a
  restart no route is served. Removing the file restores updates.
- A Go spike kept a real shared file byte-identical for 11 of 11 no-op router
  edits.

## Break map

| Row | Required test |
| --- | --- |
| Invalid sibling or candidate | Apply is refused and every file keeps its bytes. Red proof: skip the full-graph check; the test observes a published invalid file. |
| Validator gaps | Missing certificate passes `adapt` but fails `validate`; semantic Traefik errors pass syntax checks. Red proof: syntax-only validation must fail the test. |
| External change | A file changes between read and apply. Red proof: drop the hash check; the operator's edit is overwritten. |
| Crash during apply | Kill between temporary write, rename, reload and settle. Restart reconciles to a consistent file and outcome. |
| Proxy rejects a published file | Traefik log reports an error for the new snapshot; the agent restores the previous bytes. Red proof: skip restore; later changes stay blocked. |
| Delayed application | The effect appears after the watch delay. Red proof: report success immediately; a delayed or missing effect must fail the test. |
| Misleading probe | An old route, redirect, auth response or default certificate answers 200. Red proof: status-only probing must fail the test. |
| Container identity | The proxy container is recreated during apply; logs of the old container must not settle the new operation. |
| Caddy persistence modes | API load with and without `--resume`, Caddyfile edit, restart. Red proof: ignore the mode; restart serves the wrong configuration. |
| Admin off | Validation succeeds and reload fails; the module reports read-only or the accepted restart path. |
| Outcome transport | `verified`, `applied-unverified`, `rejected` and `restored` reach the UI unchanged, Direct and Edge, across reconnect. |
| Secret and unreadable files | Never read; references to them are external and cannot be validated. |
| Module switched off | A running apply blocks the switch; a reconnecting agent with the module off refuses. |

## Kickoff questions

| Question | Options | Recommendation |
| --- | --- | --- |
| Identities of imported routes | Keep file, router and service names and patch in place; or move routes into module-owned files | Keep identities and patch in place |
| Routes on shared services | Text-only; or form editing with all dependent routers shown | Text-only in the first version |
| Certificate resolvers | Select existing ones; or create them (static change, restart) | Select only |
| References the module cannot see | Accept on syntax; or require inventory evidence | Require evidence; otherwise readable but not writable |
| Router rename | Router key only; or router and its exclusive service | Router key only |
| Comments when reordering a list | Keep positions; or move with the inferred item | Keep positions |
| Caddy authority | Caddyfile first; admin API first; both as explicit modes | Caddyfile first, API/`--resume` mode detected and handled separately |
| Caddy route layout | Opt-in route import directory; edit the existing single file | Opt-in import directory; single files stay readable |
| Caddy with `admin off` | Read-only; operator-accepted restart; enable admin | Read-only unless a restart is accepted |
| An unrelated invalid file | Block with its path; repair automatically | Block, never touch unrelated files |
| Probe client | Agent-owned client in the proxy network namespace; `wget` inside the container; helper container | Agent-owned client, `wget` as fallback |
| Verification that cannot be observed | Report `applied-unverified`; add a marker route | Report `applied-unverified`; no marker route |
| Stable `yaml/v4` | Use the existing release candidate; upgrade first | Use the tested pin; upgrade as its own dependency change |
