import type { ContainerInfo, ContainerOrigin, Host, Stack, Tone } from "@/domain/model";
import { containerRuntime, runtimeStatus, RUNTIME_TONE, type RuntimeStatus } from "@/domain/status";

import type { AttentionItem, AttentionKind } from "./attention";

/**
 * State of one ring segment. Workloads on a disconnected host are
 * "unreachable": a connectivity problem, drawn red and dashed, never claimed
 * to be unhealthy or stopped. Their last known runtime state stays available.
 * "partial": some containers of a stack or group run, others are stopped.
 */
export type FleetState = RuntimeStatus | "unreachable" | "partial";

export const FLEET_TONE: Record<FleetState, Tone> = { ...RUNTIME_TONE, unreachable: "err", partial: "warn" };

export function fleetState(container: ContainerInfo, host: Host | undefined): FleetState {
  return host?.status === "offline" ? "unreachable" : containerRuntime(container, host);
}

/** What Docker reported before the host disconnected. */
export function lastKnownState(container: ContainerInfo): RuntimeStatus {
  return runtimeStatus(container.state, container.health, true);
}

/**
 * One segment of the fleet visual: a Compose stack, or the containers of one
 * origin outside Compose on a host (Run, Unraid, unclear), so standalone
 * workloads stay discoverable without one segment per container.
 */
export interface FleetUnit {
  id: string;
  kind: "stack" | "group";
  hostId: string;
  /** Stack name; groups are named by their origin in the UI. */
  name: string;
  stackId?: string;
  origin?: Exclude<ContainerOrigin, "compose">;
  containers: ContainerInfo[];
  state: FleetState;
  running: number;
}

const WORST: readonly RuntimeStatus[] = ["unhealthy", "restarting", "unknown", "starting"];

/** Aggregated state: the worst problem wins; a mix of running and stopped is "partial". */
export function unitState(containers: readonly ContainerInfo[], host: Host | undefined): FleetState {
  if (host?.status === "offline") return "unreachable";
  const states = containers.map((c) => containerRuntime(c, host));
  for (const s of WORST) if (states.includes(s)) return s;
  if (states.every((s) => s === "stopped")) return "stopped";
  if (states.some((s) => s === "stopped")) return "partial";
  return "running";
}

const GROUP_ORDER: readonly Exclude<ContainerOrigin, "compose">[] = ["standalone", "unraid", "unknown"];

/** Units per host in host order: stacks by name, then the origin groups. */
export function fleetUnits(hosts: readonly Host[], stacks: readonly Stack[], containers: readonly ContainerInfo[]): FleetUnit[] {
  const out: FleetUnit[] = [];
  const names = new Map(stacks.map((s) => [s.id, s.name]));
  for (const host of hosts) {
    const own = containers.filter((c) => c.hostId === host.id);
    const byStack = new Map<string, ContainerInfo[]>();
    for (const c of own) if (c.stackId) byStack.set(c.stackId, [...(byStack.get(c.stackId) ?? []), c]);
    const stackUnits = [...byStack].map(([stackId, list]): FleetUnit => ({
      id: `stack:${stackId}`,
      kind: "stack",
      hostId: host.id,
      name: names.get(stackId) ?? stackId,
      stackId,
      containers: list,
      state: unitState(list, host),
      running: list.filter((c) => containerRuntime(c, host) === "running").length,
    }));
    stackUnits.sort((a, b) => a.name.localeCompare(b.name, "de", { numeric: true, sensitivity: "base" }));
    out.push(...stackUnits);
    for (const origin of GROUP_ORDER) {
      const list = own.filter((c) => !c.stackId && c.origin === origin);
      if (list.length)
        out.push({ id: `group:${host.id}:${origin}`, kind: "group", hostId: host.id, name: origin, origin, containers: list, state: unitState(list, host), running: list.filter((c) => containerRuntime(c, host) === "running").length });
    }
  }
  return out;
}

export interface FleetBreakdown {
  /** Stacks and groups. */
  units: number;
  running: number;
  partial: number;
  starting: number;
  /** Unhealthy or restarting on a connected host. */
  problem: number;
  stopped: number;
  unknown: number;
  /** Units without current state because their host is disconnected. */
  unreachable: number;
  hostsOffline: number;
  /** Containers behind the units, and how many of them run. */
  containers: number;
  containersRunning: number;
  /** Containers on disconnected hosts. */
  containersUnreachable: number;
}

export function fleetBreakdown(hosts: Host[], units: readonly FleetUnit[]): FleetBreakdown {
  const byId = new Map(hosts.map((h) => [h.id, h]));
  const out: FleetBreakdown = { units: units.length, running: 0, partial: 0, starting: 0, problem: 0, stopped: 0, unknown: 0, unreachable: 0, hostsOffline: hosts.filter((h) => h.status === "offline").length, containers: 0, containersRunning: 0, containersUnreachable: 0 };
  for (const u of units) {
    const s = u.state;
    if (s === "running") out.running++;
    else if (s === "partial") out.partial++;
    else if (s === "starting") out.starting++;
    else if (s === "unhealthy" || s === "restarting") out.problem++;
    else if (s === "stopped") out.stopped++;
    else if (s === "unreachable") out.unreachable++;
    else out.unknown++;
    out.containers += u.containers.length;
    out.containersRunning += u.running;
    if (byId.get(u.hostId)?.status === "offline") out.containersUnreachable += u.containers.length;
  }
  return out;
}

/**
 * Operational problems counted by the "Probleme" chip: one per disconnected
 * host (not per container on it), per unhealthy service, per failed deploy and
 * per disk about to fill. Vulnerabilities have their own chip and are not added.
 */
export const PROBLEM_KINDS: readonly AttentionKind[] = ["offline", "unhealthy", "failed", "disk"];

export function problemItems(attention: AttentionItem[]): AttentionItem[] {
  return attention.filter((a) => PROBLEM_KINDS.includes(a.kind) && a.tone === "err");
}

export function problemCounts(attention: AttentionItem[]): Partial<Record<AttentionKind, number>> {
  const out: Partial<Record<AttentionKind, number>> = {};
  for (const a of problemItems(attention)) out[a.kind] = (out[a.kind] ?? 0) + 1;
  return out;
}
