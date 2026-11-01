<!--
Source: docklet-next/issues/286
Revision: b9b59ae92f1b09eb828940860971002eab9267b6
Retrieved: 2026-11-01
License: Apache-2.0
Deviations: Original hub archive copy, target draft test, delivery state and backups overview rationale.
-->

# Backup completion: hub archive copies, targets and overview

Decision for #286, #288, #289, #290, #291, #329 and the follow-ups #227 and #202
(M16, maintainer kickoff 2026-11-01).

## 1. Hub database archive copies (#286)

A hub backup policy may name one stored target. After an archive of that policy is
created, the hub copies the archive file to the target and records the copy state in
the existing run columns (`copy_target_id`, `copy_state`, `copy_error_code`).

- **Reachable targets**: SMB and NFS targets, and local or mount targets whose
  owning host is the hub's own host (`0`). Other targets are refused when the policy
  is saved (`422 backup.target_unavailable`, existing code). The UI offers only
  reachable targets.
- **Which archives** are copied: every archive created for a policy with a target,
  manual "back up now" for that policy and scheduled runs. Uploaded archives and
  archives created without a policy are not copied.
- **Order**: the copy starts after the run is saved as succeeded and the maintenance
  barrier is released (`finishRun` after `SaveRun`). The activity ends after the copy;
  a failed copy makes the activity end as partial with the typed copy code, like a
  pruning failure. The local archive is never changed or removed by a copy.
- **Mechanism**: on host `0` the hub resolves the target, builds its mount plan (local
  driver volume for SMB/NFS, host bind plus device witness for mount targets, as for
  volume repositories), starts a tools helper with the target at `/target`, streams a
  one-entry tar of the archive through the Docker archive API to
  `<target path>/docklet-backups/hub/<archive>.tar.partial`, checks the size and
  renames it to `<archive>.tar`. No client path, upload filename or runtime path is
  involved. The hub archive directory stays a container path; nothing bind-mounts it.
- **States**: `pending` when the copy starts, then `succeeded` or `failed` with
  `copy_error_code` (`backup.copy.mount-failed`, `backup.copy.write-failed`,
  `backup.copy.insufficient-space`, `runtime.interrupted`). A crash leaves `pending`;
  startup reconciliation sets such rows to `failed` with `runtime.interrupted`.
  Hub database restore keeps its existing reset to `none`.
- **Maintenance**: the copy worker registers with the maintenance drain. A later hub
  backup or restore barrier cancels and drains it; the run becomes `failed`
  `runtime.interrupted` and the partial file is removed best effort.
- **Retention**: when local retention removes a scheduled archive whose copy
  succeeded, the hub removes that copy from the same target first (best effort; a
  failure is logged and does not stop local retention). Deleting an archive by hand
  removes only the local file; the confirmation says so. Files the hub did not record
  (other hubs, partial files) are never touched.
- **Target references**: retiring a target that a hub policy names is refused under the
  existing per-target write fence (`409 backup.target.in-use`); the reference check
  reads `system_backup_policies.target_id` in the same fenced step.
- **Restore** stays local and from uploads; a copy is restored by downloading it from
  the share and uploading it.

## 2. Target connection test in the draft (A15)

`POST /backup-targets/test` (`test-backup-target-draft`, administrator) accepts
`{ target: <create or update body>, probeHostId, targetId? }`. `target` is validated
with the existing create/update validator, where `target.hostId` keeps its meaning as
the owner of a local or mount target. `probeHostId` names the host that executes the
probe: for local and mount targets it must equal `target.hostId`
(`422 backup.target.probe-host-mismatch`); for SMB and NFS any enabled host may run it.
The probe mounts the definition and writes and removes a probe file. It never creates a repository, a recovery key, a target row or a check
row. The answer is synchronous within 60 seconds: `{ ok, code? }` with the existing
check codes.

- With `targetId` and an omitted secret, the stored secret of that target is used (edit
  dialog). Kind and location changes are refused for existing targets, as today.
- The probe uses a transient volume name `docklet-target-probe-<random>` and removes the
  volume and helper afterwards, also on failure and cancellation.
