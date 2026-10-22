// Enlarged synthetic topology for isolated layout performance tests.
// Generated and deterministic; never imported by product routes.

import type { ContainerInfo, Host, Topology } from "@/domain/model";

export interface BenchData {
  hosts: Host[];
  containers: ContainerInfo[];
  topology: Topology;
}

/** `hostCount` hosts with `perHost` containers each, spread over networks of about ten members. */
export function benchTopology(template: { host: Host; container: ContainerInfo }, hostCount: number, perHost: number): BenchData {
  const hosts: Host[] = [];
  const containers: ContainerInfo[] = [];
  const topology: Topology = { networks: [], attachments: [], dependencies: [] };
  for (let h = 0; h < hostCount; h++) {
    const hostId = `bench-${h + 1}`;
    hosts.push({ ...template.host, id: hostId, name: `Bench ${String(h + 1).padStart(2, "0")}`, status: "online" });
    const nets = Math.max(1, Math.ceil(perHost / 10));
    for (let n = 0; n < nets; n++) topology.networks.push({ id: `${hostId}-n${n}`, name: `bench_net_${n}`, hostId, driver: "bridge", subnet: `198.51.100.${(n * 16) % 256}/28`, gateway: "198.51.100.1", internal: n % 4 === 3 });
    for (let c = 0; c < perHost; c++) {
      const id = `${hostId}-c${c}`;
      const state = c % 17 === 0 ? "stopped" : "running";
      containers.push({ ...template.container, id, name: `svc-${c}`, serviceName: `svc-${c}`, stackId: `bench-stack-${Math.floor(c / 5)}`, hostId, state, health: c % 23 === 0 ? "unhealthy" : "none", dockerLabels: undefined });
      topology.attachments.push({ containerId: id, networkId: `${hostId}-n${c % nets}`, ip: `198.51.100.${c % 250}` });
      if (c % 7 === 0) topology.attachments.push({ containerId: id, networkId: `${hostId}-n${(c + 1) % nets}`, ip: `198.51.100.${(c + 1) % 250}` });
      if (c % 5 !== 0) topology.dependencies.push({ from: id, to: `${hostId}-c${c - (c % 5)}` });
    }
  }
  return { hosts, containers, topology };
}
