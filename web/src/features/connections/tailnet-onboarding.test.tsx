// Source: docklet-next/issues/573
// Revision: a5836218e8226cbe52e865f29f381b0209c69fa3
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original Tailscale UI and generation-fenced stream consumers.

import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { renderRoutes } from "@/test/render";
import OnboardingPage from "@/features/hosts/onboarding/OnboardingPage";
import { onboardingStore } from "@/features/hosts/onboarding/onboarding-store";
import { connectedStatus, tailnetFixture } from "./tailnet.test-support";

beforeEach(() => { setAppLocale("en-US"); onboardingStore.set(null); });
afterEach(() => { setAppLocale("de"); onboardingStore.set(null); vi.restoreAllMocks(); });

it.each(["oauth", "manual"] as const)("registers a tailnet host in %s mode and displays the one-time installer key without refetching it", async (mode) => {
  const f = tailnetFixture({ ...connectedStatus(), mode, provisioning: { state: mode === "manual" ? "not-applicable" : "ok" } });
  f.state.settings.mode = mode;
  const view = renderRoutes([{ path: "/hosts/new", element: <OnboardingPage /> }, { path: "/hosts", element: <p>Host list</p> }], ["/hosts/new"], f.adapter);
  await userEvent.type(await screen.findByLabelText("Host name in Docklet"), "New agent");
  const route = await screen.findByRole("radio", { name: /Tailscale Private/ });
  await waitFor(() => expect((route as HTMLButtonElement).disabled).toBe(false));
  await userEvent.click(route);
  expect(screen.queryByLabelText("Hub address for the Edge agent") === null).toBe(true);
  expect(screen.queryByText(/HTTP sends the token/) === null).toBe(true);
  expect(screen.getByText("http://192.0.2.10:3552") !== null).toBe(true);
  if (mode === "manual") await userEvent.type(screen.getByLabelText("One-use agent auth key"), "tskey-auth-example-manual");
  await userEvent.click(screen.getByRole("button", { name: "Next" }));
  await userEvent.click(await screen.findByRole("button", { name: "Create host and wait for agent" }));
  await screen.findByTestId("agent-token");
  const create = f.writes.filter((write) => write.path === "/api/environments");
  expect(create).toHaveLength(1);
  expect(create[0]?.body).toMatchObject({ route: "tailnet", isEdge: true, useApiKey: true, ...(mode === "manual" ? { tailnetAuthKey: "tskey-auth-example-manual" } : {}) });
  if (mode === "oauth") expect(create[0]?.body).not.toHaveProperty("tailnetAuthKey");
  const key = mode === "manual" ? "tskey-auth-example-manual" : "tskey-auth-example-once";
  expect(onboardingStore.get()?.tailnetAuthKey).toBe(key);
  await userEvent.click(screen.getByRole("radio", { name: /Compose/ }));
  expect(document.body.textContent).toContain(key);
  expect(document.body.textContent).toContain("TAILSCALE_AUTH_KEY");
  expect(document.body.textContent?.includes("example-client-secret")).toBe(false);
  expect(screen.queryByText(/HTTP sends the token/) === null).toBe(true);
  await userEvent.click(screen.getByRole("button", { name: "Back" }));
  await userEvent.click(screen.getByRole("button", { name: "Next" }));
  expect(f.writes.filter((write) => write.path === "/api/environments")).toHaveLength(1);
  expect(f.requests.filter((path) => /tailnet.*key|keys.*tailnet/.test(path))).toHaveLength(0);
  view.unmount();
  expect(onboardingStore.get()).toBeNull();
});

it.each([
  ["disconnected", { ...connectedStatus(), connection: { state: "off" as const }, routeAvailable: false }, /hub is not connected/],
  ["rejected", { ...connectedStatus(), provisioning: { state: "rejected" as const }, routeAvailable: false }, /invalid or revoked/],
  ["mTLS", { ...connectedStatus(), routeAvailable: false, routeCode: "tailnet.mtls-unsupported" }, /Edge mTLS is enabled/],
] as const)("keeps the Tailscale card visible and disabled for %s with its reason", async (_name, status, reason) => {
  const f = tailnetFixture(status);
  renderRoutes([{ path: "/hosts/new", element: <OnboardingPage /> }], ["/hosts/new"], f.adapter);
  const route = await screen.findByRole("radio", { name: /Tailscale Private/ });
  await waitFor(() => expect(route.closest('[data-slot="tailnet-route"]')?.getAttribute("data-ready")).toBe("true"));
  expect((route as HTMLButtonElement).disabled).toBe(true);
  await screen.findByText(reason);
  expect(screen.getByRole("link", { name: "Open Connections" }).getAttribute("href")).toBe("/settings/connections");
  await userEvent.click(route);
  expect(route.getAttribute("aria-checked")).toBe("false");
});

