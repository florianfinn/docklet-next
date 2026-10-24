// Source: docklet-next/docs/design/network-management.md
// Revision: 2e2b3e5996afd98ee8edca0a8899947b62a2a339
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original stopped endpoint address mapping, display and pool reservation tests for #204.

import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { checkConnect } from "@/domain/networks";
import { TopologyDetails } from "@/features/networks/TopologyDetails";
import { renderWithApp } from "@/test/render";
import { networkContainer, networkFixture } from "./network-commands.test-support";

describe.each([false, true])("stopped network endpoints (Edge=%s)", (edge) => {
  it("shows fixed Moby IPAM addresses and reserves them in both pool hints", async () => {
    const f = networkFixture(edge);
    f.state.networks[0]!.enableIPv6 = true;
    f.state.networks[0]!.ipam.config!.push({ subnet: "2001:db8::/64", gateway: "2001:db8::1" });
    // Container.NetworkEndpoint passes Moby EndpointIPAMConfig through, including its uppercase JSON fields.
    f.state.containers = [{ ...networkContainer, state: "exited", networkSettings: { networks: { example_net: { networkId: "net1", ipAddress: "", globalIPv6Address: "", ipamConfig: { IPv4Address: "192.0.2.2", IPv6Address: "2001:db8::2" }, aliases: ["api"] } } } }];
    const graph = await f.adapter.getTopology(); const hosts = await f.adapter.listHosts();
    const network = graph.networks.find((n) => n.id === "host~net1")!;
    expect(graph.attachments.find((a) => a.networkId === network.id)).toMatchObject({ ip: "192.0.2.2", ip6: "2001:db8::2", staticIp: true, staticIp6: true, aliases: ["api"] });
    const hint = checkConnect(graph, { id: "host~new", hostId: "host", name: "new", origin: "standalone" }, network);
    expect(hint).toMatchObject({ ip: "192.0.2.3", ip6: "2001:db8::3" });
    expect(checkConnect(graph, { id: "host~new", hostId: "host", name: "new", origin: "standalone" }, network, "192.0.2.2", "2001:db8::2")).toMatchObject({ ipError: "taken", ip6Error: "taken" });
    renderWithApp(<TopologyDetails selected={network.id} topology={graph} containers={[]} hosts={new Map(hosts.map((h) => [h.id, h]))} onSelect={() => {}} expanded={new Set()} onToggleExpand={() => {}} onConnect={() => {}} onRemove={() => {}} />, f.adapter);
    expect(screen.getByText("192.0.2.2") !== null).toBe(true);
    expect(screen.getByText("2001:db8::2") !== null).toBe(true);
  });
  it.each([null, "invalid", { IPv4Address: 7, IPv6Address: [] }])("ignores malformed optional IPAMConfig %j without losing stopped membership", async (ipamConfig) => {
    const f = networkFixture(edge);
    f.state.containers = [{ ...networkContainer, state: "exited", networkSettings: { networks: { example_net: { ipamConfig } } } }];
    const graph = await f.adapter.getTopology();
    expect(graph.attachments.find((a) => a.networkId === "host~net1")).toMatchObject({ containerId: "host~container1", ip: undefined, ip6: undefined });
  });
});
