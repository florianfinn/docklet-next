# M12 backups and restore acceptance — 2026-10-29

The accepted package is main `7bf070e50b1e9e2dfb10bb44cdf8d3d66aad32c2`, the
squash merge of the
[M12 closing PR #293](https://github.com/florianfinn/docklet-next/pull/293).
Its tree equals the reviewed feature head `503d008`.

This file records sandbox acceptance. It is not a product release or a
production deployment.

## Integrated steps

Under the milestone process, steps were pushed to the feature branch as commits,
without step PRs. CI ran on every pushed head. Each risky step had interim
reviews from a different provider than its implementation.

| Issue | Step | Review outcome |
| --- | --- | --- |
| #245 | Actor host loss ends the process; health fails; restart policy in the install snippets | R1 failed with 3 blockers, verification passed |
| #283 | Volume backup and restore lifecycle, backup targets, migration 012, agent contract v9 | Design reviews before implementation; R1 failed with 3 blockers and 4 nonblockers, verification passed |
| #284 | Live web UI: volume Backups tab, restore dialogs, `/backups` overview, Settings › Backups, dashboard | R1 failed with 5 blockers, 6 nonblockers and one product question (AUTO-17), verification passed |
| #285 | Closing integration (#293) | Opus whole-milestone review passed with 3 nonblockers; the Sol closing review and four verification rounds failed with one blocker each, the fifth verification passed on `503d008` |

## Defects found before acceptance

Three candidate runs on feature heads used the full acceptance matrix. Two of
them found seven product defects that tests had missed; the third passed:

| Run | Defect | Fixed in |
| --- | --- | --- |
| r1 | An existing empty agent token row blocked target synchronisation. | `e5218d7` |
| r1 | The restore-files root request was absolute and was rejected. | `e5218d7` |
| r1 | A contract v8 action refusal lost its declared compatibility code. | `30d80ec` |
| r3 | An oversized upload session returned `500` instead of `413`. | `bdb4456` |
| r3 | Central routes turned an agent's `409 volume.backup.busy` into a generic interruption. | `bdb4456` |
| r3 | The hub-local process stayed up after actor host loss. | `bdb4456` |
| r3 | Backup entries from a manual start lacked `stopContainers`. | `bdb4456` |

The closing reviews found two error classes:

- The host overview built backup entries without activity, policy, container
  stop and type. `95db532` made it share the entry projection.
- Diagnostic text (paths, server addresses, storage errors) reached backup
  entries, policy last runs, backup activities, policy and repository errors and
  scheduled-run history. Each verification round found one more instance;
  `4a7f78d`, `990c3dd`, `ed88e95` and `503d008` replaced them with stable
  `volume.backup.*` codes. Instances only in shared code that M12 did not change
  on a backup path were left out of this milestone (AUTO-19).

## Accepted scope and method

Hub and agent images were built from the exact package tree and deployed
together to the disposable sandbox after data backups, with the `unless-stopped`
restart policy from the install snippets. The sandbox had the hub-local host,
one Direct agent and one Edge agent.

Package checks:

- migrations 000–012 applied;
- `/api/health` UP;
- the served bundle hash matched the local build;
- both agents online and compatible at contract v9;
- the backup engine image reference matched the pinned digest.

The run applied the agreed matrix through the production UI with Playwright
Chromium, in German and English at 1440 and 390 px, on every applicable host
type:

- plans with schedule, retention and container stop, and next runs in the host's
  time zone;
- manual and scheduled backups, including a Direct backup while the hub was
  stopped;
- retention, deletion and the refusal to delete a pinned safety backup;
- full restore, restore of selected files, restore as a new volume, download and
  upload restore, with checksums against the restored snapshot;
- refusal of crafted archives (parent and absolute paths, device nodes, the
  declared size limit) before any change;
- busy refusals carrying the running activity;
- restarts of the owning process during a backup, a restore and a restore into a
  new volume, followed by recovery from the safety backup;
- the `/backups` overview, list column, badges and Dashboard entries, with an
  offline host shown as unavailable;
- refusal of contract v8 agents;
- aligned 38 s pauses: the process exits, restarts and the host is back online
  within two minutes, then a later scheduled backup runs;
- backup targets: local directory, host mount and SMB, with per-host tests,
  credential rotation, refusal to delete a target in use and synchronisation of
  an offline host on reconnect;
- recovery keys behind confirmation; the Rustic CLI opened a target repository
  with the downloaded key;
- the hub database backup, unchanged.

Two extra checks covered the fixes after the last candidate run. The overview
entries of a manual backup matched the history entries on Direct and Edge. On
every host type, a backup failing on an SMB target with a wrong credential
showed only a stable code in history, policy last run, activity detail and UI,
and no server address, share or storage error.

## Result

The acceptance runner concluded **accepted**. 69 cells passed, 6 do not apply,
and the 3 NFS target cells were excluded under the agreed rule X5: the sandbox
kernel cannot start an NFS server (`rpc.nfsd` could not open its sockets), and
the SMB cells passed on every host. Both extra checks passed.

No product defect was found. Failed first attempts were harness faults: a
missing private directory, assertions beyond the matrix, readiness conditions
and missing target fixtures. Each cell passed within the permitted reruns, and
all attempts are retained in the private evidence.

Measured in the sandbox with small test volumes, a backup took 4–6 s, or 34–35 s
when it stopped containers. A full restore took 37–38 s.

## Limits

- NFS targets were not exercised live (X5). Unit tests cover NFS option
  validation and mount arguments.
- No real NAS was mounted at `/backups`; a bind mount stood in (X2).
  Daylight-saving changes and host time-zone changes were not exercised (X3).
- Chromium only, over HTTP/1.1 through the sandbox's public proxy. Across all
  attempts the browser recorded 254 `net::ERR_NETWORK_CHANGED` transport errors;
  the final observations had none.
- No load or large-throughput measurement; Direct downloads have no byte cap
  (#290).

## Open follow-ups

#286, #287, #288, #289, #290, #291, #292, #294.
