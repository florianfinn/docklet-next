// Hub settings. They are hub-wide and independent of the sidebar host filter.

import type { HostConnection } from "./model";
import type { components } from "@/api/generated/schema";
import type { ScanSchedule } from "./scan-schedule";
import { hubAddressUrl } from "./validation";

export type AuthMode = "none" | "local" | "proxy";

export interface LocalAdminAuth {
  username: string;
  /** Only whether a password exists; secrets never leave the form. */
  passwordSet: boolean;
}

export interface ProxyAuth {
  userHeader: string;
  emailHeader: string;
  groupsHeader: string;
  /** Comma-separated CIDRs allowed to set the headers. */
  trustedProxies: string;
  logoutUrl: string;
}

export interface AuthSettings {
  mode: AuthMode;
  local: LocalAdminAuth;
  proxy: ProxyAuth;
}

/** Deliberately simple access presets; no per-feature role editor. */
export type AccessPreset = "full" | "operate";

export interface HubUser {
  id: string;
  name: string;
  email: string;
  access: AccessPreset;
  /** "all" or the stack ids the user may see. */
  scope: "all" | string[];
  admin?: boolean;
}

export interface HostDefaults {
  projectDir: string;
  installDir: string;
  connection: HostConnection;
  /** Empty means: use the installer proposal from the directory check. */
  puid: string;
  pgid: string;
  detectSocketGroup: boolean;
}

export type UpdateInterval = "6h" | "12h" | "24h";

export interface UpdateSettings {
  interval: UpdateInterval;
  notify: boolean;
}

export type BackupTargetKind = "local" | "mount" | "smb" | "nfs";

export type BackupTarget = components["schemas"]["BackupTarget"];
export type BackupTargetInput = components["schemas"]["BackupTargetInput"];
export type BackupTargetReferences = components["schemas"]["BackupTargetReferences"];

export interface BackupSettings {
  targets: BackupTarget[];
  defaultRetentionCount: number;
}

export interface SecuritySettings {
  scanEnabled: boolean;
  /** Recurring scans run at this wall-clock time in the hub time zone. */
  schedule: ScanSchedule;
}

/** How major actions (deploy, update, restore …) present their run: the action dialog opens by itself, or only a toast. */
export type ActionPresentation = "dialog" | "toast";

export interface ActivitySettings {
  /** Events kept, newest first (see HISTORY_LIMITS). */
  maxEvents: number;
  /** Finished actions kept; running actions are never removed. */
  maxFinishedActions: number;
  keepActionOutput: boolean;
  actionPresentation: ActionPresentation;
}

/** Which base URL links in notifications use. */
export type LinkBase = "auto" | "internal" | "domain";

export interface GeneralSettings {
  hubName: string;
  /**
   * IP or internal URL the agents reach the hub at (LAN, VPN). This is the
   * only address Docklet needs; no internet exposure is assumed.
   */
  hubAddress: string;
  /** Optional domain, e.g. behind an existing reverse proxy. */
  domainUrl: string;
  linkBase: LinkBase;
  timeZone: string;
}

export interface HubSettings {
  setupCompleted: boolean;
  general: GeneralSettings;
  auth: AuthSettings;
  users: HubUser[];
  hostDefaults: HostDefaults;
  updates: UpdateSettings;
  backups: BackupSettings;
  security: SecuritySettings;
  activity: ActivitySettings;
}

export type SettingsSection = Exclude<keyof HubSettings, "setupCompleted">;

/** Address edge agents dial out to: always the internal hub address. */
export function agentHubUrl(g: GeneralSettings): string {
  return hubAddressUrl(g.hubAddress);
}

/** Base URL of links in notifications: the domain when set (auto) or chosen, else the internal address. */
export function notificationBaseUrl(g: GeneralSettings): { url: string; source: "internal" | "domain" } {
  const domain = g.domainUrl.trim().replace(/\/+$/, "");
  const useDomain = !!domain && g.linkBase !== "internal";
  return useDomain ? { url: domain, source: "domain" } : { url: agentHubUrl(g), source: "internal" };
}
