# API contract and UI mapping

The registered Huma hub and agent routes produce `api/openapi.json`. Run
`pnpm install --dir web` and `pnpm run contract:generate` at the repository root
after changing a route or wire DTO. The command uses the Go `openapi` subcommand
without starting a service, then runs pinned `openapi-typescript` 7.13.0 to write
`web/src/api/generated/schema.ts`. It also refreshes the spec's provenance hash.
`pnpm run contract:check` compares both committed files with fresh output. Root
lint and test invoke this check, so CI rejects either kind of drift. Generated
DTOs belong at the transport boundary; feature components use the view models
in `web/src/domain/` and the adapter port in `web/src/data/`.
The generated declaration header takes its retrieval date from the OpenAPI
source's provenance record, so repeat generation produces identical bytes.

The generated declaration file has an exact size exemption because the current
schema produces more than 19,000 lines. ESLint ignores only that exact file;
TypeScript still compiles it as part of the web project. The generator is an
MIT-licensed development dependency and adds no runtime client.

## Activity list source completeness

`GET /api/environments/{id}/activities` adds the optional response header
`X-Docklet-Host-Reason: agent.unavailable` only to HTTP 200 pages that fall back
to hub-stored summaries because a remote transport is unavailable. The existing
paginated body keeps `success: true`: the hub read succeeded, while the agent
source is incomplete. Empty partial pages carry the same header. Stored summaries
retain their status and timestamps and are attributed to the registered host ID;
this metadata supplies no fresh observation time or execution-completion evidence.

The signal applies per page. Readers must inspect every page and treat the entire
host read as incomplete if any page is marked. Such a read must preserve earlier
agent observations instead of treating their absence as deletion. Only a complete
successful host read can clear the incomplete-source condition. Healthy complete
local/remote reads, including empty reads and recovery, omit the header.

Only typed transport-unavailable errors (Direct dial/timeout or disconnected
Edge transport) permit this fallback. Credential rejection, authorization,
upstream HTTP errors, decode/mapping failures and caller cancellation keep their
existing error behavior and do not become partial HTTP 200 responses. This
endpoint-specific hub read metadata grants no runtime execution or negative
authority. Agent compatibility is governed by the lifecycle contract below;
`tunnel/v1` and inventory-read behavior are unchanged.

## DTO to view-model gaps

Each row names one mapping gap and exactly one accountable issue or planned
milestone. Names refer to OpenAPI component schemas and current web types; where
several DTOs share one transformation, they are grouped as one feature boundary.
The planned milestones are not yet GitHub milestones. An owner must implement
the mapping and its contract tests before that feature claims live data.

