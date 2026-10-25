# Images and updates

## Boundary and accepted decisions

This is the M10 design contract for #229, based on the registered producers at
`8de82c9cf4166915137077353851d5d1ff8edeb6`. It specifies implementation and review
gates; it does not claim that the adopted routes already enforce this contract.
The [API mapping](api-contract.md), [runtime control](runtime-control.md),
[stack authoring](stack-authoring.md), [storage contract](storage-workspace.md)
and [network contract](network-management.md) remain the neighbouring contracts.

The maintainer decided on 2026-10-24:

- Auto-apply is in scope. Compose stacks have `off`, `notify` or `apply`
  policies. A ScheduledJob executes apply in the M4 hub time zone and creates a
  restore point before every applied update.
- A restore point pins the previous image and retains the deployed Compose and
  environment configuration. Restore redeploys that exact configuration and
  image state. **Volume data is not saved or restored**, and the UI says so.
- Agent/tools image watchers inform only. Rollout remains manual. An agent
  candidate and server-side self-upgrade target never exceed the running hub
  version; a newer release requires updating the hub first.
- #227 and #202 remain outside M10. Every new or modified image, registry and
  update failure returns a stable typed code and neutral message. Raw daemon
  error text stays in server logs, subject to credential redaction.

Coordinator dispositions and subsequent maintainer decisions on 2026-10-24 are
binding: D1 retains five points per logical target and requires a point before
restore too; D2 gives the stack policy precedence, two hub-owned jobs per host
and admission-frozen policy revisions; D4 makes credential identities write-only.
M1 refuses manual and automatic updates with saved but undeployed changes.
M2 requires a recorded baseline for updates and a restore point before **every
update and restore**; a normal deploy establishes the initial baseline. Restore
can recover without a current baseline, as specified below. The earlier D3
manual-without-restore-point exception is withdrawn. There is no pre-M10 legacy
migration or config-hash bootstrap path. M3 means notify updates the UI and sends
one notification entry through existing channels per new candidate version.
M4 uses admitted-user literal Compose image rewrites for version updates and
image-only replacement for same-tag rebuilds; no generated override exists.
M5 limits update, policies and restore points to Compose stacks; standalone
containers expose update information only.
#229 owns all G1–G4 implementation work and its explicit database risk gate.

M10 includes host-scoped pull, remove and prune; registry create/edit/delete/test
with encrypted, write-only credentials and agent sync; update checks; manual
stack update; policy/schedule execution; restore-point list/delete and
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
| tag-image | POST E/images/{name}/tag | A; retained API, no M10 tag UI | Existing `images:tag`; host image lease and reserved-target guard |
| upload-image | POST E/images/upload | A / `image.upload`; retained API, no M10 upload UI | Existing `images:upload`; validate all tags before load |
| search-images | GET E/images/search | A; retained read, no new M10 discovery UI | `images:read`; no reserved-namespace lookup |
| get-image-history / export-image | GET E/images/{name}/history or /export | A; retained reads, no new M10 UI | `images:read`; no snapshot/credential expansion |
| get-image-attestations | GET E/images/{name}/attestations | A; excluded from M10 UI | Retain existing read permission; reserved tags do not trigger registry lookup |
| list-image-patches / list-image-patch-targets | GET E/images/patches or /patch-targets | A; excluded from M10 UI | Retain existing `images:list` / `vulnerabilities:read`; no reserved registry lookup |
| patch-image | POST E/images/{imageId}/patch | A; excluded from M10 UI | Existing `images:patch`; producing a tag still enforces reserved targets and host lease |
| check-image-update | GET E/image-updates/check | A / `image_update.check` | `image-updates:check`; existing reference query |
| check-image-update-by-id | GET E/image-updates/check/{imageId} | A / `image_update.check_one` | `image-updates:check` |
| check-image-update-by-id-post | POST same path | A / `image_update.check_one` | Same check semantics |
| check-multiple-images | POST E/image-updates/check-batch | A / `image_update.check_batch` | `image-updates:check` |
| check-all-images | POST E/image-updates/check-all | A / `image_update.check_all` | `image-updates:check`; bounded background activity |
| get-update-info-by-refs | GET E/image-updates/by-refs | A / `image_update.by_refs` | `image-updates:read` |
| get-update-summary | GET E/image-updates/summary | A / `image_update.summary` | `image-updates:read` |
| check-project-updates | POST E/updater/projects/{projectId}/check | A / proposed `updater.project.check` | Reuse route; add missing Edge coverage |
| update-container | POST E/containers/{containerId}/update | H; existing `container.update` must no longer bypass admission | Compose child resolves to complete stack; standalone refuses `update.invalid-target` |
| run-updater | POST E/updater/run | H; existing `updater.run` becomes admission-only | Existing bounded target options; one child runtime run per selected target |
| get-updater-status / get-updater-history | GET E/updater/status or /history | A / `updater.status`, `updater.history` | `image-updates:read`; project durable runs rather than only in-memory maps |
| listContainerRegistries / createContainerRegistry | GET / POST /api/container-registries | H only; no Edge command | `registries:list` / `registries:create` |
| getContainerRegistry / updateContainerRegistry / deleteContainerRegistry | GET / PUT / DELETE /api/container-registries/{id} | H only; no Edge command | Matching `registries:read`, `registries:update`, `registries:delete` |
| testContainerRegistry | POST /api/container-registries/{id}/test | H only; no Edge command | `registries:test`; test saved credentials without echo |
| getContainerRegistryPullUsage | GET /api/container-registries/pull-usage | H only | `registries:read`; typed probe errors |
| syncContainerRegistries | POST /api/container-registries/sync | Trusted manager→A / `container_registry.sync` | Manager credential only at agent; public hub sync returns acknowledgment only |
| get-update-schedules / set-update-schedules (new) | GET / PUT E/updater/schedules | H only; never forwarded | `jobs:manage`; authoritative M10 schedules/job projection from hub storage, including offline remote hosts |
| get-job-schedules / update-job-schedules | GET / PUT E/job-schedules | Existing A / `job_schedule.*` only for other jobs | `jobs:manage`; v7 omits M10 schedule fields here and rejects writes to them; use the hub projection above |
| list-jobs / run-job | GET E/jobs / POST E/jobs/{jobId}/run | H for M10 jobs; A / `job.list`, `job.run` otherwise | `jobs:manage`; hub projects/submits M10 runs; agent-local M10 run refuses |
| list-job-runs / get-job-run | GET E/jobs/{jobId}/runs[/{runId}] | H for M10 runs; existing A commands for other jobs | `jobs:manage`; target activity comes from trusted runtime receipts |
| retry-job-run / cancel-job-run / resolve-job-run / ack-job-run | POST E/jobs/{jobId}/runs/{runId}/{retry,cancel,resolve,ack} | H for M10 runs; existing A commands for other jobs | Preserve rights/admin resolution; no blind replay of an uncertain child effect |
| restart-job-worker | POST E/jobs/{jobId}/restart | H for M10; A / `job.worker.restart` otherwise | Agent M10 restart refuses; never release an unresolved runtime effect |
| get-auto-update-policy / set-auto-update-policy (new) | GET / PUT E/updater/policies/{kind}/{targetId} | H only | `kind=project` only; standalone policy input refuses `update.invalid-target`; read/update rights below |
| list-restore-points (new) | GET E/updater/{kind}/{targetId}/restore-points | A / proposed `updater.restore.list` | Target read plus `image-updates:read`; list survives target deletion |
| list-host-restore-points (new) | GET E/updater/restore-points | A / proposed `updater.restore.host-list` | `image-updates:read`; host administrator sees orphaned point metadata |
| delete-host-restore-point (new) | DELETE E/updater/restore-points/{pointId} | H→A / proposed `updater.restore.host-delete` | Host administrator; survives target deletion, same point/run holds |
| delete-restore-point (new) | DELETE E/updater/{kind}/{targetId}/restore-points/{pointId} | H→A / proposed `updater.restore.delete` | Update rights; refuse a point held by an active/unknown restore |
| restore-update (new) | POST E/updater/{kind}/{targetId}/restore-points/{pointId}/restore | H admission only | Update/restore rights; executor uses trusted runtime execute |
| get-system-image-hints (new) | GET E/updater/system-image-hints | H aggregation; proposed A observation `updater.system.hints` | `image-updates:read`; hub applies version cap |
| trigger-upgrade / trigger-update-all | POST E/system/upgrade or /upgrade/all | Existing operator routes; `system.upgrade.run` / existing update-all forwarding | `system:upgrade`; every agent dispatch is capped server-side at running hub version |
| check-upgrade / update-all-status | GET E/system/upgrade/check or /upgrade/all/status | Existing operator reads; no watcher rollout | Existing rights; capped/blocked targets grant no replacement authority |

