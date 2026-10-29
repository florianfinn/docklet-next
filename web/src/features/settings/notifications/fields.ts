// Source: docklet-next/issues/297
// Revision: 97a16304639854dc5aca368c879689d7a2161671
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original per-channel form descriptors over the generated v10 config schemas.

import type { NotificationConfig, NotificationProvider, ProviderConfigs } from "@/domain/notifications";

/** host: a bare host name; list: one entry per line; headers: "Name: value" lines. */
export type FieldKind = "text" | "host" | "url" | "number" | "secret" | "list" | "headers" | "switch" | "select";
export interface FieldSpec {
  key: string;
  kind: FieldKind;
  options?: readonly string[];
}

const f = <P extends NotificationProvider>(...specs: { key: keyof ProviderConfigs[P] & string; kind: FieldKind; options?: readonly string[] }[]) => specs;

/** Fields the dialog edits; keys it does not show are kept from the stored config. */
export const FIELDS: { [P in NotificationProvider]: FieldSpec[] } = {
  email: f<"email">(
    { key: "smtpHost", kind: "host" }, { key: "smtpPort", kind: "number" }, { key: "tlsMode", kind: "select", options: ["starttls", "ssl", "none"] },
    { key: "smtpUsername", kind: "text" }, { key: "smtpPassword", kind: "secret" }, { key: "fromAddress", kind: "text" }, { key: "toAddresses", kind: "list" },
  ),
  discord: f<"discord">({ key: "webhookId", kind: "text" }, { key: "token", kind: "secret" }, { key: "username", kind: "text" }),
  telegram: f<"telegram">({ key: "botToken", kind: "secret" }, { key: "chatIds", kind: "list" }, { key: "notification", kind: "switch" }, { key: "preview", kind: "switch" }),
  signal: f<"signal">(
    { key: "host", kind: "host" }, { key: "port", kind: "number" }, { key: "disableTls", kind: "switch" }, { key: "source", kind: "text" },
    { key: "recipients", kind: "list" }, { key: "token", kind: "secret" }, { key: "user", kind: "text" }, { key: "password", kind: "secret" },
  ),
  slack: f<"slack">({ key: "token", kind: "secret" }, { key: "channel", kind: "text" }),
  ntfy: f<"ntfy">(
    { key: "host", kind: "host" }, { key: "port", kind: "number" }, { key: "topic", kind: "text" }, { key: "username", kind: "text" },
    { key: "password", kind: "secret" }, { key: "disableTls", kind: "switch" },
  ),
  pushover: f<"pushover">({ key: "token", kind: "secret" }, { key: "user", kind: "text" }, { key: "priority", kind: "select", options: ["-2", "-1", "0", "1", "2"] }),
  gotify: f<"gotify">({ key: "host", kind: "host" }, { key: "port", kind: "number" }, { key: "token", kind: "secret" }, { key: "disableTls", kind: "switch" }),
  matrix: f<"matrix">({ key: "host", kind: "host" }, { key: "port", kind: "number" }, { key: "rooms", kind: "text" }, { key: "username", kind: "text" }, { key: "password", kind: "secret" }),
  googlechat: f<"googlechat">({ key: "webhookUrl", kind: "secret" }),
  generic: f<"generic">(
    { key: "webhookUrl", kind: "url" }, { key: "method", kind: "select", options: ["POST", "PUT", "PATCH", "GET"] }, { key: "customHeaders", kind: "headers" },
    { key: "disableTls", kind: "switch" },
  ),
};

/** Values for keys the schema requires, so a new target always sends a complete config. */
export const DEFAULTS: { [P in NotificationProvider]: NotificationConfig } = {
  email: { smtpHost: "", smtpPort: 587, smtpUsername: "", smtpPassword: "", fromAddress: "", toAddresses: [], tlsMode: "starttls" },
  discord: { webhookId: "", token: "" },
  telegram: { botToken: "", chatIds: [], preview: false, notification: true },
  signal: { host: "", port: 8080, source: "", recipients: [], disableTls: false },
  slack: { token: "" },
  ntfy: { host: "", port: 0, topic: "", cache: true, firebase: true, disableTls: false, disableTlsVerification: false },
  pushover: { token: "", user: "", priority: 0 },
  gotify: { host: "", token: "", disableTls: false, insecureSkipVerify: false, useHeader: false },
  matrix: { host: "", rooms: "", disableTlsVerification: false },
  googlechat: { webhookUrl: "" },
  generic: { webhookUrl: "", disableTls: false, method: "POST" },
};

/** The address whose change requires re-entering a stored secret (producer rule). */
export const TARGET_FIELD: Partial<Record<NotificationProvider, string>> = { email: "smtpHost", signal: "host", ntfy: "host", gotify: "host", matrix: "host" };

const list = (value: unknown) => (Array.isArray(value) ? value.filter((v): v is string => typeof v === "string") : []);
const text = (value: unknown) => (typeof value === "string" ? value : "");

/** A non-secret one-line summary for the target list. */
export function summary(provider: NotificationProvider, c: NotificationConfig): string {
  switch (provider) {
    case "email": return `${list(c.toAddresses).join(", ")} · ${text(c.smtpHost)}`;
    case "telegram": return list(c.chatIds).join(", ");
    case "signal": return `${list(c.recipients).join(", ")} · ${text(c.host)}`;
    case "slack": return text(c.channel);
    case "ntfy": return `${text(c.host) || "ntfy.sh"}/${text(c.topic)}`;
    case "gotify": return text(c.host);
    case "matrix": return `${text(c.rooms)} · ${text(c.host)}`;
    case "discord": return text(c.username);
    case "generic": {
      try { return new URL(text(c.webhookUrl)).host; } catch { return ""; }
    }
    default: return "";
  }
}

/** Editor text for list and header fields, and the config values parsed back from it. */
export function toText(kind: FieldKind, value: unknown): string {
  if (kind === "list") return list(value).join("\n");
  if (kind === "headers") return value && typeof value === "object" ? Object.entries(value).map(([k, v]) => `${k}: ${String(v)}`).join("\n") : "";
  if (kind === "number" || kind === "select") return value === undefined ? "" : String(value);
  return text(value);
}
export function fromText(kind: FieldKind, value: string, provider: NotificationProvider, key: string): unknown {
  if (kind === "list") return value.split(/[\n,]/).map((v) => v.trim()).filter(Boolean);
  if (kind === "headers") return Object.fromEntries(value.split("\n").map((line) => line.trim()).filter(Boolean).map((line) => { const i = line.indexOf(":"); return i < 0 ? [line, ""] : [line.slice(0, i).trim(), line.slice(i + 1).trim()]; }));
  if (kind === "number" || (kind === "select" && provider === "pushover" && key === "priority")) return value.trim() === "" ? 0 : Number(value);
  return kind === "host" ? value.trim() : value;
}
