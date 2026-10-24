# Images and updates

## Boundary and accepted decisions

This is the proposed M10 contract for #229, based on the registered producers at
`8de82c9cf4166915137077353851d5d1ff8edeb6`. It specifies implementation and review
gates; it does not claim that the adopted routes already enforce this contract.
The [API mapping](api-contract.md), [runtime control](runtime-control.md),
[stack authoring](stack-authoring.md), [storage contract](storage-workspace.md)
and [network contract](network-management.md) remain the neighbouring contracts.

The maintainer decided on 2026-10-24:

- Auto-apply is in scope. Containers and stacks have `off`, `notify` or `apply`
  policies. A ScheduledJob executes apply in the M4 hub time zone and creates a
  restore point before every applied update.
- A restore point pins the previous image and retains the deployed Compose and
  environment configuration. Restore redeploys that exact configuration and
  image state. **Volume data is not saved or restored**, and the UI says so.
- Agent/tools image watchers inform only. Rollout remains manual. An agent
  candidate never exceeds the running hub version; a newer available release
  explicitly requires updating the hub first.
- #227 and #202 remain outside M10. Every new or modified image, registry and
  update failure returns a stable typed code and neutral message. Raw daemon
  error text stays in server logs, subject to credential redaction.

M10 includes host-scoped pull, remove and prune; registry create/edit/delete/test
with encrypted, write-only credentials and agent sync; update checks; manual
container/stack update; policy/schedule execution; restore-point list/delete and
restore; run-based deployment history; agent/tools version hints. Direct and
Edge execute the same host-side guards. One operator and a few users remain the
scale; no new distributed scheduler, lock service or failover is proposed.

Vulnerability scanning UI (M11), volume snapshots/backups (M12), automatic
hub/agent/tools replacement, image build/patch/attestation UI, registry browsing
beyond pulls, Swarm, and broad error/output redesign are excluded. A running
system or sandbox is not changed by this design step. Existing excluded routes
are not made into UI capabilities by appearing in OpenAPI.

## Producer, transport and consumer

| Boundary | Existing producer and actual consumer | M10 change |
| --- | --- | --- |
| Images | `backend/internal/image/{module,handler,service,service_reads}.go` → `types/image` → OpenAPI → `web/src/data/http/http-adapter.ts` list reader → `DockerImage` | Guard destructive calls and add explicit action/report mappings; only listImages is currently live |
| Checks | `backend/internal/imageupdate` and `children/tags` → `types/image.UpdateInfo`, `types/imageupdate.Response`, `types/project.UpdateInfo` → container/stack inventory | One consistent state/digest/error projection; current digest/tag fields do not alone implement version/rebuild semantics |
| Registries | `backend/internal/registry` → `types/containerregistry` → hub management and environment credential sync | Separate public projection from write-only input and trusted sync; add `hasCredential` |
| Updates | `backend/internal/updater` and its execution/recovery children → `types/updater`, existing activities/history | Admit each update/restore through the runtime owner; retain one action identity and snapshot evidence |
| Jobs | `backend/pkg/scheduler/{auto_update_job,image_update_watcher}.go` and `runs` → `backend/internal/job` → `types/jobschedule` / `types/scheduler` | Project update-check/auto-update jobs, policy revisions and child action IDs in hub time zone |
| Web target | `web/src/domain/model.ts`, `images.ts`, `updates.ts` and demo adapter | Replace demo `check/scheduled` policy with accepted modes; map generated DTOs at adapter boundary in dependent work |

The existing actor admission has epoch/token-conditional release
(`backend/pkg/scheduler/runs/admission.go`). `UpdaterService.acquireUpdateInternal`
uses a coarse `updater` scope; the auto-update job also takes `auto-update`.
Neither provides runtime parent/child exclusion. The runtime canonicalizer accepts
only project deploy/down/restart/redeploy and container start/stop/restart.
There is no restore-point producer. These are implementation gaps, not permission
to bypass the runtime owner or to treat the demo RestorePoint as persisted state.

## HTTP operations and transport authority

Paths below include `/api`. `E` abbreviates `/api/environments/{id}`. IDs on the
wire are raw host-local IDs; the web adapter alone encodes `<host>~<raw-id>`.
Local execution uses environment `0`. Direct uses authenticated HTTP. Edge uses
the explicit command table with no Direct fallback. `H` means hub-owned public
admission/configuration; `A` means allowlisted executor/read on hub-local and
agent modes. An H mutation reaching an agent outside trusted admission refuses.

