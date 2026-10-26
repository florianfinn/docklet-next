# Stack updates UI contract

## Boundary and prerequisites

This defines the #231 production HTTP consumer and #198 deployment history against `5c1c4dd77cbeb7b5f702e50937bfc64de3c3f9d8`, contract v7.
Use [images and updates](images-updates.md), [policies and PR2c addendum](update-policies.md), [activity discovery](activity-discovery.md) and [adapter rules](adapter-live-capabilities.md).
Merged prerequisites are #229 PR1/PR2a/PR2b, #225 discovery and #230 images UI; PR2c resolves DG-01/DG-03 before their consumers activate. Design approval still gates implementation.
M2 requires a restore point for every update/restore and a recorded baseline before update. Missing baseline refuses manual update and auto-apply; restore remains available.
M1 drift refuses update; M3 exposes existing notification delivery state; M5 limits actions/policies/points to whole stacks. Standalone containers show observations only.
M6 protects restore-target and healthy-baseline anchors and retains one replaceable failed-attempt slot. Volume/bind data is never saved/restored. Agent/tools hints never replace containers.

## UI to contract map

`E` means `/api/environments/{id}`, `P` the raw project ID and `R` a point ID. Validate successful envelopes and domain mapping; only the adapter splits UI IDs `host~raw-id`.