Project manual updates reuse `run-updater` with `type=project` and the exact
selected project ID. Batch options remain bounded and permission-checked per
resource; an unbounded request may not silently broaden scope. `forceUpdate`
never bypasses image/system/snapshot/admission guards. Dry runs cannot deploy.
The route review must cover legacy updater callers, scheduled jobs and internal
entry points as well as the web path. The existing public agent mutation commands
cannot remain an alternative execution path after activation.

Manual update needs `image-updates:check`, `images:pull`, `activities:read` and
`projects:deploy` plus `projects:update` for a managed stack. Restore
uses that same complete conjunction, including saved configuration writes.
Policy read needs target read
and `image-updates:read`; policy writes need `jobs:manage` and the applicable
stack update rights. Standalone containers have no policy/action endpoint.
Deleting points uses restore rights. All new `{kind}` routes accept only
`project`; other target kinds refuse `update.invalid-target`. The prerequisite
freezes the complete grant conjunction; no new user-role model is implied. Hub grants are checked before joins/conflict IDs,
sync or dispatch; agents require their
manager credential and compatible contract. Logout detaches viewers only.

Image capability IDs use adapter names: `pullImage` and `pruneImages` accept
`host` refs; `removeImage` accepts an `image` ref with `host~image` identity.
Their `hub/0` descriptors gate aggregate entry points when some host grants the
operation. These writes are supported through both Direct and Edge. Registry
capabilities use the management operation IDs above: list/create use `hub/0`;
get/update/test/delete also accept a `registry` ref with its unscoped registry ID.
`clearContainerRegistryCredential` uses the same scope and `registries:update`
grant as `updateContainerRegistry`, calling its existing `clearCredential` input.
Registry management requires global grants; host grants cannot enable it.

## Wire vocabulary and neutral failures

Keep wire DTOs in their actual current homes: `types/image`, `types/imageupdate`,
`types/containerregistry`, `types/jobschedule`, `types/updater` and the runtime
owner's `types/activity`. Do not create parallel registry/job DTO packages merely
because a planning path used a shorter name. New shapes below are proposed names;
the contract owner freezes their exact JSON/schema in the approved implementation.

| DTO | Required projection and invariants |
| --- | --- |
| Image Summary/detail | Retain ID, public tags, repo digests and consumers; add `protectionReasons` (`in-use`, `system`, `restore-point`, `active-update`), safe owning point/run IDs and `usageState=known\|unknown`. Include stopped containers; unknown never means unused |
| Pull stream | `application/x-json-stream`, one JSON value per line: activity identity (`type=activity`, `activityId`), safe status/layer/count progress, then either typed `error` or `done=true`. No success on EOF alone. Failures before streaming use typed HTTP responses; later failures are stream error frames |
| PullOptions | Reuse `imageName` and optional tag; reject conflicting tag/digest inputs, inline credentials and reserved references. Only saved registry credentials or anonymous access; no secret echo |
| PruneImagesInputBody | Retain adopted `mode` (`dangling`, `all`, `olderThan`) and `until`, including documented legacy aliases; invalid/contradictory input is 400. Selection is local to the requested host |
| ImagePruneReport | Retain `imagesDeleted`, `spaceReclaimed`; add per-ID `skipped` reason and `failed` code. Partial results preserve successful removals. Unknown reclaimed bytes are explicitly unknown, never invented |
| UpdateInfo | `state`, `checkedAt`, `imageRef`, optional current/candidate version and comparable digest/platform, `authState`, optional `code`. `hasUpdate` is true exactly for version/rebuild; legacy fields derive from the same result |
| Registry public projection | ID, URL, description, type, enabled, insecure, noncredential configuration, timestamps and `hasCredential`; exclude username/access-key identity, token/password/secret, encrypted forms, ECR tokens and authentication headers |
| Registry write input | Keep existing generic/ECR variants. Secret/credential fields are write-only; omitted fields preserve on edit. An explicit `clearCredential` clears the complete credential; reject simultaneous clear/replacement. Blank password inputs never accidentally erase a saved credential |
| RegistryTestResult | `ok`, `checkedAt`, optional typed `code`; failure uses appropriate non-2xx with the same safe result. No remote error body, username, URL userinfo, token or challenge header |
| AutoUpdatePolicy | `mode=off\|notify\|apply`, `revision`, target kind/ID and schedule job reference; write uses expected revision. Persist actor/key attribution for scheduled permission revalidation. Missing policy defaults off |
| UpdateScheduleConfig | Hub-stored polling/auto-update cron and enable/master gates, revision, hub timeZone and two projected ScheduledJob rows; expected revision on PUT. No remote agent fields or mixed-owner write |
| RestorePoint | Opaque ID, host/registration, stable target identity, source run ID, createdAt, prior immutable images, `volumeDataIncluded:false`, availability, `sourceState=settled\|partial-source`, recorded service presence/stopped state, orphaned flag and safe reason. Host list/delete require no surviving target; no Compose/env bytes |
| DeployedBaseline | Executor-persisted target/registration, source run ID, saved-input revision/hash, complete effective Compose/env/model inputs, inspected image IDs/digests and capture status; public reads expose status/code only |
| RestoreRequest | Point ID, deliberate key, expected current saved revision and acknowledgment when point files replace different saved files; no caller-supplied snapshot/image override. Bind point/current saved-input and inspected-state identities into run intent |
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
| 400 | `image.invalid-reference`, `image.invalid-prune`, `image.reserved-reference`, `image.invalid-archive`, `registry.invalid-config`, `registry.inline-credentials-not-allowed`, `update.invalid-policy`, `update.invalid-target` |
| 401/403 | Existing hub authentication/permission codes only |
| 404 | `image.not-found`, `registry.not-found`, `update.target-not-found`, `restore.not-found` |
| 409 | `image.in-use`, `image.system-protected`, `image.restore-point-protected`, `image.active-update-protected`, `image.changed`, `image.busy`, `registry.exists`, `update.policy-changed`, `update.target-changed`, `update.snapshot-unavailable`, `update.baseline-unavailable`, `update.path-retired`, `restore.in-use`, `restore.unavailable`, `update.config-drift`, `update.image-binding-unsupported`, `restore.shared-reference`, `restore.image-mismatch`, `update.schedule-hub-owned`, `update.agent-local-job-disabled`, `update.requires-hub-update` plus runtime conflicts |
| 429 | `registry.rate-limited`; bounded retry metadata only |
| 502/503 | Registry probes use 502 for `registry.authentication-required` / `registry.authentication-failed`, distinct from hub identity/permission loss; `image.pull-failed`, `image.remove-failed`, `image.tag-failed`, `image.load-failed`, `image.prune-failed`, `image.usage-unavailable`, `registry.unavailable`, `registry.sync-failed`, `registry.credential-unavailable`, `update.check-unavailable`, `update.system-version-unknown` |
| 500 | `update.deploy-failed`, `restore.deploy-failed`, `update.persistence-failed`, `registry.persistence-failed`; message describes the operation without underlying text |

