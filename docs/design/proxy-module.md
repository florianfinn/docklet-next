# Proxy module

Design for M27 (#473 Traefik, #474 Caddy). The module manages routes of an
existing Traefik or Caddy container through the host agent. It is the first
[optional module](optional-modules.md). The UI follows the
[proxy UI concept](proxy-ui-concept.md); constraints from a real installation are
in the [proxy import analysis](proxy-import-analysis.md). The decisions of the
M27 kickoff (2026-11-15) are listed in the last section.

## Responsibilities

- The proxy configuration on the host is the source of truth. The hub stores the
  instance registration, the last observation and the operation records, never a
  second copy of the routes.
- The hub validates rights, reserves the instance and forwards. It has no direct
  proxy client.
- The agent discovers proxy containers, reads the installation, admits one
  operation at a time per configuration source, applies it, verifies the result
  and journals it. Direct and Edge agents expose the same module routes.
- An instance records host, container name and identity, proxy kind, the mount
  that holds the writable configuration, its source kind and the write authority
  (for Caddy: Caddyfile or admin API).

## Host access

The agent container mounts the Docker socket, its data directory and the projects
directory at the same path; it never reads other host paths. Every entry point
(including a query by a persisted ID) re-inspects the container and its mounts
and refuses when the identity, the mount source or its kind changed. A restart
between operations keeps the registration (same container ID and source): each new
operation fences the `StartedAt` the inventory reads at admission. When no operation
is reserved or unfinished, the hub may follow a recreated container with the same
name, proxy kind, mount target, canonical source, source kind, mount mode and
read-only authority. Discovery uses the same Direct and Edge transport. A single
transaction fences registration and instance identity, checks ownership, updates
the identity and writes a finished `adoption` receipt for Recent operations.
Each read triggers at most one adoption on `container-changed` and retries with
the returned instance. Opening an instance adopts automatically once per page
open; Refresh, window focus or visibility regain (debounced two seconds) and the
read after an operation settles each allow one more, so a refusal such as an open
operation clears on the next read. Only opening switches the view: to the
`supersededBy` instance, or to the next untried instance when the page (not the
person) chose the refused one. There is no identity polling. Writes (apply and
resolver) adopt first, then store the refreshed identity, generation and
registration before admission, so a plain restart (same container, new
`StartedAt`) never refuses a write; a concurrent change of the stored container
is `container-changed`, any other concurrent change of the row is retried once and
then `operation-conflict`. A container that reports itself without readable state
is never adopted as its own recreation. Other differences keep `container-changed`. A transient
Docker error while classifying a source is an error, not a read-only source; only a
missing path and a bind-rule refusal classify. The same holds for every identity and
target read: only a typed refusal or an observed difference (removed or stopped
container, other `StartedAt`, other bytes) settles an operation. A failed read keeps
the journal unfinished for a later query; an epoch that cannot be read around the
observation gives `applied-unverified` (`not-observable`), never `verified` or
`container-changed`, and a requested restore still runs. A failed read or call while
preparing, staging, validating or publishing is not a graph refusal either: the
journal keeps its phase and the recovery table settles it from the actual target. A
static read the display shows as incomplete inventory stays such a failure in the
publication graph checks (`proxy.IncompleteRead`).

| Need | Path |
| --- | --- |
| Read a file | Docker copy API from the proxy container. The response is a tar stream: the reader takes exactly one regular-file record per requested path, checks name, type and size, and hashes the payload. Directory listings come from a bounded tar walk of headers; payloads of secret files are skipped before they are read. |
| Write, project source | A source inside the agent's projects directory (the usual case for a proxy stack managed by Docklet) is written through the agent's own mount, confined like project files (`acfs.LogicalPath`, no symlink in any segment), with a temporary file, file sync, rename and directory sync. |
| Write, other source | A named volume or another allowed bind source is read-only in M27 (Q-HELPER-META): only the tools helper could write it, and the pinned helper cannot prove that a file carries no extended attributes, ACLs or extra hard links, which a replacement would lose. Setup shows the reason `unsupported-metadata`. The helper writer and its bind path rules stay in place for a later helper with a metadata capability; a proxy configuration outside the projects directory is not a planned case. |
| Read-only sources | A file mounted as a single file, a refused bind source, a helper-only source (above) and an unknown source kind make the instance read-only with the reason shown in setup. The proxy container's own mount access does not: the agent writes a project directory itself and other sources through the helper, so a proxy may mount its configuration read-only. |
| Metadata | A replaced file keeps its mode. The owner becomes the writer identity; a file that only its current owner may read is refused, because the proxy could lose access. ACLs, extended attributes and hard links are not preserved; files that carry them are refused. |
| Run a proxy command | Docker exec in the proxy container with an argument vector built by the agent from a fixed per-kind allowlist: `caddy version`, `caddy validate`, `caddy adapt`, `caddy reload`, `traefik version`, and the probe below. Values come only from validated fields: no control characters, no leading `-`, URLs normalised. Both the transport error and the exit code are checked. Output is bounded. |
| Restart | Only for a Caddy instance with `admin off`, only inside an operation the operator confirmed for a restart: Docker restart of the instance container, then the container must be running with a new `StartedAt`. |
| Probe | BusyBox `wget` inside the proxy container with fixed options, and for Caddy the admin endpoint read through the same `wget`. Capabilities are detected once per container identity; see Verification. |
| Logs | The existing Docker log reader with `since`, wrapped by a fence that rechecks container ID and `StartedAt` before each consumed batch and before settlement. |

Limits apply to file count (512), single file size (1 MiB), import depth (8),
total read bytes (16 MiB), exec output (256 KiB) and exec time (30 s for
validate and reload, 10 s per probe). Byte limits count only payloads that are
read, so a large secret file does not block an instance.

Known limits of the first version:
- Host operation routes are administrator routes on the agent's API. An
  administrator calling them directly bypasses the hub's record and reservation;
  the agent's source lock and journal still protect the files.
- The source lock key is the canonical mount source. Two containers mounting
  overlapping but different sources get different keys.
- A file with extended attributes is refused, which includes every file on hosts
  that label files with SELinux.
- Adoption refuses while an instance has a reservation or unfinished operation;
  a container held by another registration stays stale and can be removed.
- A `recovery-required` operation whose proxy container was removed for good, or
  recreated under another name, cannot be released, because the target cannot be
  read; its reservation blocks switching the module off and deleting its data.
  Recreating a container with the same name and the same configuration mount
  makes it releasable again.
- The pinned tools image cannot prove that a file has no extended attributes,
  ACLs or extra hard links, so helper-only sources (named volumes, allowed binds)
  are read-only at setup with `unsupported-metadata`; no write reaches staging.

## Agent operations

| Operation | Contract |
| --- | --- |
| Discover | List Traefik and Caddy containers per host from the shared inventory, copied before any projection: image, version, mounts with source kind, and whether an instance already uses the container. |
| Inventory | Read the static configuration from `Cmd`, `Entrypoint`, allowlisted `TRAEFIK_*` names and the static file: entry points, certificate resolvers, file provider location, watch and throttle, plugins; for Caddy the root Caddyfile, imports, admin address and persistence mode. Only allowlisted leaves are returned, never raw inspect, environment or command lines. Unreadable required input yields an incomplete inventory and a read-only instance. |
| Read | Return the configuration files with source hashes and the classified routes. Secret files (`*.secret*`, matched on every path segment, case-insensitive) are listed by name only. |
| Apply | Accept one operation: an operation ID chosen by the hub, the instance identity, the target file, its expected hash, the hashes of every other consumed file, the change and the operator acknowledgements it needs (unresolved references by name, replacing a diverged Caddy running configuration, a Caddy restart). The acknowledgements are part of the request digest. |
| Operation | Return the journalled state of an operation ID. |
| Fence | Record an operation ID as abandoned if it was never admitted; a later Apply with that ID is refused. |
| Release | Operator override for a `recovery-required` operation: record it final as `released` with the current target (presence, hash), remove its temporary and marker files and free the source lock. Host configuration is never changed. |

## Operation lifecycle

One operation changes one configuration file. A Traefik operation edits a route
(or a shared service with its dependent routers) in an existing file or creates a
new route file; deleting a Traefik route removes the router and keeps every
service, so the file stays. A Caddy operation creates, edits or deletes one route
file, or inserts the route `import` line into the root Caddyfile. Old and new are
each a pair (presence, hash), so creation has an absent old side and a Caddy
deletion an absent new side. The only other file an operation may touch is its
own transient Traefik marker file (see Verification), which is named in the
intent and removed before the final state.

**Executor generation.** Each agent database has a journal generation, a random
ID created with the module's journal table. Read returns it; Apply, Operation and
Fence carry the expected generation and the environment registration, and the
agent refuses a mismatch with `generation-mismatch`. Every response carries the
generation it was produced in.

**Hub.** The hub commits the operation record (`pending`, with digest,
registration and generation) and an instance reservation before it dispatches
once. The reservation holds until the operation reaches a final state. When the
response is lost (timeout, Edge reconnect, hub restart), the hub queries the
operation. If the agent answers unknown in the same generation, the hub sends
Fence; only a fence recorded in that generation releases the reservation as
`not-applied`. A generation mismatch, a lost journal or a changed registration
makes the operation `recovery-required`, never `not-applied`. The hub never
sends a second Apply. The dispatch and its immediate follow-up are detached from
the HTTP request; a record that is still unfinished is followed by an
asynchronous reconciler at startup, on agent-online and every 30 seconds, one
pass per environment at a time. A typed agent refusal of a lost dispatch becomes
the reason of `not-applied`. When the agent answers Release with a final receipt
for the same operation, generation, registration and digest, the hub adopts that
receipt; a dispatch answered under a changed registration stays
`recovery-required` until such a Release; `journal-lost` is treated
like a generation mismatch.

**Agent admission.** Apply admits atomically under one lock per configuration
source (host and canonical mount source), shared by all instances and all target
files in that source, and held until the operation is final or
`recovery-required`. The target path is part of the request and its digest. An
existing ID with the same digest returns its recorded operation unchanged; a
different digest is refused with `operation-conflict`; a fenced ID is refused.
Admission commits the write-ahead intent before any effect: target path, old and
new (presence, hash, bytes), mode, the hashes of the validated graph, and an
opaque engine value from preparation (for example the probe taken before the
change). The publishing commit adds the publication time that bounds log
evidence. After admission the work no longer depends on the caller: a dropped
connection does not change the outcome, and a process exit is settled at startup.
Receipts are returned whatever the container's current identity; identity and
source are checked only before a new host effect. An admission ID stays refused
after its journal row was pruned.

**Agent phases.** Each phase is committed before the effect it names:

| Phase | Effect that follows |
| --- | --- |
| `admitted` | Snapshot and validation; no effect on the host. |
| `staging` | Write the temporary file `.docklet-<operation>.tmp` in the target directory (Caddy deletion: none). |
| `publishing` | Recheck all hashes, then rename the temporary file onto the target (Caddy deletion: rename the target to the temporary name). |
| `reloading` | Caddy only: `caddy reload`. |
| `restarting` | Caddy with `admin off` only: Docker restart, recording the `StartedAt` before it. |
| `observing` | Verification until its deadline. |
| `marking` | Traefik only, entered from `observing` when the change has no expected HTTP effect or the first response did not change: write the marker file, then observe it until the same deadline rule. |
| `restoring` | Write the old side back (rename a temporary copy of the old bytes onto the target, or remove the target if old was absent), then for Caddy reload (`admin off`: restart). |

Final outcomes: `verified`, `applied-unverified` (with a reason), `rejected`
(nothing published), `restored` (old side back on disk and, for Caddy, the old
configuration reloaded with exit 0 or the restarted container running),
`recovery-required` (the module cannot establish a consistent state; the source
lock and the instance reservation stay) and `released` (an administrator released
a `recovery-required` operation).

**Recovery.** Startup settlement and every query of an unfinished operation read
the actual target (presence, hash) and resolve by this table, which is also the
oracle of P4. "Third" is any state that is neither old nor new; the module never
overwrites it. Temporary files and the marker file of the operation are removed
in every row.

| Last committed phase | Target is old | Target is new | Target is third |
| --- | --- | --- | --- |
| `admitted` | `rejected` | `recovery-required` (committed before the snapshot, so new bytes are not yet known and cannot be told from third) | `recovery-required` |
| `staging` | `rejected` | Treated as `publishing` with new | `recovery-required` |
| `publishing` | `rejected` (the rename did not happen) | Traefik: `observing` with a fresh deadline. Caddy: `reloading` | `recovery-required` |
| `reloading` | Caddy: `recovery-required` (the reload may have applied new bytes that are no longer on disk) | Caddy: reload again, then `observing` | `recovery-required` |
| `restarting` | `recovery-required` | Container running with a `StartedAt` after the recorded one: `observing`. Not running: `restoring`. Running without a later `StartedAt`: `recovery-required` (`container-changed`) | `recovery-required` |
| `marking` | `recovery-required` | Remove the marker, then `observing` with a fresh deadline | `recovery-required` |
| `observing` | Traefik: `recovery-required` (published bytes were reverted on disk). Caddy: `recovery-required` | `observing` with a fresh deadline (Caddy reloads first; `admin off`: no restart, the running epoch is observed) | `recovery-required` |
| `restoring` | Traefik: `restored`. Caddy: reload (`admin off`: restart); `restored` on exit 0 or a running container, else `recovery-required` | Restore again | `recovery-required` |

Restore runs only when the proxy rejects the published snapshot during the
current container epoch: a Traefik error logged after `publishing` for the target
or for the aggregate snapshot, or a failed Caddy reload. A failed Caddy reload
leaves the old configuration active; the module restores the old side so that a
restart does not boot the rejected file. A timed-out exec is treated as running:
no contradicting reload or restore runs, and the operation becomes
`recovery-required`.

Startup settlement handles each unfinished row on its own and never fails
startup because of journal content; an undecodable row is answered as
`journal-lost` until it is released.

`recovery-required` is shown with the operation, both hashes and the manual
steps. An administrator can release it after a confirmation that names the
current target state. The hub sends Release in the recorded generation; the agent
answers with the recorded final state, and the hub then frees the reservation. A
generation mismatch means the agent holds no lock for the operation, so the hub
releases only its reservation. An unreachable agent refuses the release, because
its source lock cannot be confirmed free. After a container recreation with the
same configuration source at the same mount target, Release reads the target
through the current container; a changed source refuses it.

## Validation

The candidate graph is the set of consumed files with the target replaced by the
new side. An instance is writable only while its graph is eligible:

**Traefik.** The file provider consumes `.yml`, `.yaml` and `.toml`
recursively, case-insensitively, from one directory. Every consumed file must be
either a readable YAML file inside the validated subset or an opaque secret file.
A readable TOML file, a template-bearing file, a file outside the subset, an
unreadable non-secret file, a single-file provider, watch off or an incomplete
static inventory make the instance read-only with the reason. The validated
subset is YAML syntax, known field types for routers, services, middlewares and
TLS sections, the Traefik v3 rule grammar (`Host`, `HostRegexp`, `Path`,
`PathPrefix`, `PathRegexp`, `Method`, `Header`, `HeaderRegexp`, `Query`,
`QueryRegexp`, `ClientIP` with `&&`, `||`, `!` and parentheses; regular
expressions must compile), and references to routers, services, middlewares,
entry points, certificate resolvers and TLS options. A router with
`ruleSyntax: v2` is text-only.

An invalid readable file blocks every write with its path, line and parser
error; a foreign file is never repaired or removed. For a file the module created
itself, the agent keeps the bytes of its last verified version, and the operator
can restore them as an ordinary operation.

Opaque secret files are never read or written and contribute no definition
names. A reference that the operation leaves unchanged is kept as it is,
including references into opaque files. A reference that the operation adds or
changes resolves to a readable file, a plugin, an `@internal` service or a
provider in the static inventory, or it is named in the operator's
acknowledgement of unresolved references. An unresolved reference is checked
after publication: a Traefik log error for the router, or a router that does not
answer, restores the old side.

The validity of opaque files cannot be checked: when they exist, the module
applies only if the container log shows no file-provider error after the newest
modification time among the consumed files (from the tar headers) within the
current epoch, verifies the effect, and restores on a current rejection.

**Caddy.** Validation is `caddy validate` inside the running container on a
staged copy of the graph: the agent writes a staging directory
`.docklet-staging-<operation>/` inside the route directory with a copy of the root
Caddyfile whose route `import` line points at the staging copy of the route set
(existing routes plus the candidate). The live glob (`routes/*.caddy`) does not
match the staging directory. The instance is read-only when the root Caddyfile
has relative imports other than the route glob, or when any consumed file has a
secret name, because such a file cannot be copied into staging without reading
it and leaving it out would validate a different graph. Validation uses the container's
environment and user. The staging directory is removed after validation and by
startup settlement. Inserting the route `import` line validates a staged root
Caddyfile with the line appended at top level.

An operation receipt carries an optional `diagnostic` (path, line and the
engine's bounded text, at most 4 KiB, without control characters other than
newline and tab) when an engine refuses in Prepare or fails in staging,
activation or observation; it is journalled with the receipt, survives Direct and
Edge and is shown next to the reason. Engines never put opaque file content into
it. An eligibility refusal names the eligibility reason, path and line.

## Verification

The first request of every probe goes to the selected container: BusyBox `wget`
to `127.0.0.1` on the entry point's container port with the route's `Host`
header, `-S`, `-T`, `-Y off` (a proxy variable in the container environment never
redirects the probe) and a 64 KiB body limit. `hostio.ParseProbe` reads that output
for both engines; a non-zero exit with a status line is an ordinary HTTP answer,
because BusyBox exits 1 on 4xx and 5xx. The pinned BusyBox builds follow
redirects and can neither disable that nor choose SNI independently of the URL
(baseline B0, 2026-11-15). A backend that redirects therefore causes a
follow-up request to the redirect target; the module evaluates only the first
response's status line and headers from the `-S` output and never treats a later
response as evidence.

Verification compares the first response after the change with the first
response before it (probed at admission, same request):

| Change | Evidence for `verified` |
| --- | --- |
| New route | Before: the proxy's no-route response. After: any other first response. For Traefik the no-route response is a bare 404: BusyBox stops at the 4xx status line, so no headers and no body are seen (B0), and a backend 404 looks the same and is never evidence. |
| Deleted route | The reverse. |
| Destination or middleware change | The first response's status, selected headers or body hash differs from before, and is not a gateway error (502, 503, 504). |
| Caddy new or deleted route | Caddy has no distinctive no-route answer: an unmatched host gets an empty 200 (`Server: Caddy`, baseline B0) and a catch-all site answers with its own response. A create or delete therefore counts like an edit: the first response differs from before and is not a gateway error (a delete needs an answer before the change, a create one after it). |
| Caddy site | Additionally the active configuration read from the admin endpoint inside the container contains the route's host, matcher and upstreams. For a site that cannot be probed (HTTPS through an IP URL fails in Caddy), this readback alone gives `verified` with evidence `configuration`. The same holds for a pure rename (the site differs only in its `# docklet-route:` name), which cannot change the routing answer. An Epoch error after the observation window has ended reports the last reason, not `container-changed`; a later successful probe in the window clears a reason an earlier failed probe set; a probe or readback cut off by the window's end does not set the reason (as for Traefik). Routing evidence needs a successful baseline probe from Prepare; a failed baseline exec is not a status-0 answer. |

The outcome records its evidence kind: `routing` (first response),
`configuration` (Caddy readback) or `load` (Traefik marker). For Traefik, an
unchanged first response or a change without an HTTP effect (resolver, TLS
options) enters `marking`: the agent writes `docklet-check-<operation>.yml` with
one router for `<operation>.docklet-check.invalid` on a plain HTTP entry point
and a service on a closed loopback port. The file provider re-reads the whole
directory, so a first marker response other than the no-route 404, with no
provider error in the log, shows that the snapshot containing the target was
loaded: `verified` with evidence `load`. The marker file is removed before the
final state. Without a plain HTTP entry point, or when the marker never answers,
the outcome is `applied-unverified` (`no-observable-difference` or
`not-observable`). For Caddy without a readback (`admin off`), the same cases
are `applied-unverified`. No `wget`, a missing option or an unreadable admin
endpoint gives the matching reason. The tools helper reports HTTPS routing evidence independently from certificate
verification. The UI says “served certificate observed at <time>” and reports chain
trust, hostname match and validity separately; it never claims issuance by a resolver.
The wget fallback skips certificate checking and says that the probe helper is unavailable.

The deadline for Traefik is twice the discovered provider throttle plus 12 s
(10 s plus 2 s helper allowance; default throttle 2 s → 16 s; B0 measured up to
2.4 s including helper start); for Caddy 7 s after the reload (5 s plus 2 s). Logs are read from `publishing` onward within the
fenced epoch; a restart (same ID, new `StartedAt`) or a recreation (new ID) ends
observation with `applied-unverified` (`container-changed`). Missing, rotated or
file-driver logs are `logs-unavailable`, never "no error".

## Traefik

- Edits use `go.yaml.in/yaml/v4` (already a dependency, v4.0.0-rc.6) for structure
  and splice validated byte spans of the original text, as the stack update path
  already does. Anchors, aliases, tags, duplicate keys, flow-map removals and
  CRLF files are refused.
- The classifier moves from the design lab to the agent: routers are editable,
  text-only or infrastructure; middleware references resolve as local, plugin,
  internal, external or opaque.
- A route on a shared service is edited together with every router of that
  service; the form shows them all, and verification probes each of them. A
  service shared with a router in another file keeps the route text-only.
- Advanced mode adds the priority and the rule as free text, validated by the
  rule grammar above. When the router's service is in an opaque file, the router
  stays editable and the service target is shown as being in that file by name.
- Certificate resolvers are selected from the static inventory; creating them
  belongs to a later milestone.
- The consumed set follows Traefik's directory loader: every `.yml`, `.yaml` and
  `.toml` file, hidden files and directories included; one invalid file blocks
  the directory. Traefik watches only the provider directory's top-level files,
  so routes in subdirectory files are text-only (`watch-off`) and writes there
  are refused. An empty or comment-only file is an empty configuration.
- The static file is searched like Traefik's finder (`traefik.toml`, then
  `.yaml`, then `.yml` in `/etc/traefik`); its location comes from flags only, so
  an environment location without a found file makes the inventory incomplete.
  Plain integer durations are seconds; `experimental.localPlugins` count as
  plugins. Rules follow `rules.CheckRule`: empty matcher arguments are refused,
  and each matcher is accepted in exact, lower, upper and title case; catch-all
  recognition applies to every accepted spelling.
