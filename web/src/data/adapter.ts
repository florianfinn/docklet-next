// Source: approved Docklet design prototype (maintainer's own work)/react-opus/src/data/adapter.ts
// Revision: 81be6ce24058459478dde21b11c6a5605fdb3e7b4d5fc409bc2aacce9a333cfd
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Adapter port cleanup, demo-only options and coded run/event localization after the historical import. See web/PROVENANCE.md. #38 HTTP adapter, live channel, capabilities, run states and fixed history. Acceptance #85: preserve live inventory state, names, host isolation and accurate resource presentation. Issue #102: optional classified activity source read-health subscription. M7 stack authoring: hub create/save/deploy, docker run conversion and production import boundary. List summaries and owned detail reads with bounded recovery (#225, #223). Production image/registry UI and validation for #230. M10 #231: production stack updates and lifecycle UI. M11 #262: live vulnerability reads, scan controls and security settings. M13 #297 live notification targets and per-host Auto Heal settings. M14 #313–#317: authoritative origins, scoped runtime operations, retained replacement identities and hub stop-mode UI. M14 #318 host maintenance and availability web implementation. Simple-user management, assigned-stack access and mode-aware self-service. M19 #348 #349 #353 #356: live import, bulk actions, Auto Heal cooldown and global search. M20 #360: web hardening step (#359 #24 #175 #177 #383 #421 #271). M21 #437: advisory detail. M21 #438: candidate checks and verified updates. M23 #391: maintenance observation port. M23 #428 #389: identity-owned scope retention, independent recovery reasons, metrics lifetime and automatic inventory refresh tags. M24 S6: observed recovery and update UI follow-ups. M25 latest agent restart read. M26 #462: bounded shell counts and raw inventory ownership.

import type { ReadTag } from "./http/host-read-policy";
import type { components } from "@/api/generated/schema";
import type {
  AutoUpdatePolicy,
  ContainerInfo,
  ContainerOrigin,
  Host,
  LogLine,
  MetricsSnapshot,
  RuntimeKnowledge,
  ScheduledJob,
  SecurityOverview,
  ActiveSecurity,
  Stack,
} from "@/domain/model";

import type { VulnerabilityCandidatesApi } from "./vulnerability-candidates";
import type { VulnerabilityAdvisoryApi } from "./vulnerability-advisory";
import type { OpenShell } from "./shell";

import type { UpdatesApi, LifecycleInput } from "@/domain/stack-updates";
import type { AuthApi, UsersApi, AutoHealApi, HostsApi, HubApi, ImagesApi, NetworksApi, NotificationsApi, SettingsApi, VolumesApi } from "./api";
import type { CodedActionRun, CodedEvent, CodedStackDetail } from "./message-code";
import type { CapabilityDescriptor, ResourceRef } from "./capability";
import type { RegistriesApi } from "@/domain/registries";
import type { HubBackupApi } from "@/domain/hub-backup";
import type { HistoryApi } from "@/domain/history";
import type { UiPreferencesApi } from "@/domain/ui-preferences";
import type { ServiceWidgetsApi } from "@/domain/service-widgets";
import type { MaintenanceState } from "./http/maintenance-observer";

export type StackCommand = "start" | "stop" | "restart" | "redeploy" | "down" | "stop-containers";
export type ContainerCommand = "start" | "stop" | "restart";

export interface DeployInput {
  stackId: string | null;
  hostId: string;
  name: string;
  compose: string;
  env: string;
  labels: string[];
}

/** Hub conversion of one docker run command into a Compose document. */
export interface DockerRunConversion {
  compose: string;
  env: string;
  serviceName: string;
}

/** The stack exists on its host; its first deploy was refused or its admission is unknown. */
export class StackCreatedError extends Error {
  readonly stackId: string;
  /** Unknown means the hub may have started the deploy; it must not be presented as not started. */
  readonly admission: "refused" | "unknown";
  override readonly cause: unknown;
  constructor(stackId: string, cause: unknown, admission: "refused" | "unknown") {
    super(admission === "unknown" ? "stack.created-deploy-unknown" : "stack.created-not-deployed");
    this.name = "StackCreatedError";
    this.stackId = stackId;
    this.admission = admission;
    this.cause = cause;
  }
}

