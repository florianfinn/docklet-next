// Source: docklet-next/issues/204
// Revision: 09d1e632cb3a14bd48b87232acaf991b9d12145e
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original demo parity checks for advanced network settings and aliases.

import { afterEach, describe, expect, it } from "vitest";

import type { NetworkDraft } from "@/domain/networks";
import { errorCode } from "../api";
import { createDemoAdapter } from "./demo-adapter";

const draft = (patch: Partial<NetworkDraft> = {}): NetworkDraft => ({ hostId: "boreal", name: "example_net", driver: "bridge", subnet: "192.0.2.0/24", gateway: "192.0.2.1", ipRange: "", parent: "", internal: false, ...patch });
const code = (promise: Promise<unknown>) => promise.then(() => "ok", (e: unknown) => errorCode(e));
afterEach(() => localStorage.clear());

describe("advanced demo network parity", () => {
  it("returns and stores settings, skips reservations and retains normalized aliases independently of returned snapshots", async () => {
    const adapter = createDemoAdapter();
    const settings = draft({ labels: { " example.com/team ": "web" }, options: { "com.docker.network.driver.mtu": "1400" }, auxAddresses: [{ name: " reserve ", ip: "192.0.2.2" }], ipv6: true, subnet6: "2001:db8:204::/64", gateway6: "2001:db8:204::1", auxAddresses6: [{ name: "reserve", ip: "2001:0DB8:204::2" }] });
    const net = await adapter.createNetwork(settings);
    expect([net.labels, net.options, net.auxAddresses, net.auxAddresses6]).toEqual([{ "example.com/team": "web" }, settings.options, [{ name: "reserve", ip: "192.0.2.2" }], [{ name: "reserve", ip: "2001:db8:204::2" }]]);
    const attachment = await adapter.connectNetwork({ networkId: net.id, containerId: "c-whoami-test", aliases: [" API ", "api", "worker-2"] });
    expect([attachment.ip, attachment.ip6, attachment.aliases]).toEqual(["192.0.2.3", "2001:db8:204::3", ["api", "worker-2"]]);
    net.labels!["example.com/team"] = "mutated";
    attachment.aliases!.push("mutated");
    const topology = await adapter.getTopology();
    expect(topology.networks.find((n) => n.id === net.id)?.labels).toEqual({ "example.com/team": "web" });
    expect(topology.attachments.find((a) => a.networkId === net.id)?.aliases).toEqual(["api", "worker-2"]);
    // Stopped containers still protect a network from deletion.
    expect((await adapter.listContainers()).find((c) => c.id === "c-whoami-test")?.state).toBe("stopped");
    expect(await code(adapter.removeNetwork(net.id))).toBe("in-use");
    await adapter.disconnectNetwork({ networkId: net.id, containerId: "c-whoami-test" });
    expect(await code(adapter.removeNetwork(net.id))).toBe("ok");
  });

  it("revalidates keys, auxiliary addresses and aliases without relying on the dialogs", async () => {
    const adapter = createDemoAdapter();
    for (const patch of [{ labels: { "": "value" } }, { options: { key: "a", " key ": "b" } }, { auxAddresses: [{ name: "reserve", ip: "198.51.100.2" }] }]) {
      expect(await code(adapter.createNetwork(draft(patch)))).toBe("invalid-network");
    }
    const net = await adapter.createNetwork(draft({ auxAddresses: [{ name: "reserve", ip: "192.0.2.2" }] }));
    expect(await code(adapter.connectNetwork({ networkId: net.id, containerId: "c-whoami-test", aliases: ["bad_alias"] }))).toBe("invalid-network");
    expect(await code(adapter.connectNetwork({ networkId: net.id, containerId: "c-whoami-test", ip: "192.0.2.2" }))).toBe("invalid-ip");
    expect((await adapter.getTopology()).attachments.some((a) => a.networkId === net.id)).toBe(false);
  });

  it("supports IPv6-only creation and reservations without storing disabled-family fields", async () => {
    const adapter = createDemoAdapter();
    const net = await adapter.createNetwork(draft({ ipv4: false, ipv6: true, subnet: "ignored", auxAddresses: [{ name: "ignored", ip: "ignored" }], subnet6: "2001:db8:204::/64", gateway6: "2001:db8:204::1", auxAddresses6: [{ name: "reserve", ip: "2001:db8:204::2" }] }));
    expect([net.ipv4, net.subnet, net.auxAddresses]).toEqual([false, undefined, []]);
    const connected = await adapter.connectNetwork({ networkId: net.id, containerId: "c-whoami-test", aliases: ["api"] });
    expect([connected.ip, connected.ip6, connected.aliases]).toEqual([undefined, "2001:db8:204::3", ["api"]]);
  });
});
