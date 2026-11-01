<!--
Source: docklet-next/issues/327
Revision: b9b59ae92f1b09eb828940860971002eab9267b6
Retrieved: 2026-11-01
License: Apache-2.0
Deviations: Original bind-mount path rules, file access and backup rationale.
-->

# Bind mounts: path rules, files and backups

Decision for #327, #328 and #287 (M16, maintainer kickoff 2026-11-01).

Bind mounts get the same file browser and the same backup lifecycle as named volumes
([storage workspace](storage-workspace.md), [volume backups](volume-backups.md)).
Docklet is a homelab tool with normally one administrator: every operation here is
administrator-only (M15 rule table), and the rules below prevent foot-guns and
ambiguous identities, not a hostile administrator. They are not a sandbox.

## 1. Identity

A bind key is `bind:` followed by the lowercase hexadecimal UTF-8 bytes of the source
path, for example `bind:2f7372762f617070` for `/srv/app`. The source path is the
`Source` that the Docker daemon reports for a mount of type `bind`. It is a path in
the daemon host's namespace, never in the agent container's namespace; Docklet never
opens it from its own process.

- A key is valid only if it uses lowercase hex, decodes to valid UTF-8, contains no NUL,
  is absolute, equals `path.Clean` of itself, is not `/` and is at most 255 bytes long.
  Anything else is `400 volume.bind.invalid-key`, checked before any Docker call at the
  hub proxy, the agent handler and the Edge command entry, for path segments and for
  names carried in bodies (create, rename, prune candidates).
- Docker volume names cannot contain `:`, so a key never collides with a volume name.
  The key is the identity wherever a volume name is used today: routes (`{volumeName}`),
  locks, helper registry, singleflight, engine admission, policy, backup and restore
  rows, scheduler job IDs, Rustic labels and container labels. Identity fields keep the
  key; only display text uses the source path.
- One key per literal path. Two literal paths that reach the same directory (symlinks,
  Unraid `/mnt/user` versus `/mnt/cache`) are two keys. Docklet does not resolve host
  symlinks (documented limit).
- No schema migration: keys fit the existing `volume_name` columns, which carry no
  pattern constraint. The web inventory already uses `host~bind:<hex>`; the container and
  stack pages switch from their separate `bind-<host>-<slug>` links to IDs built from the
  daemon-reported source, never from a guessed Compose path.

## 2. Path rules

One package (`backend/internal/volume/bindpath`) owns the rules; files, backups, restore
and the bind list call it. Rules are evaluated on the host that executes the operation
(hub-local, Direct or Edge agent) against its own Docker daemon. "Overlap" means equal,
below or above, compared segment-wise (`/etcetera` does not overlap `/etc`).

| Rule | Refuses | Code |
| --- | --- | --- |
| R1 system | `/` and any path overlapping `/proc`, `/sys`, `/dev`, `/etc`, `/run`, `/var/run`, `/boot`, `/var/lib/docker`, `/var/lib/containerd` or the daemon's `DockerRootDir`; ancestors are refused too, so `/var/lib` or `/srv` above a Docker root under `/srv/engine` is denied | `422 volume.bind.path-denied` (`reason: system` or `docker`) |
| R2 Docklet | a path overlapping a bind source of a Docklet hub or agent container (self labels) or the host path of a stored local or mount backup target of that host | `422 volume.bind.path-denied` (`reason: docklet`) |
| R3 observed | a key without stored policy, backup or restore rows that no workload container on the host (any state) currently bind-mounts with exactly that source | `404 volume.bind.not-mounted` |
| R4 type | a source that is missing, or neither a directory nor a regular file (socket, device, FIFO), found by the source witness (§2.1) | `404 volume.bind.source-missing`, `422 volume.bind.unsupported-type` |