- **Host**: local and mount probes run on their owner (no selector); SMB/NFS
  probes run on a selectable host, default the hub host. For a remote host the hub itself
  calls the agent route `POST /environments/0/volumes/backups/targets/probe` (Edge command
  `volume.backup.target.probe`, registered explicitly) with the definition, secret
  included, exactly like its own target-set delivery. The hub proxy refuses
  client-originated requests to that agent route, like the target-set receiver. The
  agent re-validates the definition and checks that it is the named executor.
- The existing saved-target test stays unchanged.

## 3. Delivery state per host (#289)

The hub keeps two facts per host in its existing setting `backupTargetDelivery:<host>`
(JSON, no migration):

- the **accepted receipt**: authority, the host's registration revision (§4), set
  version, content hash, delivered target IDs with config versions, per-target conflicts
  and time;
- the **last attempt**: time, set version and outcome (`ok`, `backup.target.sync-failed`
  when unreachable, `backup.target.stale-set` or `backup.target.wrong-host` when
  refused).

Both facts carry the authority and registration revision they were observed under. A
receipt or attempt is stored only if that identity equals the current one, and a receipt
only if its set version is not older than a stored receipt of the same identity;
observations from an earlier identity are discarded. On read, a stored receipt or attempt
whose identity differs from the current one is ignored by every projection branch, so a
conflict recorded before a rotation never shows after it. The old `backupTargetDeliveryError:<host>`
setting is no longer written or read. `GET /backup-targets` exposes, per target and
usable host, `delivery: { state, code?, at? }`, projected in this order:

1. `conflict`: the receipt lists a location conflict for this target
   (`backup.target.location-conflict`), or the last attempt for the current set was
   refused (`stale-set`, `wrong-host`);
2. `current`: the receipt is for the current authority and registration and lists this
   target with its current config version;
3. `pending`: otherwise (no receipt for the current version yet, for example a change
   saved while the agent was stopped); `code` carries the last attempt's failure.

A host that is offline but already holds the current version stays `current`. An agent
holding a set from another authority accepts the new authority (existing behaviour),
which yields `current`.

## 4. Registration revision (#288)

Environment list, detail and change events expose `registrationRevision`, the existing
non-secret `credential_revision` counter. It changes on Direct credential rotation, on
Edge re-registration and on a hub-local URL change. If Edge re-registration does not
bump it today, the registration path bumps it. The web host key includes it, so a
backup response held across an otherwise identical rotated row is discarded.

## 5. Default retention for new plans (B8)

Hub setting `backupDefaultRetentionCount` (0–3650, default 7, 0 keeps all) in
Settings › Backups. The web uses it as the initial count of a new volume or bind plan;
existing plans and API calls without the field are unchanged. No size limit.

## 6. Backups overview (A19, #329)

`/backups` follows the approved UI:

- figures over entries of available hosts: *plans* = entries with a policy,
  enabled or manual; *succeeded* / *failed* = planned entries whose latest finished
  backup succeeded / failed (partial counts as failed); *without backup* = unprotected
  entries;
- unprotected: an eligible volume or bind (directory or regular file) with at least one
  consumer and no plan. Entries of a host whose data is unavailable are shown as unavailable and enter no
  figure; ineligible binds (system, Docker, Docklet, special files) and binds of unknown kind
  are not counted; removed entries with retained history appear in the table, not in the
  unprotected count;
- "Back up now" per row runs the plan's manual backup with the plan's stop setting; it is
  disabled while the host is offline or the entry is busy;
- targets panel: every stored target with kind, address and the newest check result;
  a link to Settings › Backups.

## 7. Web polish

- **#290** Direct and hub-local downloads use a browser download (anchor to the
  authenticated download URL, as for hub archives) and are not buffered in the page
 . Edge keeps the buffered path with the early 512 MiB refusal and typed page
  messages. This changes #290's "messages unchanged" for Direct: a failed Direct download
  shows the browser's download error; refusals after activity start are also visible in
  the activity, earlier refusals (snapshot not found, sign-in expired) only as the
  browser error. The adapter's download call returns no Blob on the anchor path.
- **#291** Manual sends `enabled: false` with the stored cron unchanged; a disabled plan
  reads back as manual with its cron preserved; the card shows one control for it.
- **#227** Volume create and rename failures other than the typed ones return
  `500 volume.operation-failed` with a neutral message; Docker's text stays in server
  logs and out of responses and activity messages. Provenance notes are corrected.
