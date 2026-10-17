/*
 * Typed domain of the prototype. UI code depends only on these types and the
 * adapter interface in data/adapter.ts, never on demo fixtures.
 */

import type { ScanSchedule } from "./scan-schedule";
import type { Schedule } from "./schedule";

export type Tone =
  | "ok"
  | "warn"
  | "err"
  | "info"
  | "accent"
  | "orange"
  | "lime"
  | "cyan"
  | "sky"
  | "violet"
  | "pink"
  | "teal"
  | "rose"
  | "indigo"
  | "gray";

export type HostConnection = "direct" | "edge";
/** "pending": registered during onboarding, agent has not connected yet. */
export type HostStatus = "online" | "offline" | "pending" | "update-required";

/** File owner IDs used by the agent for project files (PUID/PGID). */
export interface FileOwner {
  uid: number;
  gid: number;
}

/**
 * What the installer checked locally on the host and the agent reported after
 * connecting. The hub cannot check anything before an agent runs there.
 */
export interface AgentReport {
  reportedAt: string;
  projectDirOwner: FileOwner & { name: string };
  projectDirWritable: boolean;
  /** IDs the agent writes project files with and where they came from. */
  fileOwner?: FileOwner;
  fileOwnerSource?: "manual" | "directory-owner";
  socketGid?: number;
  socketGidSource?: "manual" | "detected";
  note: "ok" | "root-owned" | "unraid";
  simulated?: boolean;
}

export interface InventoryObservation {
  observedAt?: string;
  stale: boolean;
  reason?: "agent.unavailable" | "agent.invalid-token" | "agent.update-required" | "inventory.read-error" | "inventory.mapping-error";
}

export interface Host {
  inventory?: InventoryObservation;
  /** Inventory and system metadata have not been read; numeric fields are empty sentinels. */
  detailsUnavailable?: boolean;
  id: string;
  name: string;
  /** Any CSS colour; used for host bars, chips and topology zones. */
  color: string;
  status: HostStatus;
  connection: HostConnection;
  os: string;
  dockerVersion: string;
  agentVersion: string;
  address: string;
  projectDir: string;
  /** Directory holding the agent's own compose.yaml and .env; never a stack directory. */
  installDir: string;
  /** Manual PUID/PGID; undefined means the installer proposal from the directory check. */
  fileOwner?: FileOwner;
  /** Docker socket group, detected by the installer; separate from the file owner. */
  socketGid?: number;
  /** Local installer check, reported by the agent once connected; absent while pending. */
  agentReport?: AgentReport;
  /** Added through the onboarding flow in this demo session. */
  demoAdded?: boolean;
  cpuCores: number;
  memTotalGb: number;
  diskTotalGb: number;
  lastSeen: string;
}

export type ContainerState = "running" | "stopped" | "restarting" | "unknown";
export type Health = "healthy" | "unhealthy" | "starting" | "none";

/** A human-readable image reference; digests never reach normal UI. */
export interface ImageVersion {
  repository: string;
  tag: string;
  /** Release version mapped from tag or OCI label; absent when unknown. */
  version?: string;
  /** Build or publication date of this image state (ISO). */
  published?: string;
}

export type UpdateLevel = "major" | "minor" | "patch";

export type UpdateInfo =
  | { state: "current"; current: ImageVersion; checkedAt: string; registry: string }
  | {
      state: "version";
      current: ImageVersion;
      latest: ImageVersion;
      level: UpdateLevel;
      checkedAt: string;
      registry: string;
    }
  | {
      /** Same tag, newer image state; version shown only if mapped. */
      state: "rebuild";
      current: ImageVersion;
      latest: ImageVersion;
      checkedAt: string;
      registry: string;
    }
  | { state: "local"; current: ImageVersion }
  | { state: "error"; current: ImageVersion; message: string; checkedAt: string }
  | { state: "unchecked"; current: ImageVersion };

export interface PortMapping {
  host?: string;
  container: string;
  protocol?: "tcp" | "udp";
}

export interface Service {
  name: string;
  image: ImageVersion;
  state: ContainerState;
  health: Health;
  ports: PortMapping[];
  networks: string[];
  restartPolicy: string;
  memLimitMb: number;
  startedAt?: string;
  restarts24h: number;
  update: UpdateInfo;
}