- Log rejection is scoped: an error record counts when it is an aggregate
  file-provider error (`providerName` `file`) or names a router, service or
  middleware of the target (old or new side) or one of its references as a whole
  token, compared byte for byte (Traefik names are case-sensitive). Other providers' records (ACME issuance, docker) and unrelated routers
  never reject a change. The opaque-file health check counts aggregate
  file-provider errors only.
- Docker label routes are shown read-only and link to the stack editor when the
  container belongs to a Docklet stack; otherwise the detail names the container.

## Caddy

- The Caddyfile is the authority. Owned routes live in a route directory mounted
  as a directory and imported by the root Caddyfile. When the `import` line is
  missing, setup offers to insert it; pressing **Insert import line** runs one
  operation that appends it to the root Caddyfile. The same action is offered again whenever owned route
  files exist and the line is missing. Apart from that line the module never
  changes the root Caddyfile; its existing sites stay readable. A root Caddyfile
  mounted as a single file cannot be written, so setup shows the line instead.
- One site block per route, using the grammar of the prototype; unknown
  directives are refused rather than dropped. A route file is written exactly as
  submitted.
- Inventory needs an explicit `--config` path; without `--adapter caddyfile` its
  file name must start with `Caddyfile`. Otherwise Caddy's own fallback (working
  directory, adapter guessed from the name) could differ from what the module
  reads, so the instance is not inventoried.
