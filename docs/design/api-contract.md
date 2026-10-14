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
| `EnvironmentEnvironment`, `EnvironmentAgentPairResponse` | `Host`, `AgentReport`, `HostsApi` | Hub environment identity and connection state do not carry the UI onboarding and installer fields directly. | Planned M5 Hosts, transport and read inventory |
| `SystemActorHostInfo`, `SystemActorInstance` | `SystemComponent`, `HostsApi` | Actor diagnostics need a host component projection and availability state. | Planned M5 Hosts, transport and read inventory |
| No hub-discovery DTO | `HubDiscovery`, `HubApi` | Address candidates and network reachability need a server inspection contract. | Planned M5 Hosts, transport and read inventory |
| `ProjectDetails`, `ProjectRuntimeService`, `ProjectTag` | `Stack`, `StackDetail`, `Service` | Compose project records need host IDs, service status, labels and UI display fields. | Planned M5 Hosts, transport and read inventory |
| `ContainerDetails`, `ContainerSummary`, `ContainerComposeInfo` | `ContainerInfo`, `DockletAdapter` | Docker container records need origin classification, restrictions and display status. | Planned M5 Hosts, transport and read inventory |
| `DashboardSnapshot`, `DashboardSnapshotContainers`, `DashboardSnapshotImages` | `HostMetrics`, `ContainerMetrics`, `MetricsSnapshot` | Aggregate dashboard payloads do not directly provide the per-host view and metric units. | Planned M5 Hosts, transport and read inventory |
| `ActivityActivity`, `ActivityMessage`, `EventEvent` | `ActionRun`, `OpsEvent`, `CodedActionRun`, `CodedEvent` | Persisted phases, service effects and message codes must replace presentation text. | #37 |
| `JobscheduleConfig`, `JobscheduleJobStatus`, `JobscheduleJobRunResponse` | `ScheduledJob`, `DockletAdapter` | Job state and next-run time need a common schedule projection. | #37 |
| `StreamEvent`, `ActivityStreamEvent`, `EventStreamEvent` | `subscribeActions`, `subscribeChanges` | SSE channel envelopes need parsing, reconnect and query invalidation. | #38 |
| `ProjectDeployOptions`, `ProjectCreateReponse`, `SystemConvertDockerRunResponse` | `DeployInput`, `ImportResult` | UI drafts and imports need lossless Compose fields and action identity. | Planned M7 Stack authoring and deployment |
| `ContainerActionResult`, `ContainerEdit`, `ContainerHealth` | `ActionRun`, `ContainerInfo` | Lifecycle results and edit constraints need run identity and guarded actions. | Planned M6 Runtime control |
| No combined stack-log DTO | `LogLine`, `subscribeLogs` | Service source and ordering must be supplied for the combined stream. | Planned M6 Runtime control |
| `WorkspaceFileEntry`, `WorkspaceFileContent` | `DirListing`, `FileContent`, `VolumesApi` | Wire file entries need tree, path and editable-content projections. | Planned M8 Storage and file workspace |
| `VolumeVolume` | `Volume`, `VolumeDetail` | Storage kind and consumers need a host-aware projection. | Planned M8 Storage and file workspace |
| No volume-prune preview DTO | `VolumePrunePlan`, `VolumesApi` | Candidate reasons and execution re-check results need a server contract. | Planned M8 Storage and file workspace |
| `NetworkInspect`, `NetworkTopology` | `Topology`, `TopologyNetwork` | Docker topology needs a graph projection across hosts. | Planned M5 Hosts, transport and read inventory |
| `NetworkCreateRequest` | `NetworkDraft` | IPAM and validation fields need a writable form projection. | Planned M9 Network management |
| `ImageSummary`, `ImageListResponse` | `DockerImage`, `ImagesApi` | Image records need consumers and UI usage state. | Planned M5 Hosts, transport and read inventory |
| `ContainerregistryContainerRegistry` | `ImagesApi` | Registry credentials and image source metadata need a safe management projection. | Planned M10 Images and updates |
| `ImageUpdateInfo`, `ImageupdateResponse`, `ProjectServiceUpdateInfo` | `UpdateInfo`, `AutoUpdatePolicy` | Version versus rebuild and scheduling need explicit UI states. | Planned M10 Images and updates |
| `VulnerabilityRiskOverview`, `VulnerabilityRiskFinding`, `VulnerabilityScanResult` | `SecurityOverview`, `Finding`, `ScanRun` | Scan risk and finding DTOs need host/image grouping and severity presentation. | Planned M11 Vulnerability scanning |
| `BackupSystemBackupRun`, `BackupSystemBackupPolicy`, `BackupBackupFileEntry` | `BackupRun`, `BackupSettings` | Restore targets, history and file selection need one UI backup model. | Planned M12 Backups and restore |
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
