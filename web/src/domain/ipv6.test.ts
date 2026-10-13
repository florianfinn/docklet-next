import { describe, expect, it } from "vitest";

import { TOPOLOGY } from "@/data/demo/fixtures/topology";

import { canonicalIp6, checkCidr6, checkSubnet6, formatIp6, nextFreeIp6, parseCidr6, poolFullyScanned6, parseIp6 } from "./ipv6";
import { checkConnect, validateNetworkDraft, type NetworkContainer, type NetworkDraft } from "./networks";
import type { TopologyNetwork } from "./model";

const draft = (patch: Partial<NetworkDraft>): NetworkDraft => ({ hostId: "boreal", name: "net_v6", driver: "bridge", ipv4: true, ipv6: true, subnet: "", gateway: "", ipRange: "", subnet6: "", gateway6: "", ipRange6: "", parent: "", internal: false, ...patch });
const boreal = TOPOLOGY.networks.filter((n) => n.hostId === "boreal");
const net = (id: string) => TOPOLOGY.networks.find((n) => n.id === id) as TopologyNetwork;
const whoami: NetworkContainer = { id: "c-whoami-test", name: "whoami-test", hostId: "boreal", origin: "standalone" };

describe("IPv6 addresses", () => {
  it("reads full, compressed and IPv4-embedded forms and writes RFC 5952 text", () => {
    const full = parseIp6("2001:0DB8:0000:0000:0000:0000:0000:0001");
    expect([full === parseIp6("2001:db8::1"), canonicalIp6("2001:0db8:0:0:1:0:0:1"), canonicalIp6("::ffff:192.0.2.1"), formatIp6(0n), canonicalIp6("2001:db8:0:1:1:1:1:1")]).toEqual([true, "2001:db8::1:0:0:1", "::ffff:c000:201", "::", "2001:db8:0:1:1:1:1:1"]);
    expect(["2001:db8::1::2", "2001:db8:::1", "fe80::1%eth0", "2001:db8::g", "1.2.3.4::", "2001:db8:0:0:0:0:0:0:1", "[2001:db8::1]", ""].map(parseIp6)).toEqual(Array(8).fill(undefined));
  });

  it("parses every prefix length 0–128 and refuses host bits", () => {
    const err = (t: string) => {
      const r = checkCidr6(t);
      return r.ok ? r.cidr.prefix : r.error;
    };
    expect(["::/0", "2001::/16", "2001:db8:c::/64", "2001:db8::4/126", "2001:db8::5/128", "2001:db8::1/64", "2001:db8::/129", "2001:db8::", "2001:db8::/-1", "2001:db8::/64/1"].map(err)).toEqual([0, 16, 64, 126, 128, "host-bits", "invalid", "invalid", "invalid", "invalid"]);
  });

  it("applies the subnet policy separately: room for gateway and container, Docker's bridge exclusions", () => {
    const policy = (t: string) => checkSubnet6(parseCidr6(t)!);
    expect(["2001::/16", "2001:db8:c::/64", "2001:db8::/126", "2001:db8::/127", "2001:db8::/128", "ff02::/64", "::/0", "fe80::/64", "fe80::/10", "fe80:0:0:1::/64", "::ffff:c000:200/120"].map(policy)).toEqual([undefined, undefined, undefined, "prefix", "prefix", "multicast", "multicast", undefined, "link-local", undefined, "mapped"]);
  });

  it("allocates the first free pool address and treats other notations of a taken one as taken", () => {
    const n = { subnet6: "2001:db8:c::/64", gateway6: "2001:db8:c::1" };
    expect([nextFreeIp6(n, []), nextFreeIp6(n, ["2001:DB8:C:0:0:0:0:2"]), nextFreeIp6({ ...n, ipRange6: "2001:db8:c::100/120" }, [])]).toEqual(["2001:db8:c::2", "2001:db8:c::3", "2001:db8:c::100"]);
  });
});

describe("pool scan", () => {
  it("is complete only for ranges within the scan limit", () => {
    expect([poolFullyScanned6({ subnet6: "2001:db8:c::/120" }), poolFullyScanned6({ subnet6: "2001:db8:c::/116" }), poolFullyScanned6({ subnet6: "2001:db8:c::/115" }), poolFullyScanned6({ subnet6: "2001:db8:c::/64", ipRange6: "2001:db8:c::100/120" }), poolFullyScanned6({})]).toEqual([true, true, false, true, false]);
  });
});

