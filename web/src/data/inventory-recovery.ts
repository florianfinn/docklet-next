// Source: docklet-next/issues/271
// Revision: bbab488e30d2e91edbd840bca929737c3ac1ab5c
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original result-local failed-host recovery without wire or adapter-port changes.

import { replaceEqualDeep } from "@tanstack/react-query";
import type { Host } from "@/domain/model";
import { AdapterOutcomeError } from "./capability";

export const INVENTORY_RECOVERY_MS = 15_000;
const failures = new WeakMap<object, ReadonlySet<string>>();
const transient = new Set(["http.error", "inventory.read-error", "agent.unavailable", "vulnerability.read-timeout", "vulnerability.read-unavailable", "vulnerability.storage-unavailable", "capability.read-unavailable"]);

/** Only a failed read on an online host schedules recovery; catalogue unavailability never does. */
export function inventoryRecovery() {
  const failed = new Set<string>();
  return {
    failed(host: Host, error: unknown) {
      const code = error instanceof AdapterOutcomeError && error.outcome.kind === "error" ? error.outcome.code : typeof error === "string" ? error : undefined;
      if (host.status === "online" && code && transient.has(code)) failed.add(host.id);
    },
    result<T extends object>(data: T): T { failures.set(data, new Set(failed)); return data; },
  };
}

/** Keep recovery metadata when React Query reuses an equal array, including an empty result. */
export function inventoryStructuralSharing(previous: unknown, next: unknown): unknown {
  const shared: unknown = replaceEqualDeep(previous, next);
  if (shared && typeof shared === "object" && next && typeof next === "object") failures.set(shared, failures.get(next) ?? new Set());
  return shared;
}

export function inventoryRecoveryInterval(data: unknown, hosts: readonly Host[] = []): number | false {
  const failed = data && typeof data === "object" ? failures.get(data) : undefined;
  return hosts.some((host) => host.status === "online" && failed?.has(host.id)) ? INVENTORY_RECOVERY_MS : false;
}