| View or dialog | Endpoint (operation ID) | DTO fields → displayed state |
| --- | --- | --- |
| Stack list, detail/services and dashboard update radar | GET E/projects (`list-projects`); GET E/projects/P/runtime and /updates (`get-project-runtime`, `get-project-updates`) | `ProjectDetails.runtimeServices[].name,image`; `updateInfo.serviceUpdates[service].imageRef,updateInfo`, fallback `updateInfoByRef[ref]` only without a service-specific entry → per-service observation; mismatched reference → unchecked. Aggregated `status,imageCount,checkedImageCount,errorCount,lastCheckedAt` → incomplete/check time, never all-current from a partial result. |
| Container row, detail/inspector and update popover | GET E/containers (`list-containers`) | `ContainerSummary.image,imageId,updateInfo` → observation; no standalone install or policy controls. Managed children identify the complete stack before an action. |
| Observation display shared by both | Above; GET E/image-updates/by-refs (`get-update-info-by-refs`) when required by existing read composition | `ImageUpdateInfo.state` → unchecked/current/version/rebuild/local/unknown/error; `imageRef,checkedAt,currentVersion,latestVersion,currentDigest,latestDigest,platform,authState,code` → identity, time, version level or same-tag rebuild, authentication and translated reason. Ignore raw `error`. Retained observation headers `X-Docklet-Observed-At,X-Docklet-Host-Reason` → stale badge/reason. No client-invented candidate eligibility. |
| Stack recheck panel | POST E/updater/projects/P/check (`check-project-updates`) | `ProjectUpdateInfo.serviceUpdates,updateInfoByRef,status,lastCheckedAt,policyCode` → fresh stack observations and separate bookkeeping warning. Require stack `checkUpdates`. Invalidate matching inventory/policy reads. Standalone containers have display only, with no recheck gesture. |
| Manual update dialog | GET E/updater/projects/P/baseline (`get-update-baseline`), then POST E/updater/projects/P/update (`update-project-images`) | `BaselineStatus.available,code,targetIdentity,savedRevision,observedRevision,autoExcluded,sourceRunId` → baseline, drift and exclusion context; unavailable baseline shows `update.baseline-unavailable` with “deploy once” guidance inside the open dialog, drift shows `update.config-drift`; send `LifecycleRequest.expectedRevision`, UUID `Idempotency-Key`, `Prefer: respond-async`. `RuntimeAdmission.activityId,code,activity` → accepted run, never completed success. Require `installStackUpdates ∧ getUpdateBaseline`; there is no bypass. |
| Deployment tab and selected run | Shared activity discovery: GET E/activities?start=0&limit=-1&includeSummary=true (`list-activities`), never a tab-owned list request; selected runtime GET E/runtime-runs/{runId} (`get-hub-runtime-run`), otherwise GET E/activities/{activityId} (`get-activity`) | `ActivitySummary.id,environmentId,resourceType,resourceId,type,status,startedAt,endedAt,affectedServices,outcomeState,cancelRequested,recovery,stepTrace,imageLifecycle` → host-scoped editor deploy/redeploy, update and restore runs. `project_deploy/project_redeploy/project_update/project_restore`, resource `stack` or historical `project`, supply membership. `imageLifecycle.restorePointId,baselineCode,verified,code` → actual point/result evidence. Selected-run detail uses the existing shared `readDetail` and targeted per-run reader; no new limiter; `output,outputTruncated,messages` load only for that run. Never use updater history. Missing/unreadable source → neutral unavailable; partial source keeps known rows and warns. A complete empty source says no recorded deployments. |
| Restore-point list in Deployments | GET E/updater/project/P/restore-points (`list-restore-points`) | `RestorePoint.id,registrationId,targetId,targetIdentity,sourceRunId,createdAt,images[].service,reference,imageId,digest,present,running,sourceState,volumeDataIncluded,available,orphaned,code` → point identity, saved image/presence state, partial-source warning, availability and explicit no-volume-data warning. No snapshot/configuration bytes. |
| Restore confirmation dialog | Baseline read above, then POST E/updater/project/P/restore-points/R/restore (`restore-update`) | Send `expectedRevision,pointId,replaceConfiguration:true` only after explicit current-configuration replacement acknowledgment naming current revision and selected point; UUID key and async preference. Same admission/run mapping as update; require `restoreRestorePoint ∧ getUpdateBaseline ∧ listRestorePoints`. Missing current baseline does not disable restore. Show pre-restore snapshot and no-volume/bind-data warning. A racing edit remains a typed refusal. |
| Point deletion confirmation | DELETE E/updater/project/P/restore-points/R (`delete-restore-point`) | Validated `MessageResponse` success → re-read list; nested typed error → inline reason; lost response → unknown, re-read the list and reconcile that ID before any deliberate retry. Require `deleteRestorePoint ∧ listRestorePoints`; held points refuse inline. After a lost delete response, an authoritative refreshed absence or `restore.not-found` reconciles as already deleted; never infer prune safety. |
| Host admin orphan points and delete dialog in host settings | GET E/updater/restore-points; DELETE E/updater/restore-points/R (`list-host-restore-points`, `delete-host-restore-point`) | Same point fields, `orphaned,targetIdentity,registrationId` → original target metadata; no restore onto a replacement target. Require host `listHostRestorePoints`, plus `deleteHostRestorePoint` for delete; target existence is unnecessary. Delete semantics as above. |
| Protected point indication | Same point list endpoints, PR2c projection (DG-03) | Required nullable `RestorePoint.protectionReason`: explicit null means unprotected; its enum contains only `restore-target` and `healthy-baseline`. Required `holdState` = `none`, `active` or `unknown`; required `failedAttempt` boolean selects neutral failed-attempt kind, distinct from `sourceState`. Failed-attempt status grants no protection. Render anchor, hold, kind and availability separately; missing/unknown reason, hold or wrong flag type → load error and delete disabled. |
| Stack policy dialog and status panel | GET/PUT E/updater/policies/project/P (`get-auto-update-policy`, `set-auto-update-policy`) | `AutoUpdatePolicy.mode,revision,environmentId,registrationId,targetKind,targetId,targetIdentity,jobId` → off/notify/apply and job link; read with `getAutoUpdatePolicy`; PUT `mode,expectedRevision` under `setAutoUpdate` for off/notify, `setAutoUpdateApply` for apply. `checkedAt,stale,code` → check status; `applyCheckedAt,applyCode` → separate apply result; `bookkeepingAt,bookkeepingCode` → separate persistence warning. Never erase one status with another. |
| Candidate notification badges | Same policy GET | `candidates[].service,imageRef,version,digest,platform,entryId,notificationState` → candidate identity and pending/sending/sent/unconfigured/failed/not-requested. Sending means in flight or outcome unknown, including after restart; only sent means delivered. Read-only status, no resend button. |
| Host schedules in Settings Updates and dashboard schedule widget | GET/PUT E/updater/schedules (`get-update-schedules`, `set-update-schedules`) | `UpdateScheduleConfig.environmentId,registrationId,revision,pollingInterval,autoUpdateInterval,pollingEnabled,autoUpdateEnabled,masterPollingEnabled,masterAutoUpdateEnabled,timeZone,jobs` → two host jobs, master gates, six-field cron and hub zone; read with host `listSchedule`; PUT editable fields plus expected revision under `saveUpdateSchedule` (also require the read descriptor). `jobs[].id,kind,hostId,schedule,enabled,code,nextRun,currentRun,lastRun,lastSuccess` → update-check/auto-update, disabled/owner-unset reasons and server next occurrence. Absent nextRun stays unknown/disabled. Never use legacy job-schedules for M10 or browser cron arithmetic. |
| Agent/tools hint in Settings Updates | GET E/updater/system-image-hints (`get-system-image-hints`) | `SystemImageHints.hubVersion,images[].kind,reference,currentVersion,candidateVersion,compatibleReference,state,requiresHubUpdate,manualOnly,code` → current/available/pinned/unknown and hub-first guidance. Agent candidate must be valid stable SemVer, greater than current and ≤ hub. Tools use compatible reference, preserve pins; tools version numbers do not share hub version ordering. Read with host `listSystemComponents`; no mutation controls or fabricated release metadata. |
| Availability of every new surface | GET /api/capabilities?kind=…&id=… (`get-capabilities`) | `Descriptor.operationId,available,disabledReasonCode,resourceRef.kind,id` intersect implemented mapping → enabled/readable or localized unavailable. Read lazily for the opened target under DG-01; route presence never grants authority. |