| DTO schemas or absent wire shape | Web view models and port | Mapping gap | Owner |
| --- | --- | --- | --- |
| `EnvironmentEnvironment`, `EnvironmentAgentPairResponse` | `Host`, `AgentReport`, `HostsApi` | Identity, connection state, last successful contact and nullable agent checks map through the host HTTP reader. Onboarding writes and installation metadata remain separate. | M5 #81 reads and state; #84 onboarding |
| `SystemActorHostInfo`, `SystemActorInstance` | `SystemComponent`, `HostsApi` | Actor diagnostics need a host component projection and availability state. | Planned M5 Hosts, transport and read inventory |
| `EnvironmentHubDiscovery` | `HubDiscovery`, `HubApi` | Admin-only inspection supplies a pinned agent image and configured-first URL candidates with unverified reachability hints. | M5 #84 onboarding |
| `ProjectDetails`, `ProjectRuntimeService`, `ProjectTag` | `Stack`, `StackDetail`, `Service` | Host-scoped IDs, runtime service status/health/ports and project tags map into stack reads; retained observations accompany disconnected reads. | M5 #82 |
| `ContainerDetails`, `ContainerSummary`, `ContainerComposeInfo` | `ContainerInfo`, `DockletAdapter` | Container summaries map host-scoped IDs, compose/run/unclear origin, protected recreation restrictions and display state. | M5 #82 |
| `DashboardSnapshot`, `DashboardSnapshotContainers`, `DashboardSnapshotImages` | `HostMetrics`, `ContainerMetrics`, `MetricsSnapshot` | Aggregate dashboard payloads do not directly provide the per-host view and metric units. | Planned M5 Hosts, transport and read inventory |
| `ActivityActivity`, `ActivityMessage`, `EventEvent` | `ActionRun`, `OpsEvent`, `CodedActionRun`, `CodedEvent` | Map persisted step traces, affected services and message codes into the HTTP adapter view models. | #38 |
| `JobscheduleConfig`, `JobscheduleJobStatus`, `JobscheduleJobRunResponse` | `ScheduledJob`, `DockletAdapter` | Job state and next-run time need a common schedule projection using the M4 hub time zone. | Planned M10 Images and updates |
| `StreamEvent`, `ActivityStreamEvent`, `EventStreamEvent` | `subscribeActions`, `subscribeChanges` | SSE channel envelopes need parsing, reconnect and query invalidation. | #38 |
| `ProjectDeployOptions`, `ProjectCreateReponse`, `SystemConvertDockerRunResponse` | `DeployInput`, `ImportResult` | UI drafts need lossless Compose fields and action identity; one-time import stays demo-only. | [M7 stack authoring](stack-authoring.md) |
| Single-container `MessageResponse`; bulk `ContainerActionResult` remains separate | `ActionRun`, `runContainerCommand` | Map single start/stop/restart admission IDs and outcome knowledge; bulk partial results do not prove a single run succeeded. Edit/health/recreation are outside this mapping. | [M6 runtime control](runtime-control.md) |
| Project lifecycle NDJSON and message responses | `runStackCommand`, `StackCommand` | Map start→deploy/up, stop→down, restart and redeploy admission and outcomes; preserve legacy stream/message callers. | [M6 runtime control](runtime-control.md) |
| Echo WS JSON `LogMessage` (outside OpenAPI) | `LogLine`, `subscribeLogs`, `subscribeContainerLogs` | Use subscription-attempt/seq identity and optional stream/truncation metadata; seq is arrival order, without cross-service chronology or durable replay. | [M6 runtime control](runtime-control.md) |
| `WorkspaceFileEntry`, `WorkspaceFileContent` | `DirListing`, `FileContent`, `VolumesApi` | Wire file entries need tree, path and editable-content projections. | [M8 storage contract](storage-workspace.md) |
| `VolumeVolume` | `Volume`, `VolumeDetail` | Storage kind and consumers need a host-aware projection. | [M8 storage contract](storage-workspace.md) |
| `VolumePrunePlan`, `VolumePruneResult` | `VolumePrunePlan`, `VolumesApi` | Candidate reasons and execution re-check results need a server contract. | [M8 storage contract](storage-workspace.md) |
| `NetworkInspect`, `NetworkTopology` | `Topology`, `TopologyNetwork` | Docker topology needs a graph projection across hosts. | Planned M5 Hosts, transport and read inventory |
| `NetworkCreateRequest` | `NetworkDraft` | IPAM and validation fields need a writable form projection. | M9 host-side validation and IPv4/IPv6 wire mapping; see [network management](network-management.md) |
| `NetworkConnectContainerRequest`, `NetworkDisconnectContainerRequest`, `NetworkContainerEndpoint` | `ConnectNetworkInput`, `NetworkAttachment`, `connectNetwork`, `disconnectNetwork` | Container attachments need host-aware validation and fixed IPv4/IPv6 address mapping. | M9 host-side validation and IPv4/IPv6 wire mapping; see [network management](network-management.md) |
| `ImageSummary`, `ImageListResponse` | `DockerImage`, `ImagesApi` | Image records need consumers and UI usage state. | Planned M5 Hosts, transport and read inventory |
| `ImagePullOptions`, `PruneImagesInputBody`, `ImagePruneReport` | `PullImageInput`, `pullImage`, `removeImage`, `pruneImages` | Pull and removal inputs and prune results need host-scoped, guarded action mapping. | Planned M10 Images and updates |
| `ContainerregistryContainerRegistry` | `ImagesApi` | Registry credentials and image source metadata need a safe management projection. | Planned M10 Images and updates |
| `ImageUpdateInfo`, `ImageupdateResponse`, `ProjectServiceUpdateInfo` | `UpdateInfo`, `AutoUpdatePolicy` | Version versus rebuild and scheduling need explicit UI states. | Planned M10 Images and updates |
| No restore-point DTO | `restoreRestorePoint` | A saved pre-update image and configuration need a selectable restore point and action identity. | Planned M10 Images and updates |
| `VulnerabilityRiskOverview`, `VulnerabilityRiskFinding`, `VulnerabilityScanResult` | `SecurityOverview`, `Finding`, `ScanRun` | Scan risk and finding DTOs need host/image grouping and severity presentation. | Planned M11 Vulnerability scanning |
| `BackupSystemBackupRun`, `BackupSystemBackupPolicy`, `BackupBackupFileEntry` | `BackupRun`, `BackupSettings` | Restore targets, history and file selection need one UI backup model. | Planned M12 Backups and restore |
| No backup-target test DTO | `testBackupTarget` | Target connection checks need a typed result without exposing credentials. | Planned M12 Backups and restore |
| `VolumeBackupPolicy`, `VolumeBackupEntry` | `BackupPolicy`, `BackupRun` | Volume backup schedules and histories need a common run projection. | Planned M12 Backups and restore |
| `AuthAuthenticationResponse`, `AuthLogin`, `SettingsSettingDto` | `AuthSettings`, `HubSettings`, `SettingsApi` | Sign-in modes and settings categories need typed, server-backed UI preferences. | Planned M4 Identity, setup and hub settings |
| `UserPreferences` | `HubSettings` | Personal preferences need a server-backed profile projection. | Planned M4 Identity, setup and hub settings |
| `UserUser`, `UserRoleAssignmentSummary` | `HubUser` | User rights need the agreed simple access projection. | Planned M15 Simple users with resource rights |
| `NotificationResponse`, `NotificationUpdate`, `NotificationTestResponse` | `NotificationChannel`, `SettingsApi` | Channel configuration and test results need UI channel kinds and safe status. | Planned M13 Notifications and Auto Heal |
| No bounded Auto Heal DTO | `AutoHealSettings` | Restart limits and exceptions need a distinct persisted contract. | Planned M13 Notifications and Auto Heal |
| No service-widget DTO | Dashboard widget view models | Service polling, credentials and failure states have no wire contract yet. | Planned M16 Service widgets with real data |

