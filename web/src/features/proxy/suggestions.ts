// Source: docklet-next/issues/549
// Revision: 1d8ef389b0a40f3ba27fb09f8f59d8ac993180ff
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original production inventory suggestions; no lab model or write path.

import { inventoryId } from "@/data/http/inventory-mapping";
import type { ContainerInfo, Stack } from "@/domain/model";
import type { Instance, Route } from "./types";

/** Only a complete, single Host matcher supplies a domain; compound rules stay opaque. */
export function simpleHost(rule: string): string | undefined {
  return /^Host\(([`"])([^`"\\\r\n]+)\1\)$/.exec(rule)?.[2];
}
export function containerSuggestions(instance: Instance, containers: ContainerInfo[]) {
  return containers.filter(c => c.hostId === instance.environmentId && c.state === "running");
}
export function tcpPorts(container: ContainerInfo) {
  return [...new Set(container.ports.filter(p => p.protocol === "tcp").map(p => p.container))];
}
/** Empty inventory network lists carry no positive knowledge of connectivity. */
export function offNetwork(instance: Instance, target: ContainerInfo, containers: ContainerInfo[]) {
  const proxy = containers.find(c => c.id === inventoryId(instance.environmentId, instance.identity.containerId));
  return !!proxy?.networks.length && !!target.networks.length && !target.networks.some(n => proxy.networks.includes(n));
}
export function usedDomains(routes: Route[], current?: Route) {
  return [...new Set(routes.filter(r => r.id !== current?.id).flatMap(r => simpleHost(r.rule) ?? []))];
}
/** Display ownership can be a name fallback; only exact same-host Compose path evidence authorises this link. */
export function manualImportStack(instance: Instance, containers: ContainerInfo[], stacks: Stack[]) {
  const proxy = containers.find(c => c.hostId === instance.environmentId && c.id === inventoryId(instance.environmentId, instance.identity.containerId));
  const path = proxy?.dockerLabels?.["com.docker.compose.project.working_dir"];
  const matches = path ? stacks.filter(s => s.hostId === instance.environmentId && s.path === path) : [];
  return matches.length === 1 ? matches[0] : undefined;
}
