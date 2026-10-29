// Source: docklet-next/issues/297
// Revision: 97a16304639854dc5aca368c879689d7a2161671
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification target model typed by the generated v10 schemas.

import type { components } from "@/api/generated/schema";

type Schema = components["schemas"];

/** Provider-specific config shapes, one per generated OpenAPI component. */
export interface ProviderConfigs {
  email: Schema["NotificationConfigEmail"];
  discord: Schema["NotificationConfigDiscord"];
  telegram: Schema["NotificationConfigTelegram"];
  signal: Schema["NotificationConfigSignal"];
  slack: Schema["NotificationConfigSlack"];
  ntfy: Schema["NotificationConfigNtfy"];
  pushover: Schema["NotificationConfigPushover"];
  gotify: Schema["NotificationConfigGotify"];
  matrix: Schema["NotificationConfigMatrix"];
  googlechat: Schema["NotificationConfigGoogleChat"];
  generic: Schema["NotificationConfigGeneric"];
}
export type NotificationProvider = keyof ProviderConfigs;
export type WireEvent = keyof Schema["NotificationEventMap"];
export type NotificationTestResult = Schema["NotificationTestResponse"];

export const NOTIFICATION_PROVIDERS: readonly NotificationProvider[] = ["email", "discord", "telegram", "signal", "slack", "ntfy", "pushover", "gotify", "matrix", "googlechat", "generic"];

export type NotificationEvent = "deployFailed" | "hostOffline" | "backupFailed" | "autoHeal" | "updates" | "security" | "cleanup";
export const NOTIFICATION_EVENTS: readonly NotificationEvent[] = ["deployFailed", "hostOffline", "backupFailed", "autoHeal", "updates", "security", "cleanup"];
/** New targets start with the failure and recovery events. */
export const DEFAULT_EVENTS: readonly NotificationEvent[] = ["deployFailed", "hostOffline", "backupFailed", "autoHeal"];

/** One UI event writes all of its wire keys; updates covers image and container updates. */
export const EVENT_KEYS: Record<NotificationEvent, readonly WireEvent[]> = {
  deployFailed: ["deploy_failed"],
  hostOffline: ["host_offline"],
  backupFailed: ["backup_failed"],
  autoHeal: ["auto_heal"],
  updates: ["image_update", "container_update"],
  security: ["vulnerability_found"],
  cleanup: ["prune_report"],
};
const WIRE_EVENTS = Object.values(EVENT_KEYS).flat();

type ValueKind = "string" | "number" | "boolean" | "list" | "map" | "events";
/** Every config key with its JSON kind; a trailing "?" marks keys the input schema does not require. */
export const CONFIG_KEYS: { [P in NotificationProvider]: { [K in keyof ProviderConfigs[P]]-?: ValueKind | `${ValueKind}?` } } = {
  email: { smtpHost: "string", smtpPort: "number", smtpUsername: "string", smtpPassword: "string", fromAddress: "string", toAddresses: "list", tlsMode: "string", authMode: "string?", events: "events?" },
  discord: { webhookId: "string", token: "string", username: "string?", avatarUrl: "string?", events: "events?" },
  telegram: { botToken: "string", chatIds: "list", preview: "boolean", notification: "boolean", parseMode: "string?", title: "string?", events: "events?" },
  signal: { host: "string", port: "number", source: "string", recipients: "list", disableTls: "boolean", user: "string?", password: "string?", token: "string?", notifySelf: "boolean?", events: "events?" },
  slack: { token: "string", channel: "string?", botName: "string?", color: "string?", icon: "string?", threadTs: "string?", title: "string?", events: "events?" },
  ntfy: { host: "string", port: "number", topic: "string", cache: "boolean", firebase: "boolean", disableTls: "boolean", disableTlsVerification: "boolean", username: "string?", password: "string?", priority: "string?", tags: "list?", title: "string?", icon: "string?", events: "events?" },
  pushover: { token: "string", user: "string", priority: "number", devices: "list?", title: "string?", events: "events?" },
  gotify: { host: "string", token: "string", disableTls: "boolean", insecureSkipVerify: "boolean", useHeader: "boolean", port: "number?", path: "string?", priority: "number?", title: "string?", events: "events?" },
  matrix: { host: "string", rooms: "string", disableTlsVerification: "boolean", port: "number?", username: "string?", password: "string?", events: "events?" },
  googlechat: { webhookUrl: "string", events: "events?" },
  generic: { webhookUrl: "string", disableTls: "boolean", method: "string?", contentType: "string?", customHeaders: "map?", payloadTemplate: "string?", messageKey: "string?", titleKey: "string?", successBodyContains: "string?", events: "events?" },
};