- **Workload container**: any container that is not a Docklet hub or agent container and
  not a Docklet operation helper. Operation helpers are the workspace helper, backup
  engine, restore writer, source witness and target probe containers; they carry the
  existing helper or operation labels. They never count for R2, R3 or consumers, so an
  idle workspace helper for P never blocks the next operation on P. Startup orphan
  cleanup removes stale operation helpers by the same labels.
- **Which operations check what**: source-accessing operations (file browser
  reads and manifests, capture, replace, selected-file and upload restore) apply R1, R2
  and R4; admissions of new keys (first browse, first manifest, policy create, first
  manual backup) also apply R3. Operations that do not mount the source (backup list,
  download, snapshot browse, delete of a backup, policy disable or delete, overview)
  stay available for stored keys even if the path later becomes protected, so recovery
  downloads remain possible. By-ID routes (download, delete, snapshot browse) resolve
  the row's key first and then apply the same rule set.
- Single-file binds of regular files get the single-file mode (§2.2). The Docker
  socket and other special files are refused by R4 (and usually by R1 below `/run`).

### 2.1 Source witness (R4)

Every source-accessing operation mounts the source as a structured mount of type
`bind` (Docker `Mounts` API, never `Binds` strings), so Docker never creates a missing
host directory; a missing source fails container creation and maps to
`source-missing`.

- The witness decides the kind: `directory` or `file` (regular file), at every operation.
  The operation then uses the mount layout of that kind (§2.2). No kind is stored: an idle
  workspace helper whose layout differs from the witnessed kind is removed and recreated
  before use. A path whose kind changed since a snapshot is handled in §2.2 (restore).
- The workspace helper is created only after the witness; directory helpers mount the
  source at `/volume`, file helpers at `/volume/<basename>`.
- Capture, replace restore, selected-file restore and upload restore first start a
  tools-image source witness with the source at `/source`, running
  `test -d /source || test -f /source` (and reporting which), immediately before the
  engine or writer container is created. A special file fails with `unsupported-type`, a
  stat or exec failure with `volume.helper-unavailable`. No engine or writer container
  is created in either case.
- As for the accepted repository device witness, the host path can change between the
  witness and the engine create, because Docker resolves each bind at its own create.
  That window is accepted and documented.

### 2.2 Single-file binds

A regular-file bind uses the same key, rules, routes, locks, plans and activities as a
directory bind; only the mount layout and the write path differ. The tools image and its
ACFS protocol stay unchanged.

- **Layout**: helper and engine mount the file at `/volume/<basename>` (structured bind);
  `/volume` then lists exactly that one entry, so ACFS list, read and download work
  unchanged and the web shows the single-file mode.
- **Commands**: the pinned tools image has exactly `sh` (ash with `printf`), `sleep`,
  `find` (no filters), `gzip`, `stat` (with formats), `readlink`, `head` (with `-c`), `rm`,
  `mkdir`, `mv`, `rmdir`, `mktemp`, `tar` and `test` (`deploy/tools/applets.txt`); there is
  no `cat`, `cp`, `dd` or `du`. Every command below uses only these.
- **Writes**: a manifest for a file key may contain only `update_file` for that one entry;
  anything else (create, upload of another name, rename, move, delete) is
  `409 volume.bind.file-operation-unsupported` before staging. The revision and baseline
  guards run first, unchanged, and nothing touches the live file before they pass. The
  in-place write then runs as root in the existing helper, in this order:
  1. the staged content (existing staging under `/tmp/arcane-workspace`) is checked with
     `stat -c %s` against the expected length;
  2. the live file is copied to a private backup in `/tmp` with
     `head -c <live length> /volume/<basename> > <backup>` and the backup length checked;
  3. `head -c <new length> <staged> > /volume/<basename>` writes in place (same inode,
     owner, group and mode; running applications keep their open file) and the live
     length is checked;
  4. if step 3 fails or the length differs, the backup is written back the same way
     (`head -c <old length> <backup> > /volume/<basename>`) and checked; the response is
     the original error, or `500 volume.bind.recovery-failed` if the write-back failed.
  The generic workspace rollback, which unlinks entries, is never used for file keys. A
  process crash during step 3 or 4 can leave a partial file (documented limit).
