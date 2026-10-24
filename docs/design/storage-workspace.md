# Storage and file workspace

M8 shares one storage contract between hub, Direct agents, Edge agents and the
web adapter. Volume CRUD and workspace access retain their existing permissions;
backup/restore product features (M12), scanning (M11) and update watchers (M10)
remain outside this contract.

Explicit UI/API volume creation refuses an existing name on the target host with HTTP 409 `volume.exists`, while Compose deployments reuse existing volumes.

## Prune preview and execution

`GET /environments/{id}/volumes/prune` (`plan-volume-prune`) requires
`volumes:prune`. `all=false` selects only anonymous local volumes, identified by
Docker's `com.docker.volume.anonymous` label; `all=true` also selects named local
volumes. Volumes with mount options are excluded, as in
[Docker Engine prune](https://github.com/moby/moby/blob/v28.5.1/volume/service/service.go).
This follows Docker Engine API 1.42+ prune selection. Internal-labelled
volumes are excluded. Any reference from a running or stopped workload container
excludes a volume. Internal volume workspace helpers never count as consumers:
usage IDs, in-use flags, reference counts and dashboard counts all exclude them.
Other internal containers still count. Legacy helpers use the same recognition
rules as helper cleanup. Planning is read-only and does not stop helpers; a
helper-only volume can appear in the plan even during an active workspace request.
Candidates contain `name`, `driver`, `createdAt`, `reason` (`unused` for
named or `unused-anonymous`) and optional `sizeBytes`. This implementation omits `sizeBytes`: Docker's
`VolumeList` and `VolumeInspect` omit `UsageData`; obtaining it requires a
host-wide `DiskUsage` scan. Neither preview nor execution triggers that scan.
Use the separate volume-sizes operation when space estimates are needed;
unknown candidate sizes are omitted, never represented as zero estimates.

`POST` on the same path retains operation ID `prune-volumes` and activity code
`volume.prune`, but now requires `{candidates: [{name, createdAt}], all?: boolean}`. The optional
`all` records the preview selection in event metadata (default `false`); only the
explicit candidates determine the removal scope. The old
bodyless HTTP call is rejected; there are no live web callers to preserve. Plans
are stateless snapshots, not stored approvals or tokens. A submitted list may be
edited, but cannot broaden deletion beyond the submitted names. Every candidate
is inspected again, must have exactly the submitted creation timestamp and be
unused by all workload containers immediately before a non-forced Docker remove.
Internal and non-local volumes are protected even for crafted requests. Duplicate
names are processed once. Docker remains the final usage guard against a
container created between re-check and remove. Each candidate holds the same
per-volume workspace lock as rename, backup and file mutations through inspect,
helper cleanup, usage re-check and removal. After identity and protection checks,
execution stops the idle workspace helper under that lock before querying usage
again. Helper removal failure reports `volume.usage-failed` and leaves the volume
untouched; another candidate can still proceed. A candidate whose lock is already
held is skipped as `in-use`, without waiting for the workspace operation. Different-name
candidates remain independent.
Same-name replacement with a different `CreatedAt` is detected at re-check.
Docker reports `CreatedAt` with second precision: same-second recreation cannot
be distinguished. Docker also offers no atomic compare-and-remove identity
primitive, so external removal/recreation after that check is an unavoidable race.

Execution returns `removed`, `skipped` (`in-use`, `not-found`, `changed`), `failed`
(name and stable code) and `spaceReclaimed`; one candidate failure does not abort
others. Reclaimed bytes are currently unknown and reported as `0`, a lower bound;
execution does not measure volume data before removal. No Docker-wide prune
call executes a submitted plan. Manual system prune retains its existing API
but obtains a fresh plan and executes that exact candidate list.
Scheduled prune keeps its separate recoverable plan: it selects volumes from
Docker disk-usage reference counts and the internal/anonymous labels, without
excluding non-local drivers or mount options. It does not re-check `createdAt`
or hold the workspace lock, and stops helpers before removal. Alignment with
the manual volume contract remains follow-up work for the scheduled-prune owner.
If some candidates fail, manual and scheduled system prune retain successful
removals in their result and completion log while reporting the failure.

## Rename, delete and active workspace requests

Rename and delete acquire the per-volume workspace lock without waiting. A held lock,
including a download stream's read lock, returns a typed workspace-busy conflict
(HTTP 409) before touching the helper or source volume. After a download closes,
rename or delete can be retried. With the lock held, rename stops the idle helper before
validating the detached source and copying data. A failed helper removal aborts
rename. Real consumers, including stopped containers, still prevent rename.
Detached singleflight helper creation retains the initiating operation's existing
read or write lock until creation and helper registration finish. Cancellation
releases the request immediately; it cannot let prune, rename or delete remove
the volume before the late bind mount is created. Each waiter retains its own
lock reference; the final release unlocks the original lock without reacquiring
it. Image-pull deadlines still bound detached creation. Failed creation also
releases its references.
A helper left by a previous process is recognised by usage reads; startup's
orphan cleanup owns its removal. Docker's detached-source and non-forced removal
guards still protect volumes if an orphan remains.

## Workspace mapping and rights

All create, edit, upload, rename, move and delete gestures become one multipart
`PUT` to `update-volume-workspace` or `update-project-workspace`, containing one
JSON `manifest` and indexed upload parts. No extra file-mutation routes are
introduced. Preserve the producer's revision, path and upload-index conventions;
submit `fileTreeRevision` and surface conflicts before retrying from a fresh tree.
Reads use the list, file and download workspace routes. Volume reads and writes
first verify `acfs version`; incompatible helpers fail before accessing files.

| Operation | Rights |
| --- | --- |
| Volume list, sizes, counts | `volumes:list` |
| Volume detail, usage, workspace list/read/download | `volumes:read` |
| Create / remove / rename volume | `volumes:create` / `volumes:delete` / `volumes:rename` |
| Prune plan and execute | `volumes:prune` |
| Volume manifest baseline | `volumes:read` |
| Create file/folder, upload, update content | plus `volumes:upload` |
| Rename or move entry | plus `volumes:upload` and `volumes:delete` |
| Delete entry | plus `volumes:delete` |
| Restore file (existing wire operation; M12 UI) | plus `volumes:backup` |
| Project workspace reads | `projects:read` |
| Project workspace manifest | `projects:update` |

The hub enforces host-scoped rights before forwarding; the agent enforces the
same workspace manifest contract. Direct uses authenticated HTTP, Edge the
explicit command map with no Direct fallback. Hub and both agent modes register
the same volume/workspace routes. This intentionally includes
`volume.backup.import` (`POST /volumes/{volumeName}/backups/upload`) on Edge:
all registered storage routes are reachable; backup/import functionality remains
owned by M12. Route coverage tests include actual tunnel
forwarding, not merely matching command names.

## File baselines and freshness

Volume updates retain the whole-tree revision guard. An `update_file` with an
optional `baselineIndex` also compares the uploaded original bytes with the
current file, like project updates. A mismatch uses the existing HTTP 409
workspace conflict problem. Omitted baselines retain revision-only semantics.
Comparison happens at each ordered update boundary, after preceding manifest
changes, under the volume lock. A failed guard before the first mutation leaves
files untouched; a failure after earlier mutations uses the existing manifest rollback.
Reads are bounded to baseline length plus one. External processes do not share
this lock, so comparison and replacement have the same intervening-write limit
as project workspaces. The API schema, DTOs and ACFS protocol are unchanged.

After own gestures, the web consumer removes cached content and implicit adapter
baselines for touched paths and subtrees, including both sides of a rename or
move. A moved tab reads its new path; it cannot inherit a former occupant's
content. Untouched cached entries can advance after an own manifest without changing
query freshness timestamps. The adapter never advances implicit baselines; a
caller without an explicit baseline must read again after other writes.

Every tree reload, including conflict recovery, compares cached entries by size,
mtime, mode and directory/symlink type. Unchanged entries adopt the current tree
revision without extending content freshness. Changed clean entries are read
again, and changed closed entries are evicted; changed dirty entries retain their original bytes and revision until
conflict resolution. When a conflict reload finds the dirty tab's own entry
unchanged, it also clears that tab's conflict flag and permits saving the retained
draft with the refreshed revision and original content baseline. A subsequent
read can advance a dirty baseline only when its original bytes still match. Thus unrelated external changes stop causing
stale-revision conflicts after reload, while same-path edits remain protected
by revision and baseline guards. Entry identity is a metadata heuristic: an
external write that preserves size and mtime can evade both tree revision and
path identity. The producer's content baseline comparison remains the data guard
for those writes, including retries after conflict recovery.

A truncated tree (`fileTreeTruncated`) gives entries outside its listing no
identity. Tree reconciliation cannot prove those dirty entries unchanged, so a
conflicted tab may retain its old revision after unrelated external changes.
Clean tabs re-read their content; unconflicted dirty baselines can still advance
after a byte-equal read.
Project drafts follow the same freshness rule with their existing structural revision and content comparison contract;
an older changed draft never inherits a later own manifest's revision.

## One helper image per environment

`toolsImage` replaces `toolsImageRegistry`. It is a complete image reference,
with `TOOLS_IMAGE`, `TOOLS_IMAGE_FILE` and `TOOLS_IMAGE__FILE` overrides like other
settings. An agent's local override wins over hub-synchronised values. All volume
workspace, rename, migration, backup, restore and Trivy helper callers use this
same resolver. Settings persist and transport only explicit overrides; empty
means resolve `DefaultToolsImage` from the running hub or agent release when a
helper image is read. Hub settings forwarding sends that empty value or the
explicit override, so an agent uses its own release default; its local
`TOOLS_IMAGE*` override still wins. Empty environment variables do not mask a
synchronised explicit override. First start persists empty. Startup replaces
previously persisted shipped defaults (`ghcr.io/getarcaneapp/tools:latest` and
`ghcr.io/florianfinn/docklet-tools:0.1.0`) with empty, including values that were
explicitly set to those exact defaults. This migration list is closed: it holds
only values persisted before override-only storage. Release image bump PRs must
not extend it, so explicit rollback pins remain durable. Tests fix the list
to those two values and exclude `DefaultToolsImage` from retirement.
Other explicit references survive startup.
Image overrides must pass Docker reference syntax validation; empty resets the
override. The shared default is
`ghcr.io/florianfinn/docklet-tools:0.1.0@sha256:d387beb35707c5634dba1630b4153c5bfe0bd4529025297ce7c024c2f2fe9602`.
Release defaults include SemVer and the multi-platform index digest. An operator
may choose another complete reference, including a mutable `latest` tag.

Updates require a reviewed default-bump PR and explicit operator rollout; no
watcher silently replaces the image. Pull a configured image when absent; pull
failure fails the operation. The running hub/agent image is never a helper
fallback. Workspace helpers require the current ACFS protocol and return typed
code `volume.helper-incompatible` if `acfs version` exits unsuccessfully or
returns an invalid/old protocol. Docker transport, cancellation and other exec
failures instead return typed `volume.helper-unavailable`; they do not establish
image incompatibility. Both use HTTP 503 and occur before file access. The
legacy helper-container label is retained so cleanup recognises existing helpers;
it carries no image-selection policy.

## Compatibility and Before deploy

The changed prune request and new preview route require agent contract v5 and
minimum v5. The v4 fixture remains historical; the new fingerprint covers the
storage contract and inherited runtime contract. Upgrade hub and agents as one
release. Older agents are incompatible before forwarding or settings sync.
`tunnel/v1` is unchanged: only the command table and application contract change.
Before deploy, configure any former registry override as a full `toolsImage`
reference and ensure the published tools image is available. No database schema
migration is required. Exact-package Direct/Edge practical acceptance remains a
milestone release gate.