- Snippet imports (`import name` of a `(name)` block) are not resolved in this
  version; such an instance is read-only (`relative-import`).
- With `--resume` and admin off the running configuration cannot be read for the
  comparison below, so the instance is read-only (`admin-unavailable`).
- `caddy validate` is the validator; `adapt` alone accepts configurations that
  fail at provisioning. Missing DNS provider modules surface as the real
  validation error.
- An installation started with `--resume` boots from its autosaved
  configuration. Before a write the agent compares the running configuration
  (admin readback) with `caddy adapt` of the root Caddyfile after JSON
  normalisation. On a difference, Apply requires the operator's acknowledgement
  that the running configuration is replaced. The reload loads the Caddyfile and
  Caddy autosaves it, so a restart boots the same state. With `--resume` and
  `persist_config off`, the instance is read-only.
- With `admin off`, a write needs the operator's confirmation of a container
  restart. A restart that does not bring the container back restores the old side
  and restarts again. Caddy 2.10.2 does not reload on SIGUSR1.

## Data

Migration 005 adds hub tables for instances, reservations and operation records
and an agent table for the operation journal (the agent runs the same
migrations). Operation records are kept for 30 days after their final state;
unfinished ones are never pruned. For each file the module created, the agent
keeps the bytes of its last verified version until the file is deleted through
the module or the module data is deleted.