it("revokes tailnet admission live when provisioning rejects after selection", async () => {
  const f = tailnetFixture();
  renderRoutes([{ path: "/hosts/new", element: <OnboardingPage /> }], ["/hosts/new"], f.adapter);
  await userEvent.type(await screen.findByLabelText("Host name in Docklet"), "New agent");
  await userEvent.click(await screen.findByRole("radio", { name: /Tailscale Private/ }));
  await act(async () => f.send({ ...connectedStatus(), provisioning: { state: "rejected", providerStatus: 403 }, routeAvailable: false }));
  await screen.findByText(/invalid or revoked/);
  expect((screen.getByRole("button", { name: "Next" }) as HTMLButtonElement).disabled).toBe(true);
  expect(f.writes).toHaveLength(0);
});

it("writes the hub's self-hosted control server into the agent installer", async () => {
  const f = tailnetFixture({ ...connectedStatus(), mode: "manual", provisioning: { state: "not-applicable" } });
  f.state.settings.mode = "manual"; f.state.settings.customControl = true; f.state.settings.controlUrl = "https://control.example.com";
  renderRoutes([{ path: "/hosts/new", element: <OnboardingPage /> }, { path: "/hosts", element: <p>Host list</p> }], ["/hosts/new"], f.adapter);
  await userEvent.type(await screen.findByLabelText("Host name in Docklet"), "New agent");
  const route = await screen.findByRole("radio", { name: /Tailscale Private/ });
  await waitFor(() => expect((route as HTMLButtonElement).disabled).toBe(false));
  await userEvent.click(route);
  await userEvent.type(screen.getByLabelText("One-use agent auth key"), "tskey-auth-example-manual");
  await userEvent.click(screen.getByRole("button", { name: "Next" }));
  await userEvent.click(await screen.findByRole("button", { name: "Create host and wait for agent" }));
  await screen.findByTestId("agent-token");
  await userEvent.click(screen.getByRole("radio", { name: /Compose/ }));
  expect(document.body.textContent).toContain("TAILSCALE_CONTROL_URL='https://control.example.com'");
});

it("retains the created host but refuses a partial one-time installer response", async () => {
  const f = tailnetFixture();
  const fetcher = f.fetcher.getMockImplementation()!;
  f.fetcher.mockImplementation(async (input, init) => {
    const response = await fetcher(input, init);
    if (String(input) === "/api/environments" && init?.method === "POST") {
      const body = await response.json(); delete body.data.tailnetAuthKey; delete body.data.tailnetManagerUrl;
      return new Response(JSON.stringify(body), { headers: response.headers });
    }
    return response;
  });
  renderRoutes([{ path: "/hosts/new", element: <OnboardingPage /> }], ["/hosts/new"], f.adapter);
  await userEvent.type(await screen.findByLabelText("Host name in Docklet"), "New agent");
  const card = await screen.findByRole("radio", { name: /Tailscale Private/ });
  await waitFor(() => expect((card as HTMLButtonElement).disabled).toBe(false));
  await userEvent.click(card);
  await userEvent.click(screen.getByRole("button", { name: "Next" }));
  await userEvent.click(await screen.findByRole("button", { name: "Create host and wait for agent" }));
  await screen.findByRole("heading", { name: "Installation" });
  expect(onboardingStore.get()?.hostId).toBe("example-agent");
  expect(screen.queryByTestId("agent-token") === null).toBe(true);
  expect(screen.queryByText(/one-use installer key or hub address is unavailable/) !== null).toBe(true);
  expect((screen.getByRole("button", { name: "Next" }) as HTMLButtonElement).disabled).toBe(true);
  expect(f.writes.filter((write) => write.path === "/api/environments")).toHaveLength(1);
});

it("refuses the Tailscale route when the hub's control server cannot be written into an installer", async () => {
  const f = tailnetFixture({ ...connectedStatus(), mode: "manual", provisioning: { state: "not-applicable" } });
  // A URL the installer rejects (here an interior no-break space) counts as missing, whichever validator let it through.
  f.state.settings.mode = "manual"; f.state.settings.customControl = true; f.state.settings.controlUrl = "https://control.example.com/a\u00a0b";
  renderRoutes([{ path: "/hosts/new", element: <OnboardingPage /> }], ["/hosts/new"], f.adapter);
  await userEvent.type(await screen.findByLabelText("Host name in Docklet"), "New agent");
  await screen.findByText(/TAILSCALE_CONTROL_URL cannot be written into an agent installer/);
  expect((screen.getByRole("radio", { name: /Tailscale Private/ }) as HTMLButtonElement).disabled).toBe(true);
});
