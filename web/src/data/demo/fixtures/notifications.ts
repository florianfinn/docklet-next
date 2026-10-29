// Source: docklet-next/issues/297
// Revision: 97a16304639854dc5aca368c879689d7a2161671
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original synthetic notification targets and Auto Heal settings.

import type { AutoHealSettings } from "@/domain/auto-heal";
import type { NotificationTarget } from "@/domain/notifications";

/** Synthetic targets in the producer shape; example domains only, secrets blank. */
export const NOTIFICATION_TARGETS: NotificationTarget[] = [
  {
    kind: "target", id: 1, provider: "email", enabled: true, credentialsSet: ["smtpPassword"],
    config: { smtpHost: "mail.example.com", smtpPort: 587, smtpUsername: "docklet", smtpPassword: "", fromAddress: "docklet@example.com", toAddresses: ["alex@example.com"], tlsMode: "starttls" },
    events: ["deployFailed", "hostOffline", "backupFailed", "autoHeal", "security"],
  },
  {
    kind: "target", id: 2, provider: "ntfy", enabled: true, credentialsSet: [],
    config: { host: "ntfy.example.com", port: 0, topic: "docklet-demo", cache: true, firebase: true, disableTls: false, disableTlsVerification: false },
    events: ["deployFailed", "hostOffline", "autoHeal", "updates"],
  },
];

export const AUTO_HEAL_DEFAULT: AutoHealSettings = { enabled: false, maxRestarts: 3, windowMinutes: 60, excludedStacks: [], excludedContainers: [], managedByEnvironment: false };
/** Per-host overrides of the default; "delta" takes its values from environment variables. */
export const AUTO_HEAL: Record<string, Partial<AutoHealSettings>> = {
  atlas: { enabled: true, excludedStacks: ["craft"] },
  delta: { enabled: true, maxRestarts: 5, windowMinutes: 45, managedByEnvironment: true },
};