| Operation ID | Method and path | Surface / Edge command | Rights or change |
| --- | --- | --- | --- |
| list-images | GET E/images | A / `image.list` | `images:list` |
| get-image-usage-counts | GET E/images/counts | A / `image.counts` | `images:list` |
| get-image | GET E/images/{imageId} | A / `image.inspect` | `images:read` |
| pull-image | POST E/images/pull | A / `image.pull` | `images:pull`; preserve progress identity |
| remove-image | DELETE E/images/{imageId} | A / `image.delete` | `images:delete`; force cannot override guards |
| prune-images | POST E/images/prune | A / `image.prune` | `images:prune`; guarded candidate iteration |
| check-image-update | GET E/image-updates/check | A / `image_update.check` | `image-updates:check`; existing reference query |
| check-image-update-by-id | GET E/image-updates/check/{imageId} | A / `image_update.check_one` | `image-updates:check` |
| check-image-update-by-id-post | POST same path | A / `image_update.check_one` | Same check semantics |
| check-multiple-images | POST E/image-updates/check-batch | A / `image_update.check_batch` | `image-updates:check` |
| check-all-images | POST E/image-updates/check-all | A / `image_update.check_all` | `image-updates:check`; bounded background activity |
| get-update-info-by-refs | GET E/image-updates/by-refs | A / `image_update.by_refs` | `image-updates:read` |
| get-update-summary | GET E/image-updates/summary | A / `image_update.summary` | `image-updates:read` |
| check-project-updates | POST E/updater/projects/{projectId}/check | A / proposed `updater.project.check` | Reuse route; add missing Edge coverage |
| update-container | POST E/containers/{containerId}/update | H; existing `container.update` must no longer bypass admission | Update rights below; async and synchronous callers share one run |
| run-updater | POST E/updater/run | H; existing `updater.run` becomes admission-only | Existing bounded target options; one child runtime run per selected target |
| get-updater-status / get-updater-history | GET E/updater/status or /history | A / `updater.status`, `updater.history` | `image-updates:read`; project durable runs rather than only in-memory maps |
| listContainerRegistries / createContainerRegistry | GET / POST /api/container-registries | H only; no Edge command | `registries:list` / `registries:create` |
| getContainerRegistry / updateContainerRegistry / deleteContainerRegistry | GET / PUT / DELETE /api/container-registries/{id} | H only; no Edge command | Matching `registries:read`, `registries:update`, `registries:delete` |
| testContainerRegistry | POST /api/container-registries/{id}/test | H only; no Edge command | `registries:test`; test saved credentials without echo |
| getContainerRegistryPullUsage | GET /api/container-registries/pull-usage | H only | `registries:read`; typed probe errors |
| syncContainerRegistries | POST /api/container-registries/sync | Trusted manager→A / `container_registry.sync` | Manager credential only at agent; public hub sync returns acknowledgment only |
| get-job-schedules / update-job-schedules | GET / PUT E/job-schedules | A / `job_schedule.list`, `job_schedule.upsert` | `jobs:manage`; M10 update scheduling is hub-owned |
| list-jobs / run-job | GET E/jobs / POST E/jobs/{jobId}/run | A / `job.list`, `job.run` | `jobs:manage`; target effects still need current update rights |
| list-job-runs / get-job-run | GET E/jobs/{jobId}/runs[/{runId}] | A / `job.run.list`, `job.run.inspect` | `jobs:manage` |
| retry-job-run / cancel-job-run / resolve-job-run / ack-job-run | POST E/jobs/{jobId}/runs/{runId}/{retry,cancel,resolve,ack} | A / existing `job.run.*` | Preserve existing rights/admin resolution; no blind replay of an uncertain update |
| restart-job-worker | POST E/jobs/{jobId}/restart | A / `job.worker.restart` | Existing worker control; never release an unresolved runtime effect |
| get-auto-update-policy / set-auto-update-policy (new) | GET / PUT E/updater/policies/{kind}/{targetId} | H only | `kind=project\|container`; read/update rights below |
| list-restore-points (new) | GET E/updater/{kind}/{targetId}/restore-points | A / proposed `updater.restore.list` | Target read plus `image-updates:read`; metadata only |
| delete-restore-point (new) | DELETE E/updater/{kind}/{targetId}/restore-points/{pointId} | H→A / proposed `updater.restore.delete` | Update rights; refuse a point held by an active/unknown restore |
| restore-update (new) | POST E/updater/{kind}/{targetId}/restore-points/{pointId}/restore | H admission only | Update/restore rights; executor uses trusted runtime execute |
| get-system-image-hints (new) | GET E/updater/system-image-hints | H aggregation; proposed A observation `updater.system.hints` | `image-updates:read`; hub applies version cap |

Project manual updates reuse `run-updater` with `type=project` and the exact
selected project ID. Batch options remain bounded and permission-checked per
resource; an unbounded request may not silently broaden scope. `forceUpdate`
never bypasses image/system/snapshot/admission guards. Dry runs cannot deploy.
The route review must cover legacy updater callers, scheduled jobs and internal
entry points as well as the web path. The existing public agent mutation commands
cannot remain an alternative execution path after activation.

Manual update needs `image-updates:check`, `images:pull`, `activities:read` and
`projects:deploy` for a managed stack, or `containers:redeploy` for a standalone
container. Restore additionally needs `projects:update` for saved stack files
(or `containers:edit` for standalone configuration). Policy read needs target read
and `image-updates:read`; policy writes need `jobs:manage` and the applicable
update rights plus `containers:autoupdate` for container policies. Deleting points
uses restore rights. The prerequisite freezes the complete grant conjunction; no
new user-role model is implied. Hub grants are checked before joins/conflict IDs,
sync or dispatch; agents require their
manager credential and compatible contract. Logout detaches viewers only.

## Wire vocabulary and neutral failures

Keep wire DTOs in their actual current homes: `types/image`, `types/imageupdate`,
`types/containerregistry`, `types/jobschedule`, `types/updater` and the runtime
owner's `types/activity`. Do not create parallel registry/job DTO packages merely
because a planning path used a shorter name. New shapes below are proposed names;
the contract owner freezes their exact JSON/schema in the approved implementation.