Reuse runtime unknown/interrupted/cancelled/target-removed/target-unavailable codes
for those outcomes. An upstream registry 401/403 becomes a typed 502 probe
result; the public HTTP status never signals hub identity or permission loss. New codes require EN/DE mappings and catalogue/schema checks in the
owning steps. The catalogue includes the non-HTTP skip code
`update.auto-excluded`; `image.busy` and `update.config-drift` also serve as
scheduled skip reasons. Runtime outcome/conflict codes retain their existing
catalogues. Implementation must give every code named in this contract, including
stream/status/skip reasons, EN/DE text and a checked DTO/schema mapping; no raw
fallback text.
#227/#202 do not waive sanitization for the surfaces changed here.

## Image selection and destructive guards

Resolve the inspected full image ID on the executor. All aliases and digests of
that ID share its protection. Query all containers (`all=true`), including stopped
and internal containers; compare inspected image IDs, not display tags. A failed
usage scan refuses removal/prune rather than reporting zero consumers. Protect
running hub/agent images, configured release/override agent and tools images,
active helper images, every restore-point pin and active-update image ID.
Resolve system references freshly and retain known pinned IDs; unresolved
protection evidence fails closed.
Do not use a repository substring or only a current running-container filter.

`force=true` cannot remove or untag a protected ID. Every removal rechecks usage,
system protection, restore/active-run pins and candidate identity before a
non-forced, non-recursive remove. Do not let automatic child-image pruning bypass
this set. The daemon remains the final guard for external container races.

Prune enumerates selected candidates and invokes the same guarded removal service;
a Docker-wide ImagePrune call cannot preserve application-owned restore pins by
itself. Duplicates resolve once per full ID. Report protected/missing/changed
candidates as skipped, and other failures by stable code; successful candidates
remain in the result. System and scheduled prune image legs must use this guard
too. Volume/network prune semantics remain unchanged. #229 owns the guarded
image leg in system/scheduled prune, not a separate unassigned prerequisite.

Before any workload mutation, persist the restore point and install a local
retention tag for each previous image ID (plus repo digest when available).
A tag is a retention aid, not sufficient protection from `prune all`. Restore
resolves the saved immutable ID/digest and verifies the retention tag still points
to it; never resolve yesterday's mutable tag from the registry. Pin registration
and removal checks must share existing admission ownership; a pull that moves a
tag cannot invalidate a published point. External Docker removal/tag changes are
outside application exclusion and make the point unavailable, never silently
select a replacement. No update begins when pin/snapshot persistence fails.

### One image lease per host and reserved tags

Use one executor-local `runs.Admission` lease with scope `image-mutation` for
M10 image pull/tag/load/remove/prune routes, internal point/run pin writers,
update/restore pulls and tag replacement, and the guarded system/scheduled image
prune legs. Existing normal Compose deploy/redeploy pulls and builds are outside
this lease; they retain their existing runtime admission. External Docker and
those existing pulls can move a mutable tag concurrently. Re-inspection before
create and post-deploy ID verification detect that race; no daemon-wide exclusion
is claimed. Runtime target admission precedes the image lease, never the reverse.
Try-acquire returns `image.busy` without waiting or workload effects; a scheduled
apply child records a visible skip and waits for its next ordinary schedule,
never spins/replays. Scheduled apply children run serially per host.

Under the lease, persist preparing-point protection, create/verify retention tags
and commit point availability before workload mutation. After pulling, protect
the candidate ID durably as `active-update` before releasing the lease or entering
D. A pull's intermediate result is protected by the held lease until recorded.
Prune re-reads protections under the same lease before non-forced removal.
Active/unknown run protections survive restart until authoritative settlement
or existing audited administrator disposition, never lease-epoch expiry.
Preparing points and deferred pin releases reconcile conservatively: if busy,
retain protection and retry cleanup later, without changing a settled outcome.
Token-conditional release never settles a receipt or removes durable protection.

Reserve the complete local repository prefix `docklet-retention.invalid/`.
Point tags are `docklet-retention.invalid/points/<point-key>/<service-key>:saved`;
temporary run pins use `docklet-retention.invalid/runs/<run-key>/<service-key>:frozen`.
Each key is a fixed-length lowercase hexadecimal SHA-256 of its domain-separated
server identity (including the exact service key), recorded with the original
identity in private point/run metadata; never insert raw service names or IDs.
Validate the complete reference with the Docker reference parser. Uppercase,
Unicode and punctuation in Compose names cannot produce invalid repository paths.
These tags retain images only; they are never persistent deployment overrides,
pulled, or sent to a registry. Only admitted internal pin writers create/move/
remove them under the host lease. Canonicalize all target references before
comparing the reserved host/prefix, including case-normalized hosts.
Registry configuration/test cannot select this reserved host either. Every
image-producing route refuses reserved targets with `image.reserved-reference`
before daemon mutation; generic tag cannot replace an internal point/run pin.

Upload/load validates all archive tags before Docker load, including Docker
manifest RepoTags and supported OCI reference annotations. Incomplete validation
refuses `image.invalid-archive`; no load-then-inspect window. Retain upload size
limits. Excluded patch tooling obeys the same target guard.
Hide internal tags from display lists, registry lookup and update checks; count
each image ID once. A retained image with no public tag remains listable by ID
with its protection reason and safe point/run IDs. Export/history never expand
snapshots or credentials.

### Exact image selection for update and restore

Freeze each affected service's literal source reference, candidate platform
manifest digest, expected image ID and baseline/configuration identity. Pull by
immutable repository digest under the host lease, verify platform and ID, then
protect the ID and retain a temporary run pin. Assign the selected literal
reference to that verified local ID under the lease. For rebuild this moves the
existing source tag only; version selects the candidate reference and commits the
literal Compose edit as described below. Recheck the reference before create.
The old engine's unverified retag/SystemUser rewrite is replaced by these checks.

Deploy the actual Compose files with effective pull policy **never** for the
runtime command and affected services. Service pull_policy, build and missing-
image fallbacks cannot pull/build after selection. No generated override or
persistent hidden binding is used. Re-inspect each selected reference immediately
before create; a concurrent mapping change refuses rather than retagging blindly.
After deploy inspect every affected container and require its image ID to equal
the frozen expected ID. Missing/different IDs yield
`update.target-changed` or `restore.image-mismatch`, failed/partial evidence and
retained point/run protections. Compose exit alone never proves success.

Restore verifies the selected point's saved ID/digest and retention tag, then
maps the restored literal source reference to that saved ID under the lease.
Before moving any user tag for restore, inspect all host workloads, including
stopped containers and other services: if another workload uses that reference,
refuse `restore.shared-reference` before file/tag/workload mutation. Unknown
usage refuses too. A shared-tag rebuild update has ordinary pull-like tag effects
but never recreates another stack; restore cannot silently retag its shared source.
Candidate IDs remain `active-update` through L→D, verification, settlement retry
and U. Failed/partial runs never promote candidate selection to successful active
deployment state; recovery semantics and source/tag disposition are defined below.

## Registry credentials and sync

Keep `go.getarcane.app/sys/crypto` and the existing operator encryption key;
no new key service. Encrypt token/password/ECR secret material at rest on both
hub and agent. Public DTO conversion is explicit, with `hasCredential` indicating
stored configured material, not successful authentication or decryption. Credential
usernames/access-key identities are write-only under the accepted projection;
editing them requires replacement input. Never return masked placeholder secrets
that a form could save as a new password. Reject URL userinfo and avoid input
values in validation errors. Credentials must not enter audit/request-body logs.

The existing agent key may be generated into the same SQLite settings store as
the ciphertext. This protects partial disclosure, not theft of the complete DB
and its key; no stronger database-theft guarantee is claimed. Before creating or
replacing credentials (including trusted sync), baseline payloads or restore
snapshots, require an available, validated effective key that is neither missing
nor the built-in development default. Refuse with `registry.credential-unavailable`
or `update.snapshot-unavailable` before persistence or workload mutation. Never
fall back to the public development key. Failed sync leaves the prior complete
set intact; unavailable keys also refuse decryption-dependent update/restore.
No key material is returned or logged. A normal deploy may still succeed with
baseline-unavailable status as defined below when capture cannot be encrypted.