- **Capture**: the engine mounts the file read-only at `/volume/<basename>` and Rustic
  captures the directory `/volume` with `--as-path /` (the source argument is `/volume`,
  decoupled from the mount destination), so the snapshot holds `/<basename>`. Safety
  snapshots use the same construction. Retention, download and snapshot browse are
  unchanged.
- **Restore**: only replace (selected-file restore and restore as new are not offered;
  upload restore is not offered for file keys). Consumers are stopped as for directories
  and a safety snapshot is taken. Still in the durable `preparing` phase, after the
  safety snapshot and before `writing` is persisted, Rustic restores the snapshot into a
  labelled scratch volume with the explicit destination `/scratch` (the file is
  `/scratch/<basename>`) and a tools helper checks that it is a regular file and the only
  root entry. A mismatch fails preparation with `422 volume.bind.snapshot-kind-mismatch`:
  the live file is untouched and the existing preparing settlement restarts the recorded
  consumers. Only then is `writing` persisted and the writer copies the file in place
  with `head -c` and a length check, as in step 3.
  Inode, owner and mode of the live file stay. A failed write keeps the restore failed
  with consumers stopped and the safety pin, as for directories (§5); the safety snapshot
  is the recovery copy. The durable phases, settlement and responsibilities are those of
  §5.
- **Kind changes**: a directory restore of a snapshot taken while the path was a file is
  not detected (no kind is stored and no migration is added) and replaces the directory contents with that one
  file; the safety snapshot taken before the restore recovers it. This is a documented
  limit for the rare path that changes between file and directory.
- Consumers of a file key P are containers binding P itself or a directory above P.

## 3. Bind list

`GET /environments/{id}/volumes/binds` (`list-volume-binds`, administrator, Edge command
`volume.binds.list`) returns one entry per distinct bind source of workload containers
on the host:

`{ key, source, kind: directory|file|unknown, eligible, reason?, consumers: [{containerId, name, destination, readOnly, running}], lastSnapshotSize?, lastSnapshotAt?, measuredSize?, measuredAt?, measuredPartial? }`

- `eligible` applies R1–R3 and `kind`; `reason` is the refusal code of the first failing
  rule. `kind` comes from a Docker archive stat (`ContainerStatPath`) of one consumer's
  mount destination; any failure gives `unknown`, which stays eligible and is decided by
  the witness when an operation runs. The stat result is cached per container ID and
  destination for the container's lifetime.
- The list reads possibly shared container rows; it copies the mounts it needs and never
  sorts, appends to or edits shared slices or label maps (BM12).
- `lastSnapshotSize` and `lastSnapshotAt` come from the latest successful snapshot;
  `measuredSize` and `measuredAt` from the last on-demand scan (§3.1) since the agent
  started. The web shows the newer of the two with its date and source.

### 3.1 Size scan on demand

`POST /environments/{id}/volumes/binds/size` (`measure-volume-bind`, administrator, Edge
command `volume.binds.size`) with `{ key }` measures one bind now.

- It is a source-accessing admission: R1–R4 apply. A tools helper mounts the source
  read-only at `/source`. For a file the size is `stat -c %s`. For a directory the agent
  lists every path with `find /source` (streamed exec output), then reads
  `stat -c '%d %i %h %s %F'` for batches of at most 500 paths per exec and sums in Go:
  regular files only, each device/inode pair once (hard links), entries on another device
  than `/source` skipped (nested mounts are not counted). The result is the apparent size
  in bytes. Paths that cannot be read or parsed (for example names containing a newline)
  are skipped and the result is marked `partial`. No lock is taken; the scan only reads.
- Concurrent scans of the same key share one run (singleflight); the shared run is
  cancelled only when its last waiter leaves.
