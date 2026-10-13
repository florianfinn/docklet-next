import { waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { errorCode } from "../api";
import { createDemoAdapter } from "./demo-adapter";
import { DemoDb } from "./demo-db";
import { syncStackNetworks } from "./demo-networks";
import { simulationStore } from "./simulation";

beforeEach(() => simulationStore.set((s) => ({ ...s, latency: "instant", live: false, actionSpeed: 3 })));
afterEach(() => {
  simulationStore.reset();
  localStorage.clear();
});

const code = (p: Promise<unknown>) => p.then(() => "ok").catch((e: unknown) => errorCode(e));

describe("compose networks in the demo topology", () => {
  it("matches the seeded topology for every seeded stack", () => {
    const db = new DemoDb();
    const sorted = (t: typeof db.topology) => ({ ...t, attachments: t.attachments.map((a) => `${a.containerId}@${a.networkId}=${a.ip ?? ""}${a.staticIp ? "!" : ""}`).sort() });
    const before = sorted(structuredClone(db.topology));
    for (const stack of db.stacks) syncStackNetworks(db, stack.id);
    expect(sorted(db.topology)).toEqual(before);
  });

  it("gives a newly deployed stack its networks, then keeps connect, disconnect and redeploy consistent", async () => {
    const adapter = createDemoAdapter();
    const compose = "services:\n  app:\n    image: nginx:1.27\n    networks: [default, backend]\n  db:\n    image: postgres:17-alpine\n    networks: [backend]\nnetworks:\n  backend:\n    internal: true\n";
    const { stackId } = await adapter.deployStack({ stackId: null, hostId: "boreal", name: "werkstatt", compose, env: "", labels: [] });
    const netsOf = async (id: string) => (await adapter.listContainers()).find((c) => c.id === id)?.networks;
    await waitFor(async () => expect(await netsOf("c-werkstatt-app")).toEqual(["werkstatt_default", "werkstatt_backend"]), { timeout: 8000 });

    const topo = await adapter.getTopology();
    const own = topo.networks.filter((n) => n.stackId === stackId);
    expect(own.map((n) => [n.name, n.hostId, n.driver, n.internal, !!n.subnet])).toEqual([
      ["werkstatt_default", "boreal", "bridge", false, true],
      ["werkstatt_backend", "boreal", "bridge", true, true],
    ]);
    const ips = topo.attachments.filter((a) => a.containerId.startsWith("c-werkstatt-")).map((a) => a.ip);
    expect([ips.length, ips.every(Boolean), new Set(ips).size]).toEqual([3, true, 3]);
    expect(await netsOf("c-werkstatt-db")).toEqual(["werkstatt_backend"]);

    // The connect and disconnect checks see the deployed attachments.
    const backend = own[1]!.id;
    expect(await code(adapter.connectNetwork({ containerId: "c-werkstatt-db", networkId: backend }))).toBe("already-connected");
    expect(await code(adapter.connectNetwork({ containerId: "c-werkstatt-db", networkId: "n-boreal-shared" }))).toBe("ok");
    expect(await netsOf("c-werkstatt-db")).toEqual(["werkstatt_backend", "shared_proxy"]);
    expect(await code(adapter.disconnectNetwork({ containerId: "c-werkstatt-db", networkId: backend }))).toBe("ok");
    expect(await netsOf("c-werkstatt-db")).toEqual(["shared_proxy"]);
    expect(await code(adapter.removeNetwork(backend))).toBe("compose-managed");

    // A redeploy recreates the containers from the file: the runtime connect is gone, the file's networks are back.
    await adapter.deployStack({ stackId, hostId: "boreal", name: "werkstatt", compose, env: "", labels: [] });
    await waitFor(async () => expect(await netsOf("c-werkstatt-db")).toEqual(["werkstatt_backend"]), { timeout: 8000 });
    const after = await adapter.getTopology();
    expect(after.networks.filter((n) => n.stackId === stackId).length).toBe(2);
    expect(after.attachments.filter((a) => a.containerId === "c-werkstatt-app").map((a) => a.ip)).toEqual(topo.attachments.filter((a) => a.containerId === "c-werkstatt-app").map((a) => a.ip));
  });

  it("rejects a connect when the address pool is exhausted and leaves no attachment behind", async () => {
    const adapter = createDemoAdapter();
    await adapter.createNetwork({ hostId: "boreal", name: "tiny", driver: "bridge", subnet: "192.0.2.0/30", gateway: "192.0.2.1", ipRange: "", parent: "", internal: false });
    const tiny = (await adapter.getTopology()).networks.find((n) => n.name === "tiny")!;
    const boreal = (await adapter.listContainers()).filter((c) => c.hostId === "boreal" && !c.networks.includes("tiny")).slice(0, 2);
    expect(await code(adapter.connectNetwork({ containerId: boreal[0]!.id, networkId: tiny.id }))).toBe("ok");
    const before = (await adapter.getTopology()).attachments.length;
    expect(await code(adapter.connectNetwork({ containerId: boreal[1]!.id, networkId: tiny.id }))).toBe("invalid-ip");
    const after = (await adapter.getTopology()).attachments;
    expect(after.length).toBe(before);
    expect(after.filter((a) => a.networkId === tiny.id).map((a) => a.ip)).toEqual(["192.0.2.2"]);
  });

  it("references external networks without creating them and honours host mode and fixed addresses", () => {
    const db = new DemoDb();
    db.compose.monitoring = "services:\n  prometheus:\n    image: prom/prometheus:v3.5.0\n    networks:\n      shared_proxy:\n        ipv4_address: 198.51.100.110\n  grafana:\n    image: grafana/grafana:12.1.1\n    networks: [proxy]\n  node-exporter:\n    image: prom/node-exporter:v1.9.1\n    network_mode: host\nnetworks:\n  shared_proxy:\n    external: true\n  proxy:\n    external: true\n    name: missing_net\n";
    const count = db.topology.networks.length;
    syncStackNetworks(db, "monitoring");
    const of = (id: string) => db.topology.attachments.filter((a) => a.containerId === id).map((a) => [a.networkId, a.ip ?? null, a.staticIp ?? false]);
    expect(db.topology.networks.length).toBe(count);
    expect(of("c-monitoring-prometheus")).toEqual([["n-boreal-shared", "198.51.100.110", true]]);
    expect(of("c-monitoring-grafana")).toEqual([]);
    expect(of("c-monitoring-node-exporter")).toEqual([["n-boreal-host", null, false]]);
  });
});