Inline pull/check credentials are not accepted in the production contract,
including legacy `auth` and `credentials` fields and registry-auth alternatives
in body/query/headers (ordinary hub authentication is unchanged)
on Direct and Edge. Reject their presence with `registry.inline-credentials-not-allowed`
using neutral validation without echoing values. Select only saved enabled
registries on the executing host (or anonymous public access); trusted registry
sync remains the sole manager-to-agent credential write path.

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
and absence of a newer candidate are distinct from unknown/error. States:

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
A tag changing after check does not redirect the admitted update; the exact
selection and post-deploy verification above are mandatory for version and rebuild.
After update, current digest/version comes from inspected workload images; the
configured reference comes from the actual Compose file (rewritten for version,
unchanged for rebuild). Another check of the same deployed digest is current.
Standalone containers receive these observations only; checks grant no update,
policy-apply, baseline or restore capability.

## Policies and ScheduledJob projection

Every Compose stack starts `off`; `notify` checks, exposes availability and
notifies as below; `apply` checks then updates eligible stacks at the host schedule.
Manual checks/stack updates/restores remain possible with current rights in every
mode, subject to the respective point/safety checks. A Compose child resolves to
its uniquely owned stack and uses that stack's policy. Standalone containers have
update information only: no policy, apply, recreation, baseline or restore points.
Existing standalone mutation entry points refuse `update.invalid-target`.
System containers use the separate operator upgrade procedure.

### Existing labels, exclusions and notify

An adopted updater-disabled label or `autoUpdateExcludedContainers` match wins
over `apply`: skip visibly with `update.auto-excluded`. For a managed stack,
any affected excluded service prevents automatic apply of that stack; no partial
policy bypass. Manual deliberate update may select excluded targets, but never
bypasses system protection, drift, baseline, point or exact-image selection guards.
Tag strategy/constraint/pattern labels still select the candidate channel;
`apply` does not widen them. Preserve dependency ordering from updater depends-on
labels and configured stop signals. Unknown/cyclic dependency ordering refuses
before mutation with `update.invalid-target`, not an arbitrary execution order.

M3 notify means UI badge/status plus one notification entry via the existing
notification module/channels per new candidate version, not per check. Persist
its dedup identity in the **hub SQLite database**, keyed by
host/registration/stack identity and candidate version
(repository, platform, normalized release version/tag and immutable digest).
A same-tag rebuild with a new digest is a new candidate image state; repeated
checks of the same identity, reconnect, restart or policy toggles do not create
another entry. Persist the unique dedup row and notification entry/outbox link
in one hub transaction; dispatch existing channels from that durable entry.
Offline hosts produce no new observations; retained stale metadata cannot emit
a new entry. Existing channel fan-out/retry uses that same entry identity;
a delivery failure never turns a later check into a new notification. With no
configured channel, show the badge and explicit undeliverable/unconfigured
status. Do not claim a notification was delivered. `off` has no policy-triggered
check/notification/apply; `apply` may expose the same candidate observation,
without duplicating an already recorded candidate notification.

### Hub-owned schedule storage and agent neutralisation

Both M10 jobs are hub-owned: `image-polling` (update-check, hourly default) and
`auto-update` (daily midnight default), one of each per registered environment,
including local `0`. Their cron, enable/master gates and actor/key attribution
live in the **hub SQLite database keyed by environment registration ID**. Reuse
the existing coordinator and six-field cron parser with M4's hub IANA time zone;
no new cron engine or remote agent clock authority. The hub dispatches explicit
bounded read checks or admitted runtime effects to the executor on Direct/Edge.
Apply children run sequentially per host and revalidate actor/policy before each
new child admission. No missed-run catch-up burst on reconnect.

The dedicated `GET/PUT E/updater/schedules` is served by the hub under
`jobs:manage`, never generic-forwarded. Its UpdateScheduleConfig returns both
M10 schedules and job rows directly from hub storage without a remote read.
At v7 the legacy job-schedules DTO omits these two fields; a legacy attempt to
write them is refused with `update.schedule-hub-owned` before any write. Reject
mixed M10/agent-owned fields rather than introduce cross-host partial writes.
Existing A schedule commands remain for other jobs. M10 job list rows are added
from the hub records; run/read/retry/cancel/resolve/restart by M10 job ID is
intercepted before generic forwarding and uses the hub coordinator/child receipts.
Consumers can read the dedicated projection independently of non-M10 agent jobs.
Offline remote hosts can retain editable hub schedules without pretending the
executor accepted work; failures are explicit at trigger/dispatch time.

At contract v7 agents do **not** register their inherited `AutoUpdateJob` or
continuous/event-driven `ImageUpdateWatcher`. The hub likewise replaces its old
continuous watcher with the one owned scheduled check source. Agent-mode
job-schedule writes for auto-update/polling fields refuse with
`update.schedule-hub-owned`; manual run/retry/restart of either M10 job refuses
with `update.agent-local-job-disabled`. A stale persisted agent job/actor or
attempted legacy local fire is neutralised before any check/apply effect, with a
visible skipped/refused record and zero Docker mutations. It never creates a
second trigger source. Agent `image_update.*` checks still execute when explicitly
requested by the authenticated hub; system-image hint reads likewise do not
start a local watcher or rollout.

| ScheduledJob field | Wire source and rule |
| --- | --- |
| id / kind | Stable hub host/job ID; `image-polling` → `update-check`, `auto-update` → `auto-update` |
| hostId / stackId | Registered host; stackId only for an explicitly targeted child, never a fabricated host-wide stack |
| enabled / schedule / timeZone | Hub-stored cron and effective global gate; normalized six-field cron and hub zone |
| nextRunAt | Hub-calculated next occurrence; absent when disabled/unavailable, never agent/browser-local arithmetic |
| currentRun / lastRun / lastSuccess | Existing hub durable scheduler identities/status and observation time |
| activityId / target outcomes | Each target's runtime activity/point; a parent batch never substitutes for a child's unknown outcome |
| cadence / title / reason | Localized schedule/kind/typed disabled, exclusion or failure reason; no daemon text |

Use the same cron parser for validation/execution/next-run display; cover missing
and repeated DST local times. Current web ScheduledJob requires nextRunAt and
lacks disabled/unknown metadata; the dependent mapping must represent these
states. Master polling/auto-update gates remain authoritative; apply with a
disabled schedule is visibly disabled. Before each child admission, resolve
current actor/key rights, host registration, baseline and policy revision.
Freeze the policy revision at admission: later policy/permission changes govern
new admissions, not silent cancellation of an accepted run. Explicit cancellation
uses durable cancel; no new per-Docker-call authorization protocol is introduced.

## Restore points and deployment identity

Retain the latest **five available points per logical target**, with explicit
deletion and no time-based expiry. Failed updates keep their point subject to
normal retention. Active/unknown update/restore runs hold their source and
pre-change points outside trimming, so the limit can be exceeded temporarily.
Create and verify a new point before trimming older ones. Cleanup failure never
invalidates it. Release a pin only after durable point deletion and when no
other point/run references its ID. Reconcile interrupted orphan
tags conservatively under the host image lease; never bulk-prune them.

### Recorded deployed baseline and drift refusal

M1/M2 require one executor-persisted `DeployedBaseline` per Compose stack,
written after successful normal editor/runtime deploy, update or restore. It binds
target/registration and source run ID to actual deployment inputs: ordered files,
primary/auxiliary env bytes, resolved Compose model/effective name/env, saved-input
hash/revision and inspected service image IDs/digests. Encrypt payloads; public
reads expose availability/code only. No standalone baseline or logical-ID scheme.

Freeze actual inputs before deploy and capture those inputs/observed IDs after
success; never substitute newer saved bytes. Capture/encryption failure leaves
truthful deploy success plus `update.snapshot-unavailable` status, and cannot
claim an older baseline proves the new state. Failed/partial effects likewise
invalidate current baseline proof without deleting the last successful evidence
or existing points. A normal successful deploy can repair the baseline; it is
**not required before restore**.