- **#202** Scheduled prune reports a busy volume as skipped (in use) like manual prune;
  the busy message uses neutral wording and a translated code; the storage note and
  import grouping are corrected.

## 8. Break map

| ID | Entry or state | Decision | Test (red mutation) |
| --- | --- | --- | --- |
| BC1 | copy `pending` at crash; restart without replay | startup sets `failed` `runtime.interrupted`; hub restore resets `none` | restart test with a pending row (skip reconciliation) |
| BC2 | copy failure paths (mount, write, ENOSPC, cancel, terminal save failure) | local archive and its run unchanged; run `succeeded`, copy `failed` with code; activity partial; a failed copy-state write never deletes a committed archive | failure table with archive hash check (delete local archive on failure) |
| BC3 | policy target admission and retirement | unreachable target → 422 on save; retiring a target named by a hub policy → 409 under the write fence | service tests (drop the hub-policy reference check) |
| BC4 | target file atomicity and retention | write `.partial`, verify size, rename; retention removes only recorded `<archive>.tar` copies; never partial or foreign files | helper exec fake records commands (write final name directly; delete by glob) |
| BC5 | maintenance barrier vs running copy | copy registers with the drain before effects; a barrier cancels it and waits; no copy runs during maintenance | real maintenance gate, cancellation-aware blocked copy, latch at settle: assert cancel observed and copy settled before the drain returns (unregistered worker → ordering assertion fails) |
| BC6 | target credentials | SMB secret never in DTOs, activities, logs or copy error codes | redaction test over the copy path (log the mount options) |
| BC7 | draft test entry and transport | `{target, probeHostId, targetId?}`; existing validator on `target`; owner = executor for local/mount; no target, check or key writes; transient volume removed on success, failure and cancel; SMB draft, owner-local draft and mismatch through real hub → agent httptest and Edge command; client requests to the agent probe route refused by the hub proxy | handler + transport tests with call counters (remove the proxy refusal; drop owner enforcement → probe writer observed; keep the transient volume) |
| BC8 | delivery receipts, concurrent and late writers, projection | receipt only for current authority and registration and not older; held sender responses: old-registration result after re-pairing discarded, older set after newer stored; same-version success → offline → recovery; per-target location conflict wins; Direct/Edge refusals carry typed codes | real `SyncHost` + SQLite with held sender responses, assert persisted receipt and projected states; a persisted conflict and failed attempt followed by rotation read before any new delivery → pending; new authority with a lower counter accepted (write unconditionally; ignore registration revision), also under `-race` |
| BC9 | registration revision | changes on Direct rotation and Edge re-registration, never secret; web discards a held response across a rotated identical row | producer test + web adapter test (exclude revision from the host key) |
| BC10 | default retention setting | 0–3650 validated; used only for new plans | settings + web test (drop the range refusal → 4000 stored; ignore the setting → 7 shown) |
| BC11 | Direct download transport | Direct and hub-local use the anchor and return no Blob; Edge keeps the bounded path, early `download-too-large` and typed page errors | adapter + caller test per transport (anchor for Edge) |
| BC12 | manual / disabled plan round trip | stored cron preserved through manual and save | mapping round-trip test (fixed `0 0 3 * * *`) |
| BC13 | raw daemon text | create and rename internal failures → neutral code; no daemon text in response or activity | handler tests both paths (return `err.Error()`) |
| BC14 | scheduled prune busy volume | skipped `in-use`, not failed | prune scheduler test (map busy to failed) |
| BC15 | new routes and codes | OpenAPI, types/, generated DTOs, EN/DE, contract v12 includes the probe route and command | contract check; red proof reused from BC7 (unregistered probe command) |
| BC16 | overview counting | §6 population: unavailable host in no figure; ineligible and unknown-kind binds not unprotected; manual plans count as plans; partial counts as failed | web test with a delayed and an unavailable host (count unknown as unprotected) |
| BC17 | draft test result in the dialog | result keyed to the submitted draft and probe host; a response held across a field edit, host change or close/reopen is discarded; failure shown for the current draft | web test with held adapter responses (drop the identity check) |

## 9. Limits

- Copy targets are limited to targets the hub's own host can mount.
- A copy proves size, not content; restore validation checks the archive.
- Docker operators can read SMB credentials in local-driver volume options (existing
  accepted boundary).