Dashboard schedule aggregation reads only the two M10 jobs: one host descriptor read plus one schedule GET per visible authorized host, coalesced by ref; retain successful hosts alongside per-host unavailable/error state, never turn a failed host into empty/all-disabled jobs.

Whole-stack gestures replace `installStackUpdates(stackId, services?)`: remove the subset input and radar per-service sends; a service display links to the complete stack action.
Remove `checkUpdates({containerId})`, container policy writes and check/scheduled policy models from adapter, Overview and Settings. Use stack off/notify/apply plus host schedules only.
Capabilities use PR2c stack refs (`kind=stack,id=host~project`) and host refs (`kind=host,id=host`). Controls intersect every required descriptor above; a write grant never substitutes for its prerequisite reads.
Do not mount per-stack capability queries in lists/radar: inventory badges need no action capability. Fetch one coalesced descriptor read per ref when a dialog, stack detail or settings section opens, including policy, restore points, recheck, orphan lists, schedules and hints. Reuse the identity-scoped cache (cleared on identity/host invalidation, failures evicted); disable while unresolved. Independent actual calls still recheck current rights.

Validate required shapes, nullable collections and protectionReason, required failedAttempt boolean, safe revisions, IDs, times, state/mode/auth/notification enums and code types. Invalid reads are load errors, never empty/current success.
Known reasons use declared EN/DE keys; unknown string reasons use `common.commandErrors.failed`, never raw `message`, `error`, `lastError` or code.
Parse nested `error.code`, runtime `data.code` and proxy top-level `code` by response family; validate authorized conflict IDs before linking. Request JSON async lifecycle admission with matching activity IDs and a complete runtime snapshot.
`ActivityRuntimeAdmission.code` is an unconstrained string in OpenAPI. Decode only the explicit admission set `{run.started,run.joined}`, both declared by `ActivityRuntimeSnapshot.messageCode`; unknown admission codes show the generic fallback and retain ambiguous write knowledge, never success. No backend enum change is needed for this subset validation.
Legacy message-only 200, malformed/lost possible-write responses and 502/504 `agent.unavailable` retain unknown knowledge; definitive pre-admission refusal does not claim effects.

PR2c `protectionReason` comes from the same retention plan over database-available rows, independent of list-time `available/code`; unavailable points can still be anchors.
The hub overlays its admission/receipt evidence on local/Direct/Edge lists and checks holds before forwarding delete; the executor keeps its local guard. `holdState=unknown` means uncertain hub receipt, dominates active; neither permits deletion. Pre-dispatch restores already hold their point.
Explicit deletion of an unheld anchor remains possible. Missing/malformed projection fields never default to null/none; only explicit reason null is valid unprotected state. PR2c owns enum/schema/fixture parity; #231 owns runtime HTTP validation and rendering.

## Dialog state table

Owner = identity generation, host/registration, target identity, dialog generation and selected point/policy revision. Close/navigation/replacement discards stale callbacks/drafts/toasts; aborting viewers does not cancel durable work.
Same-owner 403 remains inline; identity loss clears private data. Controlled dialogs stay open for refusal; accepted lifecycle work follows its real run independently of browser loading state.