| DTO | Required projection and invariants |
| --- | --- |
| Image Summary/detail | Retain ID, tags, repo digests and consumers; add `protectionReasons` (`in-use`, `system`, `restore-point`) and `usageState=known\|unknown`. Consumer reads include stopped containers; unknown usage never means unused |
| PullOptions | Reuse `imageName`, optional tag and legacy auth inputs; normalize one Docker reference and reject conflicting tag/digest inputs. Production UI selects saved registry credentials; no secrets in responses, progress or audit |
| PruneImagesInputBody | Retain adopted `mode` (`dangling`, `all`, `olderThan`) and `until`, including documented legacy aliases; invalid/contradictory input is 400. Selection is local to the requested host |
| ImagePruneReport | Retain `imagesDeleted`, `spaceReclaimed`; add per-ID `skipped` reason and `failed` code. Partial results preserve successful removals. Unknown reclaimed bytes are explicitly unknown, never invented |
| UpdateInfo | `state`, `checkedAt`, `imageRef`, optional current/candidate version and comparable digest/platform, `authState`, optional `code`. `hasUpdate` is true exactly for version/rebuild; legacy fields derive from the same result |
| Registry public projection | ID, URL, description, type, enabled, insecure, noncredential configuration, timestamps and `hasCredential`; exclude username/access-key identity, token/password/secret, encrypted forms, ECR tokens and authentication headers |
| Registry write input | Keep existing generic/ECR variants. Secret/credential fields are write-only; omitted fields preserve on edit. An explicit `clearCredential` clears the complete credential; reject simultaneous clear/replacement. Blank password inputs never accidentally erase a saved credential |
| RegistryTestResult | `ok`, `checkedAt`, optional typed `code`; failure uses appropriate non-2xx with the same safe result. No remote error body, username, URL userinfo, token or challenge header |
| AutoUpdatePolicy | `mode=off\|notify\|apply`, `revision`, target kind/ID and schedule job reference; write uses expected revision. Persist actor/key attribution for scheduled permission revalidation. Missing policy defaults off |
| RestorePoint | Opaque ID, host/registration, stable target identity, source run ID, createdAt, per-service previous immutable image identity, `volumeDataIncluded:false`, availability and safe reason code. List returns metadata only, never Compose/env bytes |
| RestoreRequest | Point ID in path plus deliberate idempotency key; no caller-provided snapshot/image override. Server binds point revision, configuration hash and target identity into the run intent |
| Update action/result | Existing admission envelope (`activityId`, `code`, runtime Activity), target/parent, optional job run ID and restorePointId; outcome/effect knowledge and typed per-target result. No generic unfiltered `details` map |
| SystemImageHint | Component kind, observed running/effective reference/version, candidate reference/version, checkedAt, `state`, `requiresHubUpdate`, optional blocked available version and code; no mutation capability |

Failures use `{success:false,error:{code,message}}`; admitted failures also retain
`data.activityId`. Conflict envelopes preserve runtime `run.busy`, `run.joined`,
`run.cancelling`, `run.request-mismatch` and authorized active-run disclosure.
Huma validation failures are normalized at these boundaries without echoing
write-only input values. NDJSON pull errors carry the code after flush and never
emit successful completion after a failure. Server persistence, activities, audit,
job outcomes, stored check errors and SSE use the same sanitized errors; raw
Docker/registry text is not an alternative public message path.

| HTTP | Codes and condition |
| --- | --- |
| 400 | `image.invalid-reference`, `image.invalid-prune`, `registry.invalid-config`, `update.invalid-policy`, `update.invalid-target` |
| 401/403 | Existing authentication/permission codes; `registry.authentication-required` / `registry.authentication-failed` for registry probe failures (not hub sign-out evidence) |
| 404 | `image.not-found`, `registry.not-found`, `update.target-not-found`, `restore.not-found` |
| 409 | `image.in-use`, `image.system-protected`, `image.restore-point-protected`, `image.changed`, `image.busy`, `registry.exists`, `update.policy-changed`, `update.target-changed`, `update.snapshot-unavailable`, `restore.in-use`, `restore.unavailable` plus runtime conflicts |
| 429 | `registry.rate-limited`; bounded retry metadata only |
| 502/503 | `image.pull-failed`, `image.remove-failed`, `image.prune-failed`, `image.usage-unavailable`, `registry.unavailable`, `registry.sync-failed`, `registry.credential-unavailable`, `update.check-unavailable`, `update.system-version-unknown` |
| 500 | `update.deploy-failed`, `restore.deploy-failed`, `update.persistence-failed`, `registry.persistence-failed`; message describes the operation without underlying text |

Reuse runtime unknown/interrupted/cancelled/target-removed/target-unavailable codes
for those outcomes. A registry 401 is a failed registry check, not a hub identity
rejection. New codes require EN/DE mappings and catalogue/schema checks in the
owning steps. #227/#202 do not waive sanitization for the surfaces changed here.

## Image selection and destructive guards

Resolve the inspected full image ID on the executor. All aliases and digests of
that ID share its protection. Query all containers (`all=true`), including stopped
and internal containers; compare inspected image IDs, not display tags. A failed
usage scan refuses removal/prune rather than reporting zero consumers. Protect
running hub/agent images, configured release/override agent and tools images,
active helper images, and every restore-point pin. Resolve system references
freshly and retain known pinned IDs; unresolved protection evidence fails closed.
Do not use a repository substring or only a current running-container filter.

`force=true` cannot remove or untag a protected ID. Every removal rechecks usage,
system protection, restore pins and candidate identity immediately before a
non-forced, non-recursive remove. Do not let automatic child-image pruning bypass
this set. The daemon remains the final guard for external container races.

Prune enumerates selected candidates and invokes the same guarded removal service;
a Docker-wide ImagePrune call cannot preserve application-owned restore pins by
itself. Duplicates resolve once per full ID. Report protected/missing/changed
candidates as skipped, and other failures by stable code; successful candidates
remain in the result. System and scheduled prune image legs must use this guard
too. Volume/network prune semantics remain unchanged. If a neighbouring caller
cannot route through the shared guard within ownership, record a prerequisite.