export type RecreateInput = components["schemas"]["ContainerRecreateRequest"]["edit"];

export interface RuntimeAdmission { outcome: "started" | "joined"; runId: string }
export interface RuntimeCommandRequest { kind: "stack" | "container"; id: string; command: StackCommand | ContainerCommand | "update"; input?: import("@/domain/stack-updates").LifecycleInput }

export interface ImportRequest {
  kind: "github" | "dockerhub";
  ref: string;
}

/** One-time import result; values the source does not provide stay empty. */
export interface ImportResult {
  title: string;
  composeContent: string;
  envNames: string[];
  found: components["schemas"]["StackimportResult"]["found"];
  missing: components["schemas"]["StackimportResult"]["missing"];
}

/** Query topics used to invalidate cached reads after server-side changes. */
export type Topic = "hosts" | "stacks" | "containers" | "security" | "topology" | "events" | "dashboardEvents" | "schedule" | "volumes" | "settings" | "images" | "registries";

export type Unsubscribe = () => void;

export type RecentEvents = { events: CodedEvent[]; totalItems: number };
/** The Dashboard reads at most this many events of the last 24 hours. */
export const RECENT_EVENT_LIMIT = 500;

/** Activity reads are observations; a failed source cannot establish removal or completion. */
export interface ActionReadFailure {
  hostId?: string;
  reason: "unavailable" | "permission" | "authentication" | "invalid-data";
}
export type ActionDetailState = { status: "loading" } | { status: "ready"; run: CodedActionRun; refreshing?: boolean } | { status: "error"; error: unknown; run?: CodedActionRun };
export interface ActionReadHealth { failures: readonly ActionReadFailure[] }

export type CodedRuntimeActionRun = CodedActionRun & RuntimeKnowledge;

/** Administrator acknowledgment that the prior executor is stopped and state inspected. */
export interface RuntimeReleaseInput {
  acknowledgeRisk: true;
  /** Nonempty audit reason; the server validates it when runtime control is activated. */
  reason: string;
}

/**
 * Transport-neutral boundary of the UI. Adapters implement this interface.
 */
export interface DockletAdapter extends VulnerabilityCandidatesApi, VulnerabilityAdvisoryApi, UpdatesApi, AuthApi, UsersApi, HostsApi, VolumesApi, SettingsApi, NotificationsApi, AutoHealApi, ImagesApi, NetworksApi, HubApi, HubBackupApi, RegistriesApi, HistoryApi, UiPreferencesApi, ServiceWidgetsApi {
  readonly kind: "demo" | "http" | "disconnected";
  readonly mappedOperations?: ReadonlySet<string>;

  getCapability(operationId: string, resourceRef?: ResourceRef): Promise<CapabilityDescriptor>;

  listHosts(): Promise<Host[]>;
  listNetworkSummaries(signal?: AbortSignal, tag?: ReadTag): Promise<{ id: string; hostId: string }[]>;
  subscribeStackTerminals?(listener: (target: { stackId: string; hostId: string }) => void): Unsubscribe;
  listStacks(onPartial?: (rows: Stack[]) => void, signal?: AbortSignal, tag?: ReadTag): Promise<Stack[]>;
  getStack(id: string): Promise<CodedStackDetail>;
  listNavInventory(onPartial?: (rows: import("./nav-inventory").NavInventory[]) => void, signal?: AbortSignal, tag?: ReadTag): Promise<import("./nav-inventory").NavInventory[]>;
  listContainers(onPartial?: (rows: ContainerInfo[]) => void, signal?: AbortSignal, tag?: ReadTag): Promise<ContainerInfo[]>;
  getSecurity(signal?: AbortSignal): Promise<SecurityOverview>;
  /** Active finding counts per scanned local image of every host; the Dashboard's security source. */
  getActiveSecurity(signal?: AbortSignal): Promise<ActiveSecurity>;
  listEvents(): Promise<CodedEvent[]>;
  /** The last 24 hours of events, newest first, at most 500; `totalItems` counts every event of the window. */
  listDashboardEvents(): Promise<RecentEvents>;
  listSchedule(hostIds?: readonly string[], tag?: ReadTag): Promise<ScheduledJob[]>;
  listActions(): Promise<CodedActionRun[]>;