| Dialog | Server outcome/event | Visible state and next allowed action |
| --- | --- | --- |
| Update | Accepted (`run.started`/`run.joined`) | Retain real run ID and show queued/running evidence; follow the same receipt, never a replacement intent. |
| Update | Typed refusal | Keep open with localized reason. `update.baseline-unavailable` says deploy once to record a baseline; `update.config-drift` says deploy/discard saved changes. Offer no bypass. |
| Update | Outcome unknown | Show unconfirmed/unknown and inspection guidance; retain key/intent, no automatic resubmit; list absence does not settle a receipt. |
| Update | Lease busy | Inline `image.busy` or runtime conflict, authorized active-run link only; no queued retry. |
| Update | Permission lost | Same-owner 403 inline, disable send; 401/identity loss clears private dialog/requests; durable work continues. |
| Update | Late response after close/navigation | Discard stale dialog/toast effects; valid same-identity receipt may update its existing runtime owner. |
| Update | Stack/host/owner identity change | Clear revision/intent/selection; replacement needs fresh reads and deliberate confirmation. |
| Restore | Accepted | Show durable restore run and real point evidence; hold exclusion until authoritative settlement. |
| Restore | Typed refusal | Keep open with translated point, revision, snapshot or deployment reason; never substitute another point or require a normal deploy first. |
| Restore | Outcome unknown | Show unknown with point/run link; no rollback/success claim or second restore. |
| Restore | Lease busy | Inline typed conflict; leave selected point visible, no retry/delete while held. |
| Restore | Permission lost | Same-owner 403 inline; identity loss clears point, revision and configuration replacement acknowledgment. |
| Restore | Late response after close/navigation | Discard stale dialog/toast effects; durable run remains independently observable. |
| Restore | Stack/host/owner identity change | Clear point/revision/acknowledgment; old point cannot restore onto replacement target. |
| Policy | Accepted | Adopt validated mode/revision; re-read policy/schedules, close only after confirmed save. |
| Policy | Typed refusal | Stay open with reason; revision conflict needs fresh read and deliberate choice, no last-write-wins. |
| Policy | Outcome unknown | Mark unconfirmed; GET policy before another save, no automatic resend or saved claim. |
| Policy | Lease busy | Inline received reason without claiming mode changed; re-read before deliberate retry. |
| Policy | Permission lost | Same-owner 403 inline and disable save; identity loss clears private draft/results. |
| Policy | Late response after close/navigation | Discard old draft/result/toast; same-identity cache invalidation cannot update a new dialog. |
| Policy | Stack/host/owner identity change | Discard draft/revision; new target/owner requires fresh policy/capability reads. |
| Delete | Accepted | Mark deleted only on validated acknowledgment, refresh list; retention anchors require explicit confirmation too. |
| Delete | Typed refusal | `restore.in-use` stays inline and refreshes holds. After uncertain prior delete, authoritative `restore.not-found` means already deleted; otherwise refresh the stale selection. |
| Delete | Outcome unknown | Mark unconfirmed, reconcile ID through an authoritative list; unreadable list retains unknown. No automatic delete replay. |
| Delete | Lease busy | Inline `image.busy`, preserve selection; no queued retry. |
| Delete | Permission lost | Same-owner 403 inline and disable send; identity loss clears private selection/results. |
| Delete | Late response after close/navigation | Discard obsolete dialog/toast effects; do not remove a replacement target's point. |
| Delete | Stack/host/owner identity change | Clear selected ID/confirmation; new target/owner requires fresh authorized list. |
| Schedule | Accepted | Adopt validated revision/projection; re-read the server projection; master gate off stays visibly disabled and owner-unset clears when ownership is recorded, never claim enabled from draft alone. |
| Schedule | Typed refusal | Inline revision conflict (`update.policy-changed`) or `update.invalid-policy` cron error; refresh before retry. `update.schedule-owner-unset` is a pre-save read status: an authorized save records ownership. Master gates remain read-side projections. |
| Schedule | Outcome unknown | Mark save unconfirmed and GET schedule before another deliberate save; no automatic resend. |
| Schedule | Lease busy | Show received typed reason inline, retain draft without claiming saved; refresh before retry. |
| Schedule | Permission lost | Same-owner 403 inline and disable save; identity loss clears draft/results. |
| Schedule | Late response after close/navigation | Discard old dialog/toast/revision updates; current job observation retains its independent owner. |
| Schedule | Host/owner identity change | Clear draft/revision/confirmation; new host/owner needs fresh schedule and descriptors. |

Policy/schedule edits govern future admission only; switching off never claims an accepted run was cancelled.

## Break map and planned behavioural proofs

Each case uses a fresh adapter/view, actual gestures, deferred read/publication barriers and actor B against actor A's durable state. Reload/restart and exact write counts must be asserted; no sleeps. Red proof must be a matched test's behavioural assertion failure.