Update preflight and the last boundary before workload mutation compare all saved
inputs/revisions and current effective workload IDs with the baseline. Saved but
undeployed changes yield `update.config-drift`: manual refusal and visible auto-
apply skip, translated as “undeployed changes — deploy or discard first”. Missing/
unprovable baseline yields `update.snapshot-unavailable` until normal deploy.
No force/confirmation bypass, legacy bootstrap or config-hash substitution.
The version edit is a planned admitted change: check the pre-edit baseline hash
and then the exact committed post-edit revision/hash before Docker; do not treat
the worker's own verified rewrite as drift or permit any intervening editor write.
These update eligibility rules do not disable recovery through restore.

### Literal Compose edits and admitted file writes

M4 version update rewrites only the affected literal `image:` reference in the
stack's defining Compose file, to the frozen candidate reference. Require a
uniquely provable source location in supported primary-file composition; refuse
`${VAR}`, non-literal, ambiguous/include/override-defined or build-only selection
with `update.image-binding-unsupported` before mutation. Do not guess a primary
field, rewrite env variables or add a generated override. Unrelated bytes/env
remain unchanged. Same-tag rebuild replaces the local image only; files stay
byte-equal. Redeploy/start/editor deploy keep ordinary file-based semantics:
a later explicit pull/redeploy resolves that file's reference afresh; no hidden
pin silently selects an older image. Capture its actual new result as baseline.

All image edits and restore file/env replacements use the admitted actor's
project/file-write contract, expected revision and audit identity. Scheduled work
uses its revalidated recorded actor, never `SystemUser`. Refuse unreadable,
unwritable, changed or unsupported destinations before effects; external env
support does not authorize arbitrary symlink writes.

A candidate selection becomes successful deployment state only after observed
IDs and baseline settlement. Failed/partial runs never leave a hidden active
binding: retain the last successful deployment evidence, record actual partial
IDs, and release no unresolved protection. Capture failure also cannot promote
an unrecorded selection to a successful baseline. A version rewrite already committed
before failure remains an explicit saved-file change recorded with its revision;
restore replaces/reverts it with the selected point's files. A failed rebuild's
moved source tag is returned to its recorded prior local mapping under the host
lease after inspection, without undoing observed partial container effects. If
that tag changed concurrently or cleanup cannot be proved, keep U/protection and
use existing inspection/disposition; never overwrite an external writer blindly.
There is no automatic workload rollback and no persistent baseline-owned tag.

### Snapshot, restore and orphan cleanup

Before update workload mutation, commit an encrypted point of the verified
baseline's complete saved/effective inputs and old inspected image IDs with pins.
Missing capture inputs, key or pins refuse before mutation. **Volume/bind data and
external resources are not saved or restored.** Executor-local payloads are not a
second remote backup; hub metadata is explicitly stale when offline.

Restore is always available as recovery from an existing available point,
including after failed/partial update/restore, missing current baseline or failed
post-success capture; no successful normal deploy is required first. It retains
ordinary rights, admission, available selected pins, key and safe-write checks.
Before any restore effect, create its own encrypted pre-restore point containing
**current saved Compose/env bytes and currently inspected image IDs**, not a
fictional successful baseline. Mark `partial-source` when current state is partial,
unknown relative to the last baseline or has no current baseline. Record absent/
stopped services explicitly; if inspection/capture/pins cannot be completed, fail
safely with `update.snapshot-unavailable` without consuming the selected point.
This point preserves observed recovery inputs, not a claim of healthy deployment.
Restoring a `partial-source` point uses its recorded service presence/stopped state
and inspected images with its saved inputs; it cannot claim to restore a healthy
stack or substitute missing services with registry images.

Bind expected current saved revisions and inspected identity to the deliberate
restore intent. Saved drift is captured, not a baseline-related refusal; explicit
configuration-replacement acknowledgment names the revisions to be replaced,
including any committed failed version rewrite or newer editor changes. A racing
edit refuses `update.target-changed` before overwrite. Validate selected snapshot
composition and shared-reference safety before file/tag changes, then replace
files/env as the admitted actor and deploy saved IDs with pull never plus image-ID
verification. Never down-with-volumes. Replacement and Docker are not atomic;
failed/partial or unknown outcomes retain both points and actual evidence, with
no automatic rollback. Another deliberate restore remains possible after runtime
settlement, using a fresh pre-restore point, even if baseline capture failed.

Host-scoped point listing/deletion resolves metadata by point ID independently
of a live target. Here host administrator means the existing effective global
administrator with the required host-scoped rights, not a new role. Deleted
target points are `orphaned:true`, retain their original safe target identity,
remain listable to that administrator, and are not
restorable onto a replacement target. Per-target listing can still select that
old identity without a target existence check, applying the same administrator
requirement for orphan metadata. Deletion uses current host admin
rights plus point/active-run holds, not permissions on a nonexistent target.
Removing a target never deletes or hides its points implicitly. Active/unknown
runtime work continues to block target removal under the existing runtime rule;
a registration change marks old points unavailable, never adopts them as new.
On explicit orphan-point deletion, atomically remove metadata then release only
unshared tags/protections under the host lease, making the image eligible for
ordinary guarded prune. A failed tag cleanup is recorded for conservative retry;
an unowned stale internal tag alone is not a permanent application prune blocker.
Deleting an entire registered host keeps the existing registration-lifecycle
boundary; remote executor state is not secretly deleted by a hub-only action.

