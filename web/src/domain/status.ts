import { isHostAvailable } from "./hosts";
import type { ContainerInfo, ContainerState, Health, Host, Service, Stack, Tone } from "./model";

export type StackStatus = "running" | "partial" | "problem" | "stopped" | "busy" | "unreachable";

export function stackStatus(stack: Stack, host: Host | undefined): StackStatus {
  if (stack.busyActionId) return "busy";
  if (host && !isHostAvailable(host)) return "unreachable";
  if (stack.services.some((s) => s.health === "unhealthy" || s.state === "restarting")) return "problem";
  if (stack.reportedStatus) {
    if (stack.reportedStatus === "running") return "running";
    if (stack.reportedStatus === "stopped") return "stopped";
    return "partial";
  }
  if (stack.services.every((s) => s.state === "stopped")) return "stopped";
  if (stack.services.some((s) => s.state !== "running")) return "partial";
  return "running";
}

export const STACK_STATUS_TONE: Record<StackStatus, Tone> = {
  running: "ok",
  partial: "warn",
  problem: "err",
  stopped: "gray",
  busy: "accent",
  unreachable: "orange",
};

export type RuntimeStatus = "running" | "unhealthy" | "starting" | "restarting" | "stopped" | "unknown";

export function runtimeStatus(state: ContainerState, health: Health, hostOnline = true): RuntimeStatus {
  if (!hostOnline || state === "unknown") return "unknown";
  if (state === "restarting") return "restarting";
  if (state === "stopped") return "stopped";
  if (health === "unhealthy") return "unhealthy";
  if (health === "starting") return "starting";
  return "running";
}

export const RUNTIME_TONE: Record<RuntimeStatus, Tone> = {
  running: "ok",
  unhealthy: "err",
  starting: "info",
  restarting: "warn",
  stopped: "gray",
  unknown: "orange",
};

export function serviceRuntime(service: Service, host: Host | undefined): RuntimeStatus {
  return runtimeStatus(service.state, service.health, isHostAvailable(host));
}

export function containerRuntime(container: ContainerInfo, host: Host | undefined): RuntimeStatus {
  return runtimeStatus(container.state, container.health, isHostAvailable(host));
}

export function runningCount(stack: Stack): number {
  return stack.runningCount ?? stack.services.filter((s) => s.state === "running").length;
}
