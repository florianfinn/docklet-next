// Source: docklet-next/issues/529
// Revision: 6f21fc09c6e94a47d611e7371b27e8a9e5c000c7
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original local IP grammar, origin mapping and root domain normalization tests.

import { expect, it } from "vitest";

import { discoveryAppUrl, hubAddressCandidates, type HubDiscovery } from "./hub-discovery";
import { agentHubUrl } from "./settings";
import { hubAddress, ipFromLegacyUrl, normalizeDomainUrl } from "./validation";
import { freshSetupDraft } from "@/features/setup/setup-draft";

// Literal/port grammar shared with Go net/netip and net.JoinHostPort; only documentation addresses.
it.each([
  "192.0.2.10", "192.0.2.10:1", "192.0.2.10:65535", "198.51.100.20:3552", "203.0.113.2",
  "2001:db8::10", "2001:db8:0:0:0:0:0:10", "[2001:db8::10]:3552", "2001:db8::192.0.2.10",
])("accepts the IP literal vector %s", (value) => { expect(hubAddress(value)).toBeUndefined(); });

it.each([
  "hub.example.com", "hub.example.com:3552", "https://192.0.2.10", "http://hub.example.com",
  "192.0.2.10/path", "192.0.2.256", "192.00.2.10", "192.0.2.10:0", "192.0.2.10:65536",
  "192.0.2.10:", "2001:db8:::10", "[2001:db8::10]:0", "[2001:db8::10]:65536", "[2001:db8::10]",
  "2001:db8::10/path", "[2001:db8::10%eth0]:3552", "192.0.2.10?port=3552",
])("rejects the non-IP vector %s", (value) => { expect(hubAddress(value)).toBe("hubAddress"); });

it("requires an IP for empty writes", () => { expect(hubAddress(" ")).toBe("required"); });

it.each([
  ["2001:db8::10", "http://[2001:db8::10]"],
  ["[2001:db8::10]:3552", "http://[2001:db8::10]:3552"],
  ["192.0.2.10:3552", "http://192.0.2.10:3552"],
])("serializes the agent URL for %s", (hubAddress, expected) => {
  expect(agentHubUrl({ ...freshSetupDraft().general, hubAddress })).toBe(expected);
});

it.each([
  ["hub.example.com", undefined, "http://hub.example.com"],
  ["hub.example.com", "https://hub.example.com:3552", "https://hub.example.com"],
  ["hub.example.com:80", "https://hub.example.com", "https://hub.example.com:80"],
  ["HUB.example.com:3552", "https://hub.example.com", "https://hub.example.com:3552"],
  ["other.example.com", "https://hub.example.com", "http://other.example.com"],
  ["http://hub.example.com/", "https://hub.example.com", "http://hub.example.com"],
  ["https://HUB.example.com:443", undefined, "https://hub.example.com"],
  ["hub.example.com:3552", undefined, "http://hub.example.com:3552"],
  ["", undefined, ""],
])("normalizes local domain %s with APP_URL %s", (value, appUrl, expected) => {
  expect(normalizeDomainUrl(value, appUrl)).toBe(expected);
});

it.each(["https://hub.example.com/path", "hub.example.com/path", "https://hub.example.com/a/..", "https://hub.example.com/?x=1", "https://hub.example.com/#x", "ftp://hub.example.com", "https://user@hub.example.com", "hub.example.com:0", "two words", "https://hub.example.com\\path"])("rejects a non-root domain %s", (value) => {
  expect(normalizeDomainUrl(value)).toBeUndefined();
});

it("maps merged origins and source fallback without losing domain schemes or creating duplicate rows", () => {
  const discovery: HubDiscovery = { detectedAt: "2026-11-19T12:00:00Z", interfaces: [], hostAddresses: [], simulated: false, candidates: [
    { id: "domain", url: "https://hub.example.com", kind: "domain", source: "configured", origins: ["settingsDomain", "appUrl"], reach: "unknown" },
    { id: "ip", url: "http://192.0.2.10:3552", kind: "ip", source: "configured", origins: ["settingsAddress", "published"], reach: "unknown" },
    { id: "old", url: "http://[2001:db8::10]:3552", source: "request", reach: "unknown" },
  ] };
  expect(hubAddressCandidates(discovery).map((c) => [c.address, c.addressKind, c.origins, c.recommended])).toEqual([
    ["https://hub.example.com", "domain", ["settingsDomain", "appUrl"], false],
    ["192.0.2.10:3552", "ip", ["settingsAddress", "published"], true],
    ["[2001:db8::10]:3552", "ip", ["request"], false],
  ]);
  expect(discoveryAppUrl(discovery)).toBe("https://hub.example.com");
});

it("AUTO-2 legacy http(s) IP values map to the stored bare form", () => {
  expect(["http://192.0.2.10:8080", "https://[2001:db8::10]:3552", "https://[2001:db8::10]", "http://[2001:db8::10]/", "http://hub.example.com"].map(ipFromLegacyUrl))
    .toEqual(["192.0.2.10:8080", "[2001:db8::10]:3552", "2001:db8::10", "2001:db8::10", undefined]);
});