| Item | Planned test and observation | Planned red mutation |
| --- | --- | --- |
| Run pending (wire queued) | `reload-queued`: cold deployment/dialog shows Queued; actor B cannot create a second effect; reload preserves run identity. | Map queued to success or drop its exclusion. |
| Run running | `reload-running`: cold follow shows active run; concurrent same-target update refuses and independent host stays usable. | Treat a cold active summary as terminal/absent. |
| Run outcome unknown | `reload-unknown`: retained receipt unknown/disposition labels survive reload and B conflict; absent list row never settles the receipt. Lost response without known ID only offers inspection, no replay. | Map unknown to failed-unblocked or automatically resend after reload. |
| Notification pending | `reload-notification-pending`: persisted candidate entry stays pending across reload and B check/toggle. | Treat pending as sent. |
| Notification sending | `reload-notification-sending`: persisted sending after restart remains in-flight/unknown after B check; zero UI resend writes. | Map sending to sent/failed or create resend intent. |
| Stale check | `reload-stale-check`: persisted policy stale/check time and retained inventory timestamp remain visible; B apply result does not refresh check state. | Clear stale on mount or use applyCheckedAt as checkedAt. |
| Restore in progress | `reload-restoring`: cold project_restore run plus selected point evidence; B restore/delete refusal shown inline, no second effect. | Drop restore kind/point or enable a duplicate restore from local idle state. |
| Notification terminal states | `notification-states`: sent, failed, unconfigured and not-requested are distinct; B toggle and reload preserve server observation. | Collapse all delivery states into sent. |
| Check/apply/bookkeeping separation | `policy-status-channels`: B check success cannot erase prior apply drift refusal; apply cannot stale notify check; bookkeeping failure leaves successful observations visible. | Reuse one code/time pair for all three channels. |
| Inventory response boundary | `observation-shapes`: every state/auth enum, null/missing/wrong fields, invalid dates, shared-ref service candidates and ref replacement render accurately without all-current from partial state. | Cast unchecked JSON, permit stale identity match, or force unknown to current. |
| Check response boundary | `check-shapes`: project service/ref map and nested/flat single-image checks validate; optional policyCode does not replace successful check; raw error absent from UI. | Ignore wrong service map or expose raw errorMessage. |
| Baseline response boundary | `baseline-shapes`: validate target/revisions/available/autoExcluded; missing baseline displays refusal and deploy-once guidance, drift uses its distinct code; restore still gets savedRevision. | Coerce availability to true or gate restore on baseline availability. |
| Baseline refusal | `baseline-refusal`: initial missing baseline shows deploy-once guidance; after B invalidates a previously read baseline, real submit receives 409 update.baseline-unavailable and stays open; auto-apply skip uses applyCode, restore stays usable. | Hide refusal, close on submit, offer a bypass/force flag or disable restore with missing baseline. |
| Point list boundary | `point-shapes`: null/empty permitted collections, unavailable/orphan/partial-source/volume flag, image presence, required nullable protectionReason, holdState enum and failedAttempt boolean; valid null renders unprotected, missing/unknown/wrong types fail loading and keep delete disabled; reject identity mismatch/volume inclusion. | Default missing/unknown fields to null/none, accept a nonboolean failedAttempt, accept another target's point or omit no-data warning. |
| Lifecycle admission/detail boundary | `admission-shapes`: 200/202, equal IDs, admitted failure with ID, conflicting IDs, malformed snapshots, unknown admission code, proxy/transport loss and late detail; no fabricated success. | Accept message-only 200 or unknown admission as success. |
| Conflict/error boundary | `refusal-shapes`: nested image error, data runtime conflict, top-level proxy, unknown/nonstring code and optional authorized conflict ID. Gesture keeps dialog open. | Read raw message as code or expose unauthorized active ID. |
| Policy/candidate boundary | `policy-shapes`: mode/revision/registration/target, three time/code channels and all candidate/notification fields; unknown enums fail visibly. | Default invalid mode to apply or notification enum to sent. |
| Schedule boundary | `schedule-shapes`: booleans, revisions, zone, nullable job/run fields, absent nextRun, disabled/master/owner-unset; B save causes visible revision refusal; dashboard preserves other hosts on one host failure. | Fill missing nextRun with browser time, merge legacy settings, or let one host failure empty/disable all hosts. |
| Hint boundary | `hint-shapes`: stable version ordering, above-hub candidate rejected, no downgrade, unknown/custom metadata and pinned tools/compatibility. | Remove numeric cap or treat tools SemVer as hub version. |
| Host identity changes | `dialog-host-change`: held read/write, switch host and registration, release abort-ignoring response; zero old dialog/toast/cache publication to new owner. | Remove host/registration owner check. |
| Stack identity changes | `dialog-stack-change`: same display name/raw ID with new targetIdentity cannot reuse point/revision/confirmation. | Key ownership by display name/raw ID alone. |
| Owner/permission changes | `dialog-owner-change`: replace/logout viewer during held body, actor B takes policy/schedule revision, current rights disappear; privacy clear and inline same-owner 403, no late send. | Remove identity-generation guard or expectedRevision. |
| Close/navigation and late response | `dialog-late-response`: close/reopen or navigate before held refusal/success; no stale dialog/toast, durable run still discoverable. | Use only isMounted or omit dialog-generation check. |
| Capability boundaries | `capability-unavailable`: test every missing/failed/denied read and write descriptor independently, including W without R; update/restore/delete disable and direct calls send zero writes. | Gate on write alone, drop any required read conjunct or treat route presence as authority. |
| Explicit delete/anchors | `point-delete`: cancel sends zero, confirm sends one; actor B restore admission after list causes inline restore.in-use. Exact PR2c reasons/holds render independently of available; failedAttempt selects neutral kind; explicit null is unprotected, unavailable anchors retain their reason and independent holds. | Infer reason from list order/availability, label failed slot protected or close before refusal. |
| Deployment source | `deployment-membership`: editor/update/restore, host collisions, unavailable/partial/empty source and recovery; opening tab over shared discovery causes zero extra list reads; selected detail shares existing readDetail/targeted per-run reads, without a new limiter. | Omit stack resource mapping/kinds, use updater history/demo records, start a tab-owned list or bypass readDetail with duplicate/per-row detail requests. |
| Direct/Edge displayed parity | `transport-parity`: identical producer-derived inventory, checks, points, runs, policy, schedules, notifications and hints yield identical UI; failure metadata stays truthful. Coordinator later verifies actual forwarding. | Drop one projection on Edge or fabricate Direct fallback/success. |
| Production and localization | `production-updates`: real routes/gestures in EN/DE, catalogues nonempty and ICU/placeholder parity; production graph contains no demo module. | Import demo fixture or remove a locale key/ICU placeholder. |
| Run cancelling | `reload-cancelling`: cancelRequested/run.cancelling survives reload; actor B sees held conflict until real settlement, not rollback. | Clear cancel intent on mount or treat cancelling as terminal/unblocked. |
| Schedule in progress | `reload-schedule-running`: jobs.currentRun survives reload while B saves a new revision; old run remains visible and future schedule changes do not cancel it. | Erase currentRun on save or mark it succeeded from the new enabled flag. |
| Delete outcome unknown | `reload-delete-unknown`: fresh authoritative list reconciles prior uncertain delete; B deletion and restore.not-found from a later explicit retry reconcile as already deleted, unavailable list remains unknown. | Replay delete automatically or show not-found after uncertain delete as failure. |
| Point hold active | `reload-point-active`: cold list shows active for local/Direct/Edge pre-dispatch admitted restore; B delete refuses, unchanged after reload. | Omit hub overlay or map active to none. |
| Point hold unknown | `reload-point-unknown`: uncertain hub receipt dominates local active after reload; B restore/delete stays blocked and no release is inferred. | Drop unknown dominance or clear hold on reload/transport loss. |
| Capability request budget | `capability-lazy`: 100 stack rows/radar badges issue zero per-stack capability reads; opening one dialog or stack detail issues one coalesced ref read, duplicate consumers add zero. Settings sections read one descriptor ref per opened host. | Mount a query per row or remove same-ref coalescing. |
| Stack-only gestures | `stack-actions`: radar action sends whole stack with no service subset; Overview/Settings write only project policies and accepted modes; container update/policy/recheck gestures absent. | Send radar services subset or allow container target in policy/check writer. |
| Schedule dialog outcomes | `schedule-dialog`: real save exercises revision/cron refusals; initial read shows owner-unset, confirmed save clears it through fresh projection, master-off stays visible. | Close before refusal, ignore expectedRevision, retain stale owner-unset after confirmed save or derive enabled solely from local draft. |