Before any workload mutation, persist the restore point and install a local
retention tag for each previous image ID (plus repo digest when available).
A tag is a retention aid, not sufficient protection from `prune all`. Restore
resolves the saved immutable ID/digest and verifies the retention tag still points
to it; never resolve yesterday's mutable tag from the registry. Pin registration
and removal checks must share existing admission ownership; a pull that moves a
tag cannot invalidate a published point. External Docker removal/tag changes are
outside application exclusion and make the point unavailable, never silently
select a replacement. No update begins when pin/snapshot persistence fails.

The proposed image critical sections reuse `runs.Admission`, keyed by full image
ID on the executor, for pin creation/removal and guarded delete/prune. Acquire
multiple image leases in sorted ID order, without waiting while holding another;
on contention release acquired leases and refuse/skip with `image.busy`. Take
runtime target admission before image leases, never in reverse. Under these
leases, persist a preparing point and its protected image IDs, create/verify
retention tags, then commit availability before releasing. Prune re-reads pins
under the same lease immediately before removal. Point deletion checks active
restore ownership before releasing any shared pin. After process interruption,
preparing/unknown points protect their IDs until reconciliation establishes
availability or safely removes the point. An epoch change alone does not delete
those records. Lease release is token-conditional; it never settles runtime work.
This uses the existing primitive with reviewed new scope usage, owned by G4.

## Registry credentials and sync

Keep `go.getarcane.app/sys/crypto` and the existing operator encryption key;
no new key service. Encrypt token/password/ECR secret material at rest on both
hub and agent. Public DTO conversion is explicit, with `hasCredential` indicating
stored configured material, not successful authentication or decryption. Credential
usernames/access-key identities are write-only under this proposed projection;
editing them requires replacement input. Never return masked placeholder secrets
that a form could save as a new password. Reject URL userinfo and avoid input
values in validation errors. Credentials must not enter audit/request-body logs.

The hub owns registry management and saved-credential tests. A successful test
proves reachability/authentication then, not pull authorization for every image
or availability from every agent. Actual pull/check failures remain host-scoped.
Agents accept only credential sync; they do not expose list/get/edit/test APIs.
Retain the accepted all-enabled-registries-to-compatible-agents trust model from
[Edge threat review T5/S3](edge-threat-review.md). Plain secrets exist in trusted
sync transport and memory; they never appear in the sync acknowledgment. Use
current manager authentication, filtered headers, revocation and compatibility
before sync; no old-session delivery after token rotation and no sync to an
incompatible agent. Agents encrypt a validated replacement set before committing;
a failed sync retains the prior complete set and returns a neutral failure.
Disabling/removing credentials propagates as set replacement. A hub save succeeds
independently of an offline host; per-host sync failure is visible, and reconnect
sync must complete before that host assumes the new credential configuration.

## Update information and candidate identity

A check records its observation time and configured reference. Registry success
and absence of a newer candidate are distinct from unknown/error. Proposed states:

| State | Evidence and meaning |
| --- | --- |
| `unchecked` | No usable check for this identity; no claim about updates |
| `current` | Successful lookup, comparable current/candidate digests equal and no newer allowed version |
| `version` | Proven greater release SemVer in the selected repository/channel, with a resolvable candidate digest; classify major/minor/patch from versions |
| `rebuild` | Same selected tag/release version but different comparable manifest digest; no claim of fixes or security benefit |
| `local` | Inspected image has no usable registry source; no invented registry update |
| `unknown` | Missing/incomparable digest/platform/version or unsupported lookup; optional typed reason, never current |
| `error` | Authentication, rate limit, transport or lookup failure; typed code and neutral message, never current |

`authState=anonymous|credential|authentication-required|authentication-failed|unknown`
is separate from update state. An anonymous successful public lookup is valid.
A denied private lookup cannot fall back to a false current result. Previous
successful observations may be retained with their old checkedAt and explicit
stale state; they never authorize apply. A new configured image, host registration
or credentials revision invalidates the cached eligibility observation.

Compare registry manifest digests at the same level and platform (OS/architecture/
variant). Do not compare a Docker config/image ID to a registry index digest, or
an index to a platform manifest. Missing repo digest requires a resolved baseline
or yields unknown. A changed multi-platform index alone cannot prove a workload's
platform image changed. Resolve both to the host's platform manifest first.

Reuse adopted tag-policy discovery. A version candidate needs parsed stable
SemVer from an unambiguous tag or OCI version label; arbitrary tag order and
publication times are not version ordering. Respect selected channel/tag policy,
no implicit major/channel or prerelease opt-in. Same-version digest change is
rebuild; without trustworthy version metadata a changed mutable tag is rebuild
with absent release version. Digest-pinned workload references remain pinned:
check reports the pinned state and never invents a mutable-tag replacement.
Apply revalidates and freezes candidate reference/digest and baseline identity;
it pulls that digest, verifies the resulting image and deploys that exact result.
A tag changing after check does not redirect the admitted update.

## Policies and ScheduledJob projection

Proposed simple default: every target starts `off`; `notify` checks and exposes
availability without deployment, and `apply` checks then applies an eligible
version/rebuild at the host's auto-update schedule. Manual deliberate checks,
updates and restores remain possible with the appropriate rights in every mode.
A Compose child resolves to its uniquely owned stack; the stack policy is
authoritative. Do not update one service independently behind a stack's policy.
An explicit standalone policy is bound to a durable logical target and inspected
container identity; a replacement unrelated container cannot inherit it by name.
Unraid/unknown-origin/system containers are not eligible for automatic recreation.