export type NotificationConfig = Record<string, unknown>;

export interface NotificationTarget {
  kind: "target";
  id: number;
  provider: NotificationProvider;
  enabled: boolean;
  /** Stored config without secrets; credential fields arrive blank. */
  config: NotificationConfig;
  /** Credential fields with a stored secret. */
  credentialsSet: string[];
  events: NotificationEvent[];
}
/** A stored row this client cannot read safely: shown, never edited or tested. */
export interface UnsupportedNotificationTarget {
  kind: "unsupported";
  id: number;
  provider: string;
}
export type NotificationTargetRow = NotificationTarget | UnsupportedNotificationTarget;

export interface NotificationTargetInput {
  provider: NotificationProvider;
  enabled: boolean;
  /** Empty credential fields keep the stored secret. */
  config: NotificationConfig;
  events: readonly NotificationEvent[];
}

export function isProvider(value: unknown): value is NotificationProvider {
  return typeof value === "string" && (NOTIFICATION_PROVIDERS as readonly string[]).includes(value);
}

/** A missing wire key means enabled; a UI event is on when any of its keys is. */
export function eventsFromMap(map: Partial<Record<WireEvent, boolean>> | undefined): NotificationEvent[] {
  return NOTIFICATION_EVENTS.filter((event) => EVENT_KEYS[event].some((key) => map?.[key] !== false));
}

/** Always an explicit map of every wire key. */
export function eventMap(events: readonly NotificationEvent[]): Record<WireEvent, boolean> {
  return Object.fromEntries(NOTIFICATION_EVENTS.flatMap((event) => EVENT_KEYS[event].map((key) => [key, events.includes(event)]))) as Record<WireEvent, boolean>;
}

function kindOf(value: unknown): ValueKind | undefined {
  if (typeof value === "string") return "string";
  if (typeof value === "number" && Number.isFinite(value)) return "number";
  if (typeof value === "boolean") return "boolean";
  if (Array.isArray(value)) return value.every((x) => typeof x === "string") ? "list" : undefined;
  if (value && typeof value === "object") return "map";
  return undefined;
}

function validConfig(provider: NotificationProvider, config: Record<string, unknown>): boolean {
  const keys = CONFIG_KEYS[provider] as Record<string, string>;
  for (const [key, value] of Object.entries(config)) {
    const spec = keys[key];
    if (!spec) return false;
    const want = spec.replace("?", "");
    // Generated list fields are nullable on the wire.
    if (value === null && want === "list") continue;
    if (want === "events") {
      if (!value || typeof value !== "object" || Array.isArray(value)) return false;
      if (!Object.entries(value).every(([k, v]) => (WIRE_EVENTS as string[]).includes(k) && typeof v === "boolean")) return false;
      continue;
    }
    const got = kindOf(value);
    if (got !== want) return false;
    if (want === "map" && !Object.values(value as object).every((v) => typeof v === "string")) return false;
  }
  // The hub stores the posted keys only; absent keys are valid and the server validator stays authoritative.
  return true;
}

/** Validates one producer row; anything outside the typed contract becomes unsupported. */
export function parseNotificationTarget(raw: unknown): NotificationTargetRow | undefined {
  if (!raw || typeof raw !== "object") return undefined;
  const row = raw as Record<string, unknown>;
  if (typeof row.id !== "number" || !Number.isInteger(row.id)) return undefined;
  const id = row.id;
  const provider = row.provider;
  const unsupported: UnsupportedNotificationTarget = { kind: "unsupported", id, provider: typeof provider === "string" ? provider : "" };
  if (!isProvider(provider) || typeof row.enabled !== "boolean") return unsupported;
  const config = row.config;
  if (!config || typeof config !== "object" || Array.isArray(config) || !validConfig(provider, config as Record<string, unknown>)) return unsupported;
  const credentials = row.credentialsSet === null ? [] : row.credentialsSet;
  if (!Array.isArray(credentials) || !credentials.every((c) => typeof c === "string")) return unsupported;
  const typed = config as NotificationConfig;
  return { kind: "target", id, provider, enabled: row.enabled, config: typed, credentialsSet: credentials as string[], events: eventsFromMap(typed.events as Record<WireEvent, boolean> | undefined) };
}
