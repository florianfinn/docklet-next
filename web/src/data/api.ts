// Adapter contracts for hosts, storage and hub settings. Split from adapter.ts
// so each area can move to its own HTTP client later.

import type { DirListing, FileContent } from "@/domain/files";
import type { DockerImage, PruneMode } from "@/domain/images";
import type { HubDiscovery } from "@/domain/hub-discovery";
import type { AutoUpdatePolicy, FileOwner, Host, HostConnection, NetworkAttachment, Topology, TopologyNetwork } from "@/domain/model";
import type { NetworkDraft } from "@/domain/networks";
import type { BackupTarget, HubSettings, NotificationChannel, SettingsSection } from "@/domain/settings";
import type { SystemComponent } from "@/domain/system";
import type { PruneSkipReason, VolumePrunePlan } from "@/domain/volume-prune";
import type { BackupPolicy, Volume, VolumeDetail } from "@/domain/volumes";
import type { AuthMode } from "@/domain/settings";

export interface CurrentUser {
  id: string;
  username: string;
  locale?: string | null;
  requiresPasswordChange: boolean;
}

export interface AuthApi {
  getAuthMode(): Promise<AuthMode>;
  getCurrentUser(): Promise<CurrentUser>;
  login(username: string, password: string): Promise<CurrentUser>;
  logout(): Promise<void>;
  changePassword(currentPassword: string, newPassword: string): Promise<void>;
  updateMyLocale(locale: string): Promise<CurrentUser>;
}

/** Error codes of the hosts, storage and settings contracts; UIs translate them. */
export type AdapterErrorCode =
  | "not-found"
  | "exists"
  | "invalid-name"
  | "invalid-color"
  | "in-use"
  | "host-offline"
  | "host-in-use"
  | "bind-not-deletable"
  | "not-editable"
  | "no-policy"
  | "target-missing"
  | "invalid-plan"
  | "invalid-history"
  | "target-in-use"
  | "last-admin"
  | "invalid-network"
  | "builtin-network"
  | "compose-managed"
  | "host-mismatch"
  | "network-mode"
  | "already-connected"
  | "not-connected"
  | "invalid-ip";

export class AdapterError extends Error {
  readonly code: AdapterErrorCode;
  constructor(code: AdapterErrorCode) {
    super(code);
    this.code = code;
    this.name = "AdapterError";
  }
}

export function errorCode(e: unknown): AdapterErrorCode | undefined {
  return e instanceof AdapterError ? e.code : undefined;
}

export interface RegisterHostInput {
  name: string;
  color: string;
  connection: HostConnection;
  /** Direct mode: "address:port" the hub connects to. */
  address: string;
  projectDir: string;
  installDir: string;
  fileOwner?: FileOwner;
  socketGid?: number;
}

export interface UpdateHostInput {
  name: string;
  /** "#rrggbb"; used for host bars, chips, blocks and topology zones. */
  color: string;
}

export interface HostsApi {
  /** Renames or recolours a host; every view reads hosts from one query, so names and colours follow at once. */
  updateHost(hostId: string, input: UpdateHostInput): Promise<Host>;
  /** Restarts the host's own agent container and returns the action id. */
  restartAgent(hostId: string): Promise<string>;
  /** Registers a host that waits for its agent (status "pending"). */
  registerHost(input: RegisterHostInput): Promise<{ host: Host; token: string }>;
  /** Connects the agent and returns the connection run's action id. */
  connectHost(hostId: string): Promise<string>;
  removeHost(hostId: string): Promise<void>;
  /** Docklet's own components per host; they never appear in stack or container lists. */
  listSystemComponents(): Promise<SystemComponent[]>;
  /** Rotates an existing host credential and returns the new token once. */
  issueAgentToken(hostId: string): Promise<string>;
}

export interface CreateVolumeInput {
  name: string;
  hostId: string;
  labels: Record<string, string>;
}

export type { PruneSkipReason, VolumePrunePlan };

export interface VolumePruneResult {
  removed: { id: string; name: string; hostId: string; sizeBytes: number }[];
  /** Confirmed candidates that changed since the preview and therefore stayed. */
  skipped: { id: string; reason: PruneSkipReason }[];
}

