import { isHostAvailable } from "./hosts";
import type { ContainerInfo, ContainerState, Health, Host, RuntimeCondition, Service, Stack, Tone } from "./model";

/**
 * Four colours across the UI (#371): green running, yellow unhealthy, red down,
 * grey stopped; a disconnected host is red, drawn pale and dashed where shapes allow.
 */
export type StackStatus = "running" | "unhealthy" | "down" | "stopped" | "busy" | "unreachable";

/** Worst service wins (down, then unhealthy); grey only when every service is stopped. */
export function stackStatus(stack: Stack, host: Host | undefined): StackStatus {
  if (stack.busyActionId) return "busy";
  if (host && !isHostAvailable(host)) return "unreachable";
  const states = stack.services.map((s) => serviceRuntime(s, host));
  if (!states.length) return stack.reportedStatus === "running" ? "running" : "stopped";
  if (states.includes("down")) return "down";
  if (states.some((s) => s === "unhealthy" || s === "restarting")) return "unhealthy";
  if (states.every((s) => s === "stopped" || s === "unknown")) return "stopped";
  return "running";
}

export const STACK_STATUS_TONE: Record<StackStatus, Tone> = {
  running: "ok",
  unhealthy: "warn",
  down: "err",
  stopped: "gray",
  busy: "accent",
  unreachable: "err",
};

export type RuntimeStatus = "running" | "unhealthy" | "starting" | "restarting" | "down" | "stopped" | "unknown";

/**
 * Live state decides running, restarting and health at once; the hub condition decides
 * whether a stopped container failed (red) and adds the restart-loop signal. Without a
 * matching condition a stopped container is grey.
 */
export function runtimeStatus(state: ContainerState, health: Health, hostOnline = true, condition?: RuntimeCondition): RuntimeStatus {
  if (!hostOnline || state === "unknown") return "unknown";
  if (state === "restarting") return "restarting";
  if (state === "stopped") return condition?.condition === "down" ? "down" : "stopped";
  if (health === "unhealthy") return "unhealthy";
  if (condition?.condition === "unhealthy" && condition.reason === "restart-loop") return "restarting";
  if (health === "starting") return "starting";
  return "running";
}

export const RUNTIME_TONE: Record<RuntimeStatus, Tone> = {
  running: "ok",
  starting: "ok",
  unhealthy: "warn",
  restarting: "warn",
  down: "err",
  stopped: "gray",
  unknown: "gray",
};

/** The condition only applies to the incarnation it was sampled for. */
function own(condition: RuntimeCondition | undefined, id: string | undefined): RuntimeCondition | undefined {
  return condition && id && condition.containerId === id ? condition : undefined;
}

export function serviceRuntime(service: Service, host: Host | undefined): RuntimeStatus {
  return runtimeStatus(service.state, service.health, host === undefined || isHostAvailable(host), own(service.condition, service.containerId));
}

export function containerRuntime(container: ContainerInfo, host: Host | undefined): RuntimeStatus {
  return runtimeStatus(container.state, container.health, host === undefined || isHostAvailable(host), own(container.condition, container.id));
}

export function runningCount(stack: Stack): number {
  return stack.runningCount ?? stack.services.filter((s) => s.state === "running").length;
}
