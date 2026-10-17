// Source: docklet-next/docs/design/host-onboarding.md
// Revision: 81439fab6a8ec5232161dc933fbd41bec41885fa
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original production host onboarding UI acceptance tests.

import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { setAppLocale } from "@/app/i18n/locale-store";
import { createHttpAdapter } from "@/data/http/http-adapter";
import { renderRoutes, renderWithApp } from "@/test/render";
import { RotateTokenButton } from "../RotateTokenButton";
import { mapHost } from "@/data/http/hosts";
import OnboardingPage from "./OnboardingPage";
import { onboardingStore } from "./onboarding-store";

const settings = { hubName: "Example hub", hubAddress: "192.0.2.10:3552", domainUrl: "https://hub.example.com", linkBase: "auto", effectiveTimeZone: "UTC", keepActionOutput: "true", actionPresentation: "dialog", defaultHostConnection: "direct", agentProjectsDirectory: "/srv/projects", agentInstallDirectory: "/srv/projects/docklet-agent", defaultPuid: "", defaultPgid: "", detectSocketGroup: "true" };
const row = { id: "example", name: "Example", apiUrl: "http://agent.example.com:3552", enabled: true, isEdge: false, status: "pending" };

function server(admin = true) {
  const writes: Record<string, unknown>[] = [];
  const requests: string[] = [];
  let registered = false;
  let rotations = 0;
  const adapter = createHttpAdapter({
    eventSource: () => ({ close() {}, onerror: null, onopen: null, onmessage: null }),
    fetch: async (input, init) => {
      const path = new URL(String(input), "https://hub.example.com").pathname;
      requests.push(path);
      const json = (data: unknown, pagination?: object) => new Response(JSON.stringify({ success: true, data, ...(pagination ? { pagination } : {}) }), { headers: { "X-CSRF-Token": "example".padEnd(43, "x") } });
      if (path === "/api/capabilities") return json(["registerHost", "listHosts", "discoverHub", "issueAgentToken"].map(operationId => ({ operationId, available: admin, resourceRef: { kind: "hub", id: "0" } })));
      if (path === "/api/auth/me") return json({ id: "example", username: "Example", requiresPasswordChange: false });
      if (path === "/api/auth/mode") return json({ mode: "none" });
      if (path.endsWith("/settings")) return new Response(JSON.stringify(Object.entries(settings).map(([key, value]) => ({ key, value }))));
      if (path === "/api/hub/discovery") return json({ detectedAt: "2026-10-17T12:00:00Z", agentImage: "registry.example.com/docklet/agent:1.2.3", candidates: [{ id: "public", url: "https://hub.example.com", source: "configured", reach: "unknown" }, { id: "internal", url: "http://192.0.2.10:3552", source: "interface", reach: "unknown" }] });
      if (path === "/api/environments" && init?.method === "POST") {
        const body = JSON.parse(String(init.body)) as Record<string, unknown>;
        writes.push(body);
        registered = true;
        return json({ ...row, ...body, apiKey: "example" });
      }
      if (path === "/api/environments/example" && init?.method === "PUT") { rotations++; return json({ ...row, apiKey: "test-only" }); }
      if (path === "/api/environments") return json(registered ? [row] : [], { totalItems: registered ? 1 : 0 });
      throw new Error(`unexpected ${path}`);
    },
  });
  return { adapter, writes, requests, rotations: () => rotations };
}

beforeEach(() => { vi.stubEnv("DEV", false); onboardingStore.set(null); });
afterEach(() => { cleanup(); vi.unstubAllEnvs(); onboardingStore.set(null); setAppLocale("de"); });

it.each(["direct", "edge"] as const)("creates a live %s host before rendering installation artifacts", async connection => {
  const f = server();
  const { router } = renderRoutes([{ path: "/hosts/new", element: <OnboardingPage /> }, { path: "/hosts", element: <p>Host list</p> }], ["/hosts/new"], f.adapter);
  await userEvent.type(await screen.findByLabelText("Hostname in Docklet"), "Example");
  expect(screen.queryByTestId("agent-token") === null).toBe(true);
  if (connection === "edge") {
    await userEvent.click(screen.getByRole("radio", { name: /Edge/ }));
    expect((screen.getByLabelText("Interne Hub-Adresse für den Agent") as HTMLInputElement).value).toBe("https://hub.example.com");
    await userEvent.click(screen.getByRole("button", { name: /http:\/\/192.0.2.10/ }));
    expect((screen.getByLabelText("Interne Hub-Adresse für den Agent") as HTMLInputElement).value).toBe("http://192.0.2.10:3552");
  } else await userEvent.type(document.querySelector<HTMLInputElement>("#host-address")!, "agent.example.com");
  await userEvent.click(screen.getByRole("button", { name: "Weiter" }));
  await userEvent.click(await screen.findByRole("button", { name: "Host anlegen und auf Agent warten" }));
  expect((await screen.findByTestId("agent-token")).textContent).toBe("example");
  expect(f.writes).toHaveLength(1);
  expect(f.writes[0]).toMatchObject({ useApiKey: true, isEdge: connection === "edge" });
  expect(document.querySelector("pre")?.textContent?.includes("registry.example.com/docklet/agent:1.2.3")).toBe(true);
  await userEvent.click(screen.getByRole("button", { name: "Weiter" }));
  expect(router.state.location.search).toBe("?step=connect");
  expect(screen.queryByRole("button", { name: /simulieren/ }) === null).toBe(true);
  await userEvent.click(screen.getAllByRole("link", { name: "Zur Hostliste" })[0]!);
  await screen.findByText("Host list");
  expect(onboardingStore.get()).toBe(null);
  for (const host of await f.adapter.listHosts()) for (const key of ["apiKey", "accessToken", "token"]) expect(host).not.toHaveProperty(key);
});

it("rejects non-admin navigation before reading installation metadata", async () => {
  const f = server(false);
  const discover = vi.spyOn(f.adapter, "discoverHub");
  renderRoutes([{ path: "/hosts/new", element: <OnboardingPage /> }], ["/hosts/new"], f.adapter);
  await screen.findByText("Daten konnten nicht geladen werden");
  expect(screen.queryByLabelText("Hostname in Docklet") === null).toBe(true);
  expect(discover).not.toHaveBeenCalled();
  expect(f.requests.includes("/api/hub/discovery")).toBe(false);
  expect(f.writes).toHaveLength(0);
});

it("shows a rotated token once and clears it when the result dialog closes", async () => {
  const f = server();
  renderWithApp(<RotateTokenButton host={mapHost(row)} />, f.adapter);
  await userEvent.click(await screen.findByRole("button", { name: "Token rotieren" }));
  expect(f.rotations()).toBe(0);
  await userEvent.click(screen.getAllByRole("button", { name: "Token rotieren" }).at(-1)!);
  await screen.findByText("test-only");
  await userEvent.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByText("test-only") === null).toBe(true));
  await userEvent.click(screen.getByRole("button", { name: "Token rotieren" }));
  expect(screen.queryByText("test-only") === null).toBe(true);
  expect(f.rotations()).toBe(1);
});