The table records existing mismatches, not a promise that every listed API
operation is ready for production use. Some schema types are nested Docker
structures rather than separately callable operations. The owning milestone
resolves each mapping when its endpoint and UI slice are implemented.

## Runtime lifecycle contract

[Runtime control](runtime-control.md) defines the seven lifecycle operations:
project deploy/down/restart/redeploy and container start/stop/restart. They share
one application-owned runtime with receipt-backed admission, exclusion,
cancellation and settlement. Existing operation and activity-read permissions
apply before admission, joins or conflict details. Generic activity writers
cannot complete a runtime-owned run or release its execution slot.

`Prefer: respond-async` returns HTTP 202 for a new admission and HTTP 200 for a
retained or joined admission. Both carry `activityId`, `code` and `activity`;
`activityId` equals `activity.id` and the executor's run UUID. Acceptance never
proves successful execution. A retained `Idempotency-Key` resolves to its original
run after ten minutes and without fresh Docker access. Another caller or changed
digest receives `run.request-mismatch` without the bound ID. Removal retains a
registration tombstone and returns HTTP 410 `runtime.target-removed`.

Legacy project down/restart and container start/stop/restart return HTTP 200
`MessageResponse` with the run ID only after confirmed workflow success. Admitted
unknown, interrupted, cancelled, failed, waiter timeout and writable waiter
cancellation return their coded non-2xx envelopes and the same ID. Project
up/redeploy retain NDJSON activity-started and output frames. A successful stream
ends with `done`; after flushing, non-success is an error frame with `code` and
`activityId` on the existing HTTP 200 stream. Pre-flush admission failures retain
their HTTP status. Disconnect detaches the waiter and cannot cancel execution.

Runtime snapshots require the paired `outcomeState` and `cancelRequested` fields.
Generic historical or unrelated activities may omit both.

Remote admission also projects `outcomeState: unknown` while delivery is pending
or in flight and no authoritative executor answer has arrived. This alone is not
evidence of a lost outcome. Command consumers must read the existing targeted
`RuntimeHubDetail.recovery.unknownReason`: absent means delivery remains
unconfirmed without a recorded evidence-loss reason; present identifies actual
uncertainty such as transport or executor continuity loss. Generic list/SSE
knowledge alone must not trigger an outcome-loss warning on every remote Start.
If targeted detail is unavailable, keep the outcome unconfirmed; do not infer
success, progress, evidence loss or permission to replay. These semantics add no
projection field or vocabulary and apply before command UI capabilities open.

Manual recovery
requires effective global administration plus activity read/cancel access,
reason and acknowledgment that the executor stopped and Docker state was checked.
Recovery details preserve executor evidence beside the immutable hub disposition;
release cannot fabricate successful effects or replay execution.

