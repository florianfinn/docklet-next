// Source: docklet-next/issues/519
// Revision: 462df7e945082b44e1d90532d8296aa9743c4b6e
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original live hub discovery tests through the real HTTP adapter.

import { cleanup, screen } from "@testing-library/react";
import { useQuery } from "@tanstack/react-query";
import { afterEach, expect, it } from "vitest";

import { createHttpAdapter } from "@/data/http/http-adapter";
import { useAdapter } from "@/data/use-adapter";
import { hubAddressCandidates } from "@/domain/hub-discovery";
import { renderWithApp } from "@/test/render";

import { HubAddressDiscovery } from "./HubAddressDiscovery";
import { applyDiscovery, freshSetupDraft } from "./setup-draft";

afterEach(() => { cleanup(); localStorage.clear(); });

function LiveDiscovery() {
  const live = useAdapter();
  const discovery = useQuery({ queryKey: ["hub", "discovery", "probe"], queryFn: () => live.discoverHub() });
  return <HubAddressDiscovery discovery={discovery} address="" origin={null} onPick={() => undefined} />;
}

const bridge = {
  detectedAt: "2026-11-19T12:00:00Z",
  agentImage: "registry.example.com/docklet/agent:1.2.3",
  containerDetection: "container",
  container: { name: "docklet-hub", listenPort: 3552, networks: [{ name: "docklet_default", driver: "bridge" }], publishedPorts: [{ hostIp: "", hostPort: 8080 }] },
  candidates: [
    { id: "configured-domain", url: "https://hub.example.com", kind: "domain", source: "configured", reach: "unknown" },
    { id: "published-loopback", url: "http://127.0.0.1:8080", kind: "ip", source: "published", reach: "same-host" },
    { id: "request-ip", url: "http://[2001:db8::10]:8080", kind: "ip", source: "request", reach: "unknown" },
  ],
};

function adapter(body: unknown) {
  return createHttpAdapter({ fetch: async (input) => {
    const path = new URL(String(input), "https://hub.example.com").pathname;
    const json = (data: unknown) => new Response(JSON.stringify({ success: true, data }), { headers: { "X-CSRF-Token": "example".padEnd(43, "x") } });
    if (path === "/api/auth/mode") return json({ mode: "none", setupPending: false });
    if (path === "/api/auth/me") return json({ id: "admin", username: "owner", requiresPasswordChange: false, isAdmin: true, assignments: [] });
    if (path === "/api/capabilities") return json([{ operationId: "discoverHub", available: true, resourceRef: { kind: "hub", id: "0" } }]);
    if (path === "/api/hub/discovery") return json(body);
    return new Response("{}", { status: 503 });
  } });
}

it("maps the live container, recommends the reachable IP and never prefills a name or loopback", async () => {
  const discovery = await adapter(bridge).discoverHub();
  expect(discovery.container).toEqual({ name: "docklet-hub", networkMode: "bridge", listenPort: 3552, publishedPorts: [{ hostPort: 8080, containerPort: 3552 }] });
  expect(discovery.containerDetection).toBe("container");
  const candidates = hubAddressCandidates(discovery);
  expect(candidates.map((c) => [c.address, c.kind, c.recommended])).toEqual([
    ["hub.example.com", "configured", false],
    ["127.0.0.1:8080", "host-address", false],
    ["[2001:db8::10]:8080", "request", true],
  ]);
  const filled = applyDiscovery(freshSetupDraft(), discovery);
  expect([filled.general.hubAddress, filled.hubAddressOrigin]).toEqual(["[2001:db8::10]:8080", { kind: "discovered", candidateId: "request-ip" }]);
  // Only a name: the internal address stays empty for the operator.
  const domainOnly = await adapter({ ...bridge, candidates: [bridge.candidates[0]] }).discoverHub();
  expect(applyDiscovery(freshSetupDraft(), domainOnly).general.hubAddress).toBe("");
});

it("shows the container line, and the unknown-detection hint without a container", async () => {
  const live = adapter(bridge);
  const { unmount } = renderWithApp(<LiveDiscovery />, live);
  expect((await screen.findByText(/docklet-hub/)).textContent).toContain("bridge");
  unmount();
  const unknown = adapter({ ...bridge, containerDetection: "unknown", container: undefined, candidates: [bridge.candidates[0]] });
  renderWithApp(<LiveDiscovery />, unknown);
  expect(await screen.findByText(/Container nicht erkannt/) !== null).toBe(true);
});

it("refuses a candidate with an unknown source or kind", async () => {
  await expect(adapter({ ...bridge, candidates: [{ ...bridge.candidates[0], source: "guessed" }] }).discoverHub()).rejects.toThrow("hosts.invalid-candidate");
  await expect(adapter({ ...bridge, candidates: [{ ...bridge.candidates[0], kind: "name" }] }).discoverHub()).rejects.toThrow("hosts.invalid-candidate");
});
