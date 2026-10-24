import { describe, expect, it } from "vitest";

import { TOPOLOGY } from "@/data/demo/fixtures/topology";

import type { Topology } from "./model";
import { checkConnect, checkDisconnect, nextFreeIp, parseCidr, removeBlock, sortNetworks, validateNetworkDraft, type NetworkDraft } from "./networks";

const draft = (p: Partial<NetworkDraft>): NetworkDraft => ({ hostId: "atlas", name: "lab_net", driver: "bridge", subnet: "", gateway: "", ipRange: "", parent: "", internal: false, ...p });
const atlas = TOPOLOGY.networks.filter((n) => n.hostId === "atlas");
const net = (id: string) => TOPOLOGY.networks.find((n) => n.id === id)!;
const container = (id: string, hostId: string, origin: "compose" | "standalone" | "unraid" = "standalone") => ({ id, name: id, hostId, origin });

describe("network drafts", () => {
  it("accepts a bridge without subnet and refuses Docker's names, duplicates and host bits", () => {
    expect(validateNetworkDraft(draft({}), atlas)).toEqual({});
    expect(validateNetworkDraft(draft({ name: "host" }), atlas).name).toBe("reserved");
    expect(validateNetworkDraft(draft({ name: "media_default" }), atlas).name).toBe("exists");
    expect(validateNetworkDraft(draft({ name: "-x" }), atlas).name).toBe("invalid");
    expect(parseCidr("198.51.100.5/28")).toBe(undefined);
    expect(validateNetworkDraft(draft({ subnet: "198.51.100.5/28" }), atlas).subnet).toBe("invalid");
  });

  it("checks gateway and range against the subnet and overlaps on the same host", () => {
    expect(validateNetworkDraft(draft({ subnet: "198.51.100.144/28" }), atlas).subnet).toBe("overlap");
    expect(validateNetworkDraft(draft({ subnet: "198.51.100.208/28", gateway: "198.51.100.1" }), atlas).gateway).toBe("outside");
    expect(validateNetworkDraft(draft({ subnet: "198.51.100.208/28", gateway: "198.51.100.208" }), atlas).gateway).toBe("network-address");
    expect(validateNetworkDraft(draft({ gateway: "198.51.100.209" }), atlas).gateway).toBe("needs-subnet");
    expect(validateNetworkDraft(draft({ subnet: "198.51.100.208/28", ipRange: "198.51.100.192/27" }), atlas).ipRange).toBe("outside");
    expect(validateNetworkDraft(draft({ subnet: "198.51.100.208/28", gateway: "198.51.100.209", ipRange: "198.51.100.216/29" }), atlas)).toEqual({});
  });

  it("requires parent, subnet and gateway for LAN drivers and one macvlan per parent", () => {
    expect(validateNetworkDraft(draft({ driver: "macvlan" }), atlas)).toEqual({ subnet: "required", gateway: "required", parent: "required" });
    expect(validateNetworkDraft(draft({ driver: "macvlan", parent: "br0", subnet: "192.0.2.0/24", gateway: "192.0.2.1" }), atlas).parent).toBe("parent-in-use");
    // Overlapping LAN pools conflict on this host even on different parents.
    expect(validateNetworkDraft(draft({ driver: "ipvlan", parent: "eth1", subnet: "192.0.2.0/24", gateway: "192.0.2.1" }), atlas).subnet).toBe("overlap");
  });
});

it("checks each address family on the same host, including LAN pools on other parents", () => {
  const existing = { id: "existing", hostId: "atlas", name: "existing", driver: "macvlan", internal: false, parent: "eth0", subnet: "192.0.2.0/24", subnet6: "2001:db8:204::/64", ipv6: true };
  const dual = draft({ driver: "ipvlan", parent: "eth1", subnet: "192.0.2.0/24", gateway: "192.0.2.1", ipv6: true, subnet6: "2001:db8:204::/64", gateway6: "2001:db8:204::1" });
  expect(validateNetworkDraft(dual, [existing])).toEqual({ subnet: "overlap", subnet6: "overlap" });
  expect(validateNetworkDraft(dual, [{ ...existing, hostId: "other" }])).toEqual({});
  expect(validateNetworkDraft({ ...dual, ipv4: false }, [existing])).toEqual({ subnet6: "overlap" });
  expect(validateNetworkDraft({ ...dual, ipv6: false }, [existing])).toEqual({ subnet: "overlap" });
  expect(validateNetworkDraft(draft({ name: "INGRESS" }), []).name).toBe("reserved");
});