## Measured evidence (research 2026-11-13, sandbox)

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

Each row names the test seam, the assertion and the red mutation; a red proof is
a failing assertion, never a compile error or timeout.

| # | Row | Test seam and assertion | Red mutation |
| --- | --- | --- | --- |
| P1 | Invalid sibling | Real graph enumerator, validator and writer over a directory with a valid target and an invalid sibling (YAML, nested TOML, upper-case extension): zero publications, all hashes unchanged. | Validate only the target: the valid target is published. |
| P2 | Validator gaps | Caddy runner with exec returning exit 1 and nil error; Traefik fixtures with wrong field types and malformed rules: rejected, no publication. | Syntax-only validation or ignoring the exit code admits the candidate. |
| P3 | External change | Change target bytes, add or remove a sibling, replace the mount between read and the final recheck: refused with the path, operator bytes kept. | Drop the final recheck: the operator's bytes are overwritten. |
| P4 | Recovery table | Restart harness with a fault after each phase commit and after each effect, for Traefik edit and create and Caddy create, edit and delete, with the target set to old, new and third: every cell of the recovery table, asserting (presence, hash), temporary files, reload calls, lock and reservation, and the final state. | Remove one table branch: the asserted cell differs. |
| P5 | Proxy rejection | Log fake with the measured error shapes after `published`, and a stale error before it: restored bytes for the current error, no restore for the stale one; failing restore gives `recovery-required`. | Skip restore: target keeps rejected bytes; or ignore the epoch: a stale error restores a good change. |
| P6 | Delayed effect | Injected clock and a held effect: no `verified` before release; deadline without effect gives `applied-unverified` (`no-observable-difference`). | Report success on the first probe. |
| P7 | Misleading probe | Real argv builder and `-S` parser: a redirecting backend (only the first response counts), a gateway error, an unchanged response and an auth challenge are classified as specified. | Evaluate the last response of the `-S` output. |
| P8 | Container identity | Held observation and restart fence `StartedAt`; recreation follows only the identical-source/name/kind rule with no open operation, with one finished adoption receipt. All other differences retain `container-changed`; old logs never settle new work. | Drop the fence. |
| P9 | Caddy persistence | Inventory from measured command lines: `--resume` with diverged running config refuses Apply without the acknowledgement and with zero effects; with it, reload from the Caddyfile; `--resume` with `persist_config off` read-only. Restart persistence is checked in acceptance (PX13, PX18). | Skip the comparison: a diverged instance is written without acknowledgement. |
| P10 | Admin off | Without the restart acknowledgement: refused before any write. With it: validate, publish, restart, observe the new epoch; a container that does not come back restores and restarts again; failing again gives `recovery-required`. | Reload instead of restart, or skip the restore after a failed start. |
| P11 | One dispatch and generation | Real Direct proxy and Edge tunnel, response dropped: the hub queries and fences, sends Apply exactly once; after an effect the agent journal is replaced: `recovery-required`, never `not-applied`; a stale result from the previous registration is ignored. | Resend Apply on a lost response: two dispatches recorded. Separately accept a fence from another generation: `not-applied` is recorded. |
| P12 | Duplicate admission | A completed operation replayed with the same ID and digest returns the identical recorded result, not a stale-hash refusal, and runs nothing; same ID with another digest: `operation-conflict`; fenced ID: refused. Hash and lock checks stay active. | Remove only the receipt lookup: the replay returns a stale-hash refusal. |
| P13 | Source lock | Two instances on the same source writing different files, and two operations on one instance, latched in the protected interval: the second cannot enter until the first is final. | Key the lock by target path: both are inside the interval at the latch. |
| P14 | Secret files | Recording copy fake with nested, upper-case and symlinked `*.secret*` entries: zero payload reads, names listed. | Match by extension only: a payload is read. |
| P15 | Byte identity | Golden bytes per edit kind on a fixture shaped like the shared real file: only the edited spans change. | Reserialize the document: comments change. |
| P16 | Argument safety | Exec payload capture with hostile Host, path and file values: argv equals the expected vector; control characters and leading `-` refused. | Interpolate a value into `sh -c`. |
| P17 | Source eligibility | Real inspect for project source, named volume, bind refused by the rules, single-file mount and source replacement: writer chosen correctly, refused kinds make zero writer calls. | Allow a single-file mount: a rename is attempted. |
| P18 | Bounded input | Tar with traversal, links, oversized and extra records; oversized exec output; cancelled stream: typed refusal, nothing written. | Remove the size check: the oversized payload is accepted. |
| P19 | Outcome transport | Every final and pending state and reason through Direct and Edge to the web consumer, which renders the matching state. | Map `applied-unverified` or `recovery-required` to success in the decoder. |
| P20 | Migration | Fresh database and upgrade from 004 keep unrelated rows; restart decodes every unfinished state; an unknown state fails safe. | Drop a persisted field: the restart consumer misreads the state. |
| P21 | Shared inventory | Concurrent projections through the real coalesced container list: input unchanged, race detector clean. | Sort mounts in place. |
| P22 | Eligibility guard | Watch off, TOML present, template file, incomplete inventory, `ruleSyntax: v2`, `--resume` without persistence, service shared across files: read-only or text-only before effects. | Remove the guard: apply runs. |
| P23 | Caddy staging | Valid old graph, invalid candidate through the real staging builder and runner: rejected; staging removed, also after a crash. A secret-named consumed file makes the instance read-only with zero payload reads. | Validate the live root: the candidate passes. |
| P24 | Release | `recovery-required` operation, Release in the same generation: final `released`, files byte-identical, lock and reservation free; generation mismatch frees only the hub reservation; unreachable agent refused, reservation kept. | Release the reservation while the agent is unreachable. |
| P25 | Shared service | Fixture with a service shared by three routers: one edit patches only the service and router spans, all dependants are probed; a dependant in another file makes the route text-only. | Probe only the edited router: a broken dependant is reported verified. |
| P26 | Rule grammar | Parser corpus of valid v3 rules (all matchers, nesting, negation) and invalid ones (unknown matcher, unbalanced parentheses, bad regex, v2-only syntax): accepted and refused as listed. | Accept unknown matchers: an invalid rule is published. |
| P27 | Unresolved reference | New reference to a name defined only in an opaque file: refused without acknowledgement; with it, published; a log error for the router or a non-answering router restores. | Skip the post-check: a typo stays published. |
| P28 | Import line | Root Caddyfile without the line: insert operation appends exactly one top-level line, validated in staging, reloaded, restored on failure; offered again after the line is removed; single-file root read-only. | Insert inside the last site block: validation of the candidate differs from the asserted bytes. |
| P29 | Marker | Unchanged first response and resolver-only change: marker written and observed, `verified` with evidence `load`, marker removed; crash after `marking` removes the marker; provider error restores and leaves no marker. | Keep the marker after settlement. |
| P30 | Last verified bytes | Module-owned file made invalid outside the module: restore offers the last verified bytes and publishes them through the normal protocol; a foreign file only reports path, line and error. | Offer restore for a foreign file. |