Deployment history links editor deploy, update and restore by stable host/target/
run identity. Baselines, points and pin holds are independent of the 100-activity
output limit and 24-hour compact-receipt minimum; history pruning cannot erase
them. No new unbounded activity-history store is introduced.

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
| Idle / T | Hub schedule fires, notify | Coordinator checks rights/identity; C → result → T, UI observation plus one deduplicated existing-channel entry for a new candidate version; no deploy |
| Idle / T | Hub schedule fires, apply | Coordinator deduplicates trigger; validate rights, policy revision, fresh target and compatibility; admit Q with stable child run ID, then C |
| Idle / T | Manual update requested | Hub permission and idempotency checks admit Q → C independently of off/notify; request retry resolves same run |
| C | Check says current/local/unknown/error or no allowed candidate | T with current/skipped/typed failure as appropriate; no snapshot-required deployment, no false updated result |
| Before Q / C | Apply encounters disabled label/excluded service | Visible update.auto-excluded skip for the whole affected stack; manual deliberate selection still needs all hard guards |
| C | Eligible candidate, baseline and no saved-file drift | Verify admitted baseline/current saved revision, candidate digest and policy identity; persist encrypted point/pins and P before workload mutation |
| Before Q / C / P | No recorded baseline, saved/deployed drift or unsupported image binding | Refuse manual update and visibly skip auto-apply with typed reason; no force/confirmation bypass, no destructive call. Normal deploy must establish a valid baseline |
| C / P | Missing/default key or inline credentials | Refuse credential/snapshot write before effects with typed neutral code; never store under a public development key or echo credential input |
| C / P | Snapshot, pin or persistence fails | Fail before deployment; retain any uncertain pin metadata for reconciliation, zero destructive workload calls |
| C / P / L / R | Image lease busy | Manual T failed `image.busy` before workload effects; scheduled child visibly skips until next ordinary schedule. Retain preparing protections; deferred cleanup retries conservatively |
| P | Start update pull | L takes the host image lease, pulls the frozen digest, verifies image/platform ID and persists active-update candidate protection plus temporary retention pin before releasing the lease |
| L | Pull fails or pulled image does not match | T failed with `image.pull-failed` or `update.target-changed`; retain pre-update point, existing workload/config untouched |
| L | Verified pull succeeds | Recheck baseline/current saved revision; version commits admitted literal edit, rebuild changes only tag mapping; D uses actual files, verified ID and effective pull never |
| L / D / R / U | Prune/remove between pull and deploy or during run | Held host lease or durable active-update/point protection prevents removal; skip/refuse with image.busy or typed protection reason; no re-pull fallback |
| Any / T | Generic tag/load targets reserved namespace | Refuse before daemon mutation; successful archive inspection/namespace guard cannot be deferred until after load |
| D / R | Deploy returns successfully | Inspect every affected container against frozen image IDs; mismatch is failed/partial, never success. With verified IDs, capture baseline and settle outcome/activity; release run protection only after durable settlement, without persistent binding |
| D / R / normal deploy | Effects succeed but new baseline capture fails | Keep truthful workflow success with visible baseline-unavailable code/status; no old baseline may prove the new state. Later update refuses until normal deploy records it; restore remains available and captures current files/IDs as partial-source |
| D / R | Deploy fails midway | Keep point and actual partial evidence; when worker/children settle, T failed with typed deploy code; failed candidate never becomes active deployment state. Record committed version edit or reconcile prior rebuild tag, preserve partial IDs; allow deliberate restore after settlement, no automatic restore |
| Q / C / P / L / D / R / U | Restore requested during update/restore | Same target or parent exclusion returns authorized `run.busy`/`run.cancelling`; no queued automatic restore, second executor or point deletion |
| Idle / T | Restore requested | No current-baseline prerequisite. Check selected point, expected saved revision and replacement acknowledgment; Q captures current saved files + inspected IDs in a pre-restore point, then R / pull never / exact-ID verification |
| D / R failed or capture unavailable → T | Restore requested | Inspect current state; create partial-source pre-restore point from current saved files/IDs, including committed failed rewrite; restore selected point without normal deploy first |
| Before Q / L / R | Unsupported source or shared restore tag | Update refuses nonliteral/ambiguous source with `update.image-binding-unsupported`; restore refuses another workload's shared tag with `restore.shared-reference` before writes/effects |
| T | Explicit normal pull/redeploy after update | Resolve the actual rewritten version reference or unchanged rebuild tag; no hidden override. Verify/capture actual result; ordinary Compose pull stays outside M10 image lease |
| Idle / T / U | Target deletion with retained points | Existing active/unknown runtime work refuses target removal. Otherwise points become orphaned, remain host-listable/deletable, and cannot restore onto a replacement target |
| Orphaned point | Host administrator deletes point | Enforce active/unknown holds and host rights independently of missing target; delete metadata, release only unshared pins under host image lease, make image prune-eligible |
| Agent-local old/stale job | Auto-update or inherited image watcher fires | v7 does not register the source and guards stale invocation: typed skipped/refused outcome, zero independent checks/apply; no new runtime worker |
| Q / C / P / L / D / R | Browser disconnect/logout | Detach waiter; accepted application-lifetime work continues; no new run from a reconnect |
| Q | Hub restart before dispatch | Existing runtime startup settlement marks abandoned local/admitted work interrupted, effects unverified; scheduler reconciles child ID, never resends effects |
| C / P / L / D / R | Hub restart with local executor | Prove old process ended; settle interrupted before readiness, retain point and observed partial state; do not continue workflow from a phase label |
| Q / C / P / L / D / R / U | Hub restart with remote dispatch committed | Restore exclusion, enter/retain U and query matching receipt; no fresh worker, no lease-age expiry and no automatic retry |
| C / P / L / D / R | Agent restart with continuous storage | Existing receiver settles abandoned accepted/executing run interrupted before readiness; hub adopts authoritative receipt; never resumes a destructive step |
| Idle / Q / C | Host offline or Edge disconnected before new admission | Refuse new work with target-unavailable; a scheduler target is skipped/failed explicitly, never silently moved to Direct |
| Q / C / P / L / D / R | Host offline/Edge disconnect after possible acceptance | U with held scope; query same run on recovery. Lost transport proves neither failure nor rollback; point and active candidate remain protected |
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
| Before dispatch | Agent self-upgrade target blank, above running hub or unprovable | Hub caps dispatch; v7 executor refuses blank target with `update.invalid-target`; update-all cannot substitute newest-release metadata for actual running hub version |
| T | Another trigger, same request key, or retry control | Retained key returns original outcome. New deliberate intent rechecks actual state. Job retry cannot replay a child with unknown or partial effects merely because its wrapper failed |

Scope for a Compose-container update is the complete uniquely resolved stack;
route users must be told that it is a stack update. It cannot silently remove
siblings' exclusion or replay only an arbitrary container from a failed batch.
Standalone update/restore input refuses `update.invalid-target`. Runtime update
intent includes frozen candidate/snapshot identity; restore intent includes
point/current-files/inspected-state identity. Neither can be represented by reusing an ordinary redeploy digest with different hidden work.
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
action subject to the same server-side ceiling, never automatic watcher rollout.

### Server-side agent upgrade ceiling

#229 includes the narrow `system/children/upgrade` and hub-forwarding integration
needed to enforce this invariant on the existing operator routes, independent
of UI hints. Before any agent `POST .../system/upgrade`, the hub requires an
explicit release target, validates its compatibility/artifact and compares it
with the **currently running hub process version**. Blank targets refuse
`update.invalid-target`; above-hub targets refuse `update.requires-hub-update`.
Unverifiable custom/digest targets or an unknown ceiling refuse
`update.system-version-unknown`. No agent-side newest-release fallback is
allowed. An already above-hub agent gets
a visible mismatch, not an automatic downgrade.

For update-all, select a real compatible capped candidate for each agent and
send an explicit targetVersion on every dispatch. Do not use the hub release
checker's NewestVersion as the ceiling. Re-read the running hub ceiling before
each send; an intended hub upgrade is not evidence that a newer hub is running.
If the hub must be updated first, stop/refuse that agent target visibly; a later
manual attempt after the new hub actually runs can select a higher target.

At v7 the agent receiver rejects a blank target with `update.invalid-target`;
it never falls back to newest. The hub is the sole running-version ceiling owner,
checking both single-agent and update-all paths on Direct/Edge. Do not add an
agent-side hub-version check, runtime-registration field or ceiling refresh
protocol. Existing manager authentication/current registration remain the trust
boundary; a stolen manager credential retains the accepted root-equivalent risk.
The existing manual-upgrade exception for older incompatible agents still uses
hub-capped explicit dispatch without a new legacy metadata protocol. Manual hub
self-upgrade is separate; its intended future release cannot raise the cap early.

The current agent contract/minimum is **6** with fixture `agent-hub-v6.json`.
The target is **7/minimum 7**, preserving v3–v6 fixtures and adding a v7 fixture whose
exact bytes determine the fingerprint. It covers the updated error/DTO semantics,
image protection, credential sync projection, update/restore runtime operations,
policy/snapshot identity, new routes and allowlists, without hub-version metadata.
Activate routes, behavior,
OpenAPI/generated DTOs, grants, fixture/fingerprint and compatibility enforcement
together after prerequisites and risk review. Older or mismatched agents stay
update-required before forwarding or registry sync; only existing health/version/
manual self-upgrade exceptions remain. `tunnel/v1` transport does not change.
Shared product SemVer is a release-package decision, not this contract integer.
Correct the stale runtime v5 wording in api-contract.md when v7 activates.

## Accepted dispositions and foundation ownership

These decisions were accepted on 2026-10-24. They supersede the original open
recommendations; implementation still waits for independent design review PASS.

| ID | Accepted decision / owned foundation requirement |
| --- | --- |
| D1 | Latest five points per logical target, explicit deletion, active/unknown holds and a mandatory point before restore; host orphan cleanup keeps protection actionable |
| D2 | Stack policy wins for managed children; M5 standalone containers have information only. Two hub-owned jobs per registered host, existing master gates, serial apply children and policy revision frozen at admission |
| D3 → M1/M2 | Recorded deployed baseline after successful normal deploy/update/restore. Refuse saved-but-undeployed drift for manual and automatic update. Every update/restore needs a point; no legacy/bootstrap path or manual bypass. Baseline capture failure is visible and never rewrites a successful deploy as failed |
| D4 | Username/access-key identity and secrets are write-only; actual DTO homes types/containerregistry and types/jobschedule; generic/ECR baseline, no registry browser UI; production pull/check rejects inline credentials |
| M4/M5 | Literal admitted-user version edits, image-only rebuild, ordinary file-based redeploy; stack-only actions/policies/points, standalone information only; restore remains available after failed/partial runs or capture failure |
| M3 | UI status plus one entry through existing notification channels per distinct candidate version, persistently deduplicated rather than emitted per check |
| G1 — #229 | Extend runtime operation/options/target vocabulary, hub admission and executor/project hooks for update/restore, literal image edits, verified image selection, baseline capture and symmetric parent exclusion. Current seven-operation core alone is insufficient |
| G2 — #229 | Hub-owned schedule storage/triggers in backend/pkg/scheduler and job projection; actor/key revalidation, serial child linkage, v7 agent job neutralisation and no-replay recovery |
| G3 — #229 | Forward migration, model registry/integrity entries, baseline/policy/point/protection persistence and validated crypto wiring. The #229 risk review explicitly covers the database migration; never edit shipped migrations |
| G4 — #229 | One host image lease and guarded deletion/pins across image, system and scheduled prune callers; reserved namespace/tag/load protection and orphan-point cleanup |
| Agent upgrade cap — #229 | Narrow system upgrade/forwarding integration enforces explicit targets no higher than the actual running hub on single-agent and update-all paths, on both transports |