## Typed reason and catalogue coverage

`PolicyReason` means the exact 62-value OpenAPI enum `UpdaterAutoUpdatePolicy.applyCode`; `RuntimeCode` means `ActivityRuntimeSnapshot.messageCode` (`api/openapi.json:1`).
Every row parameterizes reason-catalogue/real-gesture tests for declared codes and nonempty EN/DE keys. Remove/substitute each mapping for red proof; unknown reasons display only common.commandErrors.failed. These are plans, not executed mutations.

| Declared code (source) | EN/DE catalogue key |
| --- | --- |
| `image.active-update-protected` (`PolicyReason`) | `images.errors.image_active-update-protected` |
| `image.busy` (`PolicyReason`) | `images.errors.image_busy` |
| `image.changed` (`PolicyReason`) | `images.errors.image_changed` |
| `image.in-use` (`PolicyReason`) | `images.errors.image_in-use` |
| `image.invalid-archive` (`PolicyReason`) | `images.errors.image_invalid-archive` |
| `image.invalid-prune` (`PolicyReason`) | `images.errors.image_invalid-prune` |
| `image.invalid-reference` (`PolicyReason`) | `images.errors.image_invalid-reference` |
| `image.load-failed` (`PolicyReason`) | `images.errors.image_load-failed` |
| `image.not-found` (`PolicyReason`) | `images.errors.image_not-found` |
| `image.prune-failed` (`PolicyReason`) | `images.errors.image_prune-failed` |
| `image.pull-failed` (`PolicyReason`) | `images.errors.image_pull-failed` |
| `image.remove-failed` (`PolicyReason`) | `images.errors.image_remove-failed` |
| `image.reserved-reference` (`PolicyReason`) | `images.errors.image_reserved-reference` |
| `image.restore-point-protected` (`PolicyReason`) | `images.errors.image_restore-point-protected` |
| `image.system-protected` (`PolicyReason`) | `images.errors.image_system-protected` |
| `image.tag-failed` (`PolicyReason`) | `images.errors.image_tag-failed` |
| `image.usage-unavailable` (`PolicyReason`) | `images.errors.image_usage-unavailable` |
| `job.skipped` (`PolicyReason`) | `images.errors.job_skipped` |
| `registry.authentication-failed` (`PolicyReason`) | `images.errors.registry_authentication-failed` |
| `registry.authentication-required` (`PolicyReason`) | `images.errors.registry_authentication-required` |
| `registry.credential-unavailable` (`PolicyReason`) | `images.errors.registry_credential-unavailable` |
| `registry.exists` (`PolicyReason`) | `images.errors.registry_exists` |
| `registry.inline-credentials-not-allowed` (`PolicyReason`) | `images.errors.registry_inline-credentials-not-allowed` |
| `registry.invalid-config` (`PolicyReason`) | `images.errors.registry_invalid-config` |
| `registry.not-found` (`PolicyReason`) | `images.errors.registry_not-found` |
| `registry.persistence-failed` (`PolicyReason`) | `images.errors.registry_persistence-failed` |
| `registry.rate-limited` (`PolicyReason`) | `images.errors.registry_rate-limited` |
| `registry.sync-failed` (`PolicyReason`) | `images.errors.registry_sync-failed` |
| `registry.unavailable` (`PolicyReason`) | `images.errors.registry_unavailable` |
| `restore.deploy-failed` (`PolicyReason`) | `images.errors.restore_deploy-failed` |
| `restore.image-mismatch` (`PolicyReason`) | `images.errors.restore_image-mismatch` |
| `restore.in-use` (`PolicyReason`) | `images.errors.restore_in-use` |
| `restore.not-found` (`PolicyReason`) | `images.errors.restore_not-found` |
| `restore.shared-reference` (`PolicyReason`) | `images.errors.restore_shared-reference` |
| `restore.unavailable` (`PolicyReason`) | `images.errors.restore_unavailable` |
| `run.busy` (`PolicyReason`) | `images.errors.run_busy` |
| `run.cancelling` (`PolicyReason`) | `images.errors.run_cancelling` |
| `runtime.interrupted` (`PolicyReason`) | `images.errors.runtime_interrupted` |
| `runtime.outcome-unknown` (`PolicyReason`) | `images.errors.runtime_outcome-unknown` |
| `runtime.parent-unresolved` (`PolicyReason`) | `images.errors.runtime_parent-unresolved` |
| `runtime.target-changed` (`PolicyReason`) | `images.errors.runtime_target-changed` |
| `runtime.target-unavailable` (`PolicyReason`) | `images.errors.runtime_target-unavailable` |
| `update.agent-local-job-disabled` (`PolicyReason`) | `images.errors.update_agent-local-job-disabled` |
| `update.auto-excluded` (`PolicyReason`) | `images.errors.update_auto-excluded` |
| `update.baseline-unavailable` (`PolicyReason`) | `images.errors.update_baseline-unavailable` |
| `update.check-unavailable` (`PolicyReason`) | `images.errors.update_check-unavailable` |
| `update.config-drift` (`PolicyReason`) | `images.errors.update_config-drift` |
| `update.deploy-failed` (`PolicyReason`) | `images.errors.update_deploy-failed` |
| `update.image-binding-unsupported` (`PolicyReason`) | `images.errors.update_image-binding-unsupported` |
| `update.invalid-policy` (`PolicyReason`) | `images.errors.update_invalid-policy` |
| `update.invalid-target` (`PolicyReason`) | `images.errors.update_invalid-target` |
| `update.path-retired` (`PolicyReason`) | `images.errors.update_path-retired` |
| `update.permission-denied` (`PolicyReason`) | `images.errors.update_permission-denied` |
| `update.persistence-failed` (`PolicyReason`) | `images.errors.update_persistence-failed` |
| `update.policy-changed` (`PolicyReason`) | `images.errors.update_policy-changed` |
| `update.requires-hub-update` (`PolicyReason`) | `images.errors.update_requires-hub-update` |
| `update.schedule-hub-owned` (`PolicyReason`) | `images.errors.update_schedule-hub-owned` |
| `update.schedule-owner-unset` (`PolicyReason`) | `images.errors.update_schedule-owner-unset` |
| `update.snapshot-unavailable` (`PolicyReason`) | `images.errors.update_snapshot-unavailable` |
| `update.system-version-unknown` (`PolicyReason`) | `images.errors.update_system-version-unknown` |
| `update.target-changed` (`PolicyReason`) | `images.errors.update_target-changed` |
| `update.target-not-found` (`PolicyReason`) | `images.errors.update_target-not-found` |
| `run.started`, `run.joined` (`RuntimeCode`, admission subset) | `activity.status.queued` / `activity.status.running`, selected by validated snapshot status; admission itself is not completion. |
| `run.request-mismatch` (`RuntimeCode`) | `activity.runtime.mismatch` |
| `runtime.target-removed` (`RuntimeCode`) | `activity.runtime.targetRemoved` |
| `runtime.cancelled` (`RuntimeCode`) | `activity.runtime.cancelledBody` |
| `runtime.failed` (`RuntimeCode`) | `activity.runtime.failedBody` |
| `runtime.wait-timeout`, `runtime.waiter-disconnected` (`RuntimeCode`) | `activity.runtime.unconfirmed` plus `activity.runtime.unconfirmedBody`; receipt evidence selects unknown/terminal variants. |

