# API contract and UI mapping

The registered Huma hub routes are the source of `api/openapi.json`. Run
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
| `ProjectDeployOptions`, `ProjectCreateReponse`, `SystemConvertDockerRunResponse` | `DeployInput`, `ImportResult` | UI drafts and imports need lossless Compose fields and action identity. | Planned M7 Stack authoring and deployment |
| `ContainerActionResult`, `ContainerEdit`, `ContainerHealth` | `ActionRun`, `ContainerInfo` | Lifecycle results and edit constraints need run identity and guarded actions. | Planned M6 Runtime control |
| Project lifecycle operation responses | `runStackCommand`, `StackCommand` | Start, stop, restart and redeploy need guarded stack runs and stable action IDs. | Planned M6 Runtime control |
| No combined stack-log DTO | `LogLine`, `subscribeLogs`, `subscribeContainerLogs` | Service source and ordering must be supplied for stack and container streams. | Planned M6 Runtime control |
| `WorkspaceFileEntry`, `WorkspaceFileContent` | `DirListing`, `FileContent`, `VolumesApi` | Wire file entries need tree, path and editable-content projections. | Planned M8 Storage and file workspace |
| `VolumeVolume` | `Volume`, `VolumeDetail` | Storage kind and consumers need a host-aware projection. | Planned M8 Storage and file workspace |
| No volume-prune preview DTO | `VolumePrunePlan`, `VolumesApi` | Candidate reasons and execution re-check results need a server contract. | Planned M8 Storage and file workspace |
| `NetworkInspect`, `NetworkTopology` | `Topology`, `TopologyNetwork` | Docker topology needs a graph projection across hosts. | Planned M5 Hosts, transport and read inventory |
| `NetworkCreateRequest` | `NetworkDraft` | IPAM and validation fields need a writable form projection. | Planned M9 Network management |
| `NetworkConnectContainerRequest`, `NetworkDisconnectContainerRequest`, `NetworkContainerEndpoint` | `ConnectNetworkInput`, `NetworkAttachment`, `connectNetwork`, `disconnectNetwork` | Container attachments need host-aware validation and fixed IPv4/IPv6 address mapping. | Planned M9 Network management |
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

## Stable message codes

`types/activity/code.go` owns the code catalogue. OpenAPI exposes these values as
`messageCode` enums for activity and event responses. Display text is separate;
callers creating events must send a listed code. A run status transition uses
`activity.<status>`; admission results use `run.*`. The catalogue is:

| Family | Codes |
| --- | --- |
| `run` | `run.started`, `run.joined`, `run.fresh`, `run.busy`, `run.cancelling` |
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
initialization and do not fail startup on invalid values. Cancellation is a
local worker signal; the activity DTO has no cancellation-request timestamp.