#229 is the single foundation owner for these runtime/project, scheduler,
persistence/crypto and system integration paths within M10; they are not
unassigned dependencies for other milestones. Build them before coordinated
contract-v7 activation and dependent web work. Update the owning issue/path
contract through the coordinator; this documentation revision does not write
GitHub state or implement these paths. Identity policy and unrelated volume /
network behavior remain neighbours. The risk review covers agent exposure,
credential storage, the forward migration and update/restore lifecycle; a design
PASS alone is not implementation/release acceptance. No product question remains
open from D1–D4 after M1–M5; new material deviations still require a decision.

## Delivery

Three sequential PRs under #229 target `feature/m10-images-updates`; PR1 merges
before PR2a, then PR2b and their dependent consumers. The coordinator records the complete path
ownership and shared-file ordering before implementation; no neighbour becomes
implicitly writable. PR2a owns lifecycle and migrations (G1/G3); PR2b owns
policies, hub schedules, agent job neutralisation and notify (G2/M3).

| Delivery | Owned paths within the approved integration scope | Tests and independent risk review |
| --- | --- | --- |
| PR1 — image/registry safety and contract v7 base | `backend/internal/{image,imageupdate,registry,settings}` and matching DTOs; generic protection model/new migration/schema registry; narrow `system/children/{prune,upgrade}`, scheduler prune and authenticated forwarding; `types/contract`, Edge table, OpenAPI/generated DTOs and EN/DE catalogues | Image/lease/prune/reserved-tag/archive guard mutations, credential redaction/key/sync transport, UpdateInfo, upgrade cap and compatibility fixtures. Risk review: agent surface/allowlist and credential storage; review any new protection-table migration |
| PR2a/PR2b — sequential lifecycle and scheduling deliveries | `backend/internal/{updater,project,activity,job,notification}`, `backend/pkg/{projects,scheduler}`, `third_party/arcane/updater`; narrow container ingress/bootstrap guards; baseline/point/policy/schedule/dedup models/new migrations/schema registry; runtime/updater/job DTOs, contract/Edge/OpenAPI/generated DTO/catalogue extensions | Full lifecycle/state-table barrier and red proofs, actual file/actor/revision edits, failed-run restore, SQLite migration/reopen, schedule ownership/serial execution, notification dedup, orphan cleanup and stack-only boundaries. Risk review: restore lifecycle and database migration, plus changed agent commands |

PR1 introduces 7/minimum 7 and the v7 fixture/fingerprint; PR2 extends this
**unreleased** contract and regenerates its fingerprint in the same owning step.
Mixed intermediate builds fail fingerprint compatibility. PR1 exposes only its
implemented capabilities: no update/restore/policy/schedule activation until PR2
contracts, guards, migrations and risk review pass. Preserve v3–v6 fixtures;
no intermediate v7 package is a product release. PR1 may unblock image/registry/
update-info consumers; lifecycle consumers wait for PR2. Closing review and exact-
package Direct/Edge acceptance still cover the complete integrated milestone.

### R3 implementation dispositions

At lifecycle activation, retire tag-discovering `POST E/projects/{projectId}/update-services`,
webhook container/project `update` and updater `run`, the legacy single-container
updater and `/updater/run` with typed, translated `update.path-retired`. Only the
new stack-update admission may update images. Discovery-free workspace calls
remain ordinary deploy operations. Webhook runtime start/stop/restart/redeploy/
up/down remain; webhook update authority awaits its separate actor-model review.
PR2a owns narrow webhook/legacy-ingress changes and tests zero file/pull/recreate
effects on refused hub/agent paths; PR1 introduces no workload-update capability.

For PR2a restore, refuse `restore.shared-reference` only if a tag must move and
another running/stopped workload uses that reference with a different saved ID,
or usage is unknown. No move needs no shared refusal. Preflight refuses points
mapping one reference to several IDs (`restore.unavailable`), and divergent update
baselines (`update.baseline-unavailable`). Partial-source restore creates only
recorded-present services, then stops recorded-stopped services; never creates
recorded-absent services. Missing/unprovable baseline and post-deploy capture
failure use `update.baseline-unavailable` (409/scheduled skip; deploy once first);
key/pin/point failures retain `update.snapshot-unavailable`.

Under one image lease hold, check sharing, assign tags and persist protection;
release, then perform expected-revision file writes, then deploy. Busy cannot
leave a file write. Record any file refusal after tag assignment. Drop/reuse a
point at settlement if the run had no workload, file or tag effect; retain failed
points with any effect. PR2a/PR2b extend the unreleased v7 fingerprint sequentially.

## Test matrix and practical acceptance

Tests use controlled effects and real SQLite/HTTP/Edge paths where applicable;
CI needs no live Docker, agent or socket. Synchronize with the actual guard/effect
boundary, never sleeps. Each new behavioral guard needs a targeted red mutation
(remove the guard, observe the intended assertion fail, restore and pass).
Compiler failures, timeouts and equivalent mutations are not evidence.