401/403 use `common.commandErrors.signIn/denied`; agent transport uses `unreachable/credentialRejected/updateRequired` there. Capability fallback uses `activity.capability.unavailable` or `mappingUnavailable`.
PR2c projection enum keys are owned in `web/src/shared/messages/{en,de}.ts` under `common.restorePoints`: `protection.{restore-target,healthy-baseline}`, `unprotected`, `hold.{none,active,unknown}` and `kind.{point,failedAttempt}`. This subtree is read-only for #231; unknown projection values fail loading, never a new raw reason.

## Dependency gaps and decision boundary

| ID / owner | Missing contract and evidence | Affected stop / required prerequisite |
| --- | --- | --- |
| DG-01 / #229 PR2c | `backend/internal/activity/capabilities.go:21` (default at :46, dispatch at :65) omits needed read/delete descriptors and leaves existing update/check/policy/schedule/hint descriptors unavailable. | PR2c supplies the 14 scoped IDs and exact grant conjunctions in its addendum; consume mode-specific policy and required read/write intersections above. Do not synthesize availability; stored inventory/shared history remain independent. |
| DG-03 / #229 PR2c | `backend/internal/project/lifecycle_points.go:44` and UpdaterRestorePoint (`api/openapi.json:1`) lack protectionReason/holdState/failedAttempt; retention protections are private (`backend/internal/project/children/lifecycle/retention.go:14`). | PR2c adds required nullable protectionReason, holdState and neutral failedAttempt, planner parity, hub overlays/pre-forward delete guard and declared enums/keys. Consumer validation waits for the merged contract; never infer anchors/holds from history. |

