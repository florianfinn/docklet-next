// Source: docklet-next/issues/572
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original Edge hub address guidance and effect-synchronised acceptance coverage.; M36 #572: Edge hub address guidance.

import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import type { HubDiscovery } from "@/domain/hub-discovery";
import type { Host } from "@/domain/model";
import type { GeneralSettings } from "@/domain/settings";
import { renderWithApp } from "@/test/render";
import { ConnectStep } from "./ConnectSteps";
import { defaultAgentHubUrl } from "./hub-address";
import type { HostDraft } from "./onboarding-store";

const general: GeneralSettings = { hubName: "Example", hubAddress: "192.0.2.10", domainUrl: "https://hub.example.com", linkBase: "auto", timeZone: "UTC" };
const discovery: HubDiscovery = { detectedAt: "2026-11-22T12:00:00Z", simulated: false, interfaces: [], hostAddresses: [], container: { name: "hub", networkMode: "bridge", listenPort: 3552, publishedPorts: [{ hostPort: 3552, containerPort: 3552 }] }, candidates: [
  { id: "domain", url: "https://hub.example.com", kind: "domain", source: "configured", reach: "unknown" },
  { id: "saved", url: "http://192.0.2.10", kind: "ip", source: "configured", reach: "unknown" },
  { id: "published", url: "http://192.0.2.20:3552", kind: "ip", source: "published", reach: "unknown" },
] };

afterEach(() => { cleanup(); vi.useRealTimers(); setAppLocale("de"); });

it.each([
  ["explicit port", "192.0.2.10:3552", discovery, "http://192.0.2.10:3552"],
  ["explicit default port", "192.0.2.10:80", discovery, "http://192.0.2.10:80"],
  ["published 80", "192.0.2.10", { ...discovery, container: { ...discovery.container!, publishedPorts: [{ hostPort: 80, containerPort: 3552 }] } }, "http://192.0.2.10"],
  ["published non-80", "192.0.2.10", discovery, "http://192.0.2.20:3552"],
  ["IPv6 with port", "[2001:db8::10]:3552", discovery, "http://[2001:db8::10]:3552"],
  ["bare IPv6", "2001:db8::10", discovery, "http://192.0.2.20:3552"],
  ["no candidates", "192.0.2.10", { ...discovery, candidates: [] }, "http://192.0.2.10:3552"],
  // A published address that only the same host reaches is no default; the saved address takes the published port instead.
  ["bare IPv6, no candidates", "2001:db8::10", { ...discovery, candidates: [] }, "http://[2001:db8::10]:3552"],
  ["bare IPv6, same-host published only", "2001:db8::10", { ...discovery, candidates: [discovery.candidates![0]!, { ...discovery.candidates![2]!, reach: "same-host" }] }, "http://[2001:db8::10]:3552"],
  ["same-host published only", "192.0.2.10", { ...discovery, candidates: [discovery.candidates![0]!, discovery.candidates![1]!, { ...discovery.candidates![2]!, reach: "same-host" }] }, "http://192.0.2.10:3552"],
  ["domain remains candidate", "", { ...discovery, candidates: [discovery.candidates![0]!] }, "https://hub.example.com"],
] satisfies [string, string, HubDiscovery, string][])("chooses the Edge default: %s", (_name, hubAddress, found, want) => {
  expect(defaultAgentHubUrl({ ...general, hubAddress }, found)).toBe(want);
});

const draft: HostDraft = { name: "Agent", color: "#123456", connection: "edge", address: "", port: "3552", projectDir: "/srv/projects", installDir: "/srv/agent", manualIds: false, puid: "", pgid: "", detectSocket: true, socketGid: "", simulateFailure: false, reached: 3, hostId: "example" };
const host: Host = { id: "example", name: "Agent", color: "#123456", status: "pending", connection: "edge", address: "", projectDir: "/srv/projects", installDir: "/srv/agent", os: "", dockerVersion: "", agentVersion: "", cpuCores: 0, memTotalGb: 0, diskTotalGb: 0, lastSeen: "" };

it("shows the URL and agent routes only after 90 seconds, then hides them on pairing", async () => {
  setAppLocale("en-US");
  vi.useFakeTimers();
  function PairingObservation() {
    const [observed, setObserved] = useState(host);
    return <><button onClick={() => setObserved({ ...host, status: "online", lastSeen: "2026-11-22T12:00:00Z" })}>Pair</button><ConnectStep draft={draft} host={observed} hubUrl="http://192.0.2.20:3552" /></>;
  }
  renderWithApp(<PairingObservation />);
  await act(async () => { await vi.advanceTimersByTimeAsync(89_999); });
  expect(screen.queryByText(/The agent has not reported in/) === null).toBe(true);
  await act(async () => { await vi.advanceTimersByTimeAsync(1); });
  expect(screen.getByText("The agent has not reported in. Does it reach http://192.0.2.20:3552?") !== null).toBe(true);
  expect(screen.getByText(/Agent routes \/api\/environments\/pair and \/api\/tunnel\/\*/) !== null).toBe(true);
  // Changing the producer observation rerenders the actual effect consumer.
  await act(async () => { fireEvent.click(screen.getByRole("button", { name: "Pair" })); });
  expect(screen.queryByText(/The agent has not reported in/) === null).toBe(true);
});