| Area / method | Required observation and useful mutation |
| --- | --- |
| Registered routes/OpenAPI/generation/allowlist | Hub-only registry CRUD/test, policies and M10 job schedules/controls cannot bypass hub ownership on agents; trusted sync and executor reads work on Direct/Edge; remove ownership/command guard and fail actual dispatch assertion |
| Image service with fake daemon | Running/stopped consumers, aliases, system overrides/helpers and restore pins block force/remove/prune; uncertain usage refuses; remove each predicate independently, assert zero forbidden daemon calls |
| Prune callers and pin race barriers | Interactive/system/scheduled image legs share guards; partial report retains success; pin creation/deletion versus prune cannot lose a published pin; bypass guarded removal and fail |
| Exact binding and pull→deploy barrier (B-D1-01) | Pull frozen digest, pause before D, invoke remove/prune and attempt mutable-tag change; protected candidate survives, effective pull never and every deployed ID equals frozen ID. Drop active protection / restore mutable pull or remove ID verification: fail the actual image-ID assertion, never accept a re-pulled candidate |
| Literal edits/rebuild/redeploy (M4, B-D2-01) | Version changes only admitted literal image field; rebuild leaves files byte-equal; ${VAR}/nonliteral/include/override-defined sources refuse. Normal pull/redeploy follows actual files after either update; no hidden binding. Restore shared tag refuses before writes. Remove source/revision/shared-tag guard or reintroduce hidden override and fail actual file/ID/effect assertions |
| Reserved namespace/tag/archive (B-D1-05) | Tag and every supported Docker/OCI load manifest targeting reserved prefix refuse before daemon call, unknown archive tags refuse, internal tags stay out of registry/check/display projections; drop preflight guard and assert zero forbidden tag/load calls |
| Registry model/handler/real sync transport | Generic/ECR create/edit/clear, encrypted persisted bytes, no public credential field in success/failure/validation/activity/SSE; remove DTO redaction or log sanitization and assert recognizable synthetic secret absent |
| Missing/development keys and inline auth (B-D1-10/12) | Hub and agent credential writes/sync and baseline/point writes refuse missing/default keys; no plaintext/public-key fallback, prior set preserved. Inline auth/credentials refuse without echo over Direct/Edge. Remove each validation independently and assert zero writes/effects |
| Compatibility/revocation/sync failure | v6/fingerprint mismatch, removed/rotated registration and offline Edge send no secrets; failed sync preserves old complete set; mutation permits old-session sync and fails transport assertion |
| UpdateInfo fixtures from producer | Version major/minor/patch, same-tag rebuild, equal digest, platform/index mismatch, local/pinned, missing digest, private denial/rate limit; mutate digest comparison or unknown→current mapping and fail |
| Retired update ingress (PR2a, B-D3-01) | Tag-discovering update-services, webhook update/run, legacy single-container updater and /updater/run refuse update.path-retired on hub/agent with zero file writes/pulls/recreates; remove each ingress guard and fail the actual effect assertion |
| Admission state-table barriers | Schedule/manual collisions, stack/child both ways, distinct-host independence, restore during update, same-key retry and policy revision races; remove conflict/admission-revision recheck and assert extra effect |
| Snapshot/restore persistence | Supported complete Compose/env evidence, encrypted payload, update-without-baseline refusal, pre-effect pin commit, retained failed point, exact digest/config restore, five-point retention and shared pin holds; skip snapshot commit or use mutable tag and fail |
| Baseline, drift and admitted actor (B-D1-02/04) | Successful editor/update/restore captures actual complete inputs/IDs; capture failure leaves truthful deploy success plus unavailable baseline. Saved-but-undeployed edits block manual/auto; restore captures drift with replacement acknowledgment and racing edits refuse; update-without-baseline or mutation-without-point cannot bypass by force/confirmation. Assert expected revisions, admitted actor and no SystemUser substitution; remove drift/point/actor guard and fail zero-write or ownership assertion |
| Orphaned points and pin ownership (B-D1-06) | Delete a settled target, list/delete points as host admin without target permissions, release last unshared pin and prune; U blocks deletion, replacement target cannot inherit point. Drop host-list path or orphan pin release and fail discoverability/prune assertion; retained point pins survive ordinary down |
| Failed/capture-unavailable → restore (B-D2-02) | Crash-loop/partial update and failed baseline capture still permit deliberate restore; failed binding never active, committed version edit is reverted by restore, pre-restore point stores current files + inspected IDs as partial-source. Remove recovery path or substitute old baseline and fail snapshot bytes/ID assertion |
| Lease scope/busy and valid references (B-D2-06/07) | M10 pull/prune/update contention yields typed manual refusal or scheduled skip before effect; normal Compose pull is outside lease and ID mismatch detects its race. Uppercase/punctuation service names yield parser-valid hashed tags; cleanup contention retains protection. Widen lease accidentally or insert raw name and fail boundary/reference assertions |
| Stack-only action scope (M5, B-D2-03) | Standalone check remains readable, but update/policy/restore inputs refuse with zero recreation/baseline writes; Compose child resolves whole stack. Remove action boundary and fail actual effect assertion |
| Partial failure/process reopen | Pull fails without deploy; mid-deploy failure, hub/agent restart, lost ack/Edge disconnect, DB terminal failure, late receipt and administrator recovery; mutate automatic replay/release and assert no second effect/no premature release |
| Actor/current permissions | Policy owner/key revocation, resource identity replacement, current activity/point read denial, browser logout after accepted work; mutate permission/generation checks and fail actual effect/disclosure assertion |
| Schedule ownership and serial execution (B-D1-03/08) | Remote schedule writes/reads hit hub storage, retain hub zone/DST and disabled states; v7 old agent job/watcher fire has zero independent effect. Barrier two scheduled children: at most one applies per host, unrelated hosts independent. Restore agent trigger or concurrent children and fail actual effect-count/order assertion |
| Labels/exclusions and notifications (B-D1-09/11) | Apply cannot override disabled/excluded labels; manual selection retains hard guards; tag constraints, dependency order and stop signal survive. Hub transaction/reopen and repeated checks/restart/policy toggle emit one candidate channel entry, changed candidate emits one new entry; remove precedence/dedup and fail effect/notification-count assertion |
| System hints | Numeric SemVer ordering, H=2.0.0/L=2.1.0 cap, missing capped artifact, A>H, prerelease/custom unknown and tools compatibility; remove cap and assert no above-hub candidate or mutation calls |
| Agent self-upgrade cap (B-D1-07) | Bypass UI: Direct/Edge single upgrade, blank target (`update.invalid-target`), above-hub target and update-all with a newer checker release still refuse/dispatch only explicit capped real target. Mutate to agent newest fallback / hub NewestVersion and assert zero above-hub upgrade calls |
| Dependent web consumer | Generated DTO mapping, unavailable/stale metadata, off/notify/apply, registry 401 distinct from session 401, restore warning, action identity/history, EN/DE; real user gesture and production capability tests |

Sandbox-checkable acceptance of the exact integrated package, separately approved
and executed by the coordinator/operator after implementation:

1. Record package SHA, served bundle and compatible contract/fingerprint on hub,
   Direct and Edge executors; observe real producer fixtures rather than invent
   response shapes. No real-system deploy or release follows from this document.
2. Pull a public and authenticated image on each host; create/edit/test generic
   credentials (ECR where supported by available acceptance credentials); inspect
   all response/activity surfaces for credential echo. Offline sync is explicit.
   Reject inline credentials and reserved tag/load targets before mutation;
   missing/default-key writes fail closed in the controlled sandbox configuration.
3. Refuse removal and force removal for running and stopped usage, agent/tools/
   system images and restore pins, including aliases. Prune a mixed set and
   reconcile removed/skipped/failed entries and reclaimed-space knowledge.
4. Demonstrate an actual version update and same-tag rebuild plus denied/unknown
   checks. Record current/candidate comparable platform digests and ensure an
   unauthenticated private check is never shown as current.
5. Update a stack manually; standalone exposes update info with no actions/policy.
   Verify the point precedes mutation, then restore saved configuration/env and pinned images.
   First establish a recorded baseline through normal deploy; prove updates with
   missing baseline or undeployed edits refuse, capture failure reports unavailable,
   ${VAR}/nonliteral/override-defined sources refuse, literal version edit is visible
   in the file, rebuild leaves it unchanged, and writes retain actor/revision.
   Pause between pull and deploy to exercise protected prune/remove; verify
   pull never and exact image IDs; refuse restore beside another stack sharing
   the tag. Verify subsequent redeploy uses actual files. Check partial failure
   and capture failure followed directly by restore, without normal deploy;
   inspect current saved files/IDs in its partial-source point and revision conflict;
   restore itself always creates a point. Volume/bind data is not saved.
6. Run notify and scheduled apply in the hub zone; observe policy off/global
   disable, policy/permission change, duplicate schedule/manual overlap and a
   created restore point. Verify one notification entry per new candidate through
   existing channels, exclusion/label precedence and serial apply on a host.
   Confirm remote schedule storage is on the hub and a stale agent job/watcher
   cannot execute independently. Interrupt hub/agent/Edge at controlled boundaries and
   verify no automatic replay or false success; inspect actual workload state.
7. Exercise five-point retention and deletion without losing held/shared pins,
   stale/offline metadata, missing pin refusal, and stack deployment history
   linking editor deploy/update/restore to their actual run IDs. Delete a target,
   find/delete orphaned points on the host and prune its now-unheld image; active /
   unknown holds still refuse deletion. Ordinary stack down preserves retained point pins.
8. Demonstrate informational hints with zero watcher replacement calls, and
   server-side agent upgrade refusal for blank/above-hub targets over Direct/Edge,
   including update-all with newer release metadata. Exercise EN/DE, desktop and 390 px, current permissions,
   activity discovery integration and a production bundle without demo paths.

Record honest unsupported-baseline, external Docker-race and storage-loss limits.
Risk interim review must cover agent surface, credentials, persistence and
update/restore lifecycle before dependent web work merges; exact-package practical
acceptance, green integrated checks and closing review remain milestone gates.
