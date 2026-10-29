// Source: docklet-next/issues/297
// Revision: 97a16304639854dc5aca368c879689d7a2161671
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original demo notification targets and per-host Auto Heal settings.

import type { AutoHealApi, NotificationsApi } from "../api";
import { SettingsProblemError } from "../settings-error";
import { autoHealFieldError } from "@/domain/auto-heal";
import { CONFIG_KEYS, eventsFromMap, eventMap, type NotificationTarget } from "@/domain/notifications";
import { DemoError, wait, type DemoContext } from "./demo-context";
import { AUTO_HEAL, AUTO_HEAL_DEFAULT } from "./fixtures/notifications";

/** Secret fields of the demo model; the response blanks them like the producer. */
const SECRETS: Record<string, readonly string[]> = { email: ["smtpPassword"], discord: ["token"], telegram: ["botToken"], signal: ["password", "token"], slack: ["token"], ntfy: ["password"], pushover: ["token"], gotify: ["token"], matrix: ["password"], googlechat: ["webhookUrl"] };

export function createDemoNotifications({ db }: DemoContext): NotificationsApi & AutoHealApi {
  const autoHeal = (hostId: string) => ({ ...AUTO_HEAL_DEFAULT, ...AUTO_HEAL[hostId], ...db.autoHeal.get(hostId) });
  return {
    async listNotificationTargets() {
      await wait(250);
      return structuredClone(db.notificationTargets);
    },
    async saveNotificationTarget(input) {
      await wait(450);
      const keys = CONFIG_KEYS[input.provider] as Record<string, string>;
      const unknown = Object.keys(input.config).find((key) => !keys[key]);
      if (unknown) throw new SettingsProblemError("notification.config-invalid", 400, `/config/${unknown}`);
      const previous = db.notificationTargets.find((t) => t.provider === input.provider);
      const secrets = SECRETS[input.provider] ?? [];
      const stored = new Set(previous?.credentialsSet ?? []);
      for (const field of secrets) if (typeof input.config[field] === "string" && input.config[field] !== "") stored.add(field);
      const config = Object.fromEntries(Object.entries(input.config).map(([k, v]) => [k, secrets.includes(k) ? "" : v]));
      const target: NotificationTarget = { kind: "target", id: previous?.id ?? db.notificationTargets.length + 1, provider: input.provider, enabled: input.enabled, config, credentialsSet: [...stored], events: eventsFromMap(eventMap(input.events)) };
      db.notificationTargets = previous ? db.notificationTargets.map((t) => (t === previous ? target : t)) : [...db.notificationTargets, target];
      db.pushEvent({ kind: "config", level: "info", title: "Hub-Einstellungen gespeichert: Benachrichtigungen" });
      db.changed("settings");
      return structuredClone(target);
    },
    async deleteNotificationTarget(provider) {
      await wait(300);
      db.notificationTargets = db.notificationTargets.filter((t) => t.provider !== provider);
      db.changed("settings");
    },
    async testNotificationTarget(provider) {
      await wait(700);
      const target = db.notificationTargets.find((t) => t.provider === provider);
      if (!target) throw new DemoError("not-found");
      return target.enabled ? { code: "notification.test-sent" } : { code: "notification.test-sent", warning: "notification.target-disabled" };
    },
    async getAutoHealSettings(hostId) {
      await wait(200);
      const host = db.host(hostId);
      if (!host) throw new DemoError("not-found");
      if (host.status !== "online") throw new SettingsProblemError("agent.unavailable", 502);
      return autoHeal(hostId);
    },
    async saveAutoHealSettings(hostId, input) {
      await wait(400);
      const current = await this.getAutoHealSettings(hostId);
      if (current.managedByEnvironment) throw new SettingsProblemError("autoheal.settings-env-managed", 409);
      const field = autoHealFieldError(input);
      if (field) throw new SettingsProblemError("autoheal.settings-invalid", 400, `/${field}`);
      db.autoHeal.set(hostId, { ...structuredClone(input), managedByEnvironment: false });
      db.pushEvent({ kind: "config", level: "info", title: "Hub-Einstellungen gespeichert: Auto Heal" });
      db.changed("settings");
      return autoHeal(hostId);
    },
  };
}
