import type { ContainerInfo, ContainerOrigin, Host, Stack, Tone } from "@/domain/model";
import { containerRuntime, RUNTIME_TONE, type RuntimeStatus } from "@/domain/status";
import { hasUpdate } from "@/domain/updates";

import type { AttentionItem, AttentionKind } from "./attention";

/**
 * State of one ring segment in the four colours (#371). Workloads on a
 * disconnected host are "unreachable": red, pale and dashed, never claimed to be
 * down or stopped. Their last known runtime state stays available.
 */
export type FleetState = RuntimeStatus | "unreachable";

export const FLEET_TONE: Record<FleetState, Tone> = { ...RUNTIME_TONE, unreachable: "err" };

export function fleetState(container: ContainerInfo, host: Host | undefined): FleetState {
  return hostUnreachable(host) ? "unreachable" : containerRuntime(container, host);
}

/**
 * A host without current state: disconnected (Edge), or a Direct agent whose last inventory
 * read found it unavailable while its registration still says online.
 */
export function hostUnreachable(host: Pick<Host, "status" | "inventory"> | undefined): boolean {
  return host?.status === "offline" || host?.inventory?.reason === "agent.unavailable";
}

/** What Docker and the hub reported before the host disconnected. */
export function lastKnownState(container: ContainerInfo): RuntimeStatus {
  return containerRuntime(container, undefined);
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
  /** Stopped containers of a unit that is not stopped as a whole: the grey remainder. */
  stopped: number;
  /** Some container has an image update (orange inner arc). */
  update: boolean;
}

const WORST: readonly RuntimeStatus[] = ["down", "unhealthy", "restarting"];

/** Worst container wins (down, then unhealthy or restarting); grey only when all are stopped. */
export function unitState(containers: readonly ContainerInfo[], host: Host | undefined): FleetState {
  if (hostUnreachable(host)) return "unreachable";
  // A known stack whose containers were removed (taken down) is stopped, not unknown.
  if (!containers.length) return "stopped";
  const states = containers.map((c) => containerRuntime(c, host));
  for (const s of WORST) if (states.includes(s)) return s;
  if (states.every((s) => s === "unknown")) return "unknown";
  if (states.every((s) => s === "stopped" || s === "unknown")) return "stopped";
  return states.includes("running") ? "running" : "starting";
}

function unit(base: Pick<FleetUnit, "id" | "kind" | "hostId" | "name" | "stackId" | "origin">, list: ContainerInfo[], host: Host, stack?: Stack): FleetUnit {
  const state = unitState(list, host);
  const runtimes = list.map((c) => containerRuntime(c, host));
  return {
    ...base,
    containers: list,
    state,
    running: runtimes.filter((s) => s === "running").length,
    stopped: state === "stopped" || state === "unreachable" ? 0 : runtimes.filter((s) => s === "stopped").length,
    // Project update checks report on the stack services; container states stay unchecked then.
    update: list.some((c) => hasUpdate(c.update)) || !!stack?.services.some((s) => hasUpdate(s.update)),
  };
}

/** Grey share at the end of a partly stopped segment; whole stopped segments are grey already. */
export function remainderStart(start: number, end: number, unit: FleetUnit): number | undefined {
  if (!unit.stopped || !unit.containers.length) return undefined;
  return end - ((end - start) * unit.stopped) / unit.containers.length;
}

const GROUP_ORDER: readonly Exclude<ContainerOrigin, "compose">[] = ["standalone", "unraid", "unknown"];

/** Units per host in host order: stacks by name (also those without containers), then the origin groups. */
export function fleetUnits(hosts: readonly Host[], stacks: readonly Stack[], containers: readonly ContainerInfo[]): FleetUnit[] {
  const out: FleetUnit[] = [];
  const byId = new Map(stacks.map((s) => [s.id, s]));
  for (const host of hosts) {
    const own = containers.filter((c) => c.hostId === host.id);
    const byStack = new Map<string, ContainerInfo[]>();
    for (const c of own) if (c.stackId) byStack.set(c.stackId, [...(byStack.get(c.stackId) ?? []), c]);
    for (const s of stacks) if (s.hostId === host.id && !byStack.has(s.id)) byStack.set(s.id, []);
    const stackUnits = [...byStack].map(([stackId, list]) => unit({ id: `stack:${stackId}`, kind: "stack", hostId: host.id, name: byId.get(stackId)?.name ?? stackId, stackId }, list, host, byId.get(stackId)));
    stackUnits.sort((a, b) => a.name.localeCompare(b.name, "de", { numeric: true, sensitivity: "base" }));
    out.push(...stackUnits);
    for (const origin of GROUP_ORDER) {
      const list = own.filter((c) => !c.stackId && c.origin === origin);
      if (list.length) out.push(unit({ id: `group:${host.id}:${origin}`, kind: "group", hostId: host.id, name: origin, origin }, list, host));
    }
  }
  return out;
}

export interface FleetBreakdown {
  /** Stacks and groups. */
  units: number;
  /** Running or starting. */
  running: number;
  /** Unhealthy or in a restart loop on a connected host. */
  unhealthy: number;
  /** Crashed or exited with an error. */
  down: number;
  stopped: number;
  unknown: number;
  /** Units without current state because their host is disconnected. */
  unreachable: number;
  /** Units with an image update. */
  updates: number;
  hostsOffline: number;
  /** Containers behind the units, and how many of them run. */
  containers: number;
  containersRunning: number;
  /** Containers on disconnected hosts. */
  containersUnreachable: number;
}

/** Filter chip of the ring: one of the four colours, or units with updates. */
export type FleetFilter = "down" | "unhealthy" | "stopped" | "running" | "update";

export function matchesFleetFilter(u: FleetUnit, filter: FleetFilter): boolean {
  if (filter === "update") return u.update;
  if (filter === "down") return u.state === "down" || u.state === "unreachable";
  if (filter === "unhealthy") return u.state === "unhealthy" || u.state === "restarting";
  if (filter === "stopped") return u.state === "stopped" || u.state === "unknown";
  return u.state === "running" || u.state === "starting";
}

export function fleetBreakdown(hosts: Host[], units: readonly FleetUnit[]): FleetBreakdown {
  const byId = new Map(hosts.map((h) => [h.id, h]));
  const out: FleetBreakdown = { units: units.length, running: 0, unhealthy: 0, down: 0, stopped: 0, unknown: 0, unreachable: 0, updates: 0, hostsOffline: hosts.filter(hostUnreachable).length, containers: 0, containersRunning: 0, containersUnreachable: 0 };
  for (const u of units) {
    const s = u.state;
    if (s === "running" || s === "starting") out.running++;
    else if (s === "unhealthy" || s === "restarting") out.unhealthy++;
    else if (s === "down") out.down++;
    else if (s === "stopped") out.stopped++;
    else if (s === "unreachable") out.unreachable++;
    else out.unknown++;
    if (u.update) out.updates++;
    out.containers += u.containers.length;
    out.containersRunning += u.running;
    if (hostUnreachable(byId.get(u.hostId))) out.containersUnreachable += u.containers.length;
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