Proposed schedule model: reuse `image-polling` (update-check, hourly default) and
`auto-update` (auto-update, daily midnight default), one of each per environment,
with per-target policy selection. No new per-resource cron engine. The hub owns
M10 trigger time, using M4's configured IANA zone, even for remote targets.
Agents execute admitted host effects; their local clocks or inherited auto-update
jobs must not cause a second independent apply. Existing global polling/auto-update
enable switches remain master gates; setting apply must make a disabled schedule
visible rather than promise execution. No missed-run catch-up burst on reconnect.

| ScheduledJob field | Wire source and rule |
| --- | --- |
| id / kind | Stable host/job ID; `image-polling` → `update-check`, `auto-update` → `auto-update` |
| hostId / stackId | Registered host; stackId only for an explicitly targeted child projection, never a fabricated host-wide stack |
| enabled / schedule / timeZone | Authoritative schedule config and effective global gate; normalized six-field cron and hub zone |
| nextRunAt | Server-calculated next occurrence; absent when disabled/unavailable, never browser-local arithmetic or a fake timestamp |
| currentRun / lastRun / lastSuccess | Existing durable scheduler run identities/status and observation time |
| activityId / target outcomes | Link each target to its runtime activity and restore point; parent batch success cannot stand in for a child's unknown outcome |
| cadence / title / reason | Localized consumer projection of schedule/kind/typed disabled or failure reason; no daemon text |

Use the existing six-field cron parser for validation, execution and next-run
projection, with its zone/DST behavior tested at both missing and repeated local
times. Browser zone does not change execution. Existing web ScheduledJob requires
nextRunAt and lacks disabled/unknown metadata; the dependent domain mapping must
make these states explicit rather than omit a disabled job. Schedule writes use
`jobs:manage`. A scheduled target revalidates the recorded actor/key's current
rights, host registration and current policy revision before target admission.
The scheduler identity is not a standing superuser bypass. Revoked/deleted
identities produce a safe skipped/refused outcome before new target admission.
Proposed policy semantics follow existing accepted runtime intent: freeze the
policy revision at target admission; policy/permission changes govern future
admissions and do not silently cancel an accepted run. Explicit cancellation
uses the existing durable cancel path. No new agent-to-hub authorization callback
is required at each Docker call.

## Restore points and deployment identity

Proposed retention: retain the latest **five available points per logical stack
or standalone target**, with explicit deletion and no time-based expiry. Failed
or unknown updates retain their pre-update point. An active/unknown restore holds
its source point, so cleanup may temporarily exceed five. Create and verify the
new point before trimming old points; a cleanup failure must not invalidate the
new point. Cleanup releases pins only after durable point deletion and only when
no other point/run references the image. Retention-tag cleanup is idempotent;
orphan tags after interruption are reconciled conservatively, never bulk-pruned.

The executor stores an immutable snapshot before the first config/container
mutation: deployed Compose files, effective deployment env, selected auxiliary
Compose/env inputs and overrides, effective project name, configuration hash,
service image IDs/digests and enough standalone configuration to recreate exactly.
Use the last successful deployed configuration, not an unsaved editor draft or
newly edited on-disk files falsely labelled as the deployed state. An initial
legacy deployment without durable configuration evidence requires a validated
baseline; otherwise refuse update with `update.snapshot-unavailable`. Do not
silently save only the primary YAML when execution used includes/overrides/env
files. Non-capturable external inputs/build-only or unsupported standalone
configuration refuse eligibility before any destructive effect. Volume contents,
bind-mounted data and external resources are not captured.

Snapshot Compose/env may contain secrets. Encrypt stored snapshot payloads with
the existing crypto facility, restrict local persistence, and never include them
in list metadata, activities, event payloads or default generated read DTOs.
Store on the executor's persistent local storage; the hub retains run/point
metadata with host/registration identity. Offline cached metadata is explicitly
stale and never proves restorable bytes or pinned image presence. Lost storage,
a replaced registration or missing/tampered pin makes restore unavailable.
Agent storage is not replicated into a new remote backup service in M10.

Restore is a new deliberate action, with a new runtime UUID/idempotency key,
linked to the point and original update run. It acquires the same target/parent
scope as update/deploy, checks current identity/permissions and point integrity,
then writes/deploys the saved configuration using saved immutable images with no
mutable pull. It records affected old/new container identities and the stable
stack/logical standalone target. Preserve volumes; never invoke down-with-volumes.
Restoring files/config and deploying are not one Docker transaction. A failure
midway is a visible failed/interrupted action with partial or unknown effects,
not an automatic rollback or success. The chosen point remains usable after
reinspection. Recommend creating the same pre-change point before restore so a
restore can itself be undone, subject to the same retention limit.

Deployment history consumes runtime records for editor deploy, update and restore
using stable host/project/run identity. Retaining a point is independent of the
100-finished-activity output limit and 24-hour compact receipt minimum; pruning
activity text must not delete points or their pins. This does not promise an
unbounded new history store.

## Lifecycle state table and admission reuse

The table defines workflow phases, not new terminal Activity statuses. `Q`
means a durable admitted/queued intent, `C` checking/freeze, `P` snapshot/pins
committed, `L` pull, `D` deploy, `R` restore, `U` outcome unknown/held, and `T`
settled success/failed/cancelled. A check-only result can settle without P/L/D.
The public projection remains queued/running/success/failed/cancelled plus runtime
`outcomeState`, `cancelRequested` and targeted recovery reason. Unknown is not
success and never permission to replay or release a scope.