## Kickoff decisions (2026-11-15)

| Topic | Decision |
| --- | --- |
| Identities of imported routes | Keep file, router and service names and patch in place |
| Routes on shared services | Form editing with all dependent routers in the same file |
| Text-only routes | Priority and the rule as free text in Advanced mode; a router whose service is in an opaque file stays editable |
| Certificate resolvers | Select existing ones; creation with restart is a later milestone |
| References the module cannot see | Unchanged ones stay; new ones need visible evidence or the operator's acknowledgement, followed by a post-check and restore |
| Router rename | Router key only |
| Comments when reordering a list | Keep positions |
| Caddy authority | Caddyfile; `--resume` supported through an explicit reload after a divergence confirmation |
| Caddy route layout | Route import directory; the module inserts the `import` line once after confirmation and offers it again when it is missing |
| Caddy with `admin off` | Write with a confirmed container restart |
| An unrelated invalid file | Block with path, line and error; module-owned files can return to their last verified bytes |
| Probe client and evidence | `wget` in the proxy container; a probe helper in a new tools image is a later milestone |
| Verification that cannot be observed | Traefik marker route as load evidence, removed afterwards |
| Stable `yaml/v4` | Keep the tested pin; upgrade as its own dependency change |
| Writable sources | Project directory through the agent's mount; other sources through the helper with the unchanged bind rules |
| Writes beside secret files | Allowed with opaque files, provider health check, verification and restore |
| Change size | One configuration file per operation |
| Unrecoverable operation | Visible `recovery-required` with manual steps and a confirmed Release action |
| Secret file names in the UI | Names shown to administrators, never content |
