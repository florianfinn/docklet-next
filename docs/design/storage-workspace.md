# Storage and file workspace

M8 shares one storage contract between hub, Direct agents, Edge agents and the
web adapter. Volume CRUD and workspace access retain their existing permissions;
backup/restore product features (M12), scanning (M11) and update watchers (M10)
remain outside this contract.

## Prune preview and execution

`GET /environments/{id}/volumes/prune` (`plan-volume-prune`) requires
`volumes:prune`. `all=false` selects only anonymous local volumes, identified by
Docker's `com.docker.volume.anonymous` label; `all=true` also selects named local
volumes. This follows Docker Engine API 1.42+ prune selection. Internal-labelled
volumes are excluded. Any reference from a running or stopped container excludes
a volume, including a workspace helper. Planning is read-only and does not stop
helpers. Candidates contain `name`, `driver`, `createdAt`, `reason` (`unused` for
named or `unused-anonymous`) and optionally `sizeBytes` when Docker knows it.
Unknown sizes are omitted, never represented as zero estimates.

`POST` on the same path retains operation ID `prune-volumes` and activity code
`volume.prune`, but now requires `{candidates: [{name, createdAt}]}`. The old
bodyless HTTP call is rejected; there are no live web callers to preserve. Plans
are stateless snapshots, not stored approvals or tokens. A submitted list may be
edited, but cannot broaden deletion beyond the submitted names. Every candidate
is inspected again, must have exactly the submitted creation timestamp and be
unused by **all** containers immediately before a non-forced Docker remove.
Internal and non-local volumes are protected even for crafted requests. Duplicate
names are processed once. Docker remains the final usage guard against a
container created between re-check and remove. Same-name replacement is detected
at re-check; Docker offers no atomic compare-and-remove identity primitive, so
external removal/recreation after that check is an unavoidable race.

Execution returns `removed`, `skipped` (`in-use`, `not-found`, `changed`), `failed`
(name and stable code) and `spaceReclaimed`; one candidate failure does not abort
others. Reclaimed bytes sum known pre-removal Docker sizes of successful removes;
unknown contributions are zero, so this is a lower bound. No Docker-wide prune
call executes a submitted plan. Internal scheduled/system prune callers retain
their existing API but obtain a fresh plan and execute that exact candidate list.

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
the same volume/workspace routes. Route coverage tests include actual tunnel
forwarding, not merely matching command names.

## One helper image per environment

`toolsImage` replaces `toolsImageRegistry`. It is a complete image reference,
with `TOOLS_IMAGE`, `TOOLS_IMAGE_FILE` and `TOOLS_IMAGE__FILE` overrides like other
settings. An agent's local override wins over hub-synchronised values. All volume
workspace, rename, migration, backup, restore and Trivy helper callers use this
same resolver. The shared default is
`ghcr.io/florianfinn/docklet-tools:0.1.0`; #180 adds the digest after first
publication. Release defaults must then include SemVer and digest. An operator
may choose another complete reference, including a mutable `latest` tag.

Updates require a reviewed default-bump PR and explicit operator rollout; no
watcher silently replaces the image. Pull a configured image when absent; pull
failure fails the operation. The running hub/agent image is never a helper
fallback. Workspace helpers require the current ACFS protocol and return typed
code `volume.helper-incompatible` if ACFS is absent, invalid or too old. The
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
