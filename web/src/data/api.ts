// Source: approved Docklet design prototype (maintainer's own work)/react-opus/src/data/api.ts
// Revision: 357dd6dee16118e0880ca694d222699556389d977c3d15d6148e23e2acee23d7
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Adapter port cleanup, demo-only options and coded run/event localization after the historical import. See web/PROVENANCE.md. #69 connected first-run identity flow. M5 #84 part 2: live host registration, credential lifecycle and admin production onboarding. Acceptance #85: preserve live inventory state, names, host isolation and accurate resource presentation. #181 live volume management and lifecycle checks. #182 shared workspace port. #183 manifest revisions returned by volume file gestures. #183 path-aware workspace entry freshness and baseline ownership. M9 #204: validate labels, driver options, auxiliary addresses and DNS aliases. M9 #204 phase 2: capability-gated network writes and typed errors. M9 #204: HTTP network write mapping M9 #204: preserve partial topology observations. M9 #204: show partial network reads and protect unknown usage. Production image/registry UI and validation for #230. M12 #284 live volume backup domain, transport, UI and regression coverage. M13 #297 live notification targets and per-host Auto Heal settings. M14 #315 dedicated hub stack stop-mode read. M14 #318 host maintenance and availability web implementation. Simple-user management, assigned-stack access and mode-aware self-service.

// Adapter contracts for hosts, storage and hub settings. Split from adapter.ts
// so each area can move to its own HTTP client later.

import type { components } from "@/api/generated/schema";
import type { DirListing, FileContent } from "@/domain/files";
import type { DockerImage, ImagePullProgress, ImagePullResult, ImagePruneResult, PruneMode } from "@/domain/images";
import type { HubDiscovery } from "@/domain/hub-discovery";
import type { AutoUpdatePolicy, FileOwner, Host, HostConnection, NetworkAttachment, Topology, TopologyNetwork } from "@/domain/model";
import type { NetworkDraft } from "@/domain/networks";
import type { BackupTarget, BackupTargetInput, BackupTargetReferences, HubSettings, SettingsSection } from "@/domain/settings";
import type { NotificationProvider, NotificationTarget, NotificationTargetInput, NotificationTargetRow, NotificationTestResult } from "@/domain/notifications";
import type { AutoHealInput, AutoHealSettings } from "@/domain/auto-heal";
import type { SystemComponent } from "@/domain/system";
import type { PruneSkipReason, VolumePrunePlan as DomainVolumePrunePlan } from "@/domain/volume-prune";
import type { BackupBrowse, BackupOverview, BackupPolicy, BackupRepository, BackupSelection, Volume, VolumeDetail } from "@/domain/volumes";
import type { AuthMode } from "@/domain/settings";

export type AppAssignment = components["schemas"]["UserAppAssignment"];
export type ManagedUser = Pick<components["schemas"]["UserUser"], "id" | "username" | "isAdmin" | "disabledAt" | "requiresPasswordChange"> & { assignments: AppAssignment[] };
export type PersonalApiKey = Pick<components["schemas"]["ApikeyApiKey"], "id" | "name" | "keyPrefix" | "createdAt" | "expiresAt" | "lastUsedAt">;

export interface UsersApi {
  listUsers(): Promise<ManagedUser[]>;
  createUser(input: Pick<components["schemas"]["UserCreateUser"], "username" | "password">): Promise<ManagedUser>;
  updateUser(id: string, input: Pick<components["schemas"]["UserUpdateUser"], "username">): Promise<ManagedUser>;
  disableUser(id: string): Promise<void>;
  enableUser(id: string): Promise<void>;
  resetUserPassword(id: string, password: string): Promise<void>;
  deleteUser(id: string): Promise<void>;
  setUserAdministrator(id: string, isAdmin: boolean): Promise<void>;
  setUserAppAssignments(id: string, assignments: AppAssignment[]): Promise<void>;
  listMyApiKeys(): Promise<PersonalApiKey[]>;
  createMyApiKey(input: components["schemas"]["ApikeyCreateUserApiKey"]): Promise<PersonalApiKey & { key: string }>;
  deleteMyApiKey(id: string): Promise<void>;
}

export interface CurrentUser {
  isAdmin?: boolean;
  assignments?: AppAssignment[];
  id: string;
  username: string;
  locale?: string | null;
  requiresPasswordChange: boolean;
}

export interface AuthApi {
  getAuthMode(): Promise<AuthMode>;
  getAuthConfiguration(): Promise<components["schemas"]["AuthSignInConfiguration"]>;
  getCurrentUser(): Promise<CurrentUser>;
  login(username: string, password: string): Promise<CurrentUser>;
  logout(): Promise<void>;
  changePassword(currentPassword: string, newPassword: string): Promise<void>;
  updateMyLocale(locale: string): Promise<CurrentUser>;
}