Reuse the existing scheduler run coordinator and epoch/token leases for trigger
ownership. Reuse runtime durable admission, opaque-key binding, parent/child
conflicts, executor storage generation, one-shot execute/query/cancel, refusal
tombstones, bounded reconciliation and administrator disposition for effects.
The scheduler lease is not evidence that a remote executor stopped. Its epoch
change or stale token release cannot free a runtime exclusion. Matching run ID,
digest, registration and storage generation fence stale executor answers; the
existing runtime has no distributed effect-fencing lease. Do not invent one.

| State(s) | Event | Transition, persistence and externally visible consequence |
| --- | --- | --- |
| Idle / T | Schedule fires, policy off or master gate disabled | Record skipped job target if triggered; no apply admission or Docker mutation |
| Idle / T | Schedule fires, notify | Coordinator deduplicates trigger, checks rights/identity, C → check result → T; expose update info only |
| Idle / T | Schedule fires, apply | Coordinator deduplicates trigger; validate rights, policy revision, fresh target and compatibility; admit Q with stable child run ID, then C |
| Idle / T | Manual update requested | Hub permission and idempotency checks admit Q → C independently of off/notify; request retry resolves same run |
| C | Check says current/local/unknown/error or no allowed candidate | T with current/skipped/typed failure as appropriate; no snapshot-required deployment, no false updated result |
| C | Check finds eligible version/rebuild | Verify the admitted baseline identity, candidate digest, policy revision and config identity; capture/pin and persist P before workload mutation |
| C / P | Snapshot, pin or persistence fails | Fail before deployment; retain any uncertain pin metadata for reconciliation, zero destructive workload calls |
| C / P | Image lease busy or process stops while point prepares | No deployment; release only owned lease tokens, retain protected preparing IDs for reconciliation; prune skips busy IDs and cannot delete an available point's images |
| P | Start update pull | L pulls frozen digest; recheck cancellation, admission ownership and target/config identity before destructive effect |
| L | Pull fails or pulled image does not match | T failed with `image.pull-failed` or `update.target-changed`; retain pre-update point, existing workload/config untouched |
| L | Verified pull succeeds | Recheck admission ownership, target identity and configuration revision; D only if all still match |
| D | Deploy succeeds and worker/output settle | Atomically settle runtime outcome/activity; T success, link point and resulting identities; release only after durable settlement |
| D / R | Deploy fails midway | Keep point and actual partial evidence; when worker/children settle, T failed with typed deploy code; require fresh state inspection before another action; no automatic restore |
| Q / C / P / L / D / R / U | Restore requested during update/restore | Same target or parent exclusion returns authorized `run.busy`/`run.cancelling`; no queued automatic restore, second executor or point deletion |
| Idle / T | Restore requested | Validate exact point/registration/images/config; admit a distinct Q, capture pre-restore state if approved, then R with saved immutable inputs |
| Q / C / P / L / D / R | Browser disconnect/logout | Detach waiter; accepted application-lifetime work continues; no new run from a reconnect |
| Q | Hub restart before dispatch | Existing runtime startup settlement marks abandoned local/admitted work interrupted, effects unverified; scheduler reconciles child ID, never resends effects |
| C / P / L / D / R | Hub restart with local executor | Prove old process ended; settle interrupted before readiness, retain point and observed partial state; do not continue workflow from a phase label |
| Q / C / P / L / D / R / U | Hub restart with remote dispatch committed | Restore exclusion, enter/retain U and query matching receipt; no fresh worker, no lease-age expiry and no automatic retry |
| C / P / L / D / R | Agent restart with continuous storage | Existing receiver settles abandoned accepted/executing run interrupted before readiness; hub adopts authoritative receipt; never resumes a destructive step |
| Idle / Q / C | Host offline or Edge disconnected before new admission | Refuse new work with target-unavailable; a scheduler target is skipped/failed explicitly, never silently moved to Direct |
| Q / C / P / L / D / R | Host offline/Edge disconnect after possible acceptance | U with held scope; query same run on recovery. Lost transport proves neither failure nor rollback; point remains pinned |
| U | Matching authenticated active/terminal receipt arrives | Adopt phase/outcome evidence; keep held while active, release only after durable terminal application; expired/other-generation absence remains unknown |
| U | Operator requests recovery | Existing administrator stop/inspect/acknowledge procedure only; persist disposition, retain late executor evidence separately; no automatic effect replay |
| Q / C / P / L / D / R / U | Concurrent manual lifecycle action on same stack/child | Existing symmetric stack/child predicate refuses/joins only semantically identical authorized intent; unrelated hosts independent. Retain coarse updater lease if needed, never substitute it for parent exclusion |
| Before Q / Q / C / P / L | Policy changes during scheduled run | Before Q, admission compares expected policy revision and refuses stale intake. Once Q commits, the accepted revision remains authoritative for that run; edits affect future admissions. Explicit cancel persists intent rather than silently replacing the policy-bound action |
| D / R | Policy changes after first destructive effect | Record changed policy for next trigger; do not race-cancel/replay/rollback ongoing effect. Settle actual outcome; no further target admitted under stale revision |
| Idle / before Q | Identity/permission/key changes | Reject new intake/joins and unauthorized run/point reads. A pending job target must resolve current actor/key rights before Q; record neutral refusal |
| Q / C / P / L / D / R / U | Target registration or inspected workload identity changes | Stop before another destructive boundary, or retain uncertain outcome after a possibly accepted remote effect. Replaced registration/workload never inherits old intent; no automatic replay |
| Q / C / P / L / D / R / U | Identity/permission changes after admission | Deny new reads/commands and follow viewer revocation; bounded accepted worker settles or U holds. Recovery authority uses current identity; no success inferred from logout |
| Any active phase | Cancel requested or workflow deadline | Persist intent, signal owner; hold until actual worker/children/output settle. No rollback guarantee; unanswered remote result remains U |
| Any settling phase | Terminal persistence fails | Keep owner/exclusion/point; retry storage settlement only, never Docker. Lost final response after commit resolves via the same receipt |
| T | Another trigger, same request key, or retry control | Retained key returns original outcome. New deliberate intent rechecks actual state. Job retry cannot replay a child with unknown or partial effects merely because its wrapper failed |