M2 is binding; no other required endpoint is missing at this base. No open product question remains.

## Test plan and file ownership

#231 owns web domain update/policy/point/schedule models, data ports/queries/live integration and focused HTTP observation/lifecycle/policy/schedule/hint decoders/tests.
UI scope covers stacks/detail/Deployments, container display, shared updates, stack command hooks, Settings Updates/host settings, dashboard update/schedule widgets and EN/DE catalogues. Remove old subset/container-policy gestures in their actual callers/adapter methods.
Suite groups: observation, dialogs/lifetime, policy/schedules, points, parity and history. Record each break-map and per-code mutation in evidence; keep shared activity/coalescer contracts intact.
Backend/types/OpenAPI/generated DTOs are read-only and owned by PR2c. Its enum text keys are read-only for #231; #230 image/registry UI, migrations and #242 retention remain neighbours. Update only touched adopted-file provenance headers/sidecar hashes.
Phase 1 changes this note only and runs text scope/Node/publication/structure/commit checks. Phase 2 uses focused Vitest/TypeScript/ESLint, ICU/catalogue/production checks and full root lint/test under the load rule (GOMAXPROCS=2, VITEST_MAX_WORKERS=2).
Restore each mutation before integrated checks; report actual exits/counts. Design approval does not replace implementation review or coordinator-owned exact-package real UI Direct/Edge acceptance with restart, two actors, EN/DE and narrow screen.