export interface VolumesApi {
  listVolumes(onPartial?: (rows: Volume[]) => void): Promise<Volume[]>;
  getVolume(id: string): Promise<VolumeDetail & { scope?: string }>;
  createVolume(input: CreateVolumeInput): Promise<Volume>;
  deleteVolume(id: string): Promise<void>;
  renameVolume(id: string, name: string): Promise<Volume>;
  /** Preview of a prune for the given hosts; computed by the adapter, not the browser. */
  planVolumePrune(input: { hostIds: string[] }): Promise<VolumePrunePlan>;
  /**
   * Removes the confirmed volumes. Each one is checked again at execution
   * time: consumers that appeared since the preview, bind mounts and
   * disconnected hosts keep their volume.
   */
  pruneVolumes(input: { hostIds: string[]; volumeIds: string[] }): Promise<VolumePruneResult>;
  listFiles(volumeId: string, path: string): Promise<DirListing>;
  readFile(volumeId: string, path: string): Promise<FileContent>;
  writeFile(volumeId: string, path: string, content: string): Promise<void>;
  createEntry(volumeId: string, dir: string, name: string, type: "dir" | "file"): Promise<void>;
  renameEntry(volumeId: string, path: string, name: string): Promise<void>;
  deleteEntry(volumeId: string, path: string): Promise<void>;
  /** Text content is kept; other files are stored as size only. */
  uploadFile(volumeId: string, dir: string, file: { name: string; size: number; content?: string }): Promise<void>;
  setBackupPolicy(volumeId: string, policy: BackupPolicy | null): Promise<void>;
  runBackup(volumeId: string): Promise<string>;
  restoreBackup(volumeId: string, runId: string, mode: "replace" | "new"): Promise<string>;
}

export type CreateNetworkInput = NetworkDraft;

export interface ConnectNetworkInput {
  networkId: string;
  containerId: string;
  /** Static address; empty takes the next free pool address. Needs a configured subnet. */
  ip?: string;
  /** Static IPv6 address; empty takes the next free IPv6 pool address. Needs a configured IPv6 prefix. */
  ip6?: string;
}

/**
 * Docker networks of all hosts. Changes report the "topology" and
 * "containers" topics; every call is checked again on the adapter side.
 */
export interface NetworksApi {
  getTopology(onPartial?: (rows: Topology) => void): Promise<Topology>;
  /** Field errors throw "invalid-network"; disconnected hosts "host-offline". */
  createNetwork(input: CreateNetworkInput): Promise<TopologyNetwork>;
  /** Refused for Docker's own networks, Compose-owned networks and while containers are attached. */
  removeNetwork(id: string): Promise<void>;
  connectNetwork(input: ConnectNetworkInput): Promise<NetworkAttachment>;
  disconnectNetwork(input: { networkId: string; containerId: string }): Promise<void>;
}

export interface HubApi {
  /** Server-side inspection of the hub's own container and networks for address proposals. */
  discoverHub(): Promise<HubDiscovery>;
}

export interface PullImageInput {
  hostId: string;
  /** "repository[:tag]"; the tag defaults to latest. */
  reference: string;
}

export interface ImagesApi {
  /** Images of all connected and disconnected hosts with consumers derived from the containers. */
  listImages(onPartial?: (rows: DockerImage[]) => void): Promise<DockerImage[]>;
  /** Pulls an image and returns the action id. */
  pullImage(input: PullImageInput): Promise<string>;
  /** Refused with "in-use" while any container (running or stopped) uses the image. */
  removeImage(id: string): Promise<void>;
  /** Removes dangling or all unused images of the given connected hosts; null when there is nothing to remove. */
  pruneImages(input: { hostIds: string[]; mode: PruneMode }): Promise<string | null>;
}

export interface TestResult {
  ok: boolean;
  message: string;
}

export interface SettingsApi {
  getSetupStatus(): Promise<boolean>;
  getConnectedSettings(): Promise<Pick<HubSettings, "general" | "activity" | "hostDefaults">>;
  getSettings(): Promise<HubSettings>;
  saveSettings<K extends SettingsSection>(section: K, value: HubSettings[K]): Promise<void>;
  /** First-run setup; secrets are not part of the payload. */
  completeSetup(input: Pick<HubSettings, "general" | "auth" | "hostDefaults">): Promise<void>;
  testBackupTarget(target: BackupTarget): Promise<TestResult>;
  testNotification(channel: NotificationChannel): Promise<TestResult>;
  setContainerAutoUpdate(containerId: string, policy: AutoUpdatePolicy): Promise<void>;
}