/** Error codes of the hosts, storage and settings contracts; UIs translate them. */
export type AdapterErrorCode =
  | "overlap"
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
  | "detail-unavailable"
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
  /** Lowercase hex; null resets the stable palette default. */
  color: string | null;
}

export interface HostCheckResult { status: "online" | "standby-opened" }

export interface HostsApi {
  /** Renames or recolours a host; every view reads hosts from one query, so names and colours follow at once. */
  updateHost(hostId: string, input: UpdateHostInput): Promise<Host>;
  /** Restarts the host's own agent container and returns the action id. */
  restartAgent(hostId: string): Promise<string>;
  /** Registers a host that waits for its agent (status "pending"). */
  registerHost(input: RegisterHostInput): Promise<{ host: Host; token: string }>;
  /** Connects the agent and returns the connection run's action id. */
  connectHost(hostId: string): Promise<string | HostCheckResult>;
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

export type { PruneSkipReason };

export interface VolumePrunePlan extends DomainVolumePrunePlan {
  all?: boolean;
  candidateReasons?: Record<string, "unused" | "unused-anonymous">;
  failedHosts?: { hostId: string; code: string }[];
}

export type VolumePruneCandidate = Pick<Volume, "id" | "hostId" | "name" | "createdAt">;

export interface VolumePruneResult {
  removed: { id: string; name: string; hostId: string; sizeBytes: number }[];
  /** Confirmed candidates that changed since the preview and therefore stayed. */
  skipped: { id: string; reason: PruneSkipReason | "changed" }[];
  failed?: { id: string; name: string; hostId: string; code: string }[];
  /** Advisory lower bound; zero does not establish that no space was freed. */
  spaceReclaimed?: number;
}

export type WorkspaceTree = components["schemas"]["WorkspaceWorkspace"];
export type WorkspaceContent = components["schemas"]["WorkspaceFileContent"];
export type WorkspaceReadOnlyReason = WorkspaceContent["readOnlyReason"];
export interface WorkspaceUpload { name: string; size: number; content?: string; blob?: Blob }
export type WorkspaceGesture =
  | { kind: "create-file"; path: string; content: string }
  | { kind: "create-folder"; path: string }
  | { kind: "update"; path: string; content: string; baseline: string }
  | { kind: "upload"; path: string; file: WorkspaceUpload }
  | { kind: "rename"; path: string; name: string }
  | { kind: "move"; path: string; parent: string }
  | { kind: "delete"; path: string };
export type WorkspaceWriteResult = void | { revision: string; entryIdentity?: string };
export interface WorkspaceBaseline { revision: string; content: string }
export type VolumeFileContent = FileContent & { revision?: string; entryIdentity?: string; readOnlyReason?: WorkspaceReadOnlyReason };
export type VolumeDirListing = DirListing & { revision?: string; truncated?: boolean; workspaceTree?: WorkspaceTree };

/** Optional while adapters without workspace support remain disconnected. */
export interface ProjectWorkspaceApi {
  getProjectWorkspace?(stackId: string): Promise<WorkspaceTree>;
  readProjectFile?(stackId: string, path: string): Promise<WorkspaceContent>;
  downloadProjectFile?(stackId: string, path: string): Promise<Blob>;
  updateProjectWorkspace?(stackId: string, revision: string, changes: WorkspaceGesture[]): Promise<WorkspaceTree>;
}

export interface VolumesApi extends ProjectWorkspaceApi {
  listVolumes(onPartial?: (rows: Volume[]) => void): Promise<Volume[]>;
  getVolume(id: string): Promise<VolumeDetail & { scope?: string }>;
  createVolume(input: CreateVolumeInput): Promise<Volume>;
  deleteVolume(id: string): Promise<void>;
  renameVolume(id: string, name: string): Promise<Volume>;
  /** Preview of a prune for the given hosts; computed by the adapter, not the browser. */
  planVolumePrune(input: { hostIds: string[]; all?: boolean }): Promise<VolumePrunePlan>;
  /**
   * Removes the confirmed volumes. Each one is checked again at execution
   * time: consumers that appeared since the preview, bind mounts and
   * disconnected hosts keep their volume.
   */
  pruneVolumes(input: { hostIds: string[]; volumeIds: string[]; candidates?: VolumePruneCandidate[]; all?: boolean }): Promise<VolumePruneResult>;
  listFiles(volumeId: string, path: string): Promise<VolumeDirListing>;
  readFile(volumeId: string, path: string): Promise<VolumeFileContent>;
  writeFile(volumeId: string, path: string, content: string, baseline?: WorkspaceBaseline): Promise<WorkspaceWriteResult>;
  createEntry(volumeId: string, dir: string, name: string, type: "dir" | "file", revision?: string): Promise<WorkspaceWriteResult>;
  renameEntry(volumeId: string, path: string, name: string, revision?: string): Promise<WorkspaceWriteResult>;
  deleteEntry(volumeId: string, path: string, revision?: string): Promise<WorkspaceWriteResult>;
  uploadFile(volumeId: string, dir: string, file: WorkspaceUpload, revision?: string): Promise<WorkspaceWriteResult>;
  downloadFile?(volumeId: string, path: string): Promise<Blob>;
  setBackupPolicy(volumeId: string, policy: BackupPolicy | null): Promise<void>;
  runBackup(volumeId: string, options?: { stopContainers?: boolean; targetId?: string }): Promise<string>;
  restoreBackup(volumeId: string, runId: string, mode: "replace" | "new", newVolumeName?: string): Promise<string>;
  restoreBackupFiles(volumeId: string, runId: string, selection: BackupSelection): Promise<string>;
  browseBackup(volumeId: string, runId: string, path?: string, start?: number): Promise<BackupBrowse>;
  downloadBackup(volumeId: string, runId: string): Promise<Blob>;
  deleteBackup(volumeId: string, runId: string): Promise<void>;
  uploadRestore(volumeId: string, file: File, mode?: "replace" | "new", newVolumeName?: string): Promise<string>;
  getBackupOverview(hostId: string): Promise<BackupOverview>;
  listBackupOverviews(onPartial?: (rows: BackupOverview[]) => void): Promise<BackupOverview[]>;
  getBackupRepository(hostId: string): Promise<BackupRepository>;
  checkBackupRepository(hostId: string): Promise<string>;
  listBackupTargets(): Promise<BackupTarget[]>;
  createBackupTarget(input: BackupTargetInput): Promise<BackupTarget>;
  updateBackupTarget(id: string, input: BackupTargetInput): Promise<BackupTarget>;
  deleteBackupTarget(id: string): Promise<void>;
  testBackupTarget(id: string, hostId: string): Promise<string>;
  getBackupTargetReferences(id: string, hostId: string): Promise<BackupTargetReferences>;
  getBackupRecoveryKey(id: string, hostId?: string): Promise<string>;
  attachLocalBackupRecoveryKey(hostId: string, recoveryKey: string): Promise<void>;
}

export type CreateNetworkInput = NetworkDraft;

export interface ConnectNetworkInput {
  /** DNS labels; whitespace trimmed, case normalized and duplicates removed. */
  aliases?: string[];
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
  /** The caller supplies its existing query snapshot for host-local read fallback. */
  getTopology(onPartial?: (rows: Topology) => void, retained?: Topology): Promise<Topology>;
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
  onProgress?: (progress: ImagePullProgress) => void;
  hostId: string;
  /** "repository[:tag]"; the tag defaults to latest. */
  reference: string;
}

export interface ImagesApi {
  /** Images of all connected and disconnected hosts with consumers derived from the containers. */
  listImages(onPartial?: (rows: DockerImage[]) => void): Promise<DockerImage[]>;
  /** Resolves after stream completion; the activity frame is optional. */
  pullImage(input: PullImageInput): Promise<ImagePullResult>;
  /** Refused with "in-use" while any container (running or stopped) uses the image. */
  removeImage(id: string, force?: boolean): Promise<void>;
  /** Returns actual per-host results, preserving partial success. */
  pruneImages(input: { hostIds: string[]; mode: PruneMode }): Promise<ImagePruneResult>;
}

export interface SettingsApi {
  getSetupStatus(): Promise<boolean>;
  getConnectedSettings(): Promise<Pick<HubSettings, "general" | "activity" | "hostDefaults">>;
  getConnectedSettings(section: "hostDefaults"): Promise<Pick<HubSettings, "hostDefaults">>;
  getConnectedSettings(section: "installation"): Promise<Pick<HubSettings, "general" | "hostDefaults">>;
  /** Reads only the hub stop mode, the value a stack Stop dispatches by. */
  getStackStopMode?(): Promise<"down" | "stop">;
  getSettings(): Promise<HubSettings>;
  saveSettings<K extends SettingsSection>(section: K, value: HubSettings[K]): Promise<void>;
  /** First-run setup; secrets are not part of the payload. */
  completeSetup(input: Pick<HubSettings, "general" | "auth" | "hostDefaults">): Promise<void>;
  setContainerAutoUpdate(containerId: string, policy: AutoUpdatePolicy): Promise<void>;
}

/** Hub-global notification targets, one per provider; global administrators only. */
export interface NotificationsApi {
  listNotificationTargets(): Promise<NotificationTargetRow[]>;
  saveNotificationTarget(input: NotificationTargetInput): Promise<NotificationTarget>;
  deleteNotificationTarget(provider: NotificationProvider): Promise<void>;
  /** Sends a test through the saved target. */
  testNotificationTarget(provider: NotificationProvider): Promise<NotificationTestResult>;
}

/** Auto Heal settings belong to the host that restarts its containers. */
export interface AutoHealApi {
  getAutoHealSettings(hostId: string): Promise<AutoHealSettings>;
  saveAutoHealSettings(hostId: string, input: AutoHealInput): Promise<AutoHealSettings>;
}
