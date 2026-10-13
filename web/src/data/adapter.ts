import type {
  ActionRun,
  AutoUpdatePolicy,
  ContainerInfo,
  ContainerOrigin,
  Host,
  LogLine,
  MetricsSnapshot,
  OpsEvent,
  PortMapping,
  ScheduledJob,
  SecurityOverview,
  Stack,
  StackDetail,
} from "@/domain/model";

import type { HostsApi, HubApi, ImagesApi, NetworksApi, SettingsApi, VolumesApi } from "./api";

export type StackCommand = "start" | "stop" | "restart" | "redeploy";
export type ContainerCommand = "start" | "stop" | "restart";

/** Optional failure injection for demonstrating error paths. */
export type FailAt = "none" | "pull" | "start" | "health";

export interface DeployInput {
  stackId: string | null;
  hostId: string;
  name: string;
  compose: string;
  env: string;
  labels: string[];
  failAt?: FailAt;
}

export interface RecreateInput {
  image: string;
  ports: PortMapping[];
  env: Record<string, string>;
  restartPolicy: string;
}

export interface ImportRequest {
  kind: "github" | "dockerhub";
  ref: string;
}

/** One-time import result; values the source does not provide stay empty. */
export interface ImportResult {
  title: string;
  found: string[];
  missing: string[];
  services: { key: string; body: Record<string, unknown> }[];
  simulated: boolean;
}

/** Query topics used to invalidate cached reads after server-side changes. */
export type Topic = "hosts" | "stacks" | "containers" | "security" | "topology" | "events" | "schedule" | "volumes" | "settings" | "images";

export type Unsubscribe = () => void;

/**
 * Transport-neutral boundary of the UI. The demo adapter simulates every call
 * in memory; a future HTTP/SSE adapter implements the same interface.
 */
export interface DockletAdapter extends HostsApi, VolumesApi, SettingsApi, ImagesApi, NetworksApi, HubApi {
  readonly kind: "demo" | "http";

  listHosts(): Promise<Host[]>;
  listStacks(): Promise<Stack[]>;
  getStack(id: string): Promise<StackDetail>;
  listContainers(): Promise<ContainerInfo[]>;
  getSecurity(): Promise<SecurityOverview>;
  listEvents(): Promise<OpsEvent[]>;
  listSchedule(): Promise<ScheduledJob[]>;
  listActions(): Promise<ActionRun[]>;

  runStackCommand(stackId: string, command: StackCommand, failAt?: FailAt): Promise<string>;
  installStackUpdates(stackId: string, services?: string[], failAt?: FailAt): Promise<string>;
  checkUpdates(scope: { stackId?: string; containerId?: string }): Promise<string>;
  deployStack(input: DeployInput): Promise<{ actionId: string; stackId: string }>;
  saveStackFiles(stackId: string, compose: string, env: string): Promise<void>;
  restoreRestorePoint(stackId: string, restorePointId: string): Promise<string>;
  setStackLabels(stackId: string, labels: string[]): Promise<void>;
  setAutoUpdate(stackId: string, policy: AutoUpdatePolicy): Promise<void>;
  restartService(stackId: string, service: string): Promise<string>;
  importSource(request: ImportRequest): Promise<ImportResult>;

  runContainerCommand(containerId: string, command: ContainerCommand): Promise<string>;
  recreateContainer(containerId: string, input: RecreateInput, failAt?: FailAt): Promise<string>;
  setContainerOrigin(containerId: string, origin: ContainerOrigin): Promise<void>;
  installContainerUpdate(containerId: string): Promise<string>;

  reconnectHost(hostId: string): Promise<string>;
  runScan(): Promise<string>;
  setScanEnabled(enabled: boolean): Promise<void>;

  subscribeActions(listener: (runs: ActionRun[]) => void): Unsubscribe;
  subscribeMetrics(listener: (snapshot: MetricsSnapshot) => void): Unsubscribe;
  subscribeLogs(stackId: string, listener: (lines: LogLine[]) => void): Unsubscribe;
  /** One container's log: its service in the stack stream, or its own stream outside Compose. */
  subscribeContainerLogs(containerId: string, listener: (lines: LogLine[]) => void): Unsubscribe;
  subscribeChanges(listener: (topics: Topic[]) => void): Unsubscribe;
}