describe("dual-stack and IPv6-only network drafts", () => {
  it("validates IPv6 independently of IPv4 and leaves IPv4-only drafts unchanged", () => {
    expect(validateNetworkDraft(draft({ subnet6: "2001:db8:d::/64", gateway6: "2001:db8:d::1" }), boreal)).toEqual({});
    // Gateway outside the prefix, the anycast address, a range outside, an overlap in other notation.
    expect(validateNetworkDraft(draft({ subnet6: "2001:db8:d::/64", gateway6: "2001:db8:e::1", ipRange6: "2001:db8:e::/80" }), boreal)).toEqual({ gateway6: "outside", ipRange6: "outside" });
    expect(validateNetworkDraft(draft({ subnet6: "2001:db8:d::/64", gateway6: "2001:db8:d::" }), boreal).gateway6).toBe("network-address");
    expect(validateNetworkDraft(draft({ subnet6: "2001:0db8:000b::/64" }), boreal).subnet6).toBe("overlap");
    expect(validateNetworkDraft(draft({ subnet6: "2001:db8:b::/48" }), boreal).subnet6).toBe("overlap");
    // Narrow ranges inside the prefix are fine; a single anycast address is not a pool.
    expect([validateNetworkDraft(draft({ subnet6: "2001:db8:d::/64", ipRange6: "2001:db8:d::10/126" }), boreal), validateNetworkDraft(draft({ subnet6: "2001:db8:d::/64", ipRange6: "2001:db8:d::/128" }), boreal)]).toEqual([{}, { ipRange6: "network-address" }]);
    expect([validateNetworkDraft(draft({ subnet6: "2001:db8:d::/127" }), boreal).subnet6, validateNetworkDraft(draft({ subnet6: "2001:db8:100::/40" }), boreal).subnet6]).toEqual(["prefix", undefined]);
    // An IPv4 error stays an IPv4 error; IPv6 fields do not change it.
    expect(validateNetworkDraft(draft({ subnet: "198.51.100.96/28", subnet6: "2001:db8:d::/64" }), boreal)).toEqual({ subnet: "overlap" });
    const legacy = { hostId: "boreal", name: "old_net", driver: "bridge", subnet: "198.51.100.96/28", gateway: "", ipRange: "", parent: "", internal: false } as NetworkDraft;
    expect(validateNetworkDraft(legacy, boreal)).toEqual({ subnet: "overlap" });
  });

  it("supports IPv6-only, needs one family and requires LAN fields per enabled family", () => {
    expect(validateNetworkDraft(draft({ ipv4: false, subnet: "nonsense", subnet6: "2001:db8:d::/64" }), boreal)).toEqual({});
    expect(validateNetworkDraft(draft({ ipv4: false, ipv6: false }), boreal)).toEqual({ family: "no-family" });
    expect(validateNetworkDraft(draft({ driver: "macvlan", parent: "eth9", ipv4: false }), boreal)).toEqual({ subnet6: "required", gateway6: "required" });
    // A bridge may leave the IPv6 prefix to Docker; a gateway needs a prefix.
    expect([validateNetworkDraft(draft({}), boreal), validateNetworkDraft(draft({ gateway6: "2001:db8:d::1" }), boreal)]).toEqual([{}, { gateway6: "needs-subnet" }]);
  });
});

describe("static IPv6 on connect", () => {
  const topology = { networks: TOPOLOGY.networks, attachments: [...TOPOLOGY.attachments, { containerId: "c-cloud-cache", networkId: "n-boreal-v6", ip6: "2001:db8:c::2" }] };

  it("checks the address against prefix, gateway and taken addresses; IPv4 stays independent", () => {
    const v6 = net("n-boreal-v6");
    const ok = checkConnect(topology, whoami, v6, "", "2001:0db8:000c::20");
    expect([ok.ip6, ok.ip6Error, ok.ip, ok.ipError]).toEqual(["2001:db8:c::20", undefined, undefined, undefined]);
    const errs = ["2001:db8:d::5", "2001:db8:c::", "2001:db8:c::1", "2001:DB8:C::0:2", "zzz"].map((a) => checkConnect(topology, whoami, v6, "", a).ip6Error);
    expect(errs).toEqual(["outside", "network-address", "gateway", "taken", "invalid"]);
    // Pool address skips the gateway and the taken ::2; an IPv4 address in an IPv6-only network is refused.
    expect([checkConnect(topology, whoami, v6).ip6, checkConnect(topology, whoami, v6, "198.51.100.5").ipError]).toEqual(["2001:db8:c::3", "family-disabled"]);
    // A dual-stack network gives both; an IPv4-only network refuses an IPv6 request and keeps its IPv4 pool.
    const dual = checkConnect(topology, whoami, net("n-boreal-shared"));
    const plain = checkConnect(topology, whoami, net("n-boreal-web"), "", "2001:db8::5");
    expect([dual.ip, dual.ip6, plain.ip6Error, plain.ip]).toEqual(["198.51.100.98", "2001:db8:b::2", "family-disabled", "198.51.100.3"]);
  });
});