export type AutoUpdateMode = "check" | "scheduled";

/** Only check, or install updates on a typed schedule in the hub time zone. */
export type AutoUpdatePolicy = { mode: "check" } | { mode: "scheduled"; schedule: Schedule };

export interface Stack {
  inventory?: InventoryObservation;
  id: string;
  name: string;
  hostId: string;
  description?: string;
  labels: string[];
  url?: string;
  services: Service[];
  autoUpdate: AutoUpdatePolicy;
  lastDeployAt?: string;
  /** Running action id when the stack is busy. */
  busyActionId?: string;
  /** Docker labels of the compose project; the system label marks Docklet's own agent. */
  dockerLabels?: Record<string, string>;
}

export type DeployStepKey = "validate" | "pull" | "build" | "create" | "stop" | "start" | "health" | "scan" | "collect" | "save" | "wait" | "connect" | "other";
export type StepState = "pending" | "running" | "done" | "error" | "skipped";

export interface ActionStep {
  key: DeployStepKey;
  label: string;
  detail?: string;
  state: StepState;
  startedAt?: string;
  finishedAt?: string;
}

export type ActionKind =
  | "activity"
  | "deploy"
  | "redeploy"
  | "update"
  | "restore"
  | "start"
  | "stop"
  | "restart"
  | "check-updates"
  | "scan"
  | "recreate"
  | "reconnect"
  | "onboard"
  | "backup"
  | "volume-restore"
  | "pull"
  | "prune"
  | "agent-restart";

export type ActionStatus = "queued" | "running" | "succeeded" | "failed" | "cancelled";

/** "cmd" echoes the command, "note" marks the output as simulated; Docker writes progress to stderr. */
export type OutputStream = "cmd" | "stdout" | "stderr" | "note";

/** One line of an action's raw Docker or Compose output, separate from service runtime logs. */
export interface RunOutputLine {
  at: string;
  stream: OutputStream;
  text: string;
  step?: DeployStepKey;
  /** Diagnosis lines that explain a failure or a warning. */
  level?: "error" | "warn";
}

export interface ActionRun {
  id: string;
  kind: ActionKind;
  title: string;
  target: string;
  hostId?: string;
  stackId?: string;
  containerId?: string;
  services: string[];
  steps: ActionStep[];
  status: ActionStatus;
  progress: number;
  startedAt: string;
  finishedAt?: string;
  error?: string;
  log: string[];
  /** Raw, chronological operation output. */
  output: RunOutputLine[];
  outputTruncated?: boolean;
  /** Restore point created before this run; offered manually after failures. */
  restorePointId?: string;
}

export interface RestorePoint {
  id: string;
  stackId: string;
  createdAt: string;
  reason: string;
  images: { service: string; image: ImageVersion }[];
  composeSnapshot: string;
}

export interface StackDetail extends Stack {
  compose: string;
  env: string;
  deployments: ActionRun[];
  restorePoints: RestorePoint[];
}

export type ContainerOrigin = "compose" | "standalone" | "unraid" | "unknown";

export interface ContainerInfo {
  imageId?: string;
  inventory?: InventoryObservation;
  restrictions?: { recreate: boolean; reason: string };
  id: string;
  name: string;
  hostId: string;
  stackId?: string;
  serviceName?: string;
  origin: ContainerOrigin;
  /** Why origin is unclear or how it was detected; shown in explanations. */
  originNote?: string;
  image: ImageVersion;
  state: ContainerState;
  health: Health;
  ports: PortMapping[];
  networks: string[];
  mounts: string[];
  env: Record<string, string>;
  restartPolicy: string;
  memLimitMb: number;
  startedAt?: string;
  update: UpdateInfo;
  /** Standalone containers only; stack services follow their stack's policy. */
  autoUpdate?: AutoUpdatePolicy;
  /** Docker labels; the system label marks Docklet's own agent. */
  dockerLabels?: Record<string, string>;
}

export type LogLevel = "info" | "warn" | "error" | "debug";

export interface LogLine {
  id: string;
  ts: string;
  service: string;
  level: LogLevel;
  message: string;
}

export type EventKind = "activity" | "deploy" | "update" | "restart" | "health" | "host" | "backup" | "scan" | "config";
export type EventLevel = "info" | "ok" | "warn" | "error";