Scope for a Compose-container update is the complete uniquely resolved stack;
route users must be told that it is a stack update. It cannot silently remove
siblings' exclusion or replay only an arbitrary container from a failed batch.
Standalone effects use the full container identity plus stable logical target
mapping through recreation. Runtime update intent must include frozen candidate
and snapshot identity; restore intent includes point/config identity. Neither can
be represented by reusing an ordinary redeploy digest with different hidden work.
A bounded preflight resolves those inputs before new admission; retained-key
retries resolve the existing receipt first and never acquire a different intent.
The worker verifies the frozen inputs and refuses drift instead of selecting a
newer candidate under the old run ID.

## Agent/tools hints and compatibility

Observe the effective configured agent/tools references, including local tools
overrides and release defaults from the storage contract. Only query release
metadata; never pull/restart/replace them as a watcher side effect. Unknown custom
references or non-SemVer development versions yield a typed unknown hint.

For a running hub H and agent A, let L be the newest compatible stable agent
release known in the selected channel. The offered candidate is the newest
compatible release greater than A and **less than or equal to H**. With hub 2.0.0,
agent 1.9.0 and release 2.1.0, offer at most a published compatible 2.0.0 and set
`requiresHubUpdate` for 2.1.0. If no capped release exists, offer none. Never
invent a 2.0.0 artifact or propose a downgrade when A already exceeds H; report
that mismatch for manual reconciliation. Compare parsed SemVer, not strings.
The hub computes the cap, not untrusted agent-supplied hint text.

Tools versions have their own release sequence: use the tools reference declared
compatible by the running hub/agent release, not an assumption that tools SemVer
equals hub SemVer. Preserve explicit operator pins. A tools hint likewise cannot
exceed the release compatibility mapping or silently overwrite TOOLS_IMAGE.
The existing manual self-upgrade compatibility exception remains an operator
action, not automatic rollout authority.

The current agent contract/minimum is **6** with fixture `agent-hub-v6.json`.
Propose **7/minimum 7**, preserving v3–v6 fixtures and adding a v7 fixture whose
exact bytes determine the fingerprint. It covers the updated error/DTO semantics,
image protection, credential sync projection, update/restore runtime operations,
policy/snapshot identity, new routes and allowlists. Activate routes, behavior,
OpenAPI/generated DTOs, grants, fixture/fingerprint and compatibility enforcement
together after prerequisites and risk review. Older or mismatched agents stay
update-required before forwarding or registry sync; only existing health/version/
manual self-upgrade exceptions remain. `tunnel/v1` transport does not change.
Shared product SemVer is a release-package decision, not this contract integer.
Correct the stale runtime v5 wording in api-contract.md when v7 activates.

## Ownership prerequisites and approval questions

The following are proposed resolutions for coordinator/maintainer approval before
affected implementation. They are not silently accepted scope changes.

| ID | Open question / evidence | Recommendation and owner |
| --- | --- | --- |
| D1 | How many restore points and is restore itself reversible? | Latest five per logical target, explicit deletion, active/unknown holds; snapshot before restore too. #229 contract owner, approve before snapshot implementation |
| D2 | Stack versus Compose-child policy and schedule granularity? | Stack policy wins for managed children; standalone policy only for supported standalone targets; reuse two host-level jobs and existing master gates; policy revisions freeze at admission, later edits need explicit cancel for accepted work. #229 + scheduler owner |
| D3 | What proves the prior deployed state for legacy/multi-file stacks and standalone containers? | Capture complete effective inputs at successful deployment; refuse unprovable/unsupported baselines rather than call current drafts deployed. Project/runtime owner must accept baseline and restore execution contract |
| G1 dependency-gap | Runtime update/restore operations and shared parent exclusion are absent; runtime/project are read-only here | Runtime foundation owner supplies narrowly reviewed operation/options/target and executor hooks, authorization and receipt tests before #229 activates writes. Do not wrap an unguarded updater behind a runtime-looking Activity |
| G2 dependency-gap | Scheduler ownership lies in `backend/pkg/scheduler`; current jobs run on host and actor leases reset by epoch | Scheduler owner supplies hub-owned triggers, actor revalidation before child admission, child-runtime linkage and no-replay recovery; use existing coordinator/leases. Merge prerequisite before apply integration |
| G3 dependency-gap | Restore points/policies need durable storage, model registry and a forward migration outside listed paths | Database owner supplies immutable forward migration/integrity entries and crypto/storage wiring; add a database risk gate. Never edit shipped baseline migrations |
| G4 dependency-gap | Shared protection must cover system/scheduled prune image callers outside image ownership | Owning system/scheduler steps route image deletion through guarded service and coordinate pin admission. Until merged, do not advertise prune-safe restore points |
| D4 | Public credential projection and ownership path spelling? | Treat username/access-key identity as write-only too; use actual types/containerregistry and types/jobschedule with coordinator-confirmed ownership. Keep generic/ECR baseline; no registry browser UI |

