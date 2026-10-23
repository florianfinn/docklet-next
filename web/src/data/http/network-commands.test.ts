// Source: docklet-next/docs/design/network-management.md
// Revision: 2e2b3e5996afd98ee8edca0a8899947b62a2a339
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original network command and producer-envelope tests for #204.

import { describe, expect, it, vi } from "vitest";
import { networkDraft, networkFixture, recordedNetworkResponses } from "./network-commands.test-support";

const wireCalls = (f: ReturnType<typeof networkFixture>) => f.state.calls.filter((c) => c.method !== "GET");

describe.each([false, true])("network writes through the hub (Edge=%s)", (edge) => {
  it("creates a dual-stack network with labels, options and per-family auxiliary addresses", async () => {
    const f = networkFixture(edge);
    expect((await f.adapter.listHosts())[0]?.connection).toBe(edge ? "edge" : "direct");
    const changed = vi.fn(); const stop = f.adapter.subscribeChanges(changed);
    const created = await f.adapter.createNetwork(networkDraft({ ipv6: true, subnet6: "2001:db8:204::/64", gateway6: "2001:db8:204::1", ipRange6: "2001:db8:204::100/120", labels: { "example.com/team": "web" }, options: { "com.docker.network.driver.mtu": "1400" }, auxAddresses: [{ name: "reserve", ip: "192.0.2.2" }], auxAddresses6: [{ name: "reserve", ip: "2001:db8:204::2" }] }));
    expect(created).toMatchObject({ id: "host~created", hostId: "host", labels: { "example.com/team": "web" }, auxAddresses: [{ name: "reserve", ip: "192.0.2.2" }], auxAddresses6: [{ name: "reserve", ip: "2001:db8:204::2" }] });
    expect(wireCalls(f)).toEqual([{ path: "/api/environments/host/networks", method: "POST", credentials: "same-origin", cache: "no-store", body: { name: "example_net", options: { driver: "bridge", internal: false, enableIPv4: true, enableIPv6: true, labels: { "example.com/team": "web" }, options: { "com.docker.network.driver.mtu": "1400" }, ipam: { config: [{ subnet: "192.0.2.0/24", gateway: "192.0.2.1", ipRange: "192.0.2.128/25", auxAddress: { reserve: "192.0.2.2" } }, { subnet: "2001:db8:204::/64", gateway: "2001:db8:204::1", ipRange: "2001:db8:204::100/120", auxAddress: { reserve: "2001:db8:204::2" } }] } } } }]);
    expect(changed).toHaveBeenCalledWith(["topology", "containers"]); stop();
  });
  it("sends enableIPv4=false and only the IPv6 pool for an IPv6-only network", async () => {
    const f = networkFixture(edge);
    const created = await f.adapter.createNetwork(networkDraft({ ipv4: false, ipv6: true, subnet6: "2001:db8:204::/64", gateway6: "2001:db8:204::1" }));
    expect((wireCalls(f)[0]?.body as { options: { enableIPv4?: boolean } }).options.enableIPv4).toBe(false);
    expect(created).toMatchObject({ ipv4: false, ipv6: true, subnet6: "2001:db8:204::/64" });
    expect(created.subnet).toBeUndefined();
  });
  it("maps a LAN parent separately into options.parent, overriding a competing option", async () => {
    const f = networkFixture(edge);
    await f.adapter.createNetwork(networkDraft({ driver: "macvlan", parent: "eth1", options: { parent: "ignored", mode: "bridge" } }));
    expect(wireCalls(f)[0]?.body).toMatchObject({ options: { driver: "macvlan", options: { parent: "eth1", mode: "bridge" } } });
  });
  it("omits empty automatic bridge pools instead of submitting an invalid empty subnet", async () => {
    const f = networkFixture(edge);
    await f.adapter.createNetwork(networkDraft({ subnet: "", gateway: "", ipRange: "" }));
    expect(wireCalls(f)[0]?.body).toMatchObject({ options: { enableIPv4: true, ipam: { config: [] } } });
  });
  it("deletes, connects fixed addresses/aliases, and disconnects using raw Docker IDs", async () => {
    const f = networkFixture(edge);
    await f.adapter.removeNetwork("host~net1");
    const attached = await f.adapter.connectNetwork({ networkId: "host~net1", containerId: "host~container1", ip: "192.0.2.7", ip6: "2001:db8::7", aliases: ["api"] });
    expect(attached).toMatchObject({ containerId: "host~container1", networkId: "host~net1", ip: "192.0.2.7", ip6: "2001:db8::7", aliases: ["api"], staticIp: true, staticIp6: true });
    await f.adapter.disconnectNetwork({ networkId: "host~net1", containerId: "host~container1" });
    expect(wireCalls(f).map(({ path, method, body }) => ({ path, method, body }))).toEqual([{ path: "/api/environments/host/networks/net1", method: "DELETE", body: undefined }, { path: "/api/environments/host/networks/net1/connect", method: "POST", body: { containerId: "container1", ipv4Address: "192.0.2.7", ipv6Address: "2001:db8::7", aliases: ["api"] } }, { path: "/api/environments/host/networks/net1/disconnect", method: "POST", body: { containerId: "container1" } }]);
  });
  it.each([
    ["delete-stopped", "in-use", "remove"], ["outside", "invalid-ip", "connect"], ["already-stopped", "already-connected", "connect"], ["not-connected", "not-connected", "disconnect"],
  ])("maps recorded %s to the typed %s error", async (key, code, operation) => {
    const f = networkFixture(edge); const response = recordedNetworkResponses[key]!; f.state.response = { body: response, status: response.status! };
    const result = operation === "remove" ? f.adapter.removeNetwork("host~net1") : operation === "connect" ? f.adapter.connectNetwork({ networkId: "host~net1", containerId: "host~container1" }) : f.adapter.disconnectNetwork({ networkId: "host~net1", containerId: "host~container1" });
    await expect(result).rejects.toMatchObject({ code });
  });
  it.each(["compose-managed", "builtin-network", "overlap"])("preserves %s from the documented network error envelope", async (code) => {
    const f = networkFixture(edge); f.state.response = { body: { ...recordedNetworkResponses["delete-stopped"], code: `network.${code}` }, status: 409 };
    await expect(code === "overlap" ? f.adapter.createNetwork(networkDraft()) : f.adapter.removeNetwork("host~net1")).rejects.toMatchObject({ code });
  });
  it("keeps unknown codes on the generic outcome path", async () => {
    const f = networkFixture(edge); f.state.response = { body: { ...recordedNetworkResponses["create-invalid"], code: "network.future-code" }, status: 400 };
    await expect(f.adapter.createNetwork(networkDraft())).rejects.toMatchObject({ outcome: { kind: "error", code: "network.future-code", status: 400 } });
  });
  it.each(["transport", "malformed", "inspect"])("reports outcome unknown after %s failure without claiming rollback", async (failure) => {
    const f = networkFixture(edge); f.state.failTransport = failure === "transport"; f.state.malformed = failure === "malformed"; f.state.inspectFails = failure === "inspect";
    await expect(f.adapter.createNetwork(networkDraft())).rejects.toMatchObject({ outcome: { kind: "error", code: "network.outcome-unknown" } });
    expect(wireCalls(f)).toHaveLength(1);
  });
  it("denies only the targeted host before sending any network request", async () => {
    const f = networkFixture(edge); f.state.deniedHosts.add("host");
    await expect(f.adapter.createNetwork(networkDraft())).rejects.toMatchObject({ outcome: { kind: "not-available", resourceRef: { kind: "host", id: "host" } } });
    await expect(f.adapter.removeNetwork("host~net1")).rejects.toMatchObject({ outcome: { kind: "not-available", resourceRef: { kind: "network", id: "host~net1" } } });
    await expect(f.adapter.connectNetwork({ networkId: "host~net1", containerId: "host~container1" })).rejects.toMatchObject({ outcome: { kind: "not-available" } });
    await expect(f.adapter.disconnectNetwork({ networkId: "host~net1", containerId: "host~container1" })).rejects.toMatchObject({ outcome: { kind: "not-available" } });
    expect(f.state.calls).toEqual([]);
    expect((await f.adapter.createNetwork(networkDraft({ hostId: "other" }))).hostId).toBe("other");
  });
  it("refuses cross-host membership without sending a network write", async () => {
    const f = networkFixture(edge);
    await expect(f.adapter.connectNetwork({ networkId: "host~net1", containerId: "other~container1" })).rejects.toMatchObject({ code: "host-mismatch" });
    expect(f.state.calls).toEqual([]);
  });
});
