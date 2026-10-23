// Source: docklet-next/issues/204
// Revision: 09d1e632cb3a14bd48b87232acaf991b9d12145e
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original advanced network domain tests.

import { describe, expect, it } from "vitest";

import type { TopologyNetwork } from "./model";
import { nextFreeIp6 } from "./ipv6";
import { checkConnect, nextFreeIp, validateNetworkDraft, validateNetworkEntries, type NetworkDraft } from "./networks";

const draft = (patch: Partial<NetworkDraft> = {}): NetworkDraft => ({ hostId: "atlas", name: "example_net", driver: "bridge", subnet: "192.0.2.0/24", gateway: "192.0.2.1", ipRange: "", parent: "", internal: false, ...patch });
const errors = (patch: Partial<NetworkDraft>) => validateNetworkDraft(draft(patch), []);
const aux = (ip: string, name = "reserved") => ({ name, ip });
const network: TopologyNetwork = { id: "net", hostId: "atlas", name: "example_net", driver: "bridge", internal: false, subnet: "192.0.2.0/24", gateway: "192.0.2.1", subnetConfigured: true, ipv6: true, subnet6: "2001:db8::/64", gateway6: "2001:db8::1", subnet6Configured: true, auxAddresses: [aux("192.0.2.2")], auxAddresses6: [aux("2001:db8::2")] };
const connect = (aliases: string[] = [], ip = "", ip6 = "") => checkConnect({ networks: [network], attachments: [] }, { id: "container", name: "example", hostId: "atlas", origin: "standalone" }, network, ip, ip6, aliases);

describe("advanced network drafts", () => {
  it.each(["labels", "options"] as const)("validates empty and normalized duplicate %s keys", (field) => {
    expect(errors({ [field]: { " ": "value" } })[field]).toBe("required");
    expect(errors({ [field]: { key: "a", " key ": "b" } })[field]).toBe("duplicate");
    expect(errors({ [field]: { key: "", "example.com/key": "value" } })).toEqual({});
    expect(validateNetworkEntries([{ key: "same", value: "one" }, { key: "same", value: "two" }])).toBe("duplicate");
  });

  it("accepts reservations inside the IPv4 subnet and rejects out-of-subnet auxiliary addresses", () => {
    expect(errors({ auxAddresses: [aux("192.0.2.2")] })).toEqual({});
    expect(errors({ auxAddresses: [aux("198.51.100.2")] }).auxAddresses).toBe("outside");
  });

  it.each([
    ["192.0.2.0", "network-address"],
    ["192.0.2.255", "network-address"],
    ["192.0.2.1", "gateway"],
    ["2001:db8::2", "invalid"],
    ["", "invalid"],
  ])("rejects IPv4 reservation %s as %s", (ip, error) => {
    expect(errors({ auxAddresses: [aux(ip)] }).auxAddresses).toBe(error);
  });

  it("requires unique non-empty reservation names, unique addresses and a subnet", () => {
    expect(errors({ auxAddresses: [aux("192.0.2.2", " ")] }).auxAddresses).toBe("required");
    expect(errors({ auxAddresses: [aux("192.0.2.2"), aux("192.0.2.3", " reserved ")] }).auxAddresses).toBe("duplicate");
    expect(errors({ auxAddresses: [aux("192.0.2.2"), aux("192.0.2.2", "other")] }).auxAddresses).toBe("duplicate");
    expect(errors({ subnet: "", gateway: "", auxAddresses: [aux("192.0.2.2")] }).auxAddresses).toBe("needs-subnet");
    expect(errors({ ipv6: true, auxAddresses6: [aux("2001:db8::2")] }).auxAddresses6).toBe("needs-subnet");
  });

  it("supports IPv6-only reservations and dual stack with per-family names", () => {
    const v6 = { ipv6: true, subnet6: "2001:db8::/64", gateway6: "2001:db8::1", auxAddresses6: [aux("2001:db8::2")] };
    expect(errors({ ...v6, ipv4: false, subnet: "ignored", auxAddresses: [aux("ignored")] })).toEqual({});
    expect(errors({ ...v6, auxAddresses: [aux("192.0.2.2")] })).toEqual({});
    expect(errors({ ...v6, auxAddresses6: [aux("2001:db8:1::2")] }).auxAddresses6).toBe("outside");
    expect(errors({ ...v6, auxAddresses6: [aux("2001:db8::1")] }).auxAddresses6).toBe("gateway");
    expect(errors({ ...v6, auxAddresses6: [aux("2001:db8::")] }).auxAddresses6).toBe("network-address");
    expect(errors({ ...v6, auxAddresses6: [aux("2001:db8::2"), aux("2001:0DB8::2", "other")] }).auxAddresses6).toBe("duplicate");
  });
});

describe("network aliases and reserved pool addresses", () => {
  it("normalizes and deduplicates valid DNS aliases", () => {
    expect(connect([" api ", "API", "worker-2", "x"]).aliases).toEqual(["api", "worker-2", "x"]);
    expect(connect([]).aliasError).toBeUndefined();
  });

  it.each(["", " ", "bad_alias", "a.example.com", "-api", "api-", "a".repeat(64)])("rejects invalid alias %j", (alias) => {
    expect(connect([alias]).aliasError).toBe("invalid");
  });

  it("skips auxiliary addresses in both pool hints and rejects a static reservation", () => {
    expect([nextFreeIp(network, []), nextFreeIp6(network, [])]).toEqual(["192.0.2.3", "2001:db8::3"]);
    expect([connect().ip, connect().ip6]).toEqual(["192.0.2.3", "2001:db8::3"]);
    expect([connect([], "192.0.2.2").ipError, connect([], "", "2001:0db8::2").ip6Error]).toEqual(["taken", "taken"]);
  });
});