The trusted agent receiver registers execute/query/cancel and registration/target
metadata routes with the existing manager credential. Ordinary bearer,
administrator and bootstrap credentials cannot grant receiver authority. Direct
and Edge requests recheck agent access and compatibility; Edge has no Direct
fallback. Numeric agent contract version and minimum are both 5. The fingerprint
hashes the exact `types/contract/testdata/agent-hub-v5.json` bytes, inheriting the
v4 runtime contract and adding the [storage contract](storage-workspace.md).
The v3 and v4 fixtures are preserved. Runtime queries have no compatibility
exception. Existing health/version/self-upgrade exceptions, `tunnel/v1`, historical OpenAPI and
shipped migrations remain unchanged.

The shared foundation owns runtime wire types, current exports and route
snapshots. Command, log and session mappings retain their own consumer boundaries;
UI capabilities stay unavailable until their complete mappings pass. Runtime JSON
logs reuse Echo WS outside the Huma export. The CLI treats EOF before explicit
`done` as interruption while preserving progress and terminal errors.

## Stable message codes

`types/activity/code.go` owns the code catalogue. OpenAPI exposes these values as
`messageCode` enums for activity and event responses. Display text is separate;
callers creating events must send a listed code. A run status transition uses
`activity.<status>`; admission results use `run.*`. The catalogue is:

| Family | Codes |
| --- | --- |
| `run` | `run.started`, `run.joined`, `run.fresh`, `run.busy`, `run.cancelling`, `run.request-mismatch` |
| `runtime` | `runtime.outcome-unknown`, `runtime.interrupted`, `runtime.cancelled`, `runtime.failed`, `runtime.wait-timeout`, `runtime.waiter-disconnected`, `runtime.parent-unresolved`, `runtime.target-removed`, `runtime.target-changed`, `runtime.target-unavailable` |
| `activity` | `activity.queued`, `activity.running`, `activity.success`, `activity.failed`, `activity.cancelled`, `activity.message` |
| `deploy` | `deploy.pull` |
| `job` | `job.queued`, `job.waiting`, `job.running`, `job.retrying`, `job.succeeded`, `job.partial`, `job.skipped`, `job.failed`, `job.needs_attention`, `job.canceled` |
| `container` | `container.die`, `container.oom`, `container.rename`, `container.unhealthy`, `container.start`, `container.stop`, `container.restart`, `container.delete`, `container.create`, `container.scan`, `container.update`, `container.deploy`, `container.kill`, `container.pause`, `container.unpause`, `container.error` |
| `image` | `image.untag`, `image.import`, `image.prune`, `image.pull`, `image.load`, `image.tag`, `image.commit`, `image.delete`, `image.scan`, `image.error`, `image.vulnerability_scan` |
| `volume` | `volume.prune`, `volume.create`, `volume.rename`, `volume.delete`, `volume.error`, `volume.file.create`, `volume.file.delete`, `volume.file.upload`, `volume.file.update`, `volume.workspace.update`, `volume.backup.create`, `volume.backup.delete`, `volume.backup.restore`, `volume.backup.restore_files`, `volume.backup.download` |
| `network` | `network.prune`, `network.create`, `network.delete`, `network.connect`, `network.disconnect`, `network.error` |
| `project` | `project.deploy`, `project.delete`, `project.start`, `project.stop`, `project.create`, `project.update`, `project.error` |
| `system` | `system.prune`, `system.auto_update`, `system.upgrade` |
| `user` | `user.login`, `user.logout` |
| `environment` | `environment.create`, `environment.connect`, `environment.disconnect`, `environment.update`, `environment.delete`, `environment.api_key.regenerated`, `environment.mtls.ca_generated`, `environment.mtls.cert_issued`, `environment.mtls.enroll`, `environment.mtls.download` |
| `webhook` | `webhook.create`, `webhook.update`, `webhook.delete`, `webhook.trigger` |
| `notification` | `notification.send` |

## Run gate

The agreed single-hub model, including admission, cancellation, restart
recovery, operational error codes and adapter contract consequences, is in the
[run-gate record](run-gate.md).

Run detail returns the most recent 256 distinct step transitions. Repeated
messages within one step do not consume this limit. A closed step before the
last step is reported as `success` when the next step starts, even if that step
logged warning or error messages; the last step inherits the run's terminal
status. Clients should use message levels for per-step warnings and errors.

Event history is capped at 500 entries. Completed activity history is capped
at 100 entries per environment. Environment overrides follow Arcane settings
initialization and do not fail startup on invalid values. Generic cancellation
resolves runtime receipt ownership before falling back to a legacy worker signal.
Receipt-backed cancellation and outcome knowledge do not introduce new terminal
activity statuses.