G1–G4 pause only their affected implementation. Plan them in the owning milestone
or an explicitly accepted foundation follow-up with an owner, issue, PR and risk
review; merge them before dependent activation and recheck the milestone ready
gate. The Phase 1 package changes only this record and the design index. No
runtime, scheduler, database, identity or project neighbour is edited here.

## Test matrix and practical acceptance

Tests use controlled effects and real SQLite/HTTP/Edge paths where applicable;
CI needs no live Docker, agent or socket. Synchronize with the actual guard/effect
boundary, never sleeps. Each new behavioral guard needs a targeted red mutation
(remove the guard, observe the intended assertion fail, restore and pass).
Compiler failures, timeouts and equivalent mutations are not evidence.

| Area / method | Required observation and useful mutation |
| --- | --- |
| Registered routes/OpenAPI/generation/allowlist | Hub-only registry CRUD/test and policy/admission routes absent from agent; trusted sync and executor reads reachable on Direct/Edge; remove route guard or command mapping and fail real dispatch assertion |
| Image service with fake daemon | Running/stopped consumers, aliases, system overrides/helpers and restore pins block force/remove/prune; uncertain usage refuses; remove each predicate independently, assert zero forbidden daemon calls |
| Prune callers and pin race barriers | Interactive/system/scheduled image legs share guards; partial report retains success; pin creation/deletion versus prune cannot lose a published pin; bypass guarded removal and fail |
| Registry model/handler/real sync transport | Generic/ECR create/edit/clear, encrypted persisted bytes, no public credential field in success/failure/validation/activity/SSE; remove DTO redaction or log sanitization and assert recognizable synthetic secret absent |
| Compatibility/revocation/sync failure | v6/fingerprint mismatch, removed/rotated registration and offline Edge send no secrets; failed sync preserves old complete set; mutation permits old-session sync and fails transport assertion |
| UpdateInfo fixtures from producer | Version major/minor/patch, same-tag rebuild, equal digest, platform/index mismatch, local/pinned, missing digest, private denial/rate limit; mutate digest comparison or unknown→current mapping and fail |
| Admission state-table barriers | Schedule/manual collisions, stack/child both ways, distinct-host independence, restore during update, same-key retry and policy revision races; remove conflict/admission-revision recheck and assert extra effect |
| Snapshot/restore persistence | Multi-file/env/standalone evidence, encrypted payload, missing baseline refusal, pre-effect pin commit, retained failed point, exact digest/config restore, five-point retention and shared pin holds; skip snapshot commit or use mutable tag and fail |
| Partial failure/process reopen | Pull fails without deploy; mid-deploy failure, hub/agent restart, lost ack/Edge disconnect, DB terminal failure, late receipt and administrator recovery; mutate automatic replay/release and assert no second effect/no premature release |
| Actor/current permissions | Policy owner/key revocation, resource identity replacement, current activity/point read denial, browser logout after accepted work; mutate permission/generation checks and fail actual effect/disclosure assertion |
| Schedule projection | Hub zone, disabled/unknown next run, DST transitions, existing cron and global gates, linked parent/child outcomes; mutate browser/agent-local schedule computation or success aggregation |
| System hints | Numeric SemVer ordering, H=2.0.0/L=2.1.0 cap, missing capped artifact, A>H, prerelease/custom unknown and tools compatibility; remove cap and assert no above-hub candidate or mutation calls |
| Dependent web consumer | Generated DTO mapping, unavailable/stale metadata, off/notify/apply, registry 401 distinct from session 401, restore warning, action identity/history, EN/DE; real user gesture and production capability tests |

Sandbox-checkable acceptance of the exact integrated package, separately approved
and executed by the coordinator/operator after implementation:

1. Record package SHA, served bundle and compatible contract/fingerprint on hub,
   Direct and Edge executors; observe real producer fixtures rather than invent
   response shapes. No real-system deploy or release follows from this document.
2. Pull a public and authenticated image on each host; create/edit/test generic
   credentials (ECR where supported by available acceptance credentials); inspect
   all response/activity surfaces for credential echo. Offline sync is explicit.
3. Refuse removal and force removal for running and stopped usage, agent/tools/
   system images and restore pins, including aliases. Prune a mixed set and
   reconcile removed/skipped/failed entries and reclaimed-space knowledge.
4. Demonstrate an actual version update and same-tag rebuild plus denied/unknown
   checks. Record current/candidate comparable platform digests and ensure an
   unauthenticated private check is never shown as current.
5. Update a stack and supported standalone container manually, verify the point
   precedes mutation, then restore saved configuration/env and pinned images.
   Check failure midway and concurrent restore refusal. Show explicitly that
   volume/bind data was not saved and remains outside restore guarantees.
6. Run notify and scheduled apply in the hub zone; observe policy off/global
   disable, policy/permission change, duplicate schedule/manual overlap and a
   created restore point. Interrupt hub/agent/Edge at controlled boundaries and
   verify no automatic replay or false success; inspect actual workload state.
7. Exercise five-point retention and deletion without losing held/shared pins,
   stale/offline metadata, missing pin refusal, and stack deployment history
   linking editor deploy/update/restore to their actual run IDs.
8. Demonstrate the agent version cap and tools informational hint with zero
   replacement calls. Exercise EN/DE, desktop and 390 px, current permissions,
   activity discovery integration and a production bundle without demo paths.

Record honest unsupported-baseline, external Docker-race and storage-loss limits.
Risk interim review must cover agent surface, credentials, persistence and
update/restore lifecycle before dependent web work merges; exact-package practical
acceptance, green integrated checks and closing review remain milestone gates.
