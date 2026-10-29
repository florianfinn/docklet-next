// Source: docklet-next/issues/297
// Revision: 97a16304639854dc5aca368c879689d7a2161671
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original live notification target and Auto Heal settings transport.

import { CancelledError } from "@tanstack/react-query";
import type { AutoHealApi, NotificationsApi } from "../api";
import { AdapterOutcomeError, unavailable, type CapabilityDescriptor, type ResourceRef } from "../capability";
import { settingsProblem } from "../settings-error";
import type { AutoHealSettings } from "@/domain/auto-heal";
import { eventMap, isProvider, parseNotificationTarget, type NotificationTestResult, type WireEvent } from "@/domain/notifications";
import type { CsrfClient } from "./csrf-client";

export const NOTIFICATION_SETTINGS_OPERATIONS = [
  "listNotificationTargets", "saveNotificationTarget", "deleteNotificationTarget", "testNotificationTarget",
  "getAutoHealSettings", "saveAutoHealSettings",
] as const;

interface Options {
  baseUrl: string;
  client: CsrfClient;
  capability: (operation: string, ref: ResourceRef) => Promise<CapabilityDescriptor>;
  generation: () => number;
}

const HUB: ResourceRef = { kind: "hub", id: "0" };
const TEST_CODES = ["notification.test-sent", "notification.test-failed"];
const TEST_REASONS = ["delivery-unreachable", "delivery-rejected", "delivery-auth", "delivery-invalid"];
const invalidResponse = () => new AdapterOutcomeError({ kind: "error", code: "http.error" });

function names(value: unknown): string[] {
  if (value === null) return [];
  if (!Array.isArray(value) || value.length > 200 || !value.every((n) => typeof n === "string")) throw invalidResponse();
  return value as string[];
}

/** Validates the producer DTO, including the read-only environment flag. */
export function parseAutoHeal(raw: unknown): AutoHealSettings {
  if (!raw || typeof raw !== "object") throw invalidResponse();
  const v = raw as Record<string, unknown>;
  const int = (x: unknown, min: number, max: number) => typeof x === "number" && Number.isInteger(x) && x >= min && x <= max;
  if (typeof v.enabled !== "boolean" || typeof v.managedByEnvironment !== "boolean" || !int(v.maxRestarts, 1, 10) || !int(v.windowMinutes, 5, 1440)) throw invalidResponse();
  return { enabled: v.enabled, maxRestarts: v.maxRestarts as number, windowMinutes: v.windowMinutes as number, excludedStacks: names(v.excludedStacks), excludedContainers: names(v.excludedContainers), managedByEnvironment: v.managedByEnvironment };
}

export function parseTestResult(raw: unknown): NotificationTestResult {
  const v = raw && typeof raw === "object" ? (raw as Record<string, unknown>) : {};
  if (typeof v.code !== "string" || !TEST_CODES.includes(v.code)) throw invalidResponse();
  if (v.reason !== undefined && v.reason !== "" && !(typeof v.reason === "string" && TEST_REASONS.includes(v.reason))) throw invalidResponse();
  if (v.warning !== undefined && v.warning !== "" && v.warning !== "notification.target-disabled") throw invalidResponse();
  return {
    code: v.code as NotificationTestResult["code"],
    ...(v.reason ? { reason: v.reason as NotificationTestResult["reason"] } : {}),
    ...(v.warning ? { warning: "notification.target-disabled" as const } : {}),
  };
}

export function createNotificationSettings({ baseUrl, client, capability, generation }: Options): NotificationsApi & AutoHealApi {
  async function require(operation: string, ref: ResourceRef) {
    const descriptor = await capability(operation, ref);
    if (!descriptor.available) throw unavailable(operation, ref, descriptor.disabledReasonCode);
  }
  /** Responses that arrive after an identity change are discarded. */
  async function call(route: string, init: RequestInit = {}): Promise<unknown> {
    const identity = generation();
    const check = () => { if (identity !== generation()) throw new CancelledError(); };
    let response: Response;
    try {
      response = await client.fetch(`${baseUrl}${route}`, { credentials: "same-origin", cache: "no-store", ...init, headers: { Accept: "application/json", ...(init.body ? { "Content-Type": "application/json" } : {}), ...init.headers } });
    } catch {
      check();
      throw new AdapterOutcomeError({ kind: "error", code: "http.error" });
    }
    check();
    const body: unknown = await response.json().catch(() => undefined);
    check();
    if (!response.ok) throw settingsProblem(body, response.status);
    return body;
  }
  const enveloped = (body: unknown) => {
    const value = body && typeof body === "object" ? (body as { success?: unknown; data?: unknown }) : {};
    if (value.success !== true) throw invalidResponse();
    return value.data;
  };
  const host = (id: string) => `/environments/${encodeURIComponent(id)}/auto-heal`;

  return {
    async listNotificationTargets() {
      await require("listNotificationTargets", HUB);
      const body = await call("/environments/0/notifications/settings");
      if (body !== null && !Array.isArray(body)) throw invalidResponse();
      return (body ?? []).map((row) => { const parsed = parseNotificationTarget(row); if (!parsed) throw invalidResponse(); return parsed; });
    },
    async saveNotificationTarget(input) {
      await require("saveNotificationTarget", HUB);
      const config = { ...input.config, events: eventMap(input.events, input.config.events as Partial<Record<WireEvent, boolean>> | undefined) };
      const saved = parseNotificationTarget(await call("/environments/0/notifications/settings", { method: "POST", body: JSON.stringify({ provider: input.provider, enabled: input.enabled, config }) }));
      if (saved?.kind !== "target") throw invalidResponse();
      return saved;
    },
    async deleteNotificationTarget(provider) {
      await require("deleteNotificationTarget", HUB);
      if (!isProvider(provider)) throw invalidResponse();
      enveloped(await call(`/environments/0/notifications/settings/${provider}`, { method: "DELETE" }));
    },
    async testNotificationTarget(provider) {
      await require("testNotificationTarget", HUB);
      return parseTestResult(enveloped(await call(`/environments/0/notifications/test/${provider}`, { method: "POST" })));
    },
    async getAutoHealSettings(hostId) {
      await require("getAutoHealSettings", { kind: "host", id: hostId });
      const settings = parseAutoHeal(await call(host(hostId)));
      if (hostId !== "0") return settings;
      // The hub row is editable only on a valid "false" lock value; an unreadable lock is an error, never unlocked.
      const values = await call("/environments/0/settings");
      const lock = Array.isArray(values) ? values.find((v) => v && typeof v === "object" && v.key === "uiConfigDisabled") : undefined;
      if (lock?.value !== "true" && lock?.value !== "false") throw invalidResponse();
      return lock.value === "true" ? { ...settings, configurationLocked: true } : settings;
    },
    async saveAutoHealSettings(hostId, input) {
      await require("saveAutoHealSettings", { kind: "host", id: hostId });
      const body = { enabled: input.enabled, maxRestarts: input.maxRestarts, windowMinutes: input.windowMinutes, excludedStacks: input.excludedStacks, excludedContainers: input.excludedContainers };
      return parseAutoHeal(await call(host(hostId), { method: "PUT", body: JSON.stringify(body) }));
    },
  };
}