  /** Dormant boundaries; returning a snapshot never establishes verified Docker state. */
  getRuntimeRun?(hostId: string, runId: string): Promise<CodedRuntimeActionRun>;
  /** One plain hub read of a run record (no host refresh, no follow); the run gate confirms a stale blocker with it. */
  confirmRun?(hostId: string, runId: string): Promise<CodedRuntimeActionRun>;
  cancelRuntimeRun?(hostId: string, runId: string): Promise<CodedRuntimeActionRun>;
  /** Releases a hub exclusion with unknown outcome; never replays the command. */
  releaseRuntimeRun?(hostId: string, runId: string, input: RuntimeReleaseInput): Promise<CodedRuntimeActionRun>;

  runStackCommand(stackId: string, command: StackCommand): Promise<string>;
  installStackUpdates(stackId: string, input?: LifecycleInput): Promise<string>;
  checkUpdates(scope: { stackId?: string; containerId?: string }): Promise<string>;
  deployStack(input: DeployInput): Promise<{ actionId: string; stackId: string }>;
  saveStackFiles(stackId: string, compose: string, env: string): Promise<void>;
  restoreRestorePoint(stackId: string, restorePointId: string, input?: LifecycleInput): Promise<string>;
  setStackLabels(stackId: string, labels: string[]): Promise<void>;
  setAutoUpdate(stackId: string, policy: AutoUpdatePolicy): Promise<void>;
  restartService(stackId: string, service: string): Promise<string>;
  importSource(request: ImportRequest): Promise<ImportResult>;
  getImageMetadata?(id: string): Promise<{ architecture: string; os: string; layers: number }>;
  admitRuntimeCommand?(request: RuntimeCommandRequest, options?: { quiet?: boolean }): Promise<RuntimeAdmission>;
  convertDockerRun(hostId: string, command: string): Promise<DockerRunConversion>;

  openContainerShell: OpenShell;

  runContainerCommand(containerId: string, command: ContainerCommand): Promise<string>;
  recreateContainer(containerId: string, input: RecreateInput, idempotencyKey?: string): Promise<string>;
  getContainerEditConfig?(containerId: string): Promise<components["schemas"]["ContainerEditConfig"]>;
  setContainerOrigin(containerId: string, origin: ContainerOrigin): Promise<void>;
  installContainerUpdate(containerId: string, input?: { expectedManifest?: string; autoRestore?: boolean }): Promise<string>;

  reconnectHost(hostId: string): Promise<string | import("./api").HostCheckResult>;
  runScan(input?: { hostIds?: readonly string[]; imageId?: string }): Promise<string>;
  getScanSettings?(): Promise<import("@/domain/security").ScanSettings>;
  saveScanSettings?(value: import("@/domain/security").ScanSettingsInput): Promise<void>;
  cancelScan?(hostId: string, activityId: string): Promise<void>;
  setScanEnabled(enabled: boolean): Promise<void>;

  /** The host's active or newest agent restart, read after a reload; history is never published as a new run. */
  latestAgentRestart?(hostId: string, signal?: AbortSignal): Promise<CodedActionRun | undefined>;
  subscribeActionReadHealth?(listener: (health: ActionReadHealth) => void): Unsubscribe;
  observeActionDetail?(hostId: string, runId: string, listener: (state: ActionDetailState) => void): Unsubscribe;
  subscribeActions(listener: (runs: CodedActionRun[]) => void): Unsubscribe;
  subscribeMetrics(listener: (snapshot: MetricsSnapshot) => void): Unsubscribe;
  subscribeLogs(stackId: string, listener: (lines: LogLine[]) => void): Unsubscribe;
  /** One container's log: its service in the stack stream, or its own stream outside Compose. */
  subscribeContainerLogs(containerId: string, listener: (lines: LogLine[]) => void): Unsubscribe;
  subscribeChanges(listener: (topics: Topic[]) => void): Unsubscribe;
  /** Browser-wide hub maintenance observed by the HTTP transport (held reads, shell status). */
  maintenance?: { state(): MaintenanceState; subscribe(listener: (state: MaintenanceState) => void): Unsubscribe };
}