- The request waits at most 120 seconds; a longer scan is stopped and its helper removed,
  answering `504 volume.bind.size-timeout` (the UI suggests the snapshot size instead).
  Cancellation of the request also stops and removes the helper.
- The agent keeps the last result per key in memory (`measuredSize`, `measuredAt`,
  `measuredPartial`); it is not persisted.

## 4. Files

The volume workspace helper mounts a bind key's source at `/volume` through a structured
`bind` mount and is otherwise the same helper, ACFS protocol, revision and baseline
guards, upload limits and rights (`volumes:read`, `volumes:upload`, `volumes:delete`).

- Volume-only steps are replaced for keys: `VolumeInspect` support checks become R1, R2,
  R4 (and R3 for admissions); the Arcane refusal of bind-backed local volumes stays for
  volumes.
- Write identity keeps the existing rule, with a bind-aware consumer lookup (bind source
  equal to the key's path) instead of the volume-only resolver: consumers whose
  configured user is empty, named, root or unparsable are skipped; if the remaining
  numeric identities agree (same user string), writes use it; if a consumer cannot be
  inspected, none remain or they disagree, writes use the owner of the mount root; if
  that stat fails, root. Non-root ownership is not promised for every image.
- Helper registry, singleflight and locks are keyed by the key. Bind helpers carry the
  existing helper label plus the key, so usage reads, idle stop and startup orphan
  cleanup recognise them.
- Not offered for keys: create, delete, rename, prune and restore as new volume. Their
  routes refuse a key with `409 volume.bind.unsupported` before any Docker call; a prune
  request containing a key is refused as a whole.

## 5. Backups and restore

Bind keys use the same plans, targets, retention, safety snapshots, durable restore
phases, locks and activity ownership as volumes. Differences:

- Capture mounts the source read-only at `/volume` in the engine container (structured
  `bind`) after the source witness; the Rustic label is the key, so snapshot paths and
  tags keep their format.
- Consumers of key P are running workload containers with a bind whose source overlaps P
  (equal, below or above), deduplicated by ID. Capture with "stop containers" and replace
  restore stop and restart exactly these by recorded ID, as for volumes. A Docklet hub or
  agent container mounting an overlapping path refuses the restore before effects
  (`409 volume.restore.self-mounted`). Named volumes are not part of the overlap test.
- Operations on overlapping keys are not mutually excluded; each key has its own lock. A
  capture of `/srv` and a file edit in `/srv/app` can interleave (documented limit).
- Restore writes the numeric owners, groups and modes recorded in the snapshot (Rustic
  restore with `--delete`, or the upload writer's `tar -xp`); Docklet adds no ownership
  rewrite. Selected-file and upload restore follow the volume rules. Restore as new
  volume is unsupported for keys.
- Startup settlement is unchanged and source-independent: interrupted captures settle
  as failed after writer removal is confirmed, a `preparing` restore restarts only the
  consumers it stopped, a `writing` restore keeps its consumers stopped, its pending
  responsibility and its safety pin, and an unconfirmed writer removal fences the key.
  Recovery never mounts or inspects the source. The next source-accessing
  operation applies R1, R2 and R4.
- Activities, notifications, history rows and the overview show the source path, never
  the raw key; identity fields keep the key.

## 6. Transport, contract and rights

- Direct and Edge reuse the volume workspace and backup routes and Edge commands with the
  key in `{volumeName}`; only `volume.binds.list` and `volume.binds.size` are new and are
  registered explicitly in the command map. The workspace handler's error translator and the central backup
  `RemoteProblem` allowlist (`backuptarget/agent_access.go`) carry the new `volume.bind.*`
  codes with their status and reason, so Direct and Edge return the same typed refusal.
- Older agents reject keys, so agent contract v12 (minimum v12) includes the bind list
  and size routes and the changed key validation. Hub and agents upgrade together.
- Every bind operation is `Rule{Kind: Admin}` in the M15 rule table; the completeness
  test covers the new route. Simple users never see volumes or binds.

## 7. Break map

Each row needs a test and one red mutation that fails an assertion (not a compile error,
`-race` report alone or timeout). Rows crossing hub → agent or the Edge tunnel need one
test through the real sender and receiver.

| ID | Entry or state | Decision | Test (red mutation) |
| --- | --- | --- | --- |
| BM1 | key parsing at hub proxy (`middleware/environment.go`), agent workspace and backup handlers, Edge command entry, body-carried names | uppercase hex, invalid hex, non-UTF-8, NUL, relative, non-clean (`/a/../b`, `/a/`, `//a`), `/`, > 255 bytes → 400 before Docker | table test through real hub → agent httptest and the Edge event path, zero Docker calls (drop the clean-path check) |
| BM2 | R1 system paths incl. ancestors and `DockerRootDir` from `Info` | segment-wise overlap; `/etcetera` allowed, `/etc/x` and `/var/lib` denied; custom root `/srv/engine` denies `/srv` and `/srv/engine/x`, allows `/srv2` | unit table + daemon fake with a custom root, refusal before any writer (raw string prefix; drop the ancestor branch) |
| BM3 | R2 Docklet data and target paths; helper exemption | hub/agent data source and a stored local target path above, equal or below → denied before helper/engine creation; browse → second browse → save → capture on P with an idle workspace helper all succeed | sequence test with self-labelled container, target row and idle helper (exact match only; count helpers as protected) |
| BM4 | R3 observed vs stored rows; by-ID entries | crafted unmounted key → 404 for browse, manifest, policy create, first backup; key with stored rows keeps scheduled capture, list, download, snapshot browse and restore after container removal; a stored key whose path became protected: download and browse by ID succeed, capture/restore refused | handler tests with container-list fake and SQLite rows (skip R3 for every key; apply R3 to stored keys; apply R1/R2 to download) |
| BM5 | R4 source witness before writers | structured bind mounts only; witness responses missing / special file / directory / regular file / exec failure → `source-missing` / `unsupported-type` / directory layout / file layout / `helper-unavailable`; no engine or writer container on missing or special; an idle helper with the other layout is recreated | fake Docker records creates and mount layouts (drop the type refusal → engine create observed; switch to `Binds` → spec assertion) |
| BM6 | consumer overlap on capture-stop and replace restore | equal, parent and child sources stopped and restarted by recorded ID, deduplicated; sibling `/srv/app2` untouched for `/srv/app`; operation helpers ignored; Docklet container in set → 409 before effects | consumer test with mixed mounts and stop/start call log (exact match only; string prefix) |
| BM7 | key namespace in locks, helper registry, singleflight, admission, labels | `app` and `bind:<hex /srv/app>` never share a lock or helper; same-key contention still 409; bind helpers recognised by usage reads and orphan cleanup | latch-coordinated concurrency test (key collapsed to basename) |
| BM8 | persisted states for a key: interrupted capture, restore `preparing`, `writing`, unconfirmed writer removal, missing source at restart | settlement unchanged and source-independent; preparing restarts exactly its recorded IDs; writing keeps consumers stopped, pending responsibility and pin; unconfirmed writer fences the key; no recovery mount | restart tests on SQLite rows for a key (drop the preparing restart; drop the writing responsibility; bypass the writer fence) |
| BM9 | unsupported operations on keys | create, delete, rename, prune with a key candidate, restore-as-new → 409 `volume.bind.unsupported`, zero Docker calls | handler/body table (remove one refusal) |
| BM10 | display of keys | activity, notification, history and overview text use the source path; identity fields keep the key | producer test asserting no `bind:` in messages and the key in IDs (use key as display name) |
| BM11 | new codes, translators and contract | `volume.bind.*` in types/, OpenAPI, generated DTOs, EN/DE; workspace translator and `RemoteProblem` allowlist keep status/code/reason; one denied workspace and one denied backup operation through real Direct and Edge seams; contract v12 fixture/fingerprint; bind list through the real tunnel | contract check + seam tests (drop the code from the allowlist → generic 500 observed; unregistered command) |
| BM12 | shared read results | bind list and consumer discovery read shared container rows | unsorted shared fixture with a retained deep copy, two latch-coordinated concurrent callers on the real coalescing read, assert input unchanged; also under `-race` (in-place sort of shared mounts) |
| BM13 | rights | `list-volume-binds`, `measure-volume-bind` and reused routes administrator-only | M15 completeness test + user 403 before data (rule removed) |
| BM14 | no migration | keys stored in `volume_name`; M15 rows unchanged | upgrade compatibility test from an M15 database (ordinary evidence; no code guard to mutate) |
| BM15 | write identity for keys | bind-aware consumer lookup; skipped empty/named/root users; agreeing remaining identities → that uid:gid (fixture `1000:1000` + empty user, mount root `2000:2000` → `1000:1000`); disagreement, inspect failure or none → mount-root owner; stat failure → root | workspace test with consumer fixtures and tar header assertions (use the volume-only resolver → no consumers found) |
| BM20 | real applets | the exact commands of BM17, BM18 and BM19 run in the pinned tools image (non-empty, empty and binary file, inode/owner/mode kept; directory scan with a known total) | opt-in integration test (build tag) that runs the pinned image against a temp directory; executed once in the sandbox smoke, skipped in CI (ordinary evidence; the default-suite guard is BM17's applet-list check) |
| BM17 | file key writes | manifest with anything but one `update_file` → 409 before staging; stale revision or baseline → 409 with the live file untouched; in-place `head -c` write with length checks; an injected failure after a prefix reached the live file → write-back restores the original content, original error returned; owner/mode/inode unchanged; only applets of `deploy/tools/applets.txt` used | workspace test with an exec fake that executes against a temp directory and rejects commands outside the applet list (use `cat`; use the generic unlink rollback; drop the write-back) |
| BM18 | file key capture and restore | engine mount at `/volume/<basename>` read-only with Rustic source `/volume`; scratch restore to `/scratch` and root-entry check in `preparing`, in-place `head -c` write in `writing`; an invalid scratch root through the real `Lifecycle.Execute` → 422, live file untouched, consumers restarted; a partial live write → failed-partial, consumers stopped, pin kept; consumers of P and directories above P stopped/restarted; selected/upload/as-new refused | fake Docker recording mounts and the engine command line (source = mount destination; restore onto the file; move the root check into the write callback → 500 failed-partial observed; drop the parent-directory consumer) |
| BM19 | size scan | R1–R4 before the helper; `find` + batched `stat` summed in Go (regular files, hard links once, other devices skipped, unreadable → partial); injected deadline and cancellation stop the exec and remove the helper; singleflight cancelled only when the last waiter leaves; result in the list | Go aggregation table + fake Docker with blocking exec, latches and an injected clock (count hard links twice; count other-device entries; helper left after cancel) |
| BM16 | ownership on restore | numeric owner/group/mode of nested entries as recorded | restore test asserting recorded metadata on a nested file and directory (strip `-p` / ownership option) |

## 8. Limits

- Symlink aliases and different mount views of the same data are separate keys.
- Overlapping keys are not mutually locked; a restore stops overlapping consumers but does
  not wait for operations on other keys.
- The host path can change between the source witness and the engine create.
- In-place writes and restores of a single file are not atomic; a process crash can leave
  a partial file.
- A path that changes between file and directory is not detected at restore; the safety
  snapshot recovers it.
- Size scans count apparent sizes of regular files on the source's own filesystem.
- A size scan longer than 120 seconds is not supported; the snapshot size remains.
- External writers on the host are not fenced, as for volumes.
- Helpers and engines run as root on the host path. This is the administrator's own data;
  no further restriction is added.
