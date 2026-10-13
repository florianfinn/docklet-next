// Hub settings. They are hub-wide and independent of the sidebar host filter.

import type { HostConnection } from "./model";
import type { Retention } from "./retention";
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

export interface BackupTarget {
  id: string;
  name: string;
  kind: BackupTargetKind;
  /** Host for local directories and existing mounts. */
  hostId?: string;
  path: string;
  server?: string;
  share?: string;
  username?: string;
  /** A password exists on the hub; the value itself is never sent back. */
  secretSet?: boolean;
  options?: string;
  lastTest?: { ok: boolean; at: string; message: string };
}

export interface BackupSettings {
  targets: BackupTarget[];
  /** Starting point for new backup plans; existing plans keep their own. */
  defaultRetention: Retention;
}

export type ChannelKind = "email" | "discord" | "telegram" | "signal" | "slack" | "ntfy" | "pushover" | "gotify" | "matrix" | "googlechat" | "generic";

export type NotificationEvent = "updates" | "deployFailed" | "hostOffline" | "backupFailed" | "critical" | "unhealthy";

export interface NotificationChannel {
  id: string;
  kind: ChannelKind;
  name: string;
  /** Non-secret address shown in lists, e.g. topic, room or recipient. */
  target: string;
  secretSet: boolean;
  enabled: boolean;
  events: NotificationEvent[];
}

export interface SecuritySettings {
  scanEnabled: boolean;
  /** Recurring scans run at this wall-clock time in the hub time zone. */
  schedule: ScanSchedule;
}

export interface AutoHealSettings {
  enabled: boolean;
  maxRestarts: number;
  windowMinutes: number;
  cooldownMinutes: number;
  /** Stack ids that are never restarted automatically. */
  excludedStacks: string[];
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
  notifications: NotificationChannel[];
  security: SecuritySettings;
  autoHeal: AutoHealSettings;
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

export const CHANNEL_KINDS: ChannelKind[] = ["email", "discord", "telegram", "signal", "slack", "ntfy", "pushover", "gotify", "matrix", "googlechat", "generic"];

export const NOTIFICATION_EVENTS: NotificationEvent[] = ["updates", "deployFailed", "hostOffline", "backupFailed", "critical", "unhealthy"];
