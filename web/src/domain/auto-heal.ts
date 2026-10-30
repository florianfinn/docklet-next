// Source: docklet-next/issues/297
// Revision: 97a16304639854dc5aca368c879689d7a2161671
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original per-host Auto Heal settings model typed by the generated v10 schema.

import type { components } from "@/api/generated/schema";

export type AutoHealSettings = Omit<components["schemas"]["AutohealSettings"], "$schema" | "excludedStacks" | "excludedContainers"> & {
  excludedStacks: string[];
  excludedContainers: string[];
  /** Hub only: UI configuration is disabled, so the hub refuses saves (not an environment value). */
  configurationLocked?: boolean;
};
export type AutoHealInput = Omit<AutoHealSettings, "managedByEnvironment" | "configurationLocked">;

export const AUTO_HEAL_LIMITS = { maxRestarts: { min: 1, max: 10 }, windowMinutes: { min: 5, max: 1440 }, names: 200 } as const;
/** Window presets in minutes; a stored custom value stays selectable unchanged. */
export const WINDOW_PRESETS: readonly number[] = [15, 30, 60, 120, 240];

const STACK_NAME = /^[a-z0-9][a-z0-9_-]*$/;
const CONTAINER_NAME = /^[a-zA-Z0-9][a-zA-Z0-9_.-]*$/;

/** Mirrors the producer rule; the server stays authoritative and names the field. */
export function autoHealFieldError(input: AutoHealInput): keyof AutoHealInput | undefined {
  const { maxRestarts, windowMinutes } = AUTO_HEAL_LIMITS;
  if (!Number.isInteger(input.maxRestarts) || input.maxRestarts < maxRestarts.min || input.maxRestarts > maxRestarts.max) return "maxRestarts";
  if (!Number.isInteger(input.windowMinutes) || input.windowMinutes < windowMinutes.min || input.windowMinutes > windowMinutes.max) return "windowMinutes";
  if (input.excludedStacks.length > AUTO_HEAL_LIMITS.names || input.excludedStacks.some((n) => n.length > 63 || !STACK_NAME.test(n))) return "excludedStacks";
  if (input.excludedContainers.length > AUTO_HEAL_LIMITS.names || input.excludedContainers.some((n) => n.length > 128 || !CONTAINER_NAME.test(n))) return "excludedContainers";
  return undefined;
}