describe("connect and disconnect checks", () => {
  it("blocks host/none, network-mode containers, other hosts and repeats", () => {
    expect(checkConnect(TOPOLOGY, container("c-duplicati", "atlas"), net("n-atlas-host")).block).toBe("builtin-network");
    expect(checkConnect(TOPOLOGY, container("c-home-homeassistant", "atlas", "compose"), net("n-atlas-media")).block).toBe("network-mode");
    expect(checkConnect(TOPOLOGY, container("c-duplicati", "atlas"), net("n-boreal-shared")).block).toBe("host-mismatch");
    expect(checkConnect(TOPOLOGY, container("c-duplicati", "atlas"), net("n-atlas-bridge")).block).toBe("already-connected");
  });

  it("validates static addresses only on configured subnets and proposes the next free one", () => {
    const kuma = container("c-uptime-kuma", "boreal");
    expect(checkConnect(TOPOLOGY, kuma, net("n-boreal-shared"))).toEqual({ block: undefined, ipError: undefined, ip6Error: undefined, warnings: [], ip: "198.51.100.98", ip6: "2001:db8:b::2" });
    expect(checkConnect(TOPOLOGY, kuma, net("n-boreal-shared"), "198.51.100.110").ip).toBe("198.51.100.110");
    expect(checkConnect(TOPOLOGY, kuma, net("n-boreal-shared"), "198.51.100.97").ipError).toBe("gateway");
    expect(checkConnect(TOPOLOGY, kuma, net("n-boreal-shared"), "198.51.100.111").ipError).toBe("network-address");
    expect(checkConnect(TOPOLOGY, kuma, net("n-boreal-shared"), "198.51.100.200").ipError).toBe("outside");
    expect(checkConnect(TOPOLOGY, container("c-duplicati", "atlas", "unraid"), net("n-atlas-br0"), "192.0.2.201").ipError).toBe("taken");
    expect(checkConnect(TOPOLOGY, kuma, net("n-boreal-web"), "198.51.100.9").ipError).toBe("static-unsupported");
    // LAN networks hand out addresses from their range only.
    expect(nextFreeIp(net("n-atlas-br0"), ["192.0.2.201"])).toBe("192.0.2.192");
    expect(checkConnect(TOPOLOGY, container("c-duplicati", "atlas", "unraid"), net("n-atlas-br0")).warnings).toEqual(["unraid-template"]);
  });

  it("reports an exhausted address pool only when the pool is known", () => {
    const kuma = container("c-uptime-kuma", "boreal");
    const base = net("n-boreal-shared");
    const full4 = { ...base, subnet: "192.0.2.0/30", gateway: "192.0.2.1", ipRange: undefined };
    const topo = (ips: string[]): Pick<Topology, "networks" | "attachments"> => ({ networks: [full4], attachments: ips.map((ip, i) => ({ containerId: `x${i}`, networkId: full4.id, ip })) });
    expect(checkConnect(topo([]), kuma, full4)).toMatchObject({ ip: "192.0.2.2", ipError: undefined });
    const full = checkConnect(topo(["192.0.2.2"]), kuma, full4);
    expect([full.ip, full.ipError]).toEqual([undefined, "pool-exhausted"]);
    // A fixed address never reports exhaustion; an unknown subnet keeps Docker's choice.
    expect(checkConnect(topo(["192.0.2.2"]), kuma, full4, "192.0.2.3").ipError).toBe("network-address");
    expect(checkConnect(topo([]), kuma, { ...full4, subnet: undefined }).ipError).toBeUndefined();
    // IPv6: complete scan of a small range is exhausted, a huge prefix is not.
    const n6 = { ...full4, subnet: undefined, ipv6: true, subnet6: "2001:db8:c::/126", gateway6: "2001:db8:c::1" };
    const t6 = (ips: string[]): Pick<Topology, "networks" | "attachments"> => ({ networks: [n6], attachments: ips.map((ip6, i) => ({ containerId: `x${i}`, networkId: n6.id, ip6 })) });
    expect(checkConnect(t6(["2001:db8:c::2", "2001:db8:c::3"]), kuma, n6).ip6Error).toBe("pool-exhausted");
    expect(checkConnect(t6(["2001:db8:c::2"]), kuma, n6)).toMatchObject({ ip6: "2001:db8:c::3", ip6Error: undefined });
    expect(checkConnect(t6([]), kuma, { ...n6, subnet6: "2001:db8:c::/64" }).ip6Error).toBeUndefined();
  });

  it("warns about dependents that lose their only path and about the last network", () => {
    const topology: Topology = { ...TOPOLOGY, dependencies: [{ from: "c-paperless-webserver", to: "c-paperless-db" }] };
    const db = container("c-paperless-db", "atlas", "compose");
    const check = checkDisconnect(topology, db, net("n-atlas-paperless"));
    expect([check.block, check.warnings, check.dependents]).toEqual([undefined, ["compose-runtime", "last-network", "dependents"], ["c-paperless-webserver"]]);
    expect(checkDisconnect(topology, container("c-home-homeassistant", "atlas", "compose"), net("n-atlas-host")).block).toBe("network-mode");
    expect(checkDisconnect(topology, db, net("n-atlas-media")).block).toBe("not-connected");
  });

  it("protects Docker's networks, Compose networks and attached ones from removal", () => {
    expect(removeBlock(TOPOLOGY, net("n-atlas-bridge"))).toBe("builtin-network");
    expect(removeBlock(TOPOLOGY, net("n-atlas-media"))).toBe("compose-managed");
    expect(removeBlock(TOPOLOGY, net("n-atlas-br0"))).toBe("in-use");
    expect(removeBlock(TOPOLOGY, net("n-boreal-shared"))).toBe(undefined);
  });

  it("orders LAN networks first and Docker's own last", () => {
    expect(sortNetworks(atlas).map((n) => n.name)).toEqual(["br0", "home_default", "media_default", "paperless_default", "bridge", "host"]);
  });
});