export interface OpsEvent {
  id: string;
  at: string;
  kind: EventKind;
  level: EventLevel;
  title: string;
  hostId?: string;
  stackId?: string;
  actionId?: string;
}

export type JobKind = "backup" | "scan" | "update-check" | "auto-update";

export interface ScheduledJob {
  id: string;
  kind: JobKind;
  title: string;
  hostId?: string;
  stackId?: string;
  /** Volume of a backup job; its link opens that volume's backups. */
  volumeId?: string;
  nextRunAt: string;
  cadence: string;
}

export interface HostMetrics {
  diskUnavailable?: boolean;
  growthUnavailable?: boolean;
  inventory?: InventoryObservation;
  cpuCores?: number;
  memTotalGb?: number;
  diskTotalGb?: number;
  hostId: string;
  cpu: number;
  mem: number;
  disk: number;
  diskGrowthGbPerDay: number;
  cpuHistory: number[];
  memHistory: number[];
}

export interface ContainerMetrics {
  cpu: number;
  memMb: number;
}

export interface MetricsSnapshot {
  at: string;
  simulated: boolean;
  hosts: Record<string, HostMetrics>;
  /** Keyed by container id. */
  containers: Record<string, ContainerMetrics>;
}

export type Severity = "critical" | "high" | "medium" | "low";

export interface ScannedImage {
  ref: string;
  hostId: string;
  stackId?: string;
  usage: "running" | "stopped" | "unused";
  counts: Record<Severity, number>;
  scannedAt: string;
}

export interface Finding {
  id: string;
  severity: Severity;
  pkg: string;
  installed: string;
  fixedIn?: string;
  image: string;
  hostId: string;
  cvss: number;
  title: string;
}

export interface ScanRun {
  at: string;
  trigger: "scheduled" | "manual";
  counts: Record<Severity, number>;
}

export interface SecurityOverview {
  enabled: boolean;
  schedule: ScanSchedule;
  images: ScannedImage[];
  findings: Finding[];
  history: ScanRun[];
}

/** Built-in Docker drivers; plugins remain displayable as arbitrary strings. */
export type NetworkDriver = "bridge" | "host" | "none" | "null" | "macvlan" | "ipvlan" | "overlay";

export interface TopologyNetwork {
  inventory?: InventoryObservation;
  id: string;
  name: string;
  hostId: string;
  driver: string;
  /** CIDR; absent for host and none. */
  subnet?: string;
  gateway?: string;
  /** Slice of the subnet Docker assigns automatic addresses from (typical for LAN networks). */
  ipRange?: string;
  /** Host interface of macvlan and ipvlan networks, e.g. br0 or eth0. */
  parent?: string;
  /** Subnet set in the IPAM config; Docker accepts static container addresses only then. */
  subnetConfigured?: boolean;
  /** false: IPv6-only network (Docker EnableIPv4 off); absent means IPv4 on, as Docker reports for older engines. */
  ipv4?: boolean;
  /** Docker EnableIPv6; the IPv6 fields below are only set then. */
  ipv6?: boolean;
  /** IPv6 prefix in canonical form, e.g. 2001:db8:1::/64. */
  subnet6?: string;
  gateway6?: string;
  ipRange6?: string;
  /** IPv6 prefix set in the IPAM config; static IPv6 addresses need it. */
  subnet6Configured?: boolean;
  internal: boolean;
  /** Docker's own bridge, host and none networks; never created, removed or reconfigured. */
  builtIn?: boolean;
  /** Compose project that owns the network; Compose recreates or removes it. */
  stackId?: string;
}

export interface NetworkAttachment {
  containerId: string;
  networkId: string;
  /** Absent on host and none networks, which have no own container address. */
  ip?: string;
  /** Address fixed at connect time instead of taken from the IPAM pool. */
  staticIp?: boolean;
  /** IPv6 address in networks with IPv6; absent when unknown, never derived. */
  ip6?: string;
  staticIp6?: boolean;
}

/** Configured start order between containers (Compose depends_on); not observed traffic. */
export interface TopologyDependency {
  /** Container that waits for `to`. */
  from: string;
  to: string;
}

export interface Topology {
  networks: TopologyNetwork[];
  attachments: NetworkAttachment[];
  dependencies: TopologyDependency[];
}
