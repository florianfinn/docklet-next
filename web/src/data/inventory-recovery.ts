// Source: docklet-next/issues/271
// Revision: bbab488e30d2e91edbd840bca929737c3ac1ab5c
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original result-local failed-host recovery without wire or adapter-port changes. C2 #271: transient stale-success observations retain result-local recovery. M23 #428 #389: identity-owned scope retention, independent recovery reasons, metrics lifetime and automatic inventory refresh tags. M35 #556: results name the hosts that answered.

import { transientInventoryReasons as transient } from "./http/inventory-observations";
import type { ReadTag } from "./http/host-read-policy";
import { replaceEqualDeep } from "@tanstack/react-query";
import type { Host, InventoryObservation } from "@/domain/model";
import { AdapterOutcomeError } from "./capability";

export const INVENTORY_RECOVERY_MS = 15_000;
const failures = new WeakMap<object, ReadonlySet<string>>();
// Hosts whose batch (rows or failure) is already in a partial or final result.
const answers = new WeakMap<object, ReadonlySet<string>>();

/** Failed or transiently stale batches on online hosts recover independently of shared host observations. */
export function inventoryRecovery() {
  const failed = new Set<string>(), answered = new Set<string>();
  return {
    answered(hostId: string) { answered.add(hostId); },
    partial<T extends object>(data: T): T { answers.set(data, new Set(answered)); return data; },
    failed(host: Host, error: unknown) {
      const code = error instanceof AdapterOutcomeError && error.outcome.kind === "error" ? error.outcome.code : typeof error === "string" ? error : undefined;
      if (host.status === "online" && code && transient.has(code)) failed.add(host.id);
    },
    observed(host: Host, observation: InventoryObservation) {
      if (host.status === "online" && observation.stale && (observation.recovery ?? observation.reason) && transient.has((observation.recovery ?? observation.reason)!)) failed.add(host.id);
    },
    result<T extends object>(data: T): T { failures.set(data, new Set(failed)); answers.set(data, new Set(answered)); return data; },
  };
}

/** Keep recovery metadata when React Query reuses an equal array, including an empty result. */
export function inventoryStructuralSharing(previous: unknown, next: unknown): unknown {
  // A new answered host must reach consumers even when the rows are equal, so the reused array is replaced then.
  const answered = (v: unknown) => [...((v && typeof v === "object" ? answers.get(v) : undefined) ?? [])].sort().join(",");
  const shared: unknown = answered(previous) === answered(next) ? replaceEqualDeep(previous, next) : next;
  if (shared && typeof shared === "object" && next && typeof next === "object") { failures.set(shared, failures.get(next) ?? new Set()); copyInventoryAnswers(next, shared); }
  return shared;
}

export function inventoryRecoveryInterval(data: unknown, hosts: readonly Host[] = []): number | false {
  const failed = data && typeof data === "object" ? failures.get(data) : undefined;
  return hosts.some((host) => host.status === "online" && failed?.has(host.id)) ? INVENTORY_RECOVERY_MS : false;
}

/** Preserve result-local recovery when an owner copies retained data to annotate its scope. */
export function copyInventoryRecovery(source: object, target: object, scopeFailed: readonly Host[] = []) {
  failures.set(target, new Set([...(failures.get(source) ?? []), ...scopeFailed.filter((h) => h.status === "online").map((h) => h.id)]));
  copyInventoryAnswers(source, target);
}

/** Carry the answered hosts to a derived result (owner copy, structural sharing, localized select). */
export function copyInventoryAnswers(source: object, target: object) {
  const known = answers.get(source);
  if (known) answers.set(target, known); else answers.delete(target);
}

/** True while a host in scope has not answered yet; a result without this metadata counts as settled. */
export function inventoryHostsPending(data: unknown, scope: readonly string[]): boolean {
  const known = data && typeof data === "object" ? answers.get(data) : undefined;
  return !!known && scope.some((id) => !known.has(id));
}

/**
 * The inventory queries have no interval poll: mount, remount after the stale time, invalidation and recovery are
 * all background reads, which still read standby hosts (Q-5a).
 */
export const INVENTORY_READ_TAG: ReadTag = "background";
